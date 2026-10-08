package stubs

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"github.com/golang-jwt/jwt/v5"

	"sync"

	"remnawave-go/internal/auth"
	"remnawave-go/internal/database"
	"remnawave-go/internal/keygen"
	"remnawave-go/internal/nodes"
)

type JobResult struct {
	IsCompleted bool                   `json:"isCompleted"`
	IsFailed    bool                   `json:"isFailed"`
	Progress    map[string]interface{} `json:"progress,omitempty"`
	Result      interface{}            `json:"result"`
	CreatedAt   time.Time              `json:"-"`
}

type StubHandler struct {
	db          *gorm.DB
	keygenSvc   *keygen.Service
	appSecret   string
	jwtLifetime int
	nodeClient  *nodes.Client
	jobs        sync.Map
}

func NewHandler(db *gorm.DB, appSecret string, jwtLifetime int, nodeClient ...*nodes.Client) *StubHandler {
	var nc *nodes.Client
	if len(nodeClient) > 0 {
		nc = nodeClient[0]
	}
	return &StubHandler{
		db:          db,
		keygenSvc:   keygen.NewService(db),
		appSecret:   appSecret,
		jwtLifetime: jwtLifetime,
		nodeClient:  nc,
	}
}

func (s *StubHandler) getNodeClient() *nodes.Client {
	if s.nodeClient != nil {
		return s.nodeClient
	}
	km, err := s.keygenSvc.GetMasterKeygen()
	if err != nil {
		return nil
	}
	client, err := nodes.NewNodeClient([]byte(km.CACert), []byte(km.ClientCert), []byte(km.ClientKey), []byte(km.PrivKey), []byte(km.PubKey))
	if err != nil {
		return nil
	}
	s.nodeClient = client
	return client
}

func formatLastSeen(v interface{}) string {
	if v == nil {
		return time.Now().UTC().Format(time.RFC3339)
	}
	switch val := v.(type) {
	case string:
		if val != "" {
			t, err := time.Parse(time.RFC3339, val)
			if err == nil {
				return t.UTC().Format(time.RFC3339)
			}
			t, err = time.Parse("2006-01-02T15:04:05.000Z", val)
			if err == nil {
				return t.UTC().Format(time.RFC3339)
			}
			return val
		}
	case float64:
		t := time.Unix(int64(val), 0)
		if val > 1e11 {
			t = time.UnixMilli(int64(val))
		}
		return t.UTC().Format(time.RFC3339)
	case int64:
		t := time.Unix(val, 0)
		if val > 1e11 {
			t = time.UnixMilli(val)
		}
		return t.UTC().Format(time.RFC3339)
	case time.Time:
		return val.UTC().Format(time.RFC3339)
	}
	return time.Now().UTC().Format(time.RFC3339)
}

