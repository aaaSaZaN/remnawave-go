package system

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"remnawave-go/internal/database"

	"gorm.io/gorm"
)

var startTime = time.Now()

type Handler struct {
	db      *gorm.DB
	restore *BackupRestoreService
}

func NewHandler(db *gorm.DB) *Handler {
	return &Handler{
		db:      db,
		restore: NewBackupRestoreService(db),
	}
}

type RuntimeMetric struct {
	Rss              uint64  `json:"rss"`
	HeapUsed         uint64  `json:"heapUsed"`
	HeapTotal        uint64  `json:"heapTotal"`
	External         uint64  `json:"external"`
	ArrayBuffers     uint64  `json:"arrayBuffers"`
	EventLoopDelayMs float64 `json:"eventLoopDelayMs"`
	EventLoopP99Ms   float64 `json:"eventLoopP99Ms"`
	ActiveHandles    int     `json:"activeHandles"`
	Uptime           int64   `json:"uptime"`
	Pid              int     `json:"pid"`
	Timestamp        int64   `json:"timestamp"`
	InstanceID       string  `json:"instanceId"`
	InstanceType     string  `json:"instanceType"`
}

type HealthResponse struct {
	Response struct {
		RuntimeMetrics []RuntimeMetric `json:"runtimeMetrics"`
	} `json:"response"`
	Status string `json:"status"`
}

type BaseStat struct {
	Current    string `json:"current"`
	Previous   string `json:"previous"`
	Difference string `json:"difference"`
}

type BandwidthStatsResponse struct {
	Response struct {
		BandwidthLastTwoDays   BaseStat `json:"bandwidthLastTwoDays"`
		BandwidthLastSevenDays BaseStat `json:"bandwidthLastSevenDays"`
		BandwidthLast30Days    BaseStat `json:"bandwidthLast30Days"`
		BandwidthCalendarMonth BaseStat `json:"bandwidthCalendarMonth"`
		BandwidthCurrentYear   BaseStat `json:"bandwidthCurrentYear"`
	} `json:"response"`
}

type SystemStatsResponse struct {
	Response struct {
		Cpu struct {
			Cores int `json:"cores"`
		} `json:"cpu"`
		Memory struct {
			Total uint64 `json:"total"`
			Free  uint64 `json:"free"`
			Used  uint64 `json:"used"`
		} `json:"memory"`
		Uptime    int64 `json:"uptime"`
		Timestamp int64 `json:"timestamp"`
		Users     struct {
			StatusCounts map[string]int `json:"statusCounts"`
			TotalUsers   int            `json:"totalUsers"`
		} `json:"users"`
		OnlineStats struct {
			LastDay     int `json:"lastDay"`
			LastWeek    int `json:"lastWeek"`
			NeverOnline int `json:"neverOnline"`
			OnlineNow   int `json:"onlineNow"`
		} `json:"onlineStats"`
		Nodes struct {
			TotalOnline        int    `json:"totalOnline"`
			TotalBytesLifetime string `json:"totalBytesLifetime"`
		} `json:"nodes"`
	} `json:"response"`
}

type NodesStatisticsResponse struct {
	Response struct {
		LastSevenDays []NodeDayStat `json:"lastSevenDays"`
	} `json:"response"`
}

type NodeDayStat struct {
	NodeName   string `json:"nodeName"`
	Date       string `json:"date"`
	TotalBytes string `json:"totalBytes"`
}

type NodesMetricsResponse struct {
	Response struct {
		Nodes []NodeMetricItem `json:"nodes"`
	} `json:"response"`
}

type NodeMetricItem struct {
	NodeUUID      string        `json:"nodeUuid"`
	NodeName      string        `json:"nodeName"`
	CountryEmoji  string        `json:"countryEmoji"`
	ProviderName  string        `json:"providerName"`
	UsersOnline   int           `json:"usersOnline"`
	InboundsStats []InboundStat `json:"inboundsStats"`
}

type InboundStat struct {
	Tag      string `json:"tag"`
	Upload   string `json:"upload"`
	Download string `json:"download"`
}

type RecapResponse struct {
	Response struct {
		ThisMonth struct {
			Users   int    `json:"users"`
			Traffic string `json:"traffic"`
		} `json:"thisMonth"`
		Total struct {
			Users             int    `json:"users"`
			Nodes             int    `json:"nodes"`
			Traffic           string `json:"traffic"`
			NodesRam          string `json:"nodesRam"`
			NodesCpuCores     int    `json:"nodesCpuCores"`
			DistinctCountries int    `json:"distinctCountries"`
		} `json:"total"`
		Version string `json:"version"`
	} `json:"response"`
}

type SettingsResponse struct {
	Response interface{} `json:"response"`
}

