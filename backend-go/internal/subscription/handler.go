package subscription

import (
	"encoding/base64"
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

func (h *Handler) getHostsForUser(userID uint64) []database.Host {
	var hosts []database.Host

	var squadCount int64
	h.db.Table("internal_squad_members").Where("user_id = ?", userID).Count(&squadCount)

	if squadCount > 0 {
		query := `
SELECT hosts.*
FROM hosts
WHERE EXISTS (
    SELECT 1
    FROM internal_squad_inbounds
    INNER JOIN internal_squad_members ON internal_squad_members.internal_squad_uuid = internal_squad_inbounds.internal_squad_uuid
    WHERE internal_squad_inbounds.inbound_uuid = hosts.config_profile_inbound_uuid
      AND internal_squad_members.user_id = ?
      AND (
          EXISTS (
              SELECT 1 FROM internal_squad_host_links
              WHERE internal_squad_host_links.host_uuid = hosts.uuid
                AND internal_squad_host_links.squad_uuid = internal_squad_inbounds.internal_squad_uuid
          ) = (hosts.internal_squads_mode = 'ALLOW_ONLY')
      )
)
AND hosts.is_disabled = false
AND hosts.is_hidden = false
ORDER BY hosts.view_position ASC;
`
		if err := h.db.Raw(query, userID).Scan(&hosts).Error; err == nil && len(hosts) > 0 {
			return hosts
		}
	}

	h.db.Where("is_disabled = ? AND is_hidden = ?", false, false).
		Order("view_position ASC").
		Find(&hosts)
	return hosts
}

func (h *Handler) getHostsWithInboundsForUser(userID uint64) []XrayHostMeta {
	hosts := h.getHostsForUser(userID)
	if len(hosts) == 0 {
		return nil
	}

	inboundUUIDs := make([]string, 0, len(hosts))
	for _, host := range hosts {
		if host.ConfigProfileInboundUUID != "" {
			inboundUUIDs = append(inboundUUIDs, host.ConfigProfileInboundUUID)
		}
	}

	inboundMap := make(map[string]database.ConfigProfileInbound)
	if len(inboundUUIDs) > 0 {
		var inbounds []database.ConfigProfileInbound
		h.db.Where("uuid IN ?", inboundUUIDs).Find(&inbounds)
		for _, inb := range inbounds {
			inboundMap[inb.UUID] = inb
		}
	}

	result := make([]XrayHostMeta, 0, len(hosts))
	for i := range hosts {
		var inb *database.ConfigProfileInbound
		if val, ok := inboundMap[hosts[i].ConfigProfileInboundUUID]; ok {
			inb = &val
		}
		result = append(result, XrayHostMeta{
			Host:    &hosts[i],
			Inbound: inb,
		})
	}
	return result
}

func (h *Handler) getTemplateForUser(user *database.User, templateType string, overrideTemplateName string) string {
	extractTmplContent := func(tmpl *database.SubscriptionTemplate) string {
		if templateType == "XRAY_JSON" || templateType == "SINGBOX" {
			return tmpl.TemplateJson
		}
		return tmpl.TemplateYaml
	}

	if overrideTemplateName != "" {
		var tmpl database.SubscriptionTemplate
		if err := h.db.Where("name = ? AND template_type = ?", overrideTemplateName, templateType).First(&tmpl).Error; err == nil {
			return extractTmplContent(&tmpl)
		}
	}

	if user.ExternalSquadUUID != nil && *user.ExternalSquadUUID != "" {
		var squadTmpl database.ExternalSquadTemplate
		if err := h.db.Where("external_squad_uuid = ? AND template_type = ?", *user.ExternalSquadUUID, templateType).First(&squadTmpl).Error; err == nil {
			var tmpl database.SubscriptionTemplate
			if err := h.db.Where("uuid = ?", squadTmpl.TemplateUUID).First(&tmpl).Error; err == nil {
				return extractTmplContent(&tmpl)
			}
		}
	}

	var tmpl database.SubscriptionTemplate
	if err := h.db.Where("template_type = ?", templateType).Order("view_position asc, created_at asc").First(&tmpl).Error; err == nil {
		return extractTmplContent(&tmpl)
	}

	return ""
}

func formatHeaderValue(val string, user *database.User) string {
	val = strings.ReplaceAll(val, "{{user.username}}", user.Username)
	val = strings.ReplaceAll(val, "{{user.shortUuid}}", user.ShortUUID)
	val = strings.ReplaceAll(val, "{{user.trafficLimit}}", fmt.Sprintf("%d", user.TrafficLimitBytes))

	if strings.HasPrefix(val, "rwEncodeBase64:") {
		raw := strings.TrimPrefix(val, "rwEncodeBase64:")
		return "base64:" + base64.StdEncoding.EncodeToString([]byte(raw))
	}
	return val
}

func (h *Handler) GetSubscription(w http.ResponseWriter, r *http.Request) {
	shortUUID := chi.URLParam(r, "shortUuid")

	var user database.User
	if err := h.db.Preload("Traffic").Where("short_uuid = ?", shortUUID).First(&user).Error; err != nil {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Subscription not found"}`))
		return
	}

	if user.Status != "ACTIVE" {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message":"Account is disabled"}`))
		return
	}

	// Prepare request headers for SRR
	reqHeaders := make(map[string]string)
	for k, v := range r.Header {
		if len(v) > 0 {
			reqHeaders[strings.ToLower(k)] = v[0]
		}
	}

	clientType := chi.URLParam(r, "clientType")
	reqHeaders["x-remnawave-injected-client-type"] = clientType
	reqHeaders["x-remnawave-injected-user-username"] = user.Username
	reqHeaders["x-remnawave-injected-short-uuid"] = user.ShortUUID

	// Load SubscriptionSetting
	var settings database.SubscriptionSetting
	h.db.First(&settings)

	// Match SRR
	srrRes := MatchSRRRules(settings.ResponseRules, reqHeaders, clientType)
	var matchedRuleName *string
	matchedResponseType := "UNKNOWN"

	if srrRes.Matched {
		matchedResponseType = srrRes.ResponseType
		if srrRes.MatchedRule != nil {
			matchedRuleName = &srrRes.MatchedRule.Name
		}
	}

	// Record request in history
	clientIP := r.RemoteAddr
	if colon := strings.LastIndex(clientIP, ":"); colon != -1 {
		clientIP = clientIP[:colon]
	}
	ua := r.Header.Get("User-Agent")
	go func(uID uint64, ip, userAgent string, ruleName *string, respType string) {
		h.db.Create(&database.UserSubscriptionRequestHistory{
			UserID:          uID,
			RequestIP:       &ip,
			UserAgent:       &userAgent,
			SrrRuleName:     ruleName,
			SrrResponseType: respType,
			RequestAt:       time.Now().UTC(),
		})
	}(user.ID, clientIP, ua, matchedRuleName, matchedResponseType)

	// Handle response types
	if srrRes.Matched {
		switch srrRes.ResponseType {
		case "BLOCK":
			if srrRes.MatchedRule != nil && srrRes.MatchedRule.ResponseModifications != nil {
				for _, hm := range srrRes.MatchedRule.ResponseModifications.Headers {
					w.Header().Set(hm.Key, hm.Value)
				}
			}
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"message":"Forbidden"}`))
			return

		case "STATUS_CODE_404":
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"message":"Not found"}`))
			return

		case "STATUS_CODE_451":
			w.WriteHeader(http.StatusUnavailableForLegalReasons)
			w.Write([]byte(`{"message":"Unavailable For Legal Reasons"}`))
			return

		case "BROWSER":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			h.GetSubscriptionInfo(w, r)
			return
		}
	}

	// Prepare standard subscription headers
	var usedBytes uint64
	if user.Traffic != nil {
		usedBytes = user.Traffic.UsedTrafficBytes
	}

	expireUnix := user.ExpireAt.Unix()
	if user.ExpireAt.Year() >= 2099 || user.ExpireAt.IsZero() {
		expireUnix = 0
	}
	userInfoHeader := fmt.Sprintf("upload=0; download=%d; total=%d; expire=%d",
		usedBytes, user.TrafficLimitBytes, expireUnix)

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

	respHeaders := map[string]string{
		"subscription-userinfo":   userInfoHeader,
		"profile-update-interval": "12",
		"profile-title":           user.Username,
		"content-disposition":     fmt.Sprintf("attachment; filename=%s", user.Username),
		"profile-web-page-url":    fmt.Sprintf("%s://%s/%s", scheme, cleanDomain, shortUUID),
	}

	// Apply customResponseHeaders from settings
	if settings.CustomResponseHeaders != "" && settings.CustomResponseHeaders != "null" {
		var crh map[string]string
		if err := json.Unmarshal([]byte(settings.CustomResponseHeaders), &crh); err == nil {
			for k, v := range crh {
				respHeaders[strings.ToLower(k)] = formatHeaderValue(v, &user)
			}
		}
	}

	// Apply external squad headers
	if user.ExternalSquadUUID != nil && *user.ExternalSquadUUID != "" {
		var extSquad database.ExternalSquad
		if err := h.db.Where("uuid = ?", *user.ExternalSquadUUID).First(&extSquad).Error; err == nil {
			if extSquad.ResponseHeadersAdd != "" && extSquad.ResponseHeadersAdd != "null" {
				var headersAdd map[string]string
				if err := json.Unmarshal([]byte(extSquad.ResponseHeadersAdd), &headersAdd); err == nil {
					for k, v := range headersAdd {
						respHeaders[strings.ToLower(k)] = formatHeaderValue(v, &user)
					}
				}
			}
			if extSquad.ResponseHeadersRemove != "" && extSquad.ResponseHeadersRemove != "null" {
				var headersRemove []string
				if err := json.Unmarshal([]byte(extSquad.ResponseHeadersRemove), &headersRemove); err == nil {
					for _, k := range headersRemove {
						delete(respHeaders, strings.ToLower(k))
					}
				}
			}
		}
	}

	// Apply SRR rule headers
	if srrRes.Matched && srrRes.MatchedRule != nil && srrRes.MatchedRule.ResponseModifications != nil {
		for _, hm := range srrRes.MatchedRule.ResponseModifications.Headers {
			respHeaders[strings.ToLower(hm.Key)] = hm.Value
		}
	}

	for k, v := range respHeaders {
		w.Header().Set(k, v)
	}

	// Determine output content
	var contentType string
	var body string

	targetType := matchedResponseType
	if targetType == "UNKNOWN" {
		// Fallback detection by user-agent if SRR didn't match
		uaLower := strings.ToLower(ua)
		if strings.Contains(uaLower, "clash") {
			targetType = "CLASH"
		} else if strings.Contains(uaLower, "sing-box") || strings.Contains(uaLower, "singbox") {
			targetType = "SINGBOX"
		} else {
			targetType = "XRAY_BASE64"
		}
	}

	overrideTmpl := ""
	if srrRes.Matched && srrRes.MatchedRule != nil && srrRes.MatchedRule.ResponseModifications != nil {
		overrideTmpl = srrRes.MatchedRule.ResponseModifications.SubscriptionTemplate
	}

	if targetType == "XRAY_JSON" {
		contentType = "application/json; charset=utf-8"
		hostsMeta := h.getHostsWithInboundsForUser(user.ID)
		tmplJSON := h.getTemplateForUser(&user, "XRAY_JSON", overrideTmpl)
		jsonBody, err := GenerateXrayJSON(&user, hostsMeta, tmplJSON)
		if err != nil {
			http.Error(w, `{"message":"Failed to generate xray json"}`, http.StatusInternalServerError)
			return
		}
		body = jsonBody
	} else if targetType == "MIHOMO" || targetType == "CLASH" || targetType == "STASH" {
		contentType = "text/yaml; charset=utf-8"
		hostsMeta := h.getHostsWithInboundsForUser(user.ID)
		tmplYAML := h.getTemplateForUser(&user, targetType, overrideTmpl)
		if tmplYAML == "" && targetType != "MIHOMO" {
			tmplYAML = h.getTemplateForUser(&user, "MIHOMO", overrideTmpl)
		}
		yamlBody, err := GenerateMihomoYAML(&user, hostsMeta, tmplYAML)
		if err != nil {
			http.Error(w, `{"message":"Failed to generate yaml config"}`, http.StatusInternalServerError)
			return
		}
		body = yamlBody
	} else if targetType == "SINGBOX" {
		contentType = "application/json; charset=utf-8"
		hosts := h.getHostsForUser(user.ID)
		body = h.generator.generateSingboxJSON(&user, hosts)
	} else {
		contentType = "text/plain; charset=utf-8"
		hosts := h.getHostsForUser(user.ID)
		body = h.generator.generateBase64Links(&user, hosts)
	}

	w.Header().Set("Content-Type", contentType)
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
	if user.Traffic != nil {
		usedBytes = user.Traffic.UsedTrafficBytes
	}

	info := map[string]interface{}{
		"user": map[string]interface{}{
			"id":                user.ID,
			"username":          user.Username,
			"status":            user.Status,
			"trafficLimitBytes": user.TrafficLimitBytes,
			"usedTrafficBytes":  usedBytes,
			"expireAt":          user.ExpireAt,
		},
	}
	json.NewEncoder(w).Encode(info)
}
