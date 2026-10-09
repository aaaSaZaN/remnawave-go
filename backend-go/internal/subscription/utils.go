package subscription

import (
	"strings"

	"remnawave-go/internal/database"
)

// ParseArray accepts the model's cross-dialect array type and older serialized
// array values found in existing SQLite databases.
func ParseArray(raw interface{}) []string {
	switch value := raw.(type) {
	case database.StringArray:
		return []string(value)
	case []string:
		return value
	case string:
		if value == "" {
			return nil
		}
		var parsed database.StringArray
		if err := parsed.Scan(value); err == nil && (strings.HasPrefix(value, "[") || strings.HasPrefix(value, "{") || !strings.Contains(value, ",")) {
			return []string(parsed)
		}
		parts := strings.Split(strings.Trim(value, "{}[]\" "), ",")
		result := make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.Trim(strings.TrimSpace(part), "\"")
			if part != "" {
				result = append(result, part)
			}
		}
		return result
	default:
		return nil
	}
}

// IsExcluded checks whether a host is excluded from a given subscription type.
func IsExcluded(excludeRaw interface{}, subType string) bool {
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