type MetadataResponse struct {
	Response struct {
		Version string `json:"version"`
		Build   struct {
			Time   string `json:"time"`
			Number string `json:"number"`
		} `json:"build"`
		Git struct {
			Backend struct {
				CommitSha string `json:"commitSha"`
				Branch    string `json:"branch"`
				CommitURL string `json:"commitUrl"`
			} `json:"backend"`
			Frontend struct {
				CommitSha string `json:"commitSha"`
				CommitURL string `json:"commitUrl"`
			} `json:"frontend"`
		} `json:"git"`
	} `json:"response"`
}

func (h *Handler) GetMetadata(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var resp MetadataResponse
	resp.Response.Version = "3.4.15"
	resp.Response.Build.Time = "2026-10-03T20:00:00Z"
	resp.Response.Build.Number = "1"
	resp.Response.Git.Backend.CommitSha = "339fc6c"
	resp.Response.Git.Backend.Branch = "main"
	resp.Response.Git.Backend.CommitURL = "https://github.com/remnawave/backend"
	resp.Response.Git.Frontend.CommitSha = "339fc6c"
	resp.Response.Git.Frontend.CommitURL = "https://github.com/remnawave/frontend"

	json.NewEncoder(w).Encode(resp)
}

func formatSubscriptionSettings(s *database.SubscriptionSetting) map[string]interface{} {
	customRemarks := json.RawMessage("{}")
	if s.CustomRemarks != "" && s.CustomRemarks != "null" {
		customRemarks = json.RawMessage(s.CustomRemarks)
	}

	customHeaders := json.RawMessage("{}")
	if s.CustomResponseHeaders != "" && s.CustomResponseHeaders != "null" {
		customHeaders = json.RawMessage(s.CustomResponseHeaders)
	}

	responseRules := json.RawMessage(`{"version":"1","rules":[]}`)
	if s.ResponseRules != "" && s.ResponseRules != "null" {
		responseRules = json.RawMessage(s.ResponseRules)
	}

	hwidSettings := json.RawMessage(`{"enabled":false,"fallbackDeviceLimit":0,"maxDevicesAnnounce":null}`)
	if s.HwidSettings != "" && s.HwidSettings != "null" {
		hwidSettings = json.RawMessage(s.HwidSettings)
	}

	return map[string]interface{}{
		"uuid":                        s.UUID,
		"serveJsonAtBaseSubscription": s.ServeJsonAtBaseSubscription,
		"isShowCustomRemarks":         s.IsShowCustomRemarks,
		"customRemarks":               customRemarks,
		"customResponseHeaders":       customHeaders,
		"randomizeHosts":              s.RandomizeHosts,
		"responseRules":               responseRules,
		"hwidSettings":                hwidSettings,
		"createdAt":                   s.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
		"updatedAt":                   s.UpdatedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
	}
}

func (h *Handler) GetSubscriptionSettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	s := database.EnsureSubscriptionSetting(h.db)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": formatSubscriptionSettings(s),
	})
}

func (h *Handler) UpdateSubscriptionSettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	s := database.EnsureSubscriptionSetting(h.db)

	var body struct {
		UUID                        *string         `json:"uuid"`
		ServeJsonAtBaseSubscription *bool           `json:"serveJsonAtBaseSubscription"`
		IsShowCustomRemarks         *bool           `json:"isShowCustomRemarks"`
		CustomRemarks               json.RawMessage `json:"customRemarks"`
		CustomResponseHeaders       json.RawMessage `json:"customResponseHeaders"`
		RandomizeHosts              *bool           `json:"randomizeHosts"`
		ResponseRules               json.RawMessage `json:"responseRules"`
		HwidSettings                json.RawMessage `json:"hwidSettings"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Invalid body"})
		return
	}

	if body.ServeJsonAtBaseSubscription != nil {
		s.ServeJsonAtBaseSubscription = *body.ServeJsonAtBaseSubscription
	}
	if body.IsShowCustomRemarks != nil {
		s.IsShowCustomRemarks = *body.IsShowCustomRemarks
	}
	if len(body.CustomRemarks) > 0 && string(body.CustomRemarks) != "null" {
		s.CustomRemarks = string(body.CustomRemarks)
	}
	if len(body.CustomResponseHeaders) > 0 && string(body.CustomResponseHeaders) != "null" {
		s.CustomResponseHeaders = string(body.CustomResponseHeaders)
	}
	if body.RandomizeHosts != nil {
		s.RandomizeHosts = *body.RandomizeHosts
	}
	if len(body.ResponseRules) > 0 && string(body.ResponseRules) != "null" {
		s.ResponseRules = string(body.ResponseRules)
	}
	if len(body.HwidSettings) > 0 && string(body.HwidSettings) != "null" {
		s.HwidSettings = string(body.HwidSettings)
	}
	s.UpdatedAt = time.Now().UTC()
	h.db.Save(s)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": formatSubscriptionSettings(s),
	})
}

func (h *Handler) GetHttpStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	total, routes := GlobalRouteCounter.GetStats()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": map[string]interface{}{
			"routes": routes,
			"total":  total,
		},
	})
}

func (h *Handler) GetHealth(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	metrics := RuntimeMetric{
		Rss:              m.Sys,
		HeapUsed:         m.Alloc,
		HeapTotal:        m.TotalAlloc,
		External:         0,
		ArrayBuffers:     0,
		EventLoopDelayMs: 0.1,
		EventLoopP99Ms:   0.5,
		ActiveHandles:    runtime.NumGoroutine(),
		Uptime:           int64(time.Since(startTime).Seconds()),
		Pid:              os.Getpid(),
		Timestamp:        time.Now().UnixMilli(),
		InstanceID:       "remnawave-go",
		InstanceType:     "api",
	}

	var resp HealthResponse
	resp.Response.RuntimeMetrics = []RuntimeMetric{metrics}
	resp.Status = "ok"

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	var users []database.User
	h.db.Find(&users)

	statusCounts := map[string]int{
		"ACTIVE":   0,
		"DISABLED": 0,
		"LIMITED":  0,
		"EXPIRED":  0,
	}

	for _, u := range users {
		statusCounts[u.Status]++
	}

	var nodes []database.Node
	h.db.Find(&nodes)

	var resp SystemStatsResponse
	resp.Response.Cpu.Cores = runtime.NumCPU()
	resp.Response.Memory.Total = m.Sys
	resp.Response.Memory.Free = m.Sys - m.Alloc
	resp.Response.Memory.Used = m.Alloc
	resp.Response.Uptime = int64(time.Since(startTime).Seconds())
	resp.Response.Timestamp = time.Now().UnixMilli()

	resp.Response.Users.StatusCounts = statusCounts
	resp.Response.Users.TotalUsers = len(users)

	resp.Response.OnlineStats.LastDay = 0
	resp.Response.OnlineStats.LastWeek = 0
	resp.Response.OnlineStats.NeverOnline = len(users)
	resp.Response.OnlineStats.OnlineNow = 0

	resp.Response.Nodes.TotalOnline = len(nodes)
	resp.Response.Nodes.TotalBytesLifetime = "0"

	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) GetBandwidthStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	zeroStat := BaseStat{
		Current:    "0",
		Previous:   "0",
		Difference: "0",
	}

	var resp BandwidthStatsResponse
	resp.Response.BandwidthLastTwoDays = zeroStat
	resp.Response.BandwidthLastSevenDays = zeroStat
	resp.Response.BandwidthLast30Days = zeroStat
	resp.Response.BandwidthCalendarMonth = zeroStat
	resp.Response.BandwidthCurrentYear = zeroStat

	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) GetNodesStatistics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var resp NodesStatisticsResponse
	resp.Response.LastSevenDays = []NodeDayStat{}
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) GetNodesMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var resp NodesMetricsResponse
	resp.Response.Nodes = []NodeMetricItem{}
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) GetRecap(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var userCount int64
	h.db.Model(&database.User{}).Count(&userCount)

	var nodeCount int64
	h.db.Model(&database.Node{}).Count(&nodeCount)

	var resp RecapResponse
	resp.Response.ThisMonth.Users = int(userCount)
	resp.Response.ThisMonth.Traffic = "0 B"
	resp.Response.Total.Users = int(userCount)
	resp.Response.Total.Nodes = int(nodeCount)
	resp.Response.Total.Traffic = "0 B"
	resp.Response.Total.NodesRam = "0 B"
	resp.Response.Total.NodesCpuCores = 0
	resp.Response.Total.DistinctCountries = 0
	resp.Response.Version = "3.4.15"

	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var setting database.RemnawaveSetting
	h.db.First(&setting)
	title := "Remnawave"
	if setting.Title != "" {
		title = setting.Title
	}

	passkeySettings := map[string]interface{}{
		"enabled": false,
		"rpId":    nil,
		"origin":  nil,
	}
	if setting.PasskeySettings != "" && setting.PasskeySettings != "{}" {
		_ = json.Unmarshal([]byte(setting.PasskeySettings), &passkeySettings)
	}

	oauth2Settings := map[string]interface{}{
		"github": map[string]interface{}{
			"enabled":       false,
			"clientId":      nil,
			"clientSecret":  nil,
			"allowedEmails": []string{},
		},
		"pocketid": map[string]interface{}{
			"enabled":        false,
			"clientId":       nil,
			"clientSecret":   nil,
			"frontendDomain": nil,
			"plainDomain":    nil,
			"allowedEmails":  []string{},
		},
		"yandex": map[string]interface{}{
			"enabled":       false,
			"clientId":      nil,
			"clientSecret":  nil,
			"allowedEmails": []string{},
		},
		"keycloak": map[string]interface{}{
			"enabled":        false,
			"realm":          nil,
			"clientId":       nil,
			"clientSecret":   nil,
			"frontendDomain": nil,
			"keycloakDomain": nil,
			"allowedEmails":  []string{},
		},
		"generic": map[string]interface{}{
			"enabled":          false,
			"clientId":         nil,
			"clientSecret":     nil,
			"withPkce":         false,
			"authorizationUrl": nil,
			"tokenUrl":         nil,
			"frontendDomain":   nil,
			"allowedEmails":    []string{},
		},
		"telegram": map[string]interface{}{
			"enabled":        false,
			"clientId":       nil,
			"clientSecret":   nil,
			"allowedIds":     []string{},
			"frontendDomain": nil,
		},
	}
	if setting.OAuth2Settings != "" && setting.OAuth2Settings != "{}" {
		_ = json.Unmarshal([]byte(setting.OAuth2Settings), &oauth2Settings)
	}

	passwordSettings := map[string]interface{}{
		"enabled": setting.PasswordAuthEnabled,
	}
	if setting.PasswordSettings != "" && setting.PasswordSettings != "{}" {
		_ = json.Unmarshal([]byte(setting.PasswordSettings), &passwordSettings)
	}

	brandingSettings := map[string]interface{}{
		"title":   title,
		"logoUrl": nil,
	}
	if setting.LogoURL != "" {
		brandingSettings["logoUrl"] = setting.LogoURL
	}
	if setting.BrandingSettings != "" && setting.BrandingSettings != "{}" {
		_ = json.Unmarshal([]byte(setting.BrandingSettings), &brandingSettings)
	}

	resp := map[string]interface{}{
		"response": map[string]interface{}{
			"passkeySettings":  passkeySettings,
			"oauth2Settings":   oauth2Settings,
			"passwordSettings": passwordSettings,
			"brandingSettings": brandingSettings,
		},
	}
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var body map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"message":"Invalid request body"}`, http.StatusBadRequest)
		return
	}

	var setting database.RemnawaveSetting
	h.db.First(&setting)
	if setting.ID == 0 {
		setting.ID = 1
		setting.Title = "Remnawave"
		setting.IsLoginAllowed = true
		setting.PasswordAuthEnabled = true
		h.db.Create(&setting)
	}

	if ps, ok := body["passkeySettings"]; ok {
		b, _ := json.Marshal(ps)
		setting.PasskeySettings = string(b)
	}
	if os, ok := body["oauth2Settings"]; ok {
		b, _ := json.Marshal(os)
		setting.OAuth2Settings = string(b)
	}
	if pws, ok := body["passwordSettings"]; ok {
		b, _ := json.Marshal(pws)
		setting.PasswordSettings = string(b)
		if m, ok := pws.(map[string]interface{}); ok {
			if en, ok := m["enabled"].(bool); ok {
				setting.PasswordAuthEnabled = en
			}
		}
	}
	if bs, ok := body["brandingSettings"]; ok {
		b, _ := json.Marshal(bs)
		setting.BrandingSettings = string(b)
		if m, ok := bs.(map[string]interface{}); ok {
			if t, ok := m["title"].(string); ok && t != "" {
				setting.Title = t
			}
			if lu, ok := m["logoUrl"].(string); ok {
				setting.LogoURL = lu
			}
		}
	}
	h.db.Save(&setting)
	h.GetSettings(w, r)
}

func (h *Handler) RestoreBackup(w http.ResponseWriter, r *http.Request) {
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, `{"error":"No backup file uploaded"}`, http.StatusBadRequest)
		return
	}
	defer file.Close()

	tempPath := filepath.Join(os.TempDir(), fmt.Sprintf("remnawave_restore_%d.tar.gz", time.Now().UnixNano()))
	out, err := os.Create(tempPath)
	if err != nil {
		http.Error(w, `{"error":"Cannot create temporary file"}`, http.StatusInternalServerError)
		return
	}
	defer os.Remove(tempPath)
	defer out.Close()

	if _, err := io.Copy(out, file); err != nil {
		http.Error(w, `{"error":"Failed saving backup"}`, http.StatusInternalServerError)
		return
	}

	if err := h.restore.RestoreFromTarGz(tempPath); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"response":{"message":"Backup successfully restored"}}`))
}