func (s *StubHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/node-plugins/torrent-blocker/stats", s.GetTorrentBlockerStats)
	r.Get("/api/node-plugins/torrent-blocker", s.GetTorrentBlockerReports)
	r.Post("/api/node-plugins/torrent-blocker/truncate", s.TruncateTorrentBlockerReports)
	r.Delete("/api/node-plugins/torrent-blocker/truncate", s.TruncateTorrentBlockerReports)

	r.Get("/api/node-plugins/shared-lists/by-name", s.GetSharedListByName)
	r.Get("/api/node-plugins/shared-lists", s.GetSharedLists)
	r.Post("/api/node-plugins/shared-lists", s.CreateSharedList)
	r.Patch("/api/node-plugins/shared-lists", s.UpdateSharedList)
	r.Delete("/api/node-plugins/shared-lists", s.DeleteSharedList)
	r.Post("/api/node-plugins/shared-lists/actions/sync", s.SyncSharedLists)

	r.Get("/api/node-plugins/tags", s.GetNodePluginsTags)
	r.Patch("/api/node-plugins/tags", s.SetNodePluginsTags)
	r.Get("/api/node-plugins", s.GetNodePlugins)
	r.Post("/api/node-plugins", s.CreateNodePlugin)
	r.Get("/api/node-plugins/{uuid}", s.GetNodePluginByUuid)
	r.Patch("/api/node-plugins", s.UpdateNodePlugin)
	r.Delete("/api/node-plugins/{uuid}", s.DeleteNodePlugin)
	r.Post("/api/node-plugins/actions/clone", s.CloneNodePlugin)
	r.Post("/api/node-plugins/actions/reorder", s.ReorderNodePlugins)
	r.Post("/api/node-plugins/actions/sync", s.SyncNodePlugin)
	r.Post("/api/node-plugins/executor", s.ExecuteNodePlugin)

	r.Get("/api/node-integrations", s.GetNodeIntegrations)
	r.Post("/api/node-integrations", s.CreateNodeIntegration)
	r.Get("/api/node-integrations/{uuid}", s.GetNodeIntegration)
	r.Patch("/api/node-integrations", s.UpdateNodeIntegration)
	r.Delete("/api/node-integrations/{uuid}", s.DeleteNodeIntegration)

	r.Get("/api/keygen", s.GetKeygen)

	r.Get("/api/snippets", s.GetSnippets)
	r.Post("/api/snippets", s.CreateSnippet)
	r.Patch("/api/snippets", s.UpdateSnippet)
	r.Delete("/api/snippets", s.DeleteSnippet)
	r.Post("/api/snippets/actions/sync", s.SyncSnippets)


	r.Get("/api/passkeys", s.GetPasskeys)
	r.Patch("/api/passkeys", s.UpdatePasskey)
	r.Delete("/api/passkeys", s.DeletePasskey)
	r.Get("/api/passkeys/registration/options", s.GetPasskeyRegOptions)
	r.Post("/api/passkeys/registration/verify", s.VerifyPasskeyReg)

	r.Post("/api/node-ssh/{uuid}/ticket", s.CreateSshTicket)
	r.Post("/api/node-ssh/vault/evaluate", s.EvaluateVault)

	r.Get("/api/hwid/devices/stats", s.GetHwidStats)
	r.Get("/api/hwid/devices/top-users", s.GetHwidTopUsers)
	r.Get("/api/hwid/devices", s.GetHwidDevices)
	r.Post("/api/hwid/devices", s.CreateHwidDevice)
	r.Post("/api/hwid/devices/delete", s.DeleteHwidDevice)
	r.Post("/api/hwid/devices/delete-all", s.DeleteAllHwidDevices)
	r.Get("/api/hwid/devices/{userId}", s.GetUserHwidDevices)

	r.Post("/api/connections/by-user/{userId}", s.ConnectionsByUser)
	r.Get("/api/connections/by-user/{jobId}", s.ConnectionsByUserResult)
	r.Post("/api/connections/by-node/{nodeUuid}", s.ConnectionsByNode)
	r.Get("/api/connections/by-node/{jobId}", s.ConnectionsByNodeResult)
	r.Post("/api/connections/geocheck/{nodeUuid}", s.GeocheckByNode)
	r.Get("/api/connections/geocheck/{jobId}", s.GeocheckByNodeResult)
	r.Post("/api/connections/drop", s.DropConnections)

	r.Get("/api/metadata/node/{uuid}", s.GetNodeMetadata)
	r.Put("/api/metadata/node/{uuid}", s.UpsertNodeMetadata)
	r.Get("/api/metadata/user/{userId}", s.GetUserMetadata)
	r.Put("/api/metadata/user/{userId}", s.UpsertUserMetadata)

	r.Get("/api/subscriptions", s.GetAllSubscriptions)
	r.Get("/api/subscriptions/by-id/{userId}", s.GetSubscriptionById)
	r.Get("/api/subscriptions/by-short-uuid/{shortUuid}", s.GetSubscriptionByShortUuid)
	r.Get("/api/subscriptions/by-short-uuid/{shortUuid}/raw", s.GetRawSubscriptionByShortUuid)
	r.Get("/api/subscriptions/by-username/{username}", s.GetSubscriptionByUsername)
	r.Get("/api/subscriptions/connection-keys/{userId}", s.GetConnectionKeysByUserId)
	r.Get("/api/subscriptions/subpage-config/{shortUuid}", s.GetSubpageConfigByShortUuid)

	r.Get("/api/subscription-request-history/stats", s.GetSubHistoryStats)
	r.Get("/api/subscription-request-history", s.GetSubHistory)

	r.Post("/api/bandwidth-stats/nodes/usage", s.GetBandwidthNodesUsage)
	r.Post("/api/bandwidth-stats/nodes/users", s.GetBandwidthStatsNodeUsers)
	r.Get("/api/bandwidth-stats/nodes/{uuid}/users", s.GetBandwidthStatsNodeUsers)
	r.Get("/api/bandwidth-stats/users/{userId}", s.GetBandwidthStatsUsers)
	r.Get("/api/bandwidth-stats/nodes", s.GetBandwidthStatsNodes)
	r.Get("/api/bandwidth-stats/internal-squads/{uuid}/usage", s.GetBandwidthSquadUsage)
	r.Get("/api/bandwidth-stats/internal-squads/{squadUuid}/users/{userId}/usage", s.GetBandwidthSquadUserUsage)

	r.Get("/api/system/configuration", s.GetSystemConfiguration)
	r.Get("/api/system/stats/digest", s.GetStatsDigest)
	r.Post("/api/system/testers/srr-matcher", s.DebugSrrMatcher)
	r.Get("/api/system/tools/x25519/generate", s.GenerateX25519)
}

func formatPlugin(p *database.NodePlugin) map[string]interface{} {
	var tags []string
	if p.Tags != "" {
		_ = json.Unmarshal([]byte(p.Tags), &tags)
	}
	if tags == nil {
		tags = []string{}
	}
	var cfg interface{}
	if p.PluginConfig != "" && p.PluginConfig != "null" {
		_ = json.Unmarshal([]byte(p.PluginConfig), &cfg)
	}
	return map[string]interface{}{
		"uuid":         p.UUID,
		"viewPosition": p.ViewPosition,
		"name":         p.Name,
		"tags":         tags,
		"pluginConfig": cfg,
	}
}

