package subscription

import (
	"encoding/json"
	"strings"

	"remnawave-go/internal/database"
)

type XrayHostMeta struct {
	Host     *database.Host
	Inbound  *database.ConfigProfileInbound
}

type XrayRawStreamSettings struct {
	Network         string                 `json:"network"`
	Security        string                 `json:"security"`
	RealitySettings *XrayRawRealitySettings `json:"realitySettings,omitempty"`
	TLSSettings     *XrayRawTLSSettings     `json:"tlsSettings,omitempty"`
	WsSettings      map[string]interface{} `json:"wsSettings,omitempty"`
	XhttpSettings   map[string]interface{} `json:"xhttpSettings,omitempty"`
	TCPSettings     map[string]interface{} `json:"tcpSettings,omitempty"`
	GrpcSettings    map[string]interface{} `json:"grpcSettings,omitempty"`
}

type XrayRawRealitySettings struct {
	Show        bool     `json:"show"`
	PrivateKey  string   `json:"privateKey,omitempty"`
	ShortIds    []string `json:"shortIds,omitempty"`
	ServerNames []string `json:"serverNames,omitempty"`
	Fingerprint string   `json:"fingerprint,omitempty"`
	SpiderX     string   `json:"spiderX,omitempty"`
}

type XrayRawTLSSettings struct {
	ServerName  string   `json:"serverName,omitempty"`
	Fingerprint string   `json:"fingerprint,omitempty"`
	Alpn        []string `json:"alpn,omitempty"`
}

type XrayRawInbound struct {
	Tag            string                 `json:"tag"`
	Protocol       string                 `json:"protocol"`
	Settings       map[string]interface{} `json:"settings,omitempty"`
	StreamSettings *XrayRawStreamSettings `json:"streamSettings,omitempty"`
}

func GenerateXrayJSON(user *database.User, hosts []XrayHostMeta, templateJSON string) (string, error) {
	var baseTemplate map[string]interface{}
	if templateJSON != "" && templateJSON != "{}" && templateJSON != "null" {
		_ = json.Unmarshal([]byte(templateJSON), &baseTemplate)
	}
	if baseTemplate == nil {
		baseTemplate = map[string]interface{}{
			"log": map[string]interface{}{
				"loglevel": "warning",
			},
			"inbounds": []interface{}{
				map[string]interface{}{
					"listen":   "127.0.0.1",
					"port":     10808,
					"protocol": "socks",
					"sniffing": map[string]interface{}{
						"enabled":      true,
						"destOverride": []string{"http", "tls"},
					},
				},
			},
			"outbounds": []interface{}{
				map[string]interface{}{
					"protocol": "freedom",
					"tag":      "direct",
				},
			},
		}
	}

	var configs []map[string]interface{}

	for _, hm := range hosts {
		h := hm.Host
		if h.IsDisabled || h.IsHidden {
			continue
		}

		outbound := buildOutbound(user, hm)
		if outbound == nil {
			continue
		}

		// Clone base template
		cfg := make(map[string]interface{})
		for k, v := range baseTemplate {
			cfg[k] = v
		}

		existingOutbounds, _ := cfg["outbounds"].([]interface{})
		newOutbounds := append([]interface{}{outbound}, existingOutbounds...)
		cfg["outbounds"] = newOutbounds
		cfg["remarks"] = h.Remark

		configs = append(configs, cfg)
	}

	resBytes, err := json.Marshal(configs)
	if err != nil {
		return "[]", err
	}
	return string(resBytes), nil
}

