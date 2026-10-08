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
