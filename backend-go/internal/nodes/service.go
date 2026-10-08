package nodes

import (
	"context"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"remnawave-go/internal/database"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func NanoToMultiplier(v int64) float64 {
	if v == 0 {
		return 1.0
	}
	if v < 1000000 {
		return float64(v)
	}
	return float64(v) / 1e9
}

func MultiplierToNano(v float64) int64 {
	if v <= 0 {
		return 1000000000
	}
	return int64(math.Round(v * 1e9))
}

type NodeHotMetrics struct {
	System      *NodeSystemStatsResponse `json:"system"`
	Versions    *NodeVersionsResponse    `json:"versions"`
	XrayUptime  int64                    `json:"xrayUptime"`
	OnlineUsers int                      `json:"usersOnline"`
	LastSeen    time.Time                `json:"lastSeen"`
}

type NodeVersionsResponse struct {
	Xray string `json:"xray"`
	Node string `json:"node"`
}

type Service struct {
	db          *gorm.DB
	client      *Client
	metricsMu   sync.RWMutex
	nodeMetrics map[string]*NodeHotMetrics
}

func NewService(db *gorm.DB, client *Client) *Service {
	return &Service{
		db:          db,
		client:      client,
		nodeMetrics: make(map[string]*NodeHotMetrics),
	}
}

func (s *Service) GetNodeMetrics(nodeUUID string) *NodeHotMetrics {
	s.metricsMu.RLock()
	defer s.metricsMu.RUnlock()
	return s.nodeMetrics[nodeUUID]
}

func (s *Service) SetNodeMetrics(nodeUUID string, m *NodeHotMetrics) {
	s.metricsMu.Lock()
	defer s.metricsMu.Unlock()
	s.nodeMetrics[nodeUUID] = m
}

func (s *Service) DB() *gorm.DB {
	return s.db
}

type CreateNodeDTO struct {
	Name                      string   `json:"name"`
	Address                   string   `json:"address"`
	Port                      *int     `json:"port"`
	CountryCode               string   `json:"countryCode"`
	TrafficLimitBytes         uint64   `json:"trafficLimitBytes"`
	ConsumptionMultiplier     *float64 `json:"consumptionMultiplier"`
	NodeConsumptionMultiplier *float64 `json:"nodeConsumptionMultiplier"`
	ActiveConfigProfileUUID *string `json:"activeConfigProfileUuid"`
	ActivePluginUUID        *string `json:"activePluginUuid"`
	ProviderUUID            *string `json:"providerUuid"`
	Note                    *string `json:"note"`
	ConfigProfile           *struct {
		ActiveConfigProfileUUID string   `json:"activeConfigProfileUuid"`
		ActiveInbounds          []string `json:"activeInbounds"`
	} `json:"configProfile"`
}

func (s *Service) CheckNodeHealth(node *database.Node) (bool, string) {
	if s.client == nil {
		return false, "node client not configured"
	}
	alive, msg, err := s.client.CheckHealth(node)
	now := time.Now().UTC()
	updates := map[string]interface{}{
		"is_connected":        alive,
		"last_status_change":  &now,
		"last_status_message": nil,
	}
	if err != nil {
		errMsg := err.Error()
		updates["last_status_message"] = &errMsg
	} else if !alive {
		updates["last_status_message"] = &msg
	}
	s.db.Model(&database.Node{}).Where("uuid = ?", node.UUID).Updates(updates)
	return alive, msg
}

func (s *Service) StartNode(node *database.Node, force bool) (bool, error) {
	if s.client == nil {
		return false, nil
	}

	configMap := map[string]interface{}{}
	if node.ActiveConfigProfileUUID != nil && *node.ActiveConfigProfileUUID != "" {
		var cp database.ConfigProfile
		if err := s.db.Where("uuid = ?", *node.ActiveConfigProfileUUID).First(&cp).Error; err == nil {
			_ = json.Unmarshal([]byte(cp.Config), &configMap)
		}
	}

	payload := map[string]interface{}{
		"xrayConfig": configMap,
		"internals": map[string]interface{}{
			"forceRestart": force,
			"hashes": map[string]interface{}{
				"configHash":   "hash",
				"inboundsHash": "hash",
			},
			"metadata": map[string]interface{}{
				"uuid":        node.UUID,
				"name":        node.Name,
				"countryCode": node.CountryCode,
				"id":          node.ID,
			},
		},
	}

	ok, err := s.client.StartXray(node, payload)
	now := time.Now().UTC()
	updates := map[string]interface{}{
		"is_connected":       ok,
		"last_status_change": &now,
	}
	if err != nil {
		msg := err.Error()
		updates["last_status_message"] = &msg
	} else if ok {
		updates["last_status_message"] = nil
	}
	s.db.Model(&database.Node{}).Where("uuid = ?", node.UUID).Updates(updates)
	return ok, err
}

func (s *Service) PollNodeStats(node *database.Node) {
	if s.client == nil || !node.IsConnected {
		return
	}

	metrics := s.GetNodeMetrics(node.UUID)
	if metrics == nil {
		metrics = &NodeHotMetrics{}
	}

	// 1. Fetch system stats
	sysStats, err := s.client.GetSystemStats(node)
	if err == nil && sysStats != nil {
		metrics.System = sysStats
		if sysStats.XrayInfo != nil {
			metrics.XrayUptime = sysStats.XrayInfo.Uptime
		}
	}

	// 2. Fetch users stats (traffic delta)
	usersTraffic, err := s.client.GetUsersStats(node, true)
	if err == nil {
		now := time.Now().UTC()
		activeCount := 0

		for _, ut := range usersTraffic {
			delta := ut.Uplink + ut.Downlink
			if delta <= 0 {
				continue
			}
			activeCount++

			// Apply node consumption multiplier (stored as nano: 1.0 = 1,000,000,000)
			mult := NanoToMultiplier(node.ConsumptionMultiplier)
			if mult <= 0 {
				mult = 1.0
			}
			incTraffic := uint64(float64(delta) * mult)

			uid, err := strconv.ParseInt(ut.Username, 10, 64)
			if err != nil || uid <= 0 {
				continue
			}

			// Update user_traffic in database (ut.Username is the user ID string)
			s.db.Exec(`
				UPDATE user_traffic
				SET
					used_traffic_bytes = used_traffic_bytes + ?,
					lifetime_used_traffic_bytes = lifetime_used_traffic_bytes + ?,
					online_at = ?,
					first_connected_at = COALESCE(first_connected_at, ?),
					last_connected_node_uuid = ?
				WHERE id = ?
			`, incTraffic, incTraffic, now, now, node.UUID, uid)
		}

		if len(usersTraffic) > 0 {
			metrics.OnlineUsers = activeCount
		}
	}

	// 3. If online users is still 0, check active IP sessions for accuracy
	if metrics.OnlineUsers == 0 {
		sessions, err := s.client.GetUsersIpList(node)
		if err == nil && len(sessions) > 0 {
			metrics.OnlineUsers = len(sessions)
			now := time.Now().UTC()
			for _, sess := range sessions {
				if len(sess.IPs) > 0 {
					s.db.Exec(`
						UPDATE user_traffic
						SET online_at = ?,
						    last_connected_node_uuid = ?
						WHERE id = ?
					`, now, node.UUID, sess.UserID)
				}
			}
		}
	}

	metrics.LastSeen = time.Now()
	s.SetNodeMetrics(node.UUID, metrics)
}

func (s *Service) StartHealthCheckLoop(ctx context.Context) {
	healthTicker := time.NewTicker(30 * time.Second)
	statsTicker := time.NewTicker(5 * time.Second)
	defer healthTicker.Stop()
	defer statsTicker.Stop()

	// Initial run
	var nodes []database.Node
	if err := s.db.Where("is_disabled = ?", false).Find(&nodes).Error; err == nil {
		for _, n := range nodes {
			s.CheckNodeHealth(&n)
			s.PollNodeStats(&n)
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-healthTicker.C:
			var nodes []database.Node
			if err := s.db.Where("is_disabled = ?", false).Find(&nodes).Error; err == nil {
				for _, n := range nodes {
					s.CheckNodeHealth(&n)
				}
			}
		case <-statsTicker.C:
			var nodes []database.Node
			if err := s.db.Where("is_disabled = ? AND is_connected = ?", false, true).Find(&nodes).Error; err == nil {
				for _, n := range nodes {
					go s.PollNodeStats(&n)
				}
			}
		}
	}
}

func (s *Service) Create(dto CreateNodeDTO) (*database.Node, error) {
	port := 443
	if dto.Port != nil && *dto.Port > 0 {
		port = *dto.Port
	}
	if dto.CountryCode == "" {
		dto.CountryCode = "XX"
	}

	var maxPos int
	s.db.Model(&database.Node{}).Select("COALESCE(MAX(view_position), 0)").Scan(&maxPos)

	profileUuid := dto.ActiveConfigProfileUUID
	if dto.ConfigProfile != nil && dto.ConfigProfile.ActiveConfigProfileUUID != "" {
		profileUuid = &dto.ConfigProfile.ActiveConfigProfileUUID
	}

	cm := 1.0
	if dto.ConsumptionMultiplier != nil {
		cm = *dto.ConsumptionMultiplier
	}
	ncm := 1.0
	if dto.NodeConsumptionMultiplier != nil {
		ncm = *dto.NodeConsumptionMultiplier
	}

	node := &database.Node{
		UUID:                      uuid.NewString(),
		Name:                      dto.Name,
		Address:                   dto.Address,
		Port:                      &port,
		CountryCode:               dto.CountryCode,
		TrafficLimitBytes:         dto.TrafficLimitBytes,
		ActiveConfigProfileUUID:   profileUuid,
		ActivePluginUUID:          dto.ActivePluginUUID,
		ProviderUUID:              dto.ProviderUUID,
		Note:                      dto.Note,
		IsConnected:               false,
		IsDisabled:                false,
		ViewPosition:              maxPos + 1,
		ConsumptionMultiplier:     MultiplierToNano(cm),
		NodeConsumptionMultiplier: MultiplierToNano(ncm),
		TrafficResetDay:           1,
		NotifyPercent:             80,
		Tags:                      "[]",
		IntegrationUUIDs:          "[]",
		IPs:                       "[]",
		CreatedAt:                 time.Now().UTC(),
		UpdatedAt:                 time.Now().UTC(),
	}

	err := s.db.Create(node).Error
	if err != nil {
		return nil, err
	}

	if dto.ConfigProfile != nil && len(dto.ConfigProfile.ActiveInbounds) > 0 {
		for _, ib := range dto.ConfigProfile.ActiveInbounds {
			s.db.Create(&database.ConfigProfileInboundsToNodes{
				ConfigProfileInboundUUID: ib,
				NodeUUID:                 node.UUID,
			})
		}
	}

	if s.client != nil {
		go s.CheckNodeHealth(node)
	}

	return node, nil
}

func (s *Service) GetAll() ([]database.Node, error) {
	var nodes []database.Node
	err := s.db.Order("view_position asc, created_at asc").Find(&nodes).Error
	return nodes, err
}

func (s *Service) GetByUUID(uuid string) (*database.Node, error) {
	var node database.Node
	err := s.db.Where("uuid = ?", uuid).First(&node).Error
	if err != nil {
		return nil, err
	}
	return &node, nil
}

func (s *Service) Update(uuid string, updates map[string]interface{}) (*database.Node, error) {
	updates["updated_at"] = time.Now().UTC()
	err := s.db.Model(&database.Node{}).Where("uuid = ?", uuid).Updates(updates).Error
	if err != nil {
		return nil, err
	}
	return s.GetByUUID(uuid)
}

func (s *Service) Delete(uuid string) error {
	return s.db.Where("uuid = ?", uuid).Delete(&database.Node{}).Error
}

type InboundInfo struct {
	Tag        string `gorm:"column:tag"`
	Type       string `gorm:"column:type"`
	Network    string `gorm:"column:network"`
	Security   string `gorm:"column:security"`
	RawInbound string `gorm:"column:raw_inbound"`
}

func getVlessFlow(network, security, rawInbound string) string {
	if rawInbound != "" {
		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(rawInbound), &raw); err == nil {
			if settings, ok := raw["settings"].(map[string]interface{}); ok {
				if f, ok := settings["flow"].(string); ok && f == "xtls-rprx-vision" {
					return "xtls-rprx-vision"
				}
			}
		}
	}
	if (network == "tcp" || network == "raw") && (security == "reality" || security == "tls") {
		return "xtls-rprx-vision"
	}
	return ""
}

