package squads

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"remnawave-go/internal/database"
)

type Handler struct {
	db *gorm.DB
}

func NewHandler(db *gorm.DB) *Handler {
	return &Handler{db: db}
}

func formatInternalSquad(db *gorm.DB, s *database.InternalSquad) map[string]interface{} {
	tags := []string{}
	if s.Tags != "" {
		_ = json.Unmarshal([]byte(s.Tags), &tags)
	}

	var membersCount int64
	db.Model(&database.InternalSquadMember{}).Where("internal_squad_uuid = ?", s.UUID).Count(&membersCount)

	var inboundsCount int64
	db.Model(&database.InternalSquadInbound{}).Where("internal_squad_uuid = ?", s.UUID).Count(&inboundsCount)

	var cpiList []database.ConfigProfileInbound
	db.Table("config_profile_inbounds").
		Joins("JOIN internal_squad_inbounds ON internal_squad_inbounds.inbound_uuid = config_profile_inbounds.uuid").
		Where("internal_squad_inbounds.internal_squad_uuid = ?", s.UUID).
		Find(&cpiList)

	inboundsRes := make([]map[string]interface{}, 0, len(cpiList))
	for _, inb := range cpiList {
		var rawInb interface{}
		if inb.RawInbound != "" && inb.RawInbound != "null" {
			_ = json.Unmarshal([]byte(inb.RawInbound), &rawInb)
		}
		inboundsRes = append(inboundsRes, map[string]interface{}{
			"uuid":        inb.UUID,
			"profileUuid": inb.ProfileUUID,
			"tag":         inb.Tag,
			"type":        inb.Type,
			"network":     inb.Network,
			"security":    inb.Security,
			"port":        inb.Port,
			"rawInbound":  rawInb,
		})
	}

	return map[string]interface{}{
		"uuid":         s.UUID,
		"viewPosition": s.ViewPosition,
		"name":         s.Name,
		"tags":         tags,
		"info": map[string]interface{}{
			"membersCount":  membersCount,
			"inboundsCount": inboundsCount,
		},
		"inbounds":  inboundsRes,
		"createdAt": s.CreatedAt.UTC().Format(time.RFC3339),
		"updatedAt": s.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func (h *Handler) GetInternalSquads(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var squads []database.InternalSquad
	h.db.Order("view_position asc, created_at asc").Find(&squads)

	res := make([]map[string]interface{}, 0, len(squads))
	for _, s := range squads {
		res = append(res, formatInternalSquad(h.db, &s))
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"total":          len(res),
			"internalSquads": res,
		},
	})
}

func (h *Handler) GetInternalSquad(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	uuidParam := chi.URLParam(r, "uuid")

	var s database.InternalSquad
	if err := h.db.Where("uuid = ?", uuidParam).First(&s).Error; err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Squad not found"})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": formatInternalSquad(h.db, &s),
	})
}

func (h *Handler) CreateInternalSquad(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		Name         string   `json:"name"`
		Inbounds     []string `json:"inbounds"`
		InboundUUIDs []string `json:"inboundUuids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Invalid body"})
		return
	}

	inboundIDs := body.Inbounds
	if len(inboundIDs) == 0 && len(body.InboundUUIDs) > 0 {
		inboundIDs = body.InboundUUIDs
	}

	var count int64
	h.db.Model(&database.InternalSquad{}).Count(&count)

	newUUID := uuid.New().String()
	now := time.Now().UTC()
	inbBytes, _ := json.Marshal(inboundIDs)

	s := database.InternalSquad{
		UUID:         newUUID,
		ViewPosition: int(count) + 1,
		Name:         strings.TrimSpace(body.Name),
		Tags:         "[]",
		Description:  "",
		InboundUUIDs: string(inbBytes),
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := h.db.Create(&s).Error; err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Failed to create squad"})
		return
	}

	for _, inbID := range inboundIDs {
		h.db.Create(&database.InternalSquadInbound{
			InternalSquadUUID: newUUID,
			InboundUUID:       inbID,
		})
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": formatInternalSquad(h.db, &s),
	})
}

func (h *Handler) UpdateInternalSquad(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		UUID         string    `json:"uuid"`
		Name         *string   `json:"name"`
		Inbounds     *[]string `json:"inbounds"`
		InboundUUIDs *[]string `json:"inboundUuids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Invalid body"})
		return
	}

	var s database.InternalSquad
	if err := h.db.Where("uuid = ?", body.UUID).First(&s).Error; err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Squad not found"})
		return
	}

	if body.Name != nil {
		s.Name = strings.TrimSpace(*body.Name)
	}

	var targetInbounds *[]string
	if body.Inbounds != nil {
		targetInbounds = body.Inbounds
	} else if body.InboundUUIDs != nil {
		targetInbounds = body.InboundUUIDs
	}

	if targetInbounds != nil {
		inbBytes, _ := json.Marshal(*targetInbounds)
		s.InboundUUIDs = string(inbBytes)

		// Sync internal_squad_inbounds
		h.db.Where("internal_squad_uuid = ?", s.UUID).Delete(&database.InternalSquadInbound{})
		for _, inbID := range *targetInbounds {
			h.db.Create(&database.InternalSquadInbound{
				InternalSquadUUID: s.UUID,
				InboundUUID:       inbID,
			})
		}
	}
	s.UpdatedAt = time.Now().UTC()

	if err := h.db.Save(&s).Error; err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Failed to update squad"})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": formatInternalSquad(h.db, &s),
	})
}