func (s *StubHandler) GetNodePlugins(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var plugins []database.NodePlugin
	s.db.Order("view_position asc, created_at asc").Find(&plugins)
	res := make([]map[string]interface{}, 0, len(plugins))
	for _, p := range plugins {
		res = append(res, formatPlugin(&p))
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"total":       len(res),
			"nodePlugins": res,
		},
	})
}

func (s *StubHandler) GetNodePluginByUuid(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	uuidParam := chi.URLParam(r, "uuid")
	var p database.NodePlugin
	if err := s.db.Where("uuid = ?", uuidParam).First(&p).Error; err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Plugin not found"})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": formatPlugin(&p),
	})
}

func (s *StubHandler) CreateNodePlugin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var body struct {
		Name         string      `json:"name"`
		PluginConfig interface{} `json:"pluginConfig"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	var count int64
	s.db.Model(&database.NodePlugin{}).Count(&count)
	now := time.Now().UTC()
	cfgBytes, _ := json.Marshal(body.PluginConfig)

	p := database.NodePlugin{
		UUID:         uuid.New().String(),
		ViewPosition: int(count) + 1,
		Name:         strings.TrimSpace(body.Name),
		Tags:         "[]",
		PluginConfig: string(cfgBytes),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	s.db.Create(&p)
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": formatPlugin(&p),
	})
}

func (s *StubHandler) UpdateNodePlugin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var body struct {
		UUID         string      `json:"uuid"`
		Name         *string     `json:"name"`
		PluginConfig interface{} `json:"pluginConfig"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	var p database.NodePlugin
	if err := s.db.Where("uuid = ?", body.UUID).First(&p).Error; err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Plugin not found"})
		return
	}
	if body.Name != nil {
		p.Name = strings.TrimSpace(*body.Name)
	}
	if body.PluginConfig != nil {
		b, _ := json.Marshal(body.PluginConfig)
		p.PluginConfig = string(b)
	}
	p.UpdatedAt = time.Now().UTC()
	s.db.Save(&p)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": formatPlugin(&p),
	})
}

func (s *StubHandler) DeleteNodePlugin(w http.ResponseWriter, r *http.Request) {
	uuidParam := chi.URLParam(r, "uuid")
	s.db.Where("uuid = ?", uuidParam).Delete(&database.NodePlugin{})
	w.WriteHeader(http.StatusNoContent)
}

func (s *StubHandler) CloneNodePlugin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"uuid":         uuid.New().String(),
			"viewPosition": 1,
			"name":         "Cloned Plugin",
			"tags":         []string{},
			"pluginConfig": nil,
		},
	})
}

func (s *StubHandler) ReorderNodePlugins(w http.ResponseWriter, r *http.Request) {
	s.GetNodePlugins(w, r)
}

func (s *StubHandler) SyncNodePlugin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"response": map[string]interface{}{"success": true}})
}

func (s *StubHandler) ExecuteNodePlugin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"response": map[string]interface{}{"success": true}})
}

func (s *StubHandler) GetNodePluginsTags(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"tags": []string{},
		},
	})
}

func (s *StubHandler) SetNodePluginsTags(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"tags": []string{},
		},
	})
}

func (s *StubHandler) GetSharedLists(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var lists []database.SharedList
	s.db.Find(&lists)
	res := make([]map[string]interface{}, 0, len(lists))
	for _, l := range lists {
		var cfg interface{}
		_ = json.Unmarshal([]byte(l.Config), &cfg)
		res = append(res, map[string]interface{}{
			"name":   l.Name,
			"config": cfg,
		})
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"total":       len(res),
			"sharedLists": res,
		},
	})
}

func (s *StubHandler) GetSharedListByName(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	name := r.URL.Query().Get("name")
	if name == "" {
		name = "default"
	}
	var l database.SharedList
	if err := s.db.Where("name = ?", name).First(&l).Error; err == nil {
		var cfg interface{}
		_ = json.Unmarshal([]byte(l.Config), &cfg)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"response": map[string]interface{}{
				"name":   l.Name,
				"config": cfg,
			},
		})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"name":   name,
			"config": map[string]interface{}{},
		},
	})
}

