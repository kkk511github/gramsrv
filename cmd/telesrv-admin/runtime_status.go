package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	runtimeStateHealthy     = "healthy"
	runtimeStateDegraded    = "degraded"
	runtimeStateUnavailable = "unavailable"
	runtimeStateDisabled    = "disabled"
)

type runtimeStatusResponse struct {
	CheckedAt time.Time                       `json:"checked_at"`
	Overall   string                          `json:"overall"`
	Services  map[string]runtimeServiceStatus `json:"services"`
}

type runtimeServiceStatus struct {
	State             string   `json:"state"`
	LatencyMS         int64    `json:"latency_ms,omitempty"`
	Endpoint          string   `json:"endpoint,omitempty"`
	PoolAcquired      int32    `json:"pool_acquired,omitempty"`
	PoolTotal         int32    `json:"pool_total,omitempty"`
	Enabled           bool     `json:"enabled,omitempty"`
	Providers         []string `json:"providers,omitempty"`
	RegisteredDevices int64    `json:"registered_devices,omitempty"`
	Pending           int64    `json:"pending,omitempty"`
	Retrying          int64    `json:"retrying,omitempty"`
	Backend           string   `json:"backend,omitempty"`
	Writable          bool     `json:"writable,omitempty"`
	ObjectCount       int64    `json:"object_count,omitempty"`
	TotalBytes        int64    `json:"total_bytes,omitempty"`
}

type runtimePushStats struct {
	RegisteredDevices int64
	Pending           int64
	Retrying          int64
}

type runtimeMediaStats struct {
	ObjectCount int64
	TotalBytes  int64
}

func (s *server) handleRuntimeStatusAPI(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.collectRuntimeStatus(r.Context()))
}

func (s *server) collectRuntimeStatus(parent context.Context) runtimeStatusResponse {
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()

	type probe struct {
		id  string
		run func(context.Context) runtimeServiceStatus
	}
	probes := []probe{
		{id: "mtproto", run: func(ctx context.Context) runtimeServiceStatus {
			return probeTCPService(ctx, s.cfg.MTProtoAddr)
		}},
		{id: "admin_api", run: func(ctx context.Context) runtimeServiceStatus {
			return probeHTTPService(ctx, strings.TrimRight(s.cfg.AdminAPIURL, "/")+"/healthz")
		}},
		{id: "postgres", run: func(ctx context.Context) runtimeServiceStatus {
			return s.probePostgres(ctx)
		}},
		{id: "push", run: func(ctx context.Context) runtimeServiceStatus {
			return probePushService(ctx, s.cfg, s.runtimePushStats)
		}},
		{id: "media", run: func(ctx context.Context) runtimeServiceStatus {
			return probeMediaService(ctx, s.cfg.BlobDir, s.runtimeMediaStats)
		}},
	}

	type result struct {
		id     string
		status runtimeServiceStatus
	}
	results := make(chan result, len(probes))
	for _, item := range probes {
		item := item
		go func() {
			results <- result{id: item.id, status: item.run(ctx)}
		}()
	}

	services := make(map[string]runtimeServiceStatus, len(probes))
	for range probes {
		item := <-results
		services[item.id] = item.status
	}

	overall := runtimeStateHealthy
	for _, item := range services {
		if item.State == runtimeStateDegraded || item.State == runtimeStateUnavailable {
			overall = runtimeStateDegraded
			break
		}
	}
	return runtimeStatusResponse{CheckedAt: time.Now().UTC(), Overall: overall, Services: services}
}

func probeTCPService(ctx context.Context, listenAddr string) runtimeServiceStatus {
	endpoint := localProbeAddress(listenAddr)
	status := runtimeServiceStatus{State: runtimeStateUnavailable, Endpoint: endpoint}
	if endpoint == "" {
		return status
	}
	started := time.Now()
	conn, err := (&net.Dialer{Timeout: 1500 * time.Millisecond}).DialContext(ctx, "tcp", endpoint)
	status.LatencyMS = elapsedMilliseconds(started)
	if err != nil {
		return status
	}
	_ = conn.Close()
	status.State = runtimeStateHealthy
	return status
}

func probeHTTPService(ctx context.Context, healthURL string) runtimeServiceStatus {
	status := runtimeServiceStatus{State: runtimeStateUnavailable, Endpoint: healthURL}
	if strings.TrimSpace(healthURL) == "/healthz" {
		return status
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return status
	}
	started := time.Now()
	resp, err := (&http.Client{Timeout: 1500 * time.Millisecond}).Do(req)
	status.LatencyMS = elapsedMilliseconds(started)
	if err != nil {
		return status
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		status.State = runtimeStateHealthy
	}
	return status
}

func (s *server) probePostgres(ctx context.Context) runtimeServiceStatus {
	status := runtimeServiceStatus{State: runtimeStateUnavailable}
	if s.read == nil || s.read.pool == nil {
		return status
	}
	started := time.Now()
	if err := s.read.pool.Ping(ctx); err != nil {
		status.LatencyMS = elapsedMilliseconds(started)
		return status
	}
	status.LatencyMS = elapsedMilliseconds(started)
	pool := s.read.pool.Stat()
	status.PoolAcquired = pool.AcquiredConns()
	status.PoolTotal = pool.TotalConns()
	status.State = runtimeStateHealthy
	return status
}

