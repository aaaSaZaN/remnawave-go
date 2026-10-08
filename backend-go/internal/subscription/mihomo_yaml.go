package subscription

import (
	"encoding/json"
	"math/rand"
	"strings"

	"remnawave-go/internal/database"

	"gopkg.in/yaml.v3"
)

// generator the YAML config for Mihomo & Clash
func GenerateMihomoYAML(user *database.User, hosts []XrayHostMeta, templateYaml string) (string, error) {
	var proxyNodes []map[string]interface{}
	var proxyRemarks []string

	for _, hm := range hosts {
		h := hm.Host
		if h.IsDisabled || h.IsHidden {
			continue
		}

		node := buildMihomoProxyNode(user, hm)
		if node != nil {
			proxyNodes = append(proxyNodes, node)
			proxyRemarks = append(proxyRemarks, h.Remark)
		}
	}

	var root yaml.Node
	if templateYaml != "" {
		if err := yaml.Unmarshal([]byte(templateYaml), &root); err != nil {
			root = yaml.Node{}
		}
	}

	// If template is empty or invalid, construct default template
	if len(root.Content) == 0 {
		defaultYAML := `mixed-port: 7890
allow-lan: false
mode: rule
log-level: info
proxies: []
proxy-groups:
  - name: PROXY
    type: select
    proxies: []
rules:
  - MATCH,PROXY
`
		_ = yaml.Unmarshal([]byte(defaultYAML), &root)
	}

	// Process root node (mapping)
	var mappingNode *yaml.Node
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		mappingNode = root.Content[0]
	} else if root.Kind == yaml.MappingNode {
		mappingNode = &root
	}

	if mappingNode != nil && mappingNode.Kind == yaml.MappingNode {
		// Convert proxyNodes to yaml.Node
		var proxiesNode yaml.Node
		proxiesBytes, _ := yaml.Marshal(proxyNodes)
		_ = yaml.Unmarshal(proxiesBytes, &proxiesNode)

		var proxiesSeq *yaml.Node
		if proxiesNode.Kind == yaml.DocumentNode && len(proxiesNode.Content) > 0 {
			proxiesSeq = proxiesNode.Content[0]
		} else if proxiesNode.Kind == yaml.SequenceNode {
			proxiesSeq = &proxiesNode
		}

		// Update or inject 'proxies' key in root mapping
		var proxiesValNode *yaml.Node
		var proxyGroupsValNode *yaml.Node
		var remnawaveKeyIdx = -1

		for i := 0; i < len(mappingNode.Content); i += 2 {
			k := mappingNode.Content[i].Value
			if k == "proxies" {
				proxiesValNode = mappingNode.Content[i+1]
			} else if k == "proxy-groups" {
				proxyGroupsValNode = mappingNode.Content[i+1]
			} else if k == "remnawave" {
				remnawaveKeyIdx = i
			}
		}

		// Remove root remnawave key if present
		if remnawaveKeyIdx != -1 {
			mappingNode.Content = append(mappingNode.Content[:remnawaveKeyIdx], mappingNode.Content[remnawaveKeyIdx+2:]...)
		}

		// Set proxies
		if proxiesValNode != nil && proxiesSeq != nil {
			*proxiesValNode = *proxiesSeq
		} else if proxiesSeq != nil {
			keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: "proxies", Tag: "!!str"}
			mappingNode.Content = append([]*yaml.Node{keyNode, proxiesSeq}, mappingNode.Content...)
		}

		// Update proxy-groups
		if proxyGroupsValNode != nil && proxyGroupsValNode.Kind == yaml.SequenceNode {
			for _, grpNode := range proxyGroupsValNode.Content {
				if grpNode.Kind != yaml.MappingNode {
					continue
				}

				var grpProxiesValNode *yaml.Node
				var remnawaveGrpIdx = -1
				var includeProxies = true
				var selectRandomProxy = false
				var shuffleProxies = false

				for i := 0; i < len(grpNode.Content); i += 2 {
					gk := grpNode.Content[i].Value
					if gk == "proxies" {
						grpProxiesValNode = grpNode.Content[i+1]
					} else if gk == "remnawave" {
						remnawaveGrpIdx = i
						remNode := grpNode.Content[i+1]
						if remNode.Kind == yaml.MappingNode {
							for j := 0; j < len(remNode.Content); j += 2 {
								rk := remNode.Content[j].Value
								rv := remNode.Content[j+1].Value
								if rk == "include-proxies" && rv == "false" {
									includeProxies = false
								} else if rk == "select-random-proxy" && rv == "true" {
									selectRandomProxy = true
								} else if rk == "shuffle-proxies-order" && rv == "true" {
									shuffleProxies = true
								}
							}
						}
					}
				}

				// remove remnadata
				if remnawaveGrpIdx != -1 {
					grpNode.Content = append(grpNode.Content[:remnawaveGrpIdx], grpNode.Content[remnawaveGrpIdx+2:]...)
				}

				if !includeProxies {
					continue
				}

				var remarksToAdd []string
				if selectRandomProxy && len(proxyRemarks) > 0 {
					remarksToAdd = []string{proxyRemarks[rand.Intn(len(proxyRemarks))]}
				} else if shuffleProxies {
					shuffled := make([]string, len(proxyRemarks))
					copy(shuffled, proxyRemarks)
					rand.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
					remarksToAdd = shuffled
				} else {
					remarksToAdd = proxyRemarks
				}

				if grpProxiesValNode != nil {
					if grpProxiesValNode.Kind == yaml.SequenceNode {

						for _, r := range remarksToAdd {
							grpProxiesValNode.Content = append(grpProxiesValNode.Content, &yaml.Node{
								Kind:  yaml.ScalarNode,
								Value: r,
								Tag:   "!!str",
							})
						}
					} else {
						// (e.g. # LEAVE THIS LINE!)
						grpProxiesValNode.Kind = yaml.SequenceNode
						grpProxiesValNode.Tag = "!!seq"
						grpProxiesValNode.Content = nil
						for _, r := range remarksToAdd {
							grpProxiesValNode.Content = append(grpProxiesValNode.Content, &yaml.Node{
								Kind:  yaml.ScalarNode,
								Value: r,
								Tag:   "!!str",
							})
						}
					}
				}
			}
		}
	}

	outBytes, err := yaml.Marshal(&root)
	if err != nil {
		return "", err
	}
	return string(outBytes), nil
}