func (s *StubHandler) CreateSharedList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var body struct {
		Name   string      `json:"name"`
		Config interface{} `json:"config"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	cfgBytes, _ := json.Marshal(body.Config)
	l := database.SharedList{
		Name:      strings.TrimSpace(body.Name),
		Config:    string(cfgBytes),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	s.db.Save(&l)
	w.WriteHeader(http.StatusCreated)
	s.GetSharedLists(w, r)
}

func (s *StubHandler) UpdateSharedList(w http.ResponseWriter, r *http.Request) {
	s.CreateSharedList(w, r)
}

func (s *StubHandler) DeleteSharedList(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	s.db.Where("name = ?", body.Name).Delete(&database.SharedList{})
	w.WriteHeader(http.StatusNoContent)
}

func (s *StubHandler) SyncSharedLists(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"response": map[string]interface{}{"success": true}})
}

func (s *StubHandler) GetTorrentBlockerStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"stats": map[string]interface{}{
				"distinctNodes":      0,
				"distinctUsers":      0,
				"totalReports":       0,
				"reportsLast24Hours": 0,
			},
			"topUsers": []interface{}{},
			"topNodes": []interface{}{},
		},
	})
}

func (s *StubHandler) GetTorrentBlockerReports(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"records": []interface{}{},
			"total":   0,
		},
	})
}

func (s *StubHandler) TruncateTorrentBlockerReports(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"success": true,
		},
	})
}

func (s *StubHandler) GetNodeIntegrations(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"total":            0,
			"nodeIntegrations": []interface{}{},
		},
	})
}

func (s *StubHandler) GetNodeIntegration(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	uuidParam := chi.URLParam(r, "uuid")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"uuid": uuidParam,
			"name": "Integration",
		},
	})
}

func (s *StubHandler) CreateNodeIntegration(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	s.GetNodeIntegrations(w, r)
}

func (s *StubHandler) UpdateNodeIntegration(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	s.GetNodeIntegrations(w, r)
}

func (s *StubHandler) DeleteNodeIntegration(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func (s *StubHandler) GetKeygen(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	secretKey, err := s.keygenSvc.GenerateNodeSecretKey()
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"message":"Failed generating node secret key: %v"}`, err), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"secretKey": secretKey,
		},
	})
}

func (s *StubHandler) GetSnippets(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var snippets []database.ConfigProfileSnippet
	s.db.Find(&snippets)
	res := make([]map[string]interface{}, 0, len(snippets))
	for _, sn := range snippets {
		var content interface{}
		_ = json.Unmarshal([]byte(sn.Snippet), &content)
		res = append(res, map[string]interface{}{
			"name":    sn.Name,
			"snippet": content,
		})
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"total":    len(res),
			"snippets": res,
		},
	})
}

func (s *StubHandler) CreateSnippet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var body struct {
		Name    string      `json:"name"`
		Snippet interface{} `json:"snippet"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	b, _ := json.Marshal(body.Snippet)
	sn := database.ConfigProfileSnippet{
		Name:      strings.TrimSpace(body.Name),
		Snippet:   string(b),
		CreatedAt: time.Now().UTC(),
	}
	s.db.Save(&sn)
	w.WriteHeader(http.StatusCreated)
	s.GetSnippets(w, r)
}

func (s *StubHandler) UpdateSnippet(w http.ResponseWriter, r *http.Request) {
	s.CreateSnippet(w, r)
}

func (s *StubHandler) DeleteSnippet(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	s.db.Where("name = ?", body.Name).Delete(&database.ConfigProfileSnippet{})
	w.WriteHeader(http.StatusNoContent)
}

func (s *StubHandler) SyncSnippets(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"response": map[string]interface{}{"success": true}})
}

func (s *StubHandler) GetApiTokens(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var tokens []database.ApiToken
	s.db.Find(&tokens)
	res := make([]map[string]interface{}, 0, len(tokens))
	for _, t := range tokens {
		res = append(res, map[string]interface{}{
			"uuid":      t.UUID,
			"name":      t.Name,
			"expireAt":  t.ExpireAt.UTC().Format("2006-01-02T15:04:05.000Z"),
			"scopes":    []string{"*"},
			"createdAt": t.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
			"updatedAt": t.UpdatedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
		})
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"tokens": res,
		},
	})
}

func (s *StubHandler) CreateApiToken(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var body struct {
		Name     string    `json:"name"`
		ExpireAt time.Time `json:"expireAt"`
		Scopes   []string  `json:"scopes"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	tokBytes := make([]byte, 32)
	rand.Read(tokBytes)
	tokenStr := fmt.Sprintf("rw_%s", base64.RawURLEncoding.EncodeToString(tokBytes))
	now := time.Now().UTC()

	t := database.ApiToken{
		UUID:      uuid.New().String(),
		Name:      strings.TrimSpace(body.Name),
		ExpireAt:  body.ExpireAt,
		Token:     tokenStr,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.db.Create(&t)

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"uuid":      t.UUID,
			"name":      t.Name,
			"expireAt":  t.ExpireAt.UTC().Format("2006-01-02T15:04:05.000Z"),
			"scopes":    body.Scopes,
			"createdAt": t.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
			"updatedAt": t.UpdatedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
			"token":     tokenStr,
		},
	})
}

func (s *StubHandler) DeleteApiToken(w http.ResponseWriter, r *http.Request) {
	uuidParam := chi.URLParam(r, "uuid")
	s.db.Where("uuid = ?", uuidParam).Delete(&database.ApiToken{})
	w.WriteHeader(http.StatusNoContent)
}

func (s *StubHandler) GetApiTokenScopes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"wildcard":  "*",
			"resources": []interface{}{},
		},
	})
}

func (s *StubHandler) GetPasskeys(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var passkeys []database.Passkey
	s.db.Order("created_at desc").Find(&passkeys)
	list := make([]map[string]interface{}, len(passkeys))
	for i, pk := range passkeys {
		name := pk.PasskeyProvider
		if name == "" {
			name = "Passkey"
		}
		list[i] = map[string]interface{}{
			"id":         pk.ID,
			"name":       name,
			"createdAt":  pk.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
			"lastUsedAt": pk.UpdatedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
		}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"passkeys": list,
		},
	})
}

