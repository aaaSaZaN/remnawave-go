package nodes

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"remnawave-go/internal/database"

	"github.com/golang-jwt/jwt/v5"
)

type Client struct {
	httpClient *http.Client
	rsaPrivKey *rsa.PrivateKey
}

func NewNodeClient(caCertPEM, clientCertPEM, clientKeyPEM, jwtPrivKeyPEM, jwtPubKeyPEM []byte) (*Client, error) {
	var certPool *x509.CertPool
	if len(caCertPEM) > 0 {
		certPool = x509.NewCertPool()
		certPool.AppendCertsFromPEM(caCertPEM)
	}

	var serverName string
	if len(caCertPEM) > 0 && len(jwtPubKeyPEM) > 0 {
		serverName = DeriveSNI(string(caCertPEM), string(jwtPubKeyPEM))
	}

	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS13,
		InsecureSkipVerify: true, // skip default IP hostname validation
		ServerName:         serverName,
	}

	if certPool != nil {
		tlsConfig.RootCAs = certPool
		tlsConfig.VerifyPeerCertificate = func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return errors.New("no certificates presented by node")
			}
			certs := make([]*x509.Certificate, len(rawCerts))
			for i, asn1Data := range rawCerts {
				cert, err := x509.ParseCertificate(asn1Data)
				if err != nil {
					return fmt.Errorf("failed to parse node certificate: %w", err)
				}
				certs[i] = cert
			}

			opts := x509.VerifyOptions{
				Roots:       certPool,
				CurrentTime: time.Now(),
			}
			for _, cert := range certs[1:] {
				if opts.Intermediates == nil {
					opts.Intermediates = x509.NewCertPool()
				}
				opts.Intermediates.AddCert(cert)
			}

			_, err := certs[0].Verify(opts)
			return err
		}
	}

	if len(clientCertPEM) > 0 && len(clientKeyPEM) > 0 {
		cert, err := tls.X509KeyPair(clientCertPEM, clientKeyPEM)
		if err != nil {
			return nil, err
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	transport := &http.Transport{
		TLSClientConfig:       tlsConfig,
		ResponseHeaderTimeout: 15 * time.Second,
	}

	var rsaKey *rsa.PrivateKey
	if len(jwtPrivKeyPEM) > 0 {
		block, _ := pem.Decode(jwtPrivKeyPEM)
		if block != nil {
			if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
				if rk, ok := k.(*rsa.PrivateKey); ok {
					rsaKey = rk
				}
			} else if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
				rsaKey = k
			}
		}
	}

	return &Client{
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
		rsaPrivKey: rsaKey,
	}, nil
}

