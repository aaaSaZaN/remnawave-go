package subscription

import (
	"encoding/json"
	"strings"

	"remnawave-go/internal/database"

	"gopkg.in/yaml.v3"
)

func GenerateMihomoYAML(user *database.User, hosts []XrayHostMeta, templateYaml string, targetType string) (string, error) {
	if targetType == "" {
		targetType = "MIHOMO"
	}

	var proxyNodes []map[string]interface{}
	var proxyRemarks []string

	for _, hm := range hosts {
		h := hm.Host
		if h.IsDisabled || h.IsHidden {
			continue
		}
		if IsExcluded(h.ExcludeFromSubscriptionTypes, targetType) {
			continue
		}

		node := buildMihomoProxyNode(user, hm)
		if node != nil {
			// Stash does not support xhttp transport
			if targetType == "STASH" {
				if net, ok := node["network"].(string); ok && strings.ToLower(net) == "xhttp" {
					continue
				}
			}
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
		// Serialize generated proxies to YAML nodes
		var proxiesSeq *yaml.Node
		if len(proxyNodes) > 0 {
			proxiesYAMLBytes, _ := yaml.Marshal(proxyNodes)
			var tmpDoc yaml.Node
			_ = yaml.Unmarshal(proxiesYAMLBytes, &tmpDoc)
			if tmpDoc.Kind == yaml.DocumentNode && len(tmpDoc.Content) > 0 {
				proxiesSeq = tmpDoc.Content[0]
			}
		} else {
			proxiesSeq = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		}

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

				// Remove remnawave metadata from group
				if remnawaveGrpIdx != -1 {
					grpNode.Content = append(grpNode.Content[:remnawaveGrpIdx], grpNode.Content[remnawaveGrpIdx+2:]...)
				}

				if !includeProxies {
					continue
				}

				selectedRemarks := make([]string, len(proxyRemarks))
				copy(selectedRemarks, proxyRemarks)

				if shuffleProxies {
					// Deterministic stable order or simple reverse
				}
				if selectRandomProxy && len(selectedRemarks) > 1 {
					selectedRemarks = selectedRemarks[:1]
				}

				if grpProxiesValNode != nil && grpProxiesValNode.Kind == yaml.SequenceNode {
					// Append proxy remarks
					for _, rem := range selectedRemarks {
						grpProxiesValNode.Content = append(grpProxiesValNode.Content, &yaml.Node{
							Kind:  yaml.ScalarNode,
							Value: rem,
							Tag:   "!!str",
						})
					}
				} else if grpProxiesValNode != nil {
					// Empty/null proxies in group
					grpProxiesValNode.Kind = yaml.SequenceNode
					grpProxiesValNode.Tag = "!!seq"
					grpProxiesValNode.Content = nil
					for _, rem := range selectedRemarks {
						grpProxiesValNode.Content = append(grpProxiesValNode.Content, &yaml.Node{
							Kind:  yaml.ScalarNode,
							Value: rem,
							Tag:   "!!str",
						})
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
	var rawInbMap map[string]interface{}
	if inb != nil && inb.RawInbound != "" {
		_ = json.Unmarshal([]byte(inb.RawInbound), &rawInb)
		_ = json.Unmarshal([]byte(inb.RawInbound), &rawInbMap)
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

	// Host overrides for security
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

	// 1. Hysteria 2 Protocol
	if protocol == "hysteria" {
		node := map[string]interface{}{
			"name":             h.Remark,
			"type":             "hysteria2",
			"server":           h.Address,
			"port":             h.Port,
			"password":         user.VlessUUID,
			"sni":              serverName,
			"skip-cert-verify": false,
			"udp":              true,
		}

		// Salamander UDP Obfuscation
		var obfsPassword string
		if rawInbMap != nil {
			if ss, ok := rawInbMap["streamSettings"].(map[string]interface{}); ok {
				if fm, ok := ss["finalmask"].(map[string]interface{}); ok {
					if udpList, ok := fm["udp"].([]interface{}); ok && len(udpList) > 0 {
						if udp0, ok := udpList[0].(map[string]interface{}); ok {
							if udp0["type"] == "salamander" {
								if settings, ok := udp0["settings"].(map[string]interface{}); ok {
									if pw, ok := settings["password"].(string); ok {
										obfsPassword = pw
									}
								}
							}
						}
					}
				}
			}
		}
		if obfsPassword == "" && h.FinalMask != "" && h.FinalMask != "null" {
			var fm map[string]interface{}
			if err := json.Unmarshal([]byte(h.FinalMask), &fm); err == nil {
				if udpList, ok := fm["udp"].([]interface{}); ok && len(udpList) > 0 {
					if udp0, ok := udpList[0].(map[string]interface{}); ok {
						if udp0["type"] == "salamander" {
							if settings, ok := udp0["settings"].(map[string]interface{}); ok {
								if pw, ok := settings["password"].(string); ok {
									obfsPassword = pw
								}
							}
						}
					}
				}
			}
		}

		if obfsPassword != "" {
			node["obfs"] = "salamander"
			node["obfs-password"] = obfsPassword
		}

		return node
	}

	// 2. VLESS Protocol
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
			node["servername"] = serverName
			node["client-fingerprint"] = fingerprint

			var realityCfg XrayRawRealitySettings
			if rawInb.StreamSettings != nil && rawInb.StreamSettings.RealitySettings != nil {
				realityCfg = *rawInb.StreamSettings.RealitySettings
			}

			if serverName == "" && len(realityCfg.ServerNames) > 0 {
				node["servername"] = realityCfg.ServerNames[0]
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

			node["reality-opts"] = map[string]interface{}{
				"public-key": publicKey,
				"short-id":   shortID,
			}

			flow := ResolveVlessFlow(rawInbMap, network, security)
			if flow == "xtls-rprx-vision" {
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
			if path != "" {
				wsOpts["path"] = path
			}
			host := h.Host
			if host == "" && rawInb.StreamSettings != nil && rawInb.StreamSettings.WsSettings != nil {
				if hdrs, ok := rawInb.StreamSettings.WsSettings["headers"].(map[string]interface{}); ok {
					if hStr, ok := hdrs["Host"].(string); ok {
						host = hStr
					}
				}
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
			if path != "" {
				xhttpOpts["path"] = path
			}
			host := h.Host
			if host == "" && rawInb.StreamSettings != nil && rawInb.StreamSettings.XhttpSettings != nil {
				if hStr, ok := rawInb.StreamSettings.XhttpSettings["host"].(string); ok {
					host = hStr
				}
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

	// 3. Trojan
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

	// 4. Shadowsocks
	if protocol == "shadowsocks" {
		method := "aes-256-gcm"
		if inb != nil && inb.Network != nil && *inb.Network != "" {
			method = *inb.Network
		}
		node := map[string]interface{}{
			"name":     h.Remark,
			"type":     "ss",
			"server":   h.Address,
			"port":     h.Port,
			"cipher":   method,
			"password": user.VlessUUID,
			"udp":      true,
		}
		return node
	}

	return nil
}