func (s *StubHandler) UpdatePasskey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.ID != "" && body.Name != "" {
		s.db.Model(&database.Passkey{}).Where("id = ?", body.ID).Updates(map[string]interface{}{
			"passkey_provider": body.Name,
			"updated_at":       time.Now(),
		})
	}
	s.GetPasskeys(w, r)
}

func (s *StubHandler) DeletePasskey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.ID != "" {
		s.db.Where("id = ?", body.ID).Delete(&database.Passkey{})
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *StubHandler) GetPasskeyRegOptions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var setting database.RemnawaveSetting
	s.db.First(&setting)
	rpId := "localhost"
	if setting.PasskeySettings != "" && setting.PasskeySettings != "{}" {
		var ps map[string]interface{}
		if err := json.Unmarshal([]byte(setting.PasskeySettings), &ps); err == nil {
			if id, ok := ps["rpId"].(string); ok && id != "" {
			rpId = id
			}
		}
	}
	var admin database.Admin
	s.db.First(&admin)
	adminUuid := admin.UUID
	if adminUuid == "" {
		adminUuid = "00000000-0000-0000-0000-000000000000"
	}
	username := admin.Username
	if username == "" {
		username = "admin"
	}
	chalBytes := make([]byte, 32)
	rand.Read(chalBytes)
	challenge := base64.RawURLEncoding.EncodeToString(chalBytes)
	userIdBytes := []byte(adminUuid)
	userId := base64.RawURLEncoding.EncodeToString(userIdBytes)

	var existing []database.Passkey
	s.db.Where("admin_uuid = ?", adminUuid).Find(&existing)
	exclude := make([]map[string]interface{}, 0, len(existing))
	for _, pk := range existing {
		exclude = append(exclude, map[string]interface{}{
			"id":         pk.ID,
			"type":       "public-key",
			"transports": []string{"internal"},
		})
	}

	options := map[string]interface{}{
		"challenge": challenge,
		"rp": map[string]interface{}{
			"name": "Remnawave",
			"id":   rpId,
		},
		"user": map[string]interface{}{
			"id":          userId,
			"name":        username,
			"displayName": "Remnawave Administrator",
		},
		"pubKeyCredParams": []map[string]interface{}{
			{"type": "public-key", "alg": -7},
			{"type": "public-key", "alg": -257},
		},
		"timeout": 60000,
		"attestation": "none",
		"excludeCredentials": exclude,
		"authenticatorSelection": map[string]interface{}{
			"residentKey":      "preferred",
			"userVerification": "required",
		},
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": options,
	})
}

func (s *StubHandler) VerifyPasskeyReg(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var body struct {
		Response struct {
			ID       string `json:"id"`
			RawID    string `json:"rawId"`
			Response struct {
				ClientDataJSON    string `json:"clientDataJSON"`
				AttestationObject string `json:"attestationObject"`
			} `json:"response"`
			Type string `json:"type"`
		} `json:"response"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	var admin database.Admin
	s.db.First(&admin)
	credId := body.Response.ID
	if credId == "" {
		credId = uuid.New().String()
	}
	pk := database.Passkey{
		ID:              credId,
		AdminUUID:       admin.UUID,
		PasskeyProvider: "Touch ID / Apple",
		DeviceType:      "multiDevice",
		BackedUp:        true,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	s.db.Save(&pk)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"verified": true,
			"message":  "Passkey registered successfully",
		},
	})
}

func (s *StubHandler) GetPasskeyAuthOptions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var setting database.RemnawaveSetting
	s.db.First(&setting)
	rpId := "localhost"
	if setting.PasskeySettings != "" && setting.PasskeySettings != "{}" {
		var ps map[string]interface{}
		if err := json.Unmarshal([]byte(setting.PasskeySettings), &ps); err == nil {
			if id, ok := ps["rpId"].(string); ok && id != "" {
				rpId = id
			}
		}
	}
	if rpId == "localhost" && r.Host != "" {
		host := r.Host
		if strings.Contains(host, ":") {
			host = strings.Split(host, ":")[0]
		}
		rpId = host
	}

	chalBytes := make([]byte, 32)
	rand.Read(chalBytes)
	challenge := base64.RawURLEncoding.EncodeToString(chalBytes)

	var passkeys []database.Passkey
	s.db.Find(&passkeys)
	allow := make([]map[string]interface{}, 0, len(passkeys))
	for _, pk := range passkeys {
		transports := []string{"internal"}
		if pk.Transports != "" {
			transports = strings.Split(pk.Transports, ",")
		}
		allow = append(allow, map[string]interface{}{
			"id":         pk.ID,
			"type":       "public-key",
			"transports": transports,
		})
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"challenge":        challenge,
			"timeout":          60000,
			"rpId":             rpId,
			"allowCredentials": allow,
		},
	})
}

func (s *StubHandler) VerifyPasskeyAuth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		Response struct {
			ID string `json:"id"`
		} `json:"response"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	var admin database.Admin
	if body.Response.ID != "" {
		var pk database.Passkey
		if err := s.db.Where("id = ?", body.Response.ID).First(&pk).Error; err == nil && pk.AdminUUID != "" {
			s.db.Where("uuid = ?", pk.AdminUUID).First(&admin)
		}
	}
	if admin.UUID == "" {
		if err := s.db.First(&admin).Error; err != nil {
			http.Error(w, `{"message":"No admin found"}`, http.StatusUnauthorized)
			return
		}
	}

	token, err := auth.GenerateToken(admin.UUID, admin.Username, admin.Role, s.appSecret, s.jwtLifetime)
	if err != nil {
		http.Error(w, `{"message":"Internal server error"}`, http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"accessToken": token,
		},
	})
}