func buildOutbound(user *database.User, hm XrayHostMeta) map[string]interface{} {
	h := hm.Host
	inb := hm.Inbound

	var rawInb XrayRawInbound
	if inb != nil && inb.RawInbound != "" {
		_ = json.Unmarshal([]byte(inb.RawInbound), &rawInb)
	}

	protocol := "vless"
	if inb != nil && inb.Type != "" {
		protocol = strings.ToLower(inb.Type)
	}

	network := "tcp"
	if inb != nil && inb.Network != nil && *inb.Network != "" {
		network = strings.ToLower(*inb.Network)
	}
	if rawInb.StreamSettings != nil && rawInb.StreamSettings.Network != "" {
		network = strings.ToLower(rawInb.StreamSettings.Network)
	}

	security := "none"
	if inb != nil && inb.Security != nil && *inb.Security != "" {
		security = strings.ToLower(*inb.Security)
	}
	if rawInb.StreamSettings != nil && rawInb.StreamSettings.Security != "" {
		security = strings.ToLower(rawInb.StreamSettings.Security)
	}

	// Host overrides for security
	if h.SecurityLayer != "" && h.SecurityLayer != "DEFAULT" {
		security = strings.ToLower(h.SecurityLayer)
	}

	flow := ""
	if (network == "tcp" || network == "raw") && (security == "reality" || security == "tls") {
		flow = "xtls-rprx-vision"
	}

	settings := map[string]interface{}{
		"vnext": []interface{}{
			map[string]interface{}{
				"address": h.Address,
				"port":    h.Port,
				"users": []interface{}{
					map[string]interface{}{
						"id":         user.VlessUUID,
						"encryption": "none",
						"flow":       flow,
					},
				},
			},
		},
	}

	streamSettings := map[string]interface{}{
		"network": network,
	}

	serverName := h.Sni
	if serverName == "" && h.Address != "" {
		serverName = h.Address
	}

	fingerprint := h.Fingerprint
	if fingerprint == "" {
		fingerprint = "chrome"
	}

	if security == "reality" {
		streamSettings["security"] = "reality"

		var realityCfg XrayRawRealitySettings
		if rawInb.StreamSettings != nil && rawInb.StreamSettings.RealitySettings != nil {
			realityCfg = *rawInb.StreamSettings.RealitySettings
		}

		if serverName == "" && len(realityCfg.ServerNames) > 0 {
			serverName = realityCfg.ServerNames[0]
		}
		if fingerprint == "" && realityCfg.Fingerprint != "" {
			fingerprint = realityCfg.Fingerprint
		}

		publicKey := ""
		if realityCfg.PrivateKey != "" {
			if pk, err := DeriveX25519PublicKey(realityCfg.PrivateKey); err == nil {
				publicKey = pk
			}
		}

		shortID := ""
		if len(realityCfg.ShortIds) > 0 {
			shortID = realityCfg.ShortIds[0]
		}

		realityObj := map[string]interface{}{
			"show":        false,
			"fingerprint": fingerprint,
			"serverName":  serverName,
			"publicKey":   publicKey,
			"shortId":     shortID,
			"spiderX":     realityCfg.SpiderX,
		}
		streamSettings["realitySettings"] = realityObj

	} else if security == "tls" {
		streamSettings["security"] = "tls"

		var alpnList []string
		if h.Alpn != "" {
			for _, a := range strings.Split(h.Alpn, ",") {
				a = strings.TrimSpace(a)
				if a != "" {
					alpnList = append(alpnList, a)
				}
			}
		}

		tlsObj := map[string]interface{}{
			"serverName":  serverName,
			"fingerprint": fingerprint,
		}
		if len(alpnList) > 0 {
			tlsObj["alpn"] = alpnList
		}
		streamSettings["tlsSettings"] = tlsObj
	} else {
		streamSettings["security"] = "none"
	}

	// Transport settings
	if network == "ws" {
		wsObj := map[string]interface{}{}
		if h.Path != "" {
			wsObj["path"] = h.Path
		}
		if h.Host != "" {
			wsObj["headers"] = map[string]interface{}{
				"Host": h.Host,
			}
		}
		streamSettings["wsSettings"] = wsObj
	} else if network == "xhttp" {
		xhttpObj := map[string]interface{}{}
		if h.Path != "" {
			xhttpObj["path"] = h.Path
		}
		if h.Host != "" {
			xhttpObj["host"] = h.Host
		}
		streamSettings["xhttpSettings"] = xhttpObj
	} else if network == "grpc" {
		grpcObj := map[string]interface{}{}
		if h.Path != "" {
			grpcObj["serviceName"] = h.Path
		}
		streamSettings["grpcSettings"] = grpcObj
	}

	return map[string]interface{}{
		"tag":            "proxy",
		"protocol":       protocol,
		"settings":       settings,
		"streamSettings": streamSettings,
	}
}
