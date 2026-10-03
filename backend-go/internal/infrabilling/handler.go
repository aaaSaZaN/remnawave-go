package infrabilling

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

func formatProvider(p *database.InfraProvider) map[string]interface{} {
	return map[string]interface{}{
		"uuid":        p.UUID,
		"name":        p.Name,
		"faviconLink": p.FaviconLink,
		"loginUrl":    p.LoginURL,
		"createdAt":   p.CreatedAt.UTC().Format(time.RFC3339),
		"updatedAt":   p.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func (h *Handler) GetProviders(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var providers []database.InfraProvider
	h.db.Order("name asc").Find(&providers)

	res := make([]map[string]interface{}, 0, len(providers))
	for _, p := range providers {
		res = append(res, formatProvider(&p))
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"total":     len(res),
			"providers": res,
		},
	})
}

func (h *Handler) GetProvider(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	uuidParam := chi.URLParam(r, "uuid")

	var p database.InfraProvider
	if err := h.db.Where("uuid = ?", uuidParam).First(&p).Error; err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Provider not found"})
		return
	}

	resp := formatProvider(&p)
	resp["billingHistory"] = []interface{}{}
	resp["billingNodes"] = []interface{}{}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": resp,
	})
}

func (h *Handler) CreateProvider(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		Name        string  `json:"name"`
		FaviconLink *string `json:"faviconLink"`
		LoginURL    *string `json:"loginUrl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Invalid body"})
		return
	}

	now := time.Now().UTC()
	p := database.InfraProvider{
		UUID:        uuid.New().String(),
		Name:        strings.TrimSpace(body.Name),
		FaviconLink: body.FaviconLink,
		LoginURL:    body.LoginURL,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := h.db.Create(&p).Error; err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Failed to create provider"})
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": formatProvider(&p),
	})
}

func (h *Handler) UpdateProvider(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		UUID        string  `json:"uuid"`
		Name        *string `json:"name"`
		FaviconLink *string `json:"faviconLink"`
		LoginURL    *string `json:"loginUrl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Invalid body"})
		return
	}

	var p database.InfraProvider
	if err := h.db.Where("uuid = ?", body.UUID).First(&p).Error; err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Provider not found"})
		return
	}

	if body.Name != nil {
		p.Name = strings.TrimSpace(*body.Name)
	}
	if body.FaviconLink != nil {
		p.FaviconLink = body.FaviconLink
	}
	if body.LoginURL != nil {
		p.LoginURL = body.LoginURL
	}
	p.UpdatedAt = time.Now().UTC()

	if err := h.db.Save(&p).Error; err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Failed to update provider"})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": formatProvider(&p),
	})
}

func (h *Handler) DeleteProvider(w http.ResponseWriter, r *http.Request) {
	uuidParam := chi.URLParam(r, "uuid")
	h.db.Where("uuid = ?", uuidParam).Delete(&database.InfraProvider{})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) GetNodes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var billingNodes []database.InfraBillingNode
	h.db.Find(&billingNodes)

	res := make([]map[string]interface{}, 0, len(billingNodes))
	for _, bn := range billingNodes {
		var prov database.InfraProvider
		h.db.Where("uuid = ?", bn.ProviderUUID).First(&prov)

		res = append(res, map[string]interface{}{
			"uuid":         bn.UUID,
			"nodeUuid":     bn.NodeUUID,
			"name":         bn.Name,
			"providerUuid": bn.ProviderUUID,
			"provider": map[string]interface{}{
				"uuid":        prov.UUID,
				"name":        prov.Name,
				"loginUrl":    prov.LoginURL,
				"faviconLink": prov.FaviconLink,
			},
			"node":          nil,
			"nextBillingAt": bn.NextBillingAt.UTC().Format(time.RFC3339),
			"createdAt":     bn.CreatedAt.UTC().Format(time.RFC3339),
			"updatedAt":     bn.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"totalBillingNodes":          len(res),
			"billingNodes":               res,
			"availableBillingNodes":      []interface{}{},
			"totalAvailableBillingNodes": 0,
			"stats": map[string]interface{}{
				"upcomingNodesCount":   0,
				"currentMonthPayments": 0,
				"totalSpent":           0,
			},
		},
	})
}