func (s *StubHandler) GetOtt(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	claims := jwt.MapClaims{
		"scope": "ott",
		"iss":   "backend-tools",
		"exp":   time.Now().Add(30 * time.Second).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte("remnawave"))
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"ott": tokenString,
		},
	})
}

func (s *StubHandler) CreateSshTicket(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ticket := uuid.New().String()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"ticket":           ticket,
			"path":             "/api/node-ssh/ws",
			"expiresInSeconds": 60,
		},
	})
}

func (s *StubHandler) EvaluateVault(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var body struct {
		Blinded string `json:"blinded"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"evaluated": body.Blinded,
		},
	})
}

func (s *StubHandler) OAuth2TelegramCallback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"token": "",
		},
	})
}

func (s *StubHandler) OAuth2Authorize(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"response": map[string]interface{}{"url": ""}})
}

func (s *StubHandler) OAuth2Callback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"response": map[string]interface{}{"token": ""}})
}

func (s *StubHandler) GetHwidStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"byPlatform": []interface{}{},
			"stats": map[string]interface{}{
				"totalUniqueDevices":        0,
				"totalHwidDevices":          0,
				"averageHwidDevicesPerUser": 0,
			},
		},
	})
}

func (s *StubHandler) GetHwidTopUsers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"total": 0,
			"users": []interface{}{},
		},
	})
}

func (s *StubHandler) GetHwidDevices(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"devices": []interface{}{},
			"total":   0,
		},
	})
}

func (s *StubHandler) GetUserHwidDevices(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	userIdStr := chi.URLParam(r, "userId")
	userId, _ := strconv.ParseUint(userIdStr, 10, 64)
	var user database.User
	s.db.Where("id = ?", userId).First(&user)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"devices": []interface{}{},
			"user": map[string]interface{}{
				"id":              user.ID,
				"username":        user.Username,
				"hwidDeviceLimit": user.HWIDDeviceLimit,
			},
		},
	})
}

func (s *StubHandler) CreateHwidDevice(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	nowStr := time.Now().UTC().Format(time.RFC3339)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"hwid":        "00000000",
			"userId":      1,
			"platform":    nil,
			"osVersion":   nil,
			"deviceModel": nil,
			"userAgent":   nil,
			"requestIp":   nil,
			"createdAt":   nowStr,
			"updatedAt":   nowStr,
		},
	})
}

func (s *StubHandler) DeleteHwidDevice(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"response": map[string]interface{}{"success": true}})
}

func (s *StubHandler) DeleteAllHwidDevices(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"response": map[string]interface{}{"success": true}})
}

func (s *StubHandler) ConnectionsByUser(w http.ResponseWriter, r *http.Request) {
	userIdStr := chi.URLParam(r, "userId")
	userIdInt, _ := strconv.Atoi(userIdStr)
	jobId := uuid.New().String()

	s.jobs.Store(jobId, &JobResult{
		IsCompleted: false,
		IsFailed:    false,
		Progress: map[string]interface{}{
			"total":     0,
			"completed": 0,
			"percent":   0,
		},
		CreatedAt: time.Now(),
	})

	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				s.jobs.Store(jobId, &JobResult{
					IsCompleted: true,
					IsFailed:    true,
					CreatedAt:   time.Now(),
				})
			}
		}()

		var nodeList []database.Node
		s.db.Where("is_connected = ? AND is_disabled = ?", true, false).Find(&nodeList)

		client := s.getNodeClient()
		type NodeUserConn struct {
			NodeUUID    string                   `json:"nodeUuid"`
			NodeName    string                   `json:"nodeName"`
			CountryCode string                   `json:"countryCode"`
			IPs         []map[string]interface{} `json:"ips"`
		}
		userNodes := make([]NodeUserConn, 0)

		if client != nil && len(nodeList) > 0 {
			for _, n := range nodeList {
				users, err := client.GetUsersIpList(&n)
				if err == nil {
					for _, u := range users {
						if u.UserID == userIdInt && len(u.IPs) > 0 {
							ipsList := make([]map[string]interface{}, 0, len(u.IPs))
							for _, ipInfo := range u.IPs {
								ipsList = append(ipsList, map[string]interface{}{
									"ip":       ipInfo.IP,
									"lastSeen": formatLastSeen(ipInfo.LastSeen),
								})
							}
							userNodes = append(userNodes, NodeUserConn{
								NodeUUID:    n.UUID,
								NodeName:    n.Name,
								CountryCode: n.CountryCode,
								IPs:         ipsList,
							})
						}
					}
				}
			}
		}

		s.jobs.Store(jobId, &JobResult{
			IsCompleted: true,
			IsFailed:    false,
			Progress: map[string]interface{}{
				"total":     len(nodeList),
				"completed": len(nodeList),
				"percent":   100,
			},
			Result: map[string]interface{}{
				"success": true,
				"userId":  userIdInt,
				"nodes":   userNodes,
			},
			CreatedAt: time.Now(),
		})
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"jobId": jobId,
		},
	})
}

func (s *StubHandler) ConnectionsByUserResult(w http.ResponseWriter, r *http.Request) {
	jobId := chi.URLParam(r, "jobId")
	w.Header().Set("Content-Type", "application/json")

	val, ok := s.jobs.Load(jobId)
	if !ok {
		userIdInt, _ := strconv.Atoi(chi.URLParam(r, "userId"))
		json.NewEncoder(w).Encode(map[string]interface{}{
			"response": map[string]interface{}{
				"isCompleted": true,
				"isFailed":    false,
				"progress": map[string]interface{}{
					"total":     1,
					"completed": 1,
					"percent":   100,
				},
				"result": map[string]interface{}{
					"success": true,
					"userId":  userIdInt,
					"nodes":   []interface{}{},
				},
			},
		})
		return
	}

	job := val.(*JobResult)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": job,
	})
}

func (s *StubHandler) ConnectionsByNode(w http.ResponseWriter, r *http.Request) {
	nodeUuid := chi.URLParam(r, "nodeUuid")
	jobId := uuid.New().String()

	s.jobs.Store(jobId, &JobResult{
		IsCompleted: false,
		IsFailed:    false,
		CreatedAt:   time.Now(),
	})

	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				s.jobs.Store(jobId, &JobResult{
					IsCompleted: true,
					IsFailed:    true,
					Result: map[string]interface{}{
						"success":  false,
						"nodeUuid": nodeUuid,
						"users":    []interface{}{},
					},
					CreatedAt: time.Now(),
				})
			}
		}()

		var node database.Node
		if err := s.db.Where("uuid = ?", nodeUuid).First(&node).Error; err != nil {
			s.jobs.Store(jobId, &JobResult{
				IsCompleted: true,
				IsFailed:    true,
				Result: map[string]interface{}{
					"success":  false,
					"nodeUuid": nodeUuid,
					"users":    []interface{}{},
				},
				CreatedAt: time.Now(),
			})
			return
		}

		client := s.getNodeClient()
		if client == nil {
			s.jobs.Store(jobId, &JobResult{
				IsCompleted: true,
				IsFailed:    false,
				Result: map[string]interface{}{
					"success":  true,
					"nodeUuid": nodeUuid,
					"users":    []interface{}{},
				},
				CreatedAt: time.Now(),
			})
			return
		}

		rawUsers, err := client.GetUsersIpList(&node)
		if err != nil {
			s.jobs.Store(jobId, &JobResult{
				IsCompleted: true,
				IsFailed:    false,
				Result: map[string]interface{}{
					"success":  false,
					"nodeUuid": nodeUuid,
					"users":    []interface{}{},
				},
				CreatedAt: time.Now(),
			})
			return
		}

		type NodeUserFormatted struct {
			UserID int                      `json:"userId"`
			IPs    []map[string]interface{} `json:"ips"`
		}
		formattedUsers := make([]NodeUserFormatted, 0, len(rawUsers))
		for _, u := range rawUsers {
			ipsList := make([]map[string]interface{}, 0, len(u.IPs))
			for _, ipInfo := range u.IPs {
				ipsList = append(ipsList, map[string]interface{}{
					"ip":       ipInfo.IP,
					"lastSeen": formatLastSeen(ipInfo.LastSeen),
				})
			}
			formattedUsers = append(formattedUsers, NodeUserFormatted{
				UserID: u.UserID,
				IPs:    ipsList,
			})
		}

		s.jobs.Store(jobId, &JobResult{
			IsCompleted: true,
			IsFailed:    false,
			Result: map[string]interface{}{
				"success":  true,
				"nodeUuid": nodeUuid,
				"users":    formattedUsers,
			},
			CreatedAt: time.Now(),
		})
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"jobId": jobId,
		},
	})
}

func (s *StubHandler) ConnectionsByNodeResult(w http.ResponseWriter, r *http.Request) {
	jobId := chi.URLParam(r, "jobId")
	w.Header().Set("Content-Type", "application/json")

	val, ok := s.jobs.Load(jobId)
	if !ok {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"response": map[string]interface{}{
				"isCompleted": true,
				"isFailed":    false,
				"result": map[string]interface{}{
					"success":  true,
					"nodeUuid": jobId,
					"users":    []interface{}{},
				},
			},
		})
		return
	}

	job := val.(*JobResult)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": job,
	})
}

func (s *StubHandler) GeocheckByNode(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"jobId": uuid.New().String(),
		},
	})
}

func (s *StubHandler) GeocheckByNodeResult(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"isCompleted": true,
			"isFailed":    false,
			"result": map[string]interface{}{
				"success":   true,
				"nodeUuid":  chi.URLParam(r, "jobId"),
				"image":     nil,
				"rawReport": map[string]interface{}{},
				"message":   "Geocheck completed",
			},
		},
	})
}

func (s *StubHandler) DropConnections(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"success": true,
		},
	})
}

func (s *StubHandler) GetNodeMetadata(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	uuidParam := chi.URLParam(r, "uuid")
	var meta database.EntityMeta
	var res map[string]interface{}
	if err := s.db.Where("entity_id = ? AND entity_type = 'node'", uuidParam).First(&meta).Error; err == nil {
		_ = json.Unmarshal([]byte(meta.Metadata), &res)
	}
	if res == nil {
		res = map[string]interface{}{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"metadata": res,
		},
	})
}

func (s *StubHandler) UpsertNodeMetadata(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	uuidParam := chi.URLParam(r, "uuid")
	var body struct {
		Metadata interface{} `json:"metadata"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	b, _ := json.Marshal(body.Metadata)
	meta := database.EntityMeta{
		EntityID:   uuidParam,
		EntityType: "node",
		Metadata:   string(b),
	}
	s.db.Save(&meta)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"metadata": body.Metadata,
		},
	})
}

