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

func formatInternalSquad(s *database.InternalSquad) map[string]interface{} {
	var tags []string
	if s.Tags != "" {
		_ = json.Unmarshal([]byte(s.Tags), &tags)
	}
	if tags == nil {
		tags = []string{}
	}

	return map[string]interface{}{
		"uuid":         s.UUID,
		"viewPosition": s.ViewPosition,
		"name":         s.Name,
		"tags":         tags,
		"info": map[string]interface{}{
			"membersCount":  0,
			"inboundsCount": 0,
		},
		"inbounds":  []interface{}{},
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
		res = append(res, formatInternalSquad(&s))
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
		"response": formatInternalSquad(&s),
	})
}

func (h *Handler) CreateInternalSquad(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		Name         string   `json:"name"`
		InboundUUIDs []string `json:"inboundUuids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Invalid body"})
		return
	}

	var count int64
	h.db.Model(&database.InternalSquad{}).Count(&count)

	newUUID := uuid.New().String()
	now := time.Now().UTC()
	inbBytes, _ := json.Marshal(body.InboundUUIDs)

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

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": formatInternalSquad(&s),
	})
}

func (h *Handler) UpdateInternalSquad(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		UUID         string   `json:"uuid"`
		Name         *string  `json:"name"`
		InboundUUIDs []string `json:"inboundUuids"`
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
	if body.InboundUUIDs != nil {
		inbBytes, _ := json.Marshal(body.InboundUUIDs)
		s.InboundUUIDs = string(inbBytes)
	}
	s.UpdatedAt = time.Now().UTC()

	if err := h.db.Save(&s).Error; err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Failed to update squad"})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": formatInternalSquad(&s),
	})
}

func (h *Handler) DeleteInternalSquad(w http.ResponseWriter, r *http.Request) {
	uuidParam := chi.URLParam(r, "uuid")
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
		res = append(res, formatInternalSquad(&s))
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

func (h *Handler) GetInternalSquadAccessibleNodes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	uuidParam := chi.URLParam(r, "uuid")

	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"squadUuid":       uuidParam,
			"accessibleNodes": []interface{}{},
		},
	})
}

func (h *Handler) SquadBulkAction(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"success": true,
		},
	})
}

func formatExternalSquad(s *database.ExternalSquad) map[string]interface{} {
	var tags []string
	if s.Tags != "" {
		_ = json.Unmarshal([]byte(s.Tags), &tags)
	}
	if tags == nil {
		tags = []string{}
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

	var respHeadersRemove []string
	if s.ResponseHeadersRemove != "" {
		_ = json.Unmarshal([]byte(s.ResponseHeadersRemove), &respHeadersRemove)
	}
	if respHeadersRemove == nil {
		respHeadersRemove = []string{}
	}

	return map[string]interface{}{
		"uuid":         s.UUID,
		"viewPosition": s.ViewPosition,
		"name":         s.Name,
		"tags":         tags,
		"info": map[string]interface{}{
			"membersCount": 0,
		},
		"templates":             []interface{}{},
		"subscriptionSettings":  subSettings,
		"hostOverrides":         hostOverrides,
		"responseHeadersAdd":    respHeadersAdd,
		"responseHeadersRemove": respHeadersRemove,
		"hwidSettings":          nil,
		"customRemarks":         nil,
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
		res = append(res, formatExternalSquad(&s))
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
		"response": formatExternalSquad(&s),
	})
}

func (h *Handler) CreateExternalSquad(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		Name string `json:"name"`
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

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": formatExternalSquad(&s),
	})
}

func (h *Handler) UpdateExternalSquad(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		UUID                  string      `json:"uuid"`
		Name                  *string     `json:"name"`
		SubscriptionSettings  interface{} `json:"subscriptionSettings"`
		HostOverrides         interface{} `json:"hostOverrides"`
		ResponseHeadersAdd    interface{} `json:"responseHeadersAdd"`
		ResponseHeadersRemove []string    `json:"responseHeadersRemove"`
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
		"response": formatExternalSquad(&s),
	})
}

func (h *Handler) DeleteExternalSquad(w http.ResponseWriter, r *http.Request) {
	uuidParam := chi.URLParam(r, "uuid")
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
		res = append(res, formatExternalSquad(&s))
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
