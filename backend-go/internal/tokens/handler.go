package tokens

import (
	"encoding/json"
	"net/http"
	"time"

	"remnawave-go/internal/database"
	"remnawave-go/internal/middleware"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Handler struct {
	db        *gorm.DB
	appSecret string
}

func NewHandler(db *gorm.DB, appSecret string) *Handler {
	return &Handler{
		db:        db,
		appSecret: appSecret,
	}
}

type ApiTokenItem struct {
	UUID      string   `json:"uuid"`
	Name      string   `json:"name"`
	ExpireAt  string   `json:"expireAt"`
	Scopes    []string `json:"scopes"`
	CreatedAt string   `json:"createdAt"`
	UpdatedAt string   `json:"updatedAt"`
}

type CreateApiTokenResponse struct {
	UUID      string   `json:"uuid"`
	Name      string   `json:"name"`
	ExpireAt  string   `json:"expireAt"`
	Scopes    []string `json:"scopes"`
	CreatedAt string   `json:"createdAt"`
	UpdatedAt string   `json:"updatedAt"`
	Token     string   `json:"token"`
}

func (h *Handler) GetScopes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"response":` + string(ScopesData) + `}`))
}

func (h *Handler) GetApiTokens(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var tokens []database.ApiToken
	if err := h.db.Order("created_at desc").Find(&tokens).Error; err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": err.Error()})
		return
	}

	result := make([]ApiTokenItem, 0, len(tokens))
	for _, t := range tokens {
		var scopes []string
		if t.Scopes != "" {
			_ = json.Unmarshal([]byte(t.Scopes), &scopes)
		}
		if len(scopes) == 0 {
			scopes = []string{"*"}
		}
		result = append(result, ApiTokenItem{
			UUID:      t.UUID,
			Name:      t.Name,
			ExpireAt:  t.ExpireAt.UTC().Format("2006-01-02T15:04:05.000Z"),
			Scopes:    scopes,
			CreatedAt: t.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
			UpdatedAt: t.UpdatedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
		})
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"tokens": result,
		},
	})
}

func (h *Handler) CreateApiToken(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		Name          string   `json:"name"`
		ExpiresInDays int      `json:"expiresInDays"`
		Scopes        []string `json:"scopes"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Invalid request body"})
		return
	}

	if body.Name == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Token name is required"})
		return
	}

	if body.ExpiresInDays <= 0 {
		body.ExpiresInDays = 30
	}

	if len(body.Scopes) == 0 {
		body.Scopes = []string{"*"}
	}

	tokenUUID := uuid.New().String()
	now := time.Now()
	expireAt := now.AddDate(0, 0, body.ExpiresInDays)

	claims := &middleware.TokenClaims{
		UUID:     tokenUUID,
		Username: "",
		Role:     "API",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expireAt),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(h.appSecret))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Failed to sign token"})
		return
	}

	scopesBytes, _ := json.Marshal(body.Scopes)

	dbToken := database.ApiToken{
		UUID:      tokenUUID,
		Name:      body.Name,
		ExpireAt:  expireAt,
		Token:     tokenString,
		Scopes:    string(scopesBytes),
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := h.db.Create(&dbToken).Error; err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": err.Error()})
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": CreateApiTokenResponse{
			UUID:      tokenUUID,
			Name:      body.Name,
			ExpireAt:  expireAt.UTC().Format("2006-01-02T15:04:05.000Z"),
			Scopes:    body.Scopes,
			CreatedAt: now.UTC().Format("2006-01-02T15:04:05.000Z"),
			UpdatedAt: now.UTC().Format("2006-01-02T15:04:05.000Z"),
			Token:     tokenString,
		},
	})
}

func (h *Handler) DeleteApiToken(w http.ResponseWriter, r *http.Request) {
	tokenUUID := chi.URLParam(r, "uuid")
	if tokenUUID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := h.db.Where("uuid = ?", tokenUUID).Delete(&database.ApiToken{}).Error; err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) GetOtt(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	now := time.Now()
	claims := &middleware.TokenClaims{
		UUID:     uuid.New().String(),
		Username: "admin",
		Role:     "ADMIN",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(h.appSecret))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Failed to create OTT"})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"ott": tokenString,
		},
	})
}