func (c *Client) generateToken() (string, error) {
	if c.rsaPrivKey == nil {
		return "", nil
	}
	claims := jwt.MapClaims{
		"uuid":     nil,
		"username": nil,
		"role":     "API",
		"sub":      "remnawave",
		"iat":      time.Now().Unix(),
		"exp":      time.Now().Add(30 * 24 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(c.rsaPrivKey)
}

func (c *Client) doRequest(ctx context.Context, method, url string, body interface{}) (*http.Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	if token, err := c.generateToken(); err == nil && token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	return c.httpClient.Do(req)
}

func getNodePort(node *database.Node) int {
	if node.Port != nil && *node.Port > 0 {
		return *node.Port
	}
	return 443
}

func (c *Client) CheckHealth(node *database.Node) (bool, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	// Remnanode/TS endpoint is /node/xray/healthcheck
	url := fmt.Sprintf("https://%s:%d/node/xray/healthcheck", node.Address, getNodePort(node))
	resp, err := c.doRequest(ctx, "GET", url, nil)
	if err != nil {
		return false, err.Error(), err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Sprintf("Node returned status %d", resp.StatusCode), nil
	}

	var res struct {
		Response struct {
			IsAlive                  bool    `json:"isAlive"`
			XrayInternalStatusCached bool    `json:"xrayInternalStatusCached"`
			XrayVersion              *string `json:"xrayVersion"`
			NodeVersion              string  `json:"nodeVersion"`
		} `json:"response"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&res)

	return res.Response.IsAlive, "OK", nil
}

func (c *Client) StartXray(node *database.Node, payload map[string]interface{}) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	url := fmt.Sprintf("https://%s:%d/node/xray/start", node.Address, getNodePort(node))
	resp, err := c.doRequest(ctx, "POST", url, payload)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

func (c *Client) StopXray(node *database.Node) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	url := fmt.Sprintf("https://%s:%d/node/xray/stop", node.Address, getNodePort(node))
	resp, err := c.doRequest(ctx, "GET", url, nil)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

type NodeUserIPInfo struct {
	IP       string      `json:"ip"`
	LastSeen interface{} `json:"lastSeen"`
}

type NodeUserSession struct {
	UserID int              `json:"userId"`
	IPs    []NodeUserIPInfo `json:"ips"`
}

func (c *Client) GetUsersIpList(node *database.Node) ([]NodeUserSession, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	url := fmt.Sprintf("https://%s:%d/node/stats/get-users-ip-list", node.Address, getNodePort(node))
	resp, err := c.doRequest(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("node returned status %d", resp.StatusCode)
	}

	var res struct {
		Response struct {
			Users []struct {
				UserID interface{}      `json:"userId"`
				IPs    []NodeUserIPInfo `json:"ips"`
			} `json:"users"`
		} `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	var users []NodeUserSession
	for _, u := range res.Response.Users {
		var uid int
		switch v := u.UserID.(type) {
		case float64:
			uid = int(v)
		case string:
			uid, _ = strconv.Atoi(v)
		}
		if uid > 0 {
			users = append(users, NodeUserSession{
				UserID: uid,
				IPs:    u.IPs,
			})
		}
	}
	return users, nil
}

func (c *Client) GetUserIpList(node *database.Node, userID string) ([]NodeUserIPInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	url := fmt.Sprintf("https://%s:%d/node/stats/get-user-ip-list", node.Address, getNodePort(node))
	resp, err := c.doRequest(ctx, "POST", url, map[string]string{"userId": userID})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("node returned status %d", resp.StatusCode)
	}

	var res struct {
		Response struct {
			IPs []interface{} `json:"ips"`
		} `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	var list []NodeUserIPInfo
	for _, item := range res.Response.IPs {
		switch v := item.(type) {
		case string:
			list = append(list, NodeUserIPInfo{IP: v, LastSeen: time.Now()})
		case map[string]interface{}:
			ip, _ := v["ip"].(string)
			lastSeen := v["lastSeen"]
			list = append(list, NodeUserIPInfo{IP: ip, LastSeen: lastSeen})
		}
	}
	return list, nil
}

func (c *Client) GetLogs(node *database.Node, logType string, lines int) ([]string, error) {
	if lines <= 0 {
		lines = 500
	}
	if logType == "" {
		logType = "xray"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	url := fmt.Sprintf("https://%s:%d/node/logs?type=%s&lines=%d", node.Address, getNodePort(node), logType, lines)
	resp, err := c.doRequest(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("node returned status %d", resp.StatusCode)
	}

	var res struct {
		Response struct {
			Logs []string `json:"logs"`
		} `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return res.Response.Logs, nil
}

func (c *Client) CheckUpdates(node *database.Node, repo string) (map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	url := fmt.Sprintf("https://%s:%d/node/updates/check", node.Address, getNodePort(node))
	if repo != "" {
		url += "?repo=" + repo
	}
	resp, err := c.doRequest(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("node returned status %d", resp.StatusCode)
	}

	var res struct {
		Response map[string]interface{}
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return res.Response, nil
}

func (c *Client) ApplyUpdate(node *database.Node, payload map[string]interface{}) (map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	url := fmt.Sprintf("https://%s:%d/node/updates/apply", node.Address, getNodePort(node))
	resp, err := c.doRequest(ctx, "POST", url, payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("node returned status %d", resp.StatusCode)
	}

	var res struct {
		Response map[string]interface{}
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return res.Response, nil
}
