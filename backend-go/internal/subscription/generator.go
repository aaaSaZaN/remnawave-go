package subscription

import (
	"encoding/base64"
	"fmt"
	"strings"

	"remnawave-go/internal/database"
)

type Generator struct{}

func NewGenerator() *Generator {
	return &Generator{}
}

func (g *Generator) Generate(user *database.User, hosts []database.Host, userAgent string) (string, string) {
	if user.Status != "ACTIVE" {
		return "text/plain; charset=utf-8", "Account is not active"
	}

	ua := strings.ToLower(userAgent)

	if strings.Contains(ua, "clash") {
		return "text/yaml; charset=utf-8", g.generateClashYAML(user, hosts)
	}

	if strings.Contains(ua, "sing-box") || strings.Contains(ua, "singbox") {
		return "application/json; charset=utf-8", g.generateSingboxJSON(user, hosts)
	}

	return "text/plain; charset=utf-8", g.generateBase64Links(user, hosts)
}

func (g *Generator) generateRawLinks(user *database.User, hosts []database.Host) []string {
	var lines []string
	for _, h := range hosts {
		if h.IsDisabled || h.IsHidden || IsExcluded(h.ExcludeFromSubscriptionTypes, "XRAY_BASE64") {
			continue
		}
		security := "tls"
		if h.SecurityLayer != "" && h.SecurityLayer != "DEFAULT" {
			security = strings.ToLower(h.SecurityLayer)
		}
		link := fmt.Sprintf("vless://%s@%s:%d?security=%s&sni=%s&fp=%s#%s",
			user.VlessUUID, h.Address, h.Port, security, h.Sni, h.Fingerprint, h.Remark)
		lines = append(lines, link)
	}
	if lines == nil {
		lines = []string{}
	}
	return lines
}

func (g *Generator) generateBase64Links(user *database.User, hosts []database.Host) string {
	lines := g.generateRawLinks(user, hosts)
	raw := strings.Join(lines, "\n")
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

func (g *Generator) generateClashYAML(user *database.User, hosts []database.Host) string {
	var sb strings.Builder
	sb.WriteString("proxies:\n")
	for _, h := range hosts {
		if h.IsDisabled || h.IsHidden || IsExcluded(h.ExcludeFromSubscriptionTypes, "CLASH") {
			continue
		}
		sb.WriteString(fmt.Sprintf(`  - name: "%s"
    type: vless
    server: %s
    port: %d
    uuid: %s
    network: ws
    tls: true
    servername: %s
    client-fingerprint: %s
`, h.Remark, h.Address, h.Port, user.VlessUUID, h.Sni, h.Fingerprint))
	}
	return sb.String()
}

func (g *Generator) generateSingboxJSON(user *database.User, hosts []database.Host) string {
	var outbounds []string
	for _, h := range hosts {
		if h.IsDisabled || h.IsHidden || IsExcluded(h.ExcludeFromSubscriptionTypes, "SINGBOX") {
			continue
		}
		outbound := fmt.Sprintf(`    {
      "type": "vless",
      "tag": "%s",
      "server": "%s",
      "server_port": %d,
      "uuid": "%s",
      "tls": {
        "enabled": true,
        "server_name": "%s",
        "utls": {
          "enabled": true,
          "fingerprint": "%s"
        }
      }
    }`, h.Remark, h.Address, h.Port, user.VlessUUID, h.Sni, h.Fingerprint)
		outbounds = append(outbounds, outbound)
	}

	return fmt.Sprintf(`{
  "outbounds": [
%s
  ]
}`, strings.Join(outbounds, ",\n"))
}
