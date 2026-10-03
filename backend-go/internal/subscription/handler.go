package subscription

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"remnawave-go/internal/database"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

type Handler struct {
	db        *gorm.DB
	generator *Generator
	subDomain string
}

func NewHandler(db *gorm.DB, subDomain string) *Handler {
	return &Handler{
		db:        db,
		generator: NewGenerator(),
		subDomain: subDomain,
	}
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	units := []string{"KB", "MB", "GB", "TB", "PB"}
	val := float64(b) / float64(div)
	return fmt.Sprintf("%.2f %s", val, units[exp])
}

func (h *Handler) GetSubscription(w http.ResponseWriter, r *http.Request) {
	shortUUID := chi.URLParam(r, "shortUuid")

	var user database.User
	if err := h.db.Preload("Traffic").Where("short_uuid = ?", shortUUID).First(&user).Error; err != nil {
		http.Error(w, `{"message":"Subscription not found"}`, http.StatusNotFound)
		return
	}

	var hosts []database.Host
	h.db.Where("is_disabled = ?", false).Find(&hosts)

	ua := r.Header.Get("User-Agent")
	contentType, body := h.generator.Generate(&user, hosts, ua)

	var usedBytes uint64
	if user.Traffic != nil {
		usedBytes = user.Traffic.UsedTrafficBytes
	}

	userInfoHeader := fmt.Sprintf("upload=0; download=%d; total=%d; expire=%d",
		usedBytes, user.TrafficLimitBytes, user.ExpireAt.Unix())

	subDomain := h.subDomain
	if subDomain == "" {
		subDomain = r.Host
	}
	cleanDomain := strings.TrimPrefix(subDomain, "http://")
	cleanDomain = strings.TrimPrefix(cleanDomain, "https://")
	cleanDomain = strings.TrimSuffix(cleanDomain, "/api/sub")
	cleanDomain = strings.TrimSuffix(cleanDomain, "/")
	scheme := "http"
	if strings.HasPrefix(h.subDomain, "https://") {
		scheme = "https"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("subscription-userinfo", userInfoHeader)
	w.Header().Set("profile-web-page-url", fmt.Sprintf("%s://%s/%s", scheme, cleanDomain, shortUUID))

	w.Write([]byte(body))
}

func (h *Handler) GetSubscriptionInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	shortUUID := chi.URLParam(r, "shortUuid")

	var user database.User
	if err := h.db.Preload("Traffic").Where("short_uuid = ?", shortUUID).First(&user).Error; err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Subscription not found"})
		return
	}

	var usedBytes uint64
	var lifetimeBytes uint64
	if user.Traffic != nil {
		usedBytes = user.Traffic.UsedTrafficBytes
		lifetimeBytes = user.Traffic.LifetimeUsedTrafficBytes
	}

	daysLeft := int(time.Until(user.ExpireAt).Hours() / 24)
	if daysLeft < 0 {
		daysLeft = 0
	}

	strategy := user.TrafficLimitStrategy
	if strategy == "" {
		strategy = "NO_RESET"
	}

	var hosts []database.Host
	h.db.Where("is_disabled = ?", false).Find(&hosts)

	var links []string
	for _, host := range hosts {
		if host.IsDisabled {
			continue
		}
		security := "tls"
		if host.SecurityLayer != "" && host.SecurityLayer != "DEFAULT" {
			security = strings.ToLower(host.SecurityLayer)
		}
		link := fmt.Sprintf("vless://%s@%s:%d?security=%s&sni=%s&fp=%s#%s",
			user.VlessUUID, host.Address, host.Port, security, host.Sni, host.Fingerprint, host.Remark)
		links = append(links, link)
	}

	subDomain := h.subDomain
	if subDomain == "" {
		subDomain = r.Host
	}
	cleanDomain := strings.TrimPrefix(subDomain, "http://")
	cleanDomain = strings.TrimPrefix(cleanDomain, "https://")
	cleanDomain = strings.TrimSuffix(cleanDomain, "/api/sub")
	cleanDomain = strings.TrimSuffix(cleanDomain, "/")
	scheme := "http"
	if strings.HasPrefix(h.subDomain, "https://") {
		scheme = "https"
	}
	subURL := fmt.Sprintf("%s://%s/api/sub/%s", scheme, cleanDomain, user.ShortUUID)

	response := map[string]interface{}{
		"response": map[string]interface{}{
			"isFound": true,
			"user": map[string]interface{}{
				"shortUuid":                user.ShortUUID,
				"daysLeft":                 daysLeft,
				"trafficUsed":              formatBytes(usedBytes),
				"trafficLimit":             formatBytes(user.TrafficLimitBytes),
				"lifetimeTrafficUsed":      formatBytes(lifetimeBytes),
				"trafficUsedBytes":         fmt.Sprintf("%d", usedBytes),
				"trafficLimitBytes":        fmt.Sprintf("%d", user.TrafficLimitBytes),
				"lifetimeTrafficUsedBytes": fmt.Sprintf("%d", lifetimeBytes),
				"username":                 user.Username,
				"expiresAt":                user.ExpireAt.UTC().Format("2006-01-02T15:04:05.000Z"),
				"isActive":                 user.Status == "ACTIVE",
				"userStatus":               user.Status,
				"trafficLimitStrategy":     strategy,
			},
			"links":           links,
			"ssConfLinks":     map[string]string{},
			"subscriptionUrl": subURL,
		},
	}

	json.NewEncoder(w).Encode(response)
}

func (h *Handler) GetSubpageConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	subpageUUID := "00000000-0000-0000-0000-000000000000"
	var cfg database.SubscriptionPageConfig
	if err := h.db.First(&cfg).Error; err == nil && cfg.UUID != "" {
		subpageUUID = cfg.UUID
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"subpageConfigUuid": subpageUUID,
			"webpageAllowed":    true,
		},
	})
}