func (s *StubHandler) GetUserMetadata(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	userId := chi.URLParam(r, "userId")
	var meta database.EntityMeta
	var res map[string]interface{}
	if err := s.db.Where("entity_id = ? AND entity_type = 'user'", userId).First(&meta).Error; err == nil {
		_ = json.Unmarshal([]byte(meta.Metadata), &res)
	}
	if res == nil {
		res = map[string]interface{}{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"metadata": res,
		},
	})
}

func (s *StubHandler) UpsertUserMetadata(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	userId := chi.URLParam(r, "userId")
	var body struct {
		Metadata interface{} `json:"metadata"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	b, _ := json.Marshal(body.Metadata)
	meta := database.EntityMeta{
		EntityID:   userId,
		EntityType: "user",
		Metadata:   string(b),
	}
	s.db.Save(&meta)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"metadata": body.Metadata,
		},
	})
}

func (s *StubHandler) GetAllSubscriptions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"total":         0,
			"subscriptions": []interface{}{},
		},
	})
}

func (s *StubHandler) GetSubscriptionById(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"subscription": map[string]interface{}{},
		},
	})
}

func (s *StubHandler) GetSubscriptionByShortUuid(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"subscription": map[string]interface{}{},
		},
	})
}

func (s *StubHandler) GetRawSubscriptionByShortUuid(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"subscription": "",
		},
	})
}

func (s *StubHandler) GetSubscriptionByUsername(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"subscription": map[string]interface{}{},
		},
	})
}

func (s *StubHandler) GetConnectionKeysByUserId(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"connectionKeys": []interface{}{},
		},
	})
}

func (s *StubHandler) GetSubpageConfigByShortUuid(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"config": map[string]interface{}{},
		},
	})
}

func (s *StubHandler) GetSystemConfiguration(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"env": map[string]interface{}{},
		},
	})
}

func (s *StubHandler) GetStatsDigest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"users": map[string]interface{}{"total": 0},
			"nodes": map[string]interface{}{"total": 0},
		},
	})
}

func (s *StubHandler) DebugSrrMatcher(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"matched":       false,
			"responseType":  "ALLOW",
			"matchedRule":   nil,
			"inputHeaders":  map[string]string{},
			"outputHeaders": map[string]string{},
		},
	})
}

func (s *StubHandler) GenerateX25519(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	priv := make([]byte, 32)
	rand.Read(priv)
	pub := make([]byte, 32)
	rand.Read(pub)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"privateKey": base64.RawURLEncoding.EncodeToString(priv),
			"publicKey":  base64.RawURLEncoding.EncodeToString(pub),
		},
	})
}

func (s *StubHandler) GetSubHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"total":   0,
			"records": []interface{}{},
		},
	})
}

func (s *StubHandler) GetSubHistoryStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"byParsedApp":        []interface{}{},
			"hourlyRequestStats": []interface{}{},
		},
	})
}

func (s *StubHandler) GetBandwidthStatsNodes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"categories":    []string{},
			"sparklineData": []float64{},
			"topNodes":      []interface{}{},
			"series":        []interface{}{},
		},
	})
}

func (s *StubHandler) GetBandwidthStatsUsers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"categories":    []string{},
			"sparklineData": []float64{},
			"topNodes":      []interface{}{},
			"series":        []interface{}{},
		},
	})
}

func (s *StubHandler) GetBandwidthStatsNodeUsers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"categories":    []string{},
			"sparklineData": []float64{},
			"topUsers":      []interface{}{},
		},
	})
}

func (s *StubHandler) GetBandwidthNodesUsage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"nodes": []interface{}{},
		},
	})
}

func (s *StubHandler) GetBandwidthSquadUsage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	uuidParam := chi.URLParam(r, "uuid")
	if uuidParam == "" {
		uuidParam = "00000000-0000-0000-0000-000000000000"
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"squadUuid":  uuidParam,
			"users":      []interface{}{},
			"nextCursor": nil,
			"hasMore":    false,
		},
	})
}

func (s *StubHandler) GetBandwidthSquadUserUsage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"days": []interface{}{},
		},
	})
}