func isSS2022(rawInbound string) bool {
	if rawInbound == "" {
		return false
	}
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(rawInbound), &raw); err == nil {
		if settings, ok := raw["settings"].(map[string]interface{}); ok {
			if method, ok := settings["method"].(string); ok {
				return strings.HasPrefix(method, "2022-")
			}
		}
	}
	return false
}

func (s *Service) GetUserResolvedInbounds(userID uint64) ([]InboundInfo, error) {
	var inbounds []InboundInfo
	err := s.db.Table("internal_squad_members").
		Select("config_profile_inbounds.tag, config_profile_inbounds.type, config_profile_inbounds.network, config_profile_inbounds.security, config_profile_inbounds.raw_inbound").
		Joins("JOIN internal_squads ON internal_squad_members.internal_squad_uuid = internal_squads.uuid").
		Joins("JOIN internal_squad_inbounds ON internal_squads.uuid = internal_squad_inbounds.internal_squad_uuid").
		Joins("JOIN config_profile_inbounds ON internal_squad_inbounds.inbound_uuid = config_profile_inbounds.uuid").
		Where("internal_squad_members.user_id = ?", userID).
		Scan(&inbounds).Error
	return inbounds, err
}

func (s *Service) SyncUserToNodes(user *database.User, prevVlessUUID *string) {
	if s.client == nil || user == nil {
		return
	}

	var connectedNodes []database.Node
	if err := s.db.Where("is_connected = ? AND is_disabled = ?", true, false).Find(&connectedNodes).Error; err != nil || len(connectedNodes) == 0 {
		return
	}

	inbounds, err := s.GetUserResolvedInbounds(user.ID)
	if err != nil {
		return
	}

	isActive := strings.ToUpper(user.Status) == "ACTIVE"

	for _, node := range connectedNodes {
		var activeTags []string
		s.db.Table("config_profile_inbounds_to_nodes").
			Select("config_profile_inbounds.tag").
			Joins("JOIN config_profile_inbounds ON config_profile_inbounds_to_nodes.config_profile_inbound_uuid = config_profile_inbounds.uuid").
			Where("config_profile_inbounds_to_nodes.node_uuid = ?", node.UUID).
			Pluck("tag", &activeTags)

		tagSet := make(map[string]bool)
		for _, t := range activeTags {
			tagSet[t] = true
		}

		var matchedData []map[string]interface{}
		if isActive {
			for _, inb := range inbounds {
				if !tagSet[inb.Tag] {
					continue
				}
				inbType := inb.Type
				if inbType == "shadowsocks" && isSS2022(inb.RawInbound) {
					inbType = "shadowsocks22"
				}
				uID := strconv.FormatUint(user.ID, 10)
				switch inbType {
				case "vless":
					flow := getVlessFlow(inb.Network, inb.Security, inb.RawInbound)
					matchedData = append(matchedData, map[string]interface{}{
						"type":     "vless",
						"tag":      inb.Tag,
						"username": uID,
						"uuid":     user.VlessUUID,
						"flow":     flow,
					})
				case "hysteria":
					matchedData = append(matchedData, map[string]interface{}{
						"type":     "hysteria",
						"tag":      inb.Tag,
						"username": uID,
						"password": user.VlessUUID,
					})
				case "trojan":
					matchedData = append(matchedData, map[string]interface{}{
						"type":     "trojan",
						"tag":      inb.Tag,
						"username": uID,
						"password": user.TrojanPassword,
					})
				case "shadowsocks", "shadowsocks22":
					matchedData = append(matchedData, map[string]interface{}{
						"type":     inbType,
						"tag":      inb.Tag,
						"username": uID,
						"password": user.SsPassword,
					})
				}
			}
		}

		if len(matchedData) > 0 {
			req := AddUserRequestPayload{
				HashData: NodeUserHashData{
					VlessUUID:     user.VlessUUID,
					PrevVlessUUID: prevVlessUUID,
				},
				Data: matchedData,
			}
			go func(n database.Node, r AddUserRequestPayload) {
				_ = s.client.AddUser(&n, r)
			}(node, req)
		} else {
			req := RemoveUserRequestPayload{
				Username: strconv.FormatUint(user.ID, 10),
				HashData: NodeUserHashData{
					VlessUUID: user.VlessUUID,
				},
			}
			go func(n database.Node, r RemoveUserRequestPayload) {
				_ = s.client.RemoveUser(&n, r)
			}(node, req)
		}
	}
}

func (s *Service) RemoveUserFromNodes(user *database.User) {
	if s.client == nil || user == nil {
		return
	}

	var connectedNodes []database.Node
	if err := s.db.Where("is_connected = ? AND is_disabled = ?", true, false).Find(&connectedNodes).Error; err != nil || len(connectedNodes) == 0 {
		return
	}

	req := RemoveUserRequestPayload{
		Username: strconv.FormatUint(user.ID, 10),
		HashData: NodeUserHashData{
			VlessUUID: user.VlessUUID,
		},
	}
	for _, node := range connectedNodes {
		go func(n database.Node, r RemoveUserRequestPayload) {
			_ = s.client.RemoveUser(&n, r)
		}(node, req)
	}
}

func (s *Service) SyncAllUsersToConnectedNodes() {
	var users []database.User
	if err := s.db.Where("status = ?", "ACTIVE").Find(&users).Error; err != nil {
		return
	}
	for i := range users {
		s.SyncUserToNodes(&users[i], nil)
	}
}

