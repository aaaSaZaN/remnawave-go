package subscription

import (
	"strings"
)

// ParseArray parses PostgreSQL text array format (e.g. "{MIHOMO,CLASH}"),
// JSON array format (e.g. "[\"MIHOMO\",\"CLASH\"]"), or comma-separated lists into []string.
func ParseArray(raw string) []string {
	if raw == "" || raw == "{}" || raw == "[]" {
		return nil
	}
	clean := strings.Trim(raw, "{}[]\" ")
	if clean == "" {
		return nil
	}
	parts := strings.Split(clean, ",")
	var res []string
	for _, p := range parts {
		p = strings.Trim(strings.TrimSpace(p), "\"")
		if p != "" {
			res = append(res, p)
		}
	}
	return res
}

// IsExcluded checks whether a host is excluded from a given subscription type.
func IsExcluded(excludeRaw string, subType string) bool {
	for _, item := range ParseArray(excludeRaw) {
		if strings.EqualFold(item, subType) {
			return true
		}
	}
	return false
}

// ResolveVlessFlow determines the flow for a VLESS outbound.
// If the inbound settings explicitly set flow (even to "" or "none"), that explicit value is respected.
// Only if flow is omitted from settings does it default to "xtls-rprx-vision" for tcp/raw + reality/tls.
func ResolveVlessFlow(rawInbMap map[string]interface{}, network, security string) string {
	if rawInbMap != nil {
		if settings, ok := rawInbMap["settings"].(map[string]interface{}); ok {
			if fVal, exists := settings["flow"]; exists {
				if fStr, ok := fVal.(string); ok {
					if fStr == "xtls-rprx-vision" {
						return "xtls-rprx-vision"
					}
					return ""
				}
			}
		}
	}
	if (network == "tcp" || network == "raw") && (security == "reality" || security == "tls") {
		return "xtls-rprx-vision"
	}
	return ""
}