func buildMihomoProxyNode(user *database.User, hm XrayHostMeta) map[string]interface{} {
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
	if network == "raw" {
		network = "tcp"
	}

	security := "none"
	if inb != nil && inb.Security != nil && *inb.Security != "" {
		security = strings.ToLower(*inb.Security)
	}
	if rawInb.StreamSettings != nil && rawInb.StreamSettings.Security != "" {
		security = strings.ToLower(rawInb.StreamSettings.Security)
	}
	if h.SecurityLayer != "" && h.SecurityLayer != "DEFAULT" {
		security = strings.ToLower(h.SecurityLayer)
	}

	serverName := h.Sni
	if serverName == "" && h.Address != "" {
		serverName = h.Address
	}

	fingerprint := h.Fingerprint
	if fingerprint == "" {
		fingerprint = "chrome"
	}

	// 1. Hysteria 2
	if protocol == "hysteria" {
		return map[string]interface{}{
			"name":               h.Remark,
			"type":               "hysteria2",
			"server":             h.Address,
			"port":               h.Port,
			"password":           user.VlessUUID,
			"udp":                true,
			"sni":                serverName,
			"client-fingerprint": fingerprint,
			"skip-cert-verify":   false,
			"alpn":               []string{"h3"},
		}
	}

	// 2. VLESS
	if protocol == "vless" {
		node := map[string]interface{}{
			"name":            h.Remark,
			"type":            "vless",
			"server":          h.Address,
			"port":            h.Port,
			"uuid":            user.VlessUUID,
			"udp":             true,
			"packet-encoding": "xudp",
		}

		if network != "tcp" {
			node["network"] = network
		}

		// Security: Reality
		if security == "reality" {
			node["tls"] = true

			var realityCfg XrayRawRealitySettings
			if rawInb.StreamSettings != nil && rawInb.StreamSettings.RealitySettings != nil {
				realityCfg = *rawInb.StreamSettings.RealitySettings
			}

			if serverName == "" && len(realityCfg.ServerNames) > 0 {
				serverName = realityCfg.ServerNames[0]
			}
			if h.Fingerprint == "" && realityCfg.Fingerprint != "" {
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

			node["servername"] = serverName
			node["client-fingerprint"] = fingerprint
			node["reality-opts"] = map[string]interface{}{
				"public-key": publicKey,
				"short-id":   shortID,
			}

			if network == "tcp" {
				node["flow"] = "xtls-rprx-vision"
			}
		} else if security == "tls" {
			node["tls"] = true
			node["servername"] = serverName
			node["client-fingerprint"] = fingerprint

			if h.Alpn != "" {
				var alpnList []string
				for _, a := range strings.Split(h.Alpn, ",") {
					a = strings.TrimSpace(a)
					if a != "" {
						alpnList = append(alpnList, a)
					}
				}
				if len(alpnList) > 0 {
					node["alpn"] = alpnList
				}
			}
		}

		// Transports
		if network == "ws" {
			wsOpts := map[string]interface{}{}
			path := h.Path
			if path == "" && rawInb.StreamSettings != nil && rawInb.StreamSettings.WsSettings != nil {
				if p, ok := rawInb.StreamSettings.WsSettings["path"].(string); ok {
					path = p
				}
			}
			host := h.Host
			if host == "" && rawInb.StreamSettings != nil && rawInb.StreamSettings.WsSettings != nil {
				if hh, ok := rawInb.StreamSettings.WsSettings["headers"].(map[string]interface{}); ok {
					if hHost, ok := hh["Host"].(string); ok {
						host = hHost
					}
				}
			}
			if path != "" {
				wsOpts["path"] = path
			}
			if host != "" {
				wsOpts["headers"] = map[string]interface{}{
					"Host": host,
				}
			}
			if len(wsOpts) > 0 {
				node["ws-opts"] = wsOpts
			}
		} else if network == "xhttp" {
			xhttpOpts := map[string]interface{}{}
			path := h.Path
			if path == "" && rawInb.StreamSettings != nil && rawInb.StreamSettings.XhttpSettings != nil {
				if p, ok := rawInb.StreamSettings.XhttpSettings["path"].(string); ok {
					path = p
				}
			}
			host := h.Host
			if host == "" && rawInb.StreamSettings != nil && rawInb.StreamSettings.XhttpSettings != nil {
				if hh, ok := rawInb.StreamSettings.XhttpSettings["host"].(string); ok {
					host = hh
				}
			}
			if path != "" {
				xhttpOpts["path"] = path
			}
			if host != "" {
				xhttpOpts["host"] = host
			}
			xhttpOpts["mode"] = "auto"
			node["xhttp-opts"] = xhttpOpts
		} else if network == "grpc" {
			serviceName := h.Path
			if serviceName == "" && rawInb.StreamSettings != nil && rawInb.StreamSettings.GrpcSettings != nil {
				if sn, ok := rawInb.StreamSettings.GrpcSettings["serviceName"].(string); ok {
					serviceName = sn
				}
			}
			if serviceName != "" {
				node["grpc-opts"] = map[string]interface{}{
					"grpc-service-name": serviceName,
				}
			}
		}

		return node
	}

	// 3. fckn trojan lol
	if protocol == "trojan" {
		node := map[string]interface{}{
			"name":               h.Remark,
			"type":               "trojan",
			"server":             h.Address,
			"port":               h.Port,
			"password":           user.VlessUUID,
			"udp":                true,
			"tls":                true,
			"sni":                serverName,
			"client-fingerprint": fingerprint,
		}
		if network != "tcp" {
			node["network"] = network
		}
		return node
	}

	// 4. sock
	if protocol == "shadowsocks" || protocol == "ss" {
		return map[string]interface{}{
			"name":     h.Remark,
			"type":     "ss",
			"server":   h.Address,
			"port":     h.Port,
			"password": user.VlessUUID,
			"cipher":   "2022-blake3-aes-128-gcm",
			"udp":      true,
		}
	}

	return nil
}