func (h *Handler) CreateNode(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		Name          string    `json:"name"`
		NodeUUID      *string   `json:"nodeUuid"`
		ProviderUUID  string    `json:"providerUuid"`
		NextBillingAt time.Time `json:"nextBillingAt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Invalid body"})
		return
	}

	now := time.Now().UTC()
	bn := database.InfraBillingNode{
		UUID:          uuid.New().String(),
		Name:          strings.TrimSpace(body.Name),
		NodeUUID:      body.NodeUUID,
		ProviderUUID:  body.ProviderUUID,
		NextBillingAt: body.NextBillingAt,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := h.db.Create(&bn).Error; err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Failed to create billing node"})
		return
	}

	w.WriteHeader(http.StatusCreated)
	h.GetNodes(w, r)
}

func (h *Handler) UpdateNode(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		UUID          string     `json:"uuid"`
		Name          *string    `json:"name"`
		NodeUUID      *string    `json:"nodeUuid"`
		ProviderUUID  *string    `json:"providerUuid"`
		NextBillingAt *time.Time `json:"nextBillingAt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Invalid body"})
		return
	}

	var bn database.InfraBillingNode
	if err := h.db.Where("uuid = ?", body.UUID).First(&bn).Error; err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Billing node not found"})
		return
	}

	if body.Name != nil {
		bn.Name = strings.TrimSpace(*body.Name)
	}
	if body.NodeUUID != nil {
		bn.NodeUUID = body.NodeUUID
	}
	if body.ProviderUUID != nil {
		bn.ProviderUUID = *body.ProviderUUID
	}
	if body.NextBillingAt != nil {
		bn.NextBillingAt = *body.NextBillingAt
	}
	bn.UpdatedAt = time.Now().UTC()

	h.db.Save(&bn)
	h.GetNodes(w, r)
}

func (h *Handler) DeleteNode(w http.ResponseWriter, r *http.Request) {
	uuidParam := chi.URLParam(r, "uuid")
	h.db.Where("uuid = ?", uuidParam).Delete(&database.InfraBillingNode{})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) GetHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var records []database.InfraBillingHistory
	h.db.Order("billed_at desc").Find(&records)

	res := make([]map[string]interface{}, 0, len(records))
	for _, rec := range records {
		var prov database.InfraProvider
		h.db.Where("uuid = ?", rec.ProviderUUID).First(&prov)

		res = append(res, map[string]interface{}{
			"uuid":         rec.UUID,
			"providerUuid": rec.ProviderUUID,
			"amount":       rec.Amount,
			"billedAt":     rec.BilledAt.UTC().Format(time.RFC3339),
			"provider": map[string]interface{}{
				"uuid":        prov.UUID,
				"name":        prov.Name,
				"faviconLink": prov.FaviconLink,
			},
		})
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"total":   len(res),
			"records": res,
		},
	})
}

func (h *Handler) CreateHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		ProviderUUID string    `json:"providerUuid"`
		Amount       float64   `json:"amount"`
		BilledAt     time.Time `json:"billedAt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Invalid body"})
		return
	}

	rec := database.InfraBillingHistory{
		UUID:         uuid.New().String(),
		ProviderUUID: body.ProviderUUID,
		Amount:       body.Amount,
		BilledAt:     body.BilledAt,
	}

	h.db.Create(&rec)
	w.WriteHeader(http.StatusCreated)
	h.GetHistory(w, r)
}

func (h *Handler) DeleteHistory(w http.ResponseWriter, r *http.Request) {
	uuidParam := chi.URLParam(r, "uuid")
	h.db.Where("uuid = ?", uuidParam).Delete(&database.InfraBillingHistory{})
	w.WriteHeader(http.StatusNoContent)
}