func probePushService(ctx context.Context, cfg uiConfig, loadStats func(context.Context) (runtimePushStats, error)) runtimeServiceStatus {
	providers := configuredPushProviders(cfg)
	status := runtimeServiceStatus{
		State:     runtimeStateDisabled,
		Enabled:   cfg.PushEnabled,
		Providers: providers,
	}
	stats, statsErr := loadStats(ctx)
	status.RegisteredDevices = stats.RegisteredDevices
	status.Pending = stats.Pending
	status.Retrying = stats.Retrying
	if !cfg.PushEnabled {
		return status
	}
	if len(providers) == 0 {
		status.State = runtimeStateUnavailable
		return status
	}
	if statsErr != nil || stats.Retrying > 0 {
		status.State = runtimeStateDegraded
		return status
	}
	status.State = runtimeStateHealthy
	return status
}

func configuredPushProviders(cfg uiConfig) []string {
	providers := make([]string, 0, 2)
	if validAPNSConfig(cfg) {
		providers = append(providers, "apns")
	}
	if validFCMConfig(cfg) {
		providers = append(providers, "fcm")
	}
	return providers
}

func validAPNSConfig(cfg uiConfig) bool {
	if strings.TrimSpace(cfg.APNSTopic) == "" || strings.TrimSpace(cfg.APNSTeamID) == "" ||
		strings.TrimSpace(cfg.APNSKeyID) == "" || strings.TrimSpace(cfg.APNSPrivateKeyPath) == "" {
		return false
	}
	raw, err := os.ReadFile(cfg.APNSPrivateKeyPath)
	if err != nil {
		return false
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return false
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return false
	}
	_, ok := parsed.(*ecdsa.PrivateKey)
	return ok
}

func validFCMConfig(cfg uiConfig) bool {
	raw := []byte(strings.TrimSpace(cfg.FCMServiceAccountJSON))
	if len(raw) == 0 {
		return false
	}
	var account struct {
		ProjectID   string `json:"project_id"`
		ClientEmail string `json:"client_email"`
		PrivateKey  string `json:"private_key"`
	}
	if err := json.Unmarshal(raw, &account); err != nil {
		decoded, decodeErr := base64.StdEncoding.DecodeString(string(raw))
		if decodeErr != nil || json.Unmarshal(decoded, &account) != nil {
			return false
		}
	}
	projectID := strings.TrimSpace(cfg.FCMProjectID)
	if projectID == "" {
		projectID = strings.TrimSpace(account.ProjectID)
	}
	block, _ := pem.Decode([]byte(account.PrivateKey))
	if projectID == "" || strings.TrimSpace(account.ClientEmail) == "" || block == nil {
		return false
	}
	if parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		_, ok := parsed.(*rsa.PrivateKey)
		return ok
	}
	_, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	return err == nil
}

func probeMediaService(ctx context.Context, blobDir string, loadStats func(context.Context) (runtimeMediaStats, error)) runtimeServiceStatus {
	status := runtimeServiceStatus{State: runtimeStateUnavailable, Backend: "localfs"}
	info, err := os.Stat(blobDir)
	if err != nil || !info.IsDir() {
		return status
	}
	tmp, err := os.CreateTemp(blobDir, ".admin-health-*")
	if err != nil {
		return status
	}
	tmpPath := tmp.Name()
	if _, err = tmp.Write([]byte("ok")); err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	removeErr := os.Remove(tmpPath)
	if err != nil || closeErr != nil || removeErr != nil {
		return status
	}
	status.Writable = true
	stats, statsErr := loadStats(ctx)
	status.ObjectCount = stats.ObjectCount
	status.TotalBytes = stats.TotalBytes
	if statsErr != nil {
		status.State = runtimeStateDegraded
		return status
	}
	status.State = runtimeStateHealthy
	return status
}

func (s *server) runtimePushStats(ctx context.Context) (runtimePushStats, error) {
	var stats runtimePushStats
	if s.read == nil || s.read.pool == nil {
		return stats, context.Canceled
	}
	err := s.read.pool.QueryRow(ctx, `
SELECT
    (SELECT COUNT(*) FROM push_devices),
    (SELECT COUNT(*) FROM push_notification_outbox),
    (SELECT COUNT(*) FROM push_notification_outbox WHERE attempts > 0)
`).Scan(&stats.RegisteredDevices, &stats.Pending, &stats.Retrying)
	return stats, err
}

func (s *server) runtimeMediaStats(ctx context.Context) (runtimeMediaStats, error) {
	var stats runtimeMediaStats
	if s.read == nil || s.read.pool == nil {
		return stats, context.Canceled
	}
	err := s.read.pool.QueryRow(ctx, `
SELECT COUNT(*), COALESCE(SUM(size), 0)
FROM file_blobs
WHERE backend = 'localfs'
`).Scan(&stats.ObjectCount, &stats.TotalBytes)
	return stats, err
}

func localProbeAddress(listenAddr string) string {
	host, port, err := net.SplitHostPort(strings.TrimSpace(listenAddr))
	if err != nil || port == "" {
		return ""
	}
	switch strings.TrimSpace(host) {
	case "", "0.0.0.0", "::":
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

func elapsedMilliseconds(started time.Time) int64 {
	value := time.Since(started).Milliseconds()
	if value < 1 {
		return 1
	}
	return value
}