func (h *Handler) DeleteInternalSquad(w http.ResponseWriter, r *http.Request) {
	uuidParam := chi.URLParam(r, "uuid")
	h.db.Where("internal_squad_uuid = ?", uuidParam).Delete(&database.InternalSquadInbound{})
	h.db.Where("internal_squad_uuid = ?", uuidParam).Delete(&database.InternalSquadMember{})
	h.db.Where("uuid = ?", uuidParam).Delete(&database.InternalSquad{})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ReorderInternalSquads(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		Items []struct {
			UUID         string `json:"uuid"`
			ViewPosition int    `json:"viewPosition"`
		} `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Invalid body"})
		return
	}

	for _, item := range body.Items {
		h.db.Model(&database.InternalSquad{}).Where("uuid = ?", item.UUID).Update("view_position", item.ViewPosition)
	}

	var squads []database.InternalSquad
	h.db.Order("view_position asc, created_at asc").Find(&squads)
	res := make([]map[string]interface{}, 0, len(squads))
	for _, s := range squads {
		res = append(res, formatInternalSquad(h.db, &s))
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"total":          len(res),
			"internalSquads": res,
		},
	})
}

func (h *Handler) GetInternalSquadsTags(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var squads []database.InternalSquad
	h.db.Find(&squads)

	tagMap := make(map[string]bool)
	for _, s := range squads {
		if s.Tags != "" {
			var tags []string
			if err := json.Unmarshal([]byte(s.Tags), &tags); err == nil {
				for _, tag := range tags {
					tagMap[tag] = true
				}
			}
		}
	}

	tagList := make([]string, 0, len(tagMap))
	for tag := range tagMap {
		tagList = append(tagList, tag)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"tags": tagList,
		},
	})
}

func (h *Handler) SetInternalSquadsTags(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		UUID string   `json:"uuid"`
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Invalid body"})
		return
	}

	tagsBytes, _ := json.Marshal(body.Tags)
	h.db.Model(&database.InternalSquad{}).Where("uuid = ?", body.UUID).Update("tags", string(tagsBytes))

	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"uuid": body.UUID,
			"tags": body.Tags,
		},
	})
}

type accessibleNodeRow struct {
	NodeUUID          string `gorm:"column:node_uuid"`
	NodeName          string `gorm:"column:node_name"`
	CountryCode       string `gorm:"column:country_code"`
	ViewPosition      int    `gorm:"column:view_position"`
	ConfigProfileUUID string `gorm:"column:config_profile_uuid"`
	ConfigProfileName string `gorm:"column:config_profile_name"`
	InboundTag        string `gorm:"column:inbound_tag"`
}

func (h *Handler) GetInternalSquadAccessibleNodes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	uuidParam := chi.URLParam(r, "uuid")

	var rows []accessibleNodeRow
	h.db.Raw(`
		SELECT 
			n.uuid as node_uuid,
			n.name as node_name,
			n.country_code as country_code,
			n.view_position as view_position,
			cp.uuid as config_profile_uuid,
			cp.name as config_profile_name,
			cpi.tag as inbound_tag
		FROM nodes n
		INNER JOIN config_profiles cp ON n.active_config_profile_uuid = cp.uuid
		INNER JOIN config_profile_inbounds cpi ON cpi.profile_uuid = cp.uuid
		INNER JOIN config_profile_inbounds_to_nodes cpin ON cpin.config_profile_inbound_uuid = cpi.uuid AND cpin.node_uuid = n.uuid
		INNER JOIN internal_squad_inbounds isi ON isi.inbound_uuid = cpi.uuid
		WHERE isi.internal_squad_uuid = ?
		ORDER BY n.view_position ASC, n.created_at ASC
	`, uuidParam).Scan(&rows)

	type nodeInfo struct {
		UUID              string   `json:"uuid"`
		NodeName          string   `json:"nodeName"`
		CountryCode       string   `json:"countryCode"`
		ConfigProfileUUID string   `json:"configProfileUuid"`
		ConfigProfileName string   `json:"configProfileName"`
		ActiveInbounds    []string `json:"activeInbounds"`
	}

	orderedKeys := make([]string, 0)
	nodeMap := make(map[string]*nodeInfo)

	for _, r := range rows {
		if _, exists := nodeMap[r.NodeUUID]; !exists {
			orderedKeys = append(orderedKeys, r.NodeUUID)
			nodeMap[r.NodeUUID] = &nodeInfo{
				UUID:              r.NodeUUID,
				NodeName:          r.NodeName,
				CountryCode:       r.CountryCode,
				ConfigProfileUUID: r.ConfigProfileUUID,
				ConfigProfileName: r.ConfigProfileName,
				ActiveInbounds:    []string{},
			}
		}
		item := nodeMap[r.NodeUUID]
		item.ActiveInbounds = append(item.ActiveInbounds, r.InboundTag)
	}

	nodesList := make([]*nodeInfo, 0, len(orderedKeys))
	for _, key := range orderedKeys {
		nodesList = append(nodesList, nodeMap[key])
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"squadUuid":       uuidParam,
			"accessibleNodes": nodesList,
		},
	})
}

func (h *Handler) SquadBulkAction(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	uuidParam := chi.URLParam(r, "uuid")
	path := r.URL.Path

	if strings.Contains(path, "/internal-squads/") {
		if strings.HasSuffix(path, "/add-users") {
			// Add all users
			h.db.Exec(`
				INSERT INTO internal_squad_members (internal_squad_uuid, user_id)
				SELECT ?, id FROM users
				ON CONFLICT (internal_squad_uuid, user_id) DO NOTHING
			`, uuidParam)
		} else if strings.HasSuffix(path, "/add-many-users") {
			var body struct {
				UserIDs []uint64 `json:"userIds"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			for _, uid := range body.UserIDs {
				h.db.Exec(`
					INSERT INTO internal_squad_members (internal_squad_uuid, user_id)
					VALUES (?, ?)
					ON CONFLICT (internal_squad_uuid, user_id) DO NOTHING
				`, uuidParam, uid)
			}
		} else if strings.HasSuffix(path, "/remove-users") {
			h.db.Where("internal_squad_uuid = ?", uuidParam).Delete(&database.InternalSquadMember{})
		} else if strings.HasSuffix(path, "/remove-many-users") {
			var body struct {
				UserIDs []uint64 `json:"userIds"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if len(body.UserIDs) > 0 {
				h.db.Where("internal_squad_uuid = ? AND user_id IN ?", uuidParam, body.UserIDs).Delete(&database.InternalSquadMember{})
			}
		}
	} else if strings.Contains(path, "/external-squads/") {
		if strings.HasSuffix(path, "/add-users") {
			h.db.Model(&database.User{}).Where("1 = 1").Update("external_squad_uuid", uuidParam)
		} else if strings.HasSuffix(path, "/remove-users") {
			h.db.Model(&database.User{}).Where("external_squad_uuid = ?", uuidParam).Update("external_squad_uuid", nil)
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"success": true,
		},
	})
}

func formatExternalSquad(db *gorm.DB, s *database.ExternalSquad) map[string]interface{} {
	tags := []string{}
	if s.Tags != "" {
		_ = json.Unmarshal([]byte(s.Tags), &tags)
	}

	var subSettings interface{}
	if s.SubscriptionSettings != "" && s.SubscriptionSettings != "null" {
		_ = json.Unmarshal([]byte(s.SubscriptionSettings), &subSettings)
	}

	var hostOverrides interface{}
	if s.HostOverrides != "" && s.HostOverrides != "null" {
		_ = json.Unmarshal([]byte(s.HostOverrides), &hostOverrides)
	}

	var respHeadersAdd interface{}
	if s.ResponseHeadersAdd != "" && s.ResponseHeadersAdd != "null" {
		_ = json.Unmarshal([]byte(s.ResponseHeadersAdd), &respHeadersAdd)
	}
	if respHeadersAdd == nil {
		respHeadersAdd = map[string]interface{}{}
	}

	respHeadersRemove := []string{}
	if s.ResponseHeadersRemove != "" {
		_ = json.Unmarshal([]byte(s.ResponseHeadersRemove), &respHeadersRemove)
	}

	var hwidSettings interface{}
	if s.HwidSettings != "" && s.HwidSettings != "null" {
		_ = json.Unmarshal([]byte(s.HwidSettings), &hwidSettings)
	}

	var customRemarks interface{}
	if s.CustomRemarks != "" && s.CustomRemarks != "null" {
		_ = json.Unmarshal([]byte(s.CustomRemarks), &customRemarks)
	}

	var membersCount int64
	db.Model(&database.User{}).Where("external_squad_uuid = ?", s.UUID).Count(&membersCount)

	var tmpls []database.ExternalSquadTemplate
	db.Where("external_squad_uuid = ?", s.UUID).Find(&tmpls)
	templatesRes := make([]map[string]interface{}, 0, len(tmpls))
	for _, t := range tmpls {
		templatesRes = append(templatesRes, map[string]interface{}{
			"templateUuid": t.TemplateUUID,
			"templateType": t.TemplateType,
		})
	}

	return map[string]interface{}{
		"uuid":         s.UUID,
		"viewPosition": s.ViewPosition,
		"name":         s.Name,
		"tags":         tags,
		"info": map[string]interface{}{
			"membersCount": membersCount,
		},
		"templates":             templatesRes,
		"subscriptionSettings":  subSettings,
		"hostOverrides":         hostOverrides,
		"responseHeadersAdd":    respHeadersAdd,
		"responseHeadersRemove": respHeadersRemove,
		"hwidSettings":          hwidSettings,
		"customRemarks":         customRemarks,
		"subpageConfigUuid":     s.SubpageConfigUUID,
		"createdAt":             s.CreatedAt.UTC().Format(time.RFC3339),
		"updatedAt":             s.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func (h *Handler) GetExternalSquads(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var squads []database.ExternalSquad
	h.db.Order("view_position asc, created_at asc").Find(&squads)

	res := make([]map[string]interface{}, 0, len(squads))
	for _, s := range squads {
		res = append(res, formatExternalSquad(h.db, &s))
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"total":          len(res),
			"externalSquads": res,
		},
	})
}

func (h *Handler) GetExternalSquad(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	uuidParam := chi.URLParam(r, "uuid")

	var s database.ExternalSquad
	if err := h.db.Where("uuid = ?", uuidParam).First(&s).Error; err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Squad not found"})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": formatExternalSquad(h.db, &s),
	})
}

func (h *Handler) CreateExternalSquad(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		Name      string `json:"name"`
		Templates []struct {
			TemplateUUID string `json:"templateUuid"`
			TemplateType string `json:"templateType"`
		} `json:"templates"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Invalid body"})
		return
	}

	var count int64
	h.db.Model(&database.ExternalSquad{}).Count(&count)

	newUUID := uuid.New().String()
	now := time.Now().UTC()

	s := database.ExternalSquad{
		UUID:                  newUUID,
		ViewPosition:          int(count) + 1,
		Name:                  strings.TrimSpace(body.Name),
		Tags:                  "[]",
		SubscriptionSettings:  "{}",
		HostOverrides:         "{}",
		ResponseHeadersAdd:    "{}",
		ResponseHeadersRemove: "[]",
		CreatedAt:             now,
		UpdatedAt:             now,
	}

	if err := h.db.Create(&s).Error; err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Failed to create squad"})
		return
	}

	for _, tmpl := range body.Templates {
		h.db.Create(&database.ExternalSquadTemplate{
			ExternalSquadUUID: newUUID,
			TemplateType:      tmpl.TemplateType,
			TemplateUUID:      tmpl.TemplateUUID,
		})
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": formatExternalSquad(h.db, &s),
	})
}

func (h *Handler) UpdateExternalSquad(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		UUID      string  `json:"uuid"`
		Name      *string `json:"name"`
		Templates *[]struct {
			TemplateUUID string `json:"templateUuid"`
			TemplateType string `json:"templateType"`
		} `json:"templates"`
		SubscriptionSettings  interface{} `json:"subscriptionSettings"`
		HostOverrides         interface{} `json:"hostOverrides"`
		ResponseHeadersAdd    interface{} `json:"responseHeadersAdd"`
		ResponseHeadersRemove []string    `json:"responseHeadersRemove"`
		HwidSettings          interface{} `json:"hwidSettings"`
		CustomRemarks         interface{} `json:"customRemarks"`
		SubpageConfigUUID     *string     `json:"subpageConfigUuid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Invalid body"})
		return
	}

	var s database.ExternalSquad
	if err := h.db.Where("uuid = ?", body.UUID).First(&s).Error; err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Squad not found"})
		return
	}

	if body.Name != nil {
		s.Name = strings.TrimSpace(*body.Name)
	}
	if body.Templates != nil {
		h.db.Where("external_squad_uuid = ?", s.UUID).Delete(&database.ExternalSquadTemplate{})
		for _, tmpl := range *body.Templates {
			h.db.Create(&database.ExternalSquadTemplate{
				ExternalSquadUUID: s.UUID,
				TemplateType:      tmpl.TemplateType,
				TemplateUUID:      tmpl.TemplateUUID,
			})
		}
	}
	if body.SubscriptionSettings != nil {
		b, _ := json.Marshal(body.SubscriptionSettings)
		s.SubscriptionSettings = string(b)
	}
	if body.HostOverrides != nil {
		b, _ := json.Marshal(body.HostOverrides)
		s.HostOverrides = string(b)
	}
	if body.ResponseHeadersAdd != nil {
		b, _ := json.Marshal(body.ResponseHeadersAdd)
		s.ResponseHeadersAdd = string(b)
	}
	if body.ResponseHeadersRemove != nil {
		b, _ := json.Marshal(body.ResponseHeadersRemove)
		s.ResponseHeadersRemove = string(b)
	}
	if body.HwidSettings != nil {
		b, _ := json.Marshal(body.HwidSettings)
		s.HwidSettings = string(b)
	}
	if body.CustomRemarks != nil {
		b, _ := json.Marshal(body.CustomRemarks)
		s.CustomRemarks = string(b)
	}
	if body.SubpageConfigUUID != nil {
		s.SubpageConfigUUID = body.SubpageConfigUUID
	}
	s.UpdatedAt = time.Now().UTC()

	if err := h.db.Save(&s).Error; err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Failed to update squad"})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": formatExternalSquad(h.db, &s),
	})
}

func (h *Handler) DeleteExternalSquad(w http.ResponseWriter, r *http.Request) {
	uuidParam := chi.URLParam(r, "uuid")
	h.db.Where("external_squad_uuid = ?", uuidParam).Delete(&database.ExternalSquadTemplate{})
	h.db.Model(&database.User{}).Where("external_squad_uuid = ?", uuidParam).Update("external_squad_uuid", nil)
	h.db.Where("uuid = ?", uuidParam).Delete(&database.ExternalSquad{})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ReorderExternalSquads(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		Items []struct {
			UUID         string `json:"uuid"`
			ViewPosition int    `json:"viewPosition"`
		} `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Invalid body"})
		return
	}

	for _, item := range body.Items {
		h.db.Model(&database.ExternalSquad{}).Where("uuid = ?", item.UUID).Update("view_position", item.ViewPosition)
	}

	var squads []database.ExternalSquad
	h.db.Order("view_position asc, created_at asc").Find(&squads)
	res := make([]map[string]interface{}, 0, len(squads))
	for _, s := range squads {
		res = append(res, formatExternalSquad(h.db, &s))
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"total":          len(res),
			"externalSquads": res,
		},
	})
}

func (h *Handler) GetExternalSquadsTags(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var squads []database.ExternalSquad
	h.db.Find(&squads)

	tagMap := make(map[string]bool)
	for _, s := range squads {
		if s.Tags != "" {
			var tags []string
			if err := json.Unmarshal([]byte(s.Tags), &tags); err == nil {
				for _, tag := range tags {
					tagMap[tag] = true
				}
			}
		}
	}

	tagList := make([]string, 0, len(tagMap))
	for tag := range tagMap {
		tagList = append(tagList, tag)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"tags": tagList,
		},
	})
}

func (h *Handler) SetExternalSquadsTags(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		UUID string   `json:"uuid"`
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Invalid body"})
		return
	}

	tagsBytes, _ := json.Marshal(body.Tags)
	h.db.Model(&database.ExternalSquad{}).Where("uuid = ?", body.UUID).Update("tags", string(tagsBytes))

	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"uuid": body.UUID,
			"tags": body.Tags,
		},
	})
}

func (h *Handler) GetInternalSquadUsage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": []interface{}{},
	})
}
