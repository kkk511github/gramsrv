package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestProbeTCPServiceUsesRealListener(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	status := probeTCPService(context.Background(), listener.Addr().String())
	if status.State != runtimeStateHealthy || status.LatencyMS <= 0 {
		t.Fatalf("status = %+v, want healthy listener", status)
	}
}

func TestProbeHTTPServiceChecksStatusCode(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	status := probeHTTPService(context.Background(), upstream.URL+"/healthz")
	if status.State != runtimeStateHealthy || status.LatencyMS <= 0 {
		t.Fatalf("status = %+v, want healthy HTTP service", status)
	}
}

func TestProbeMediaServiceVerifiesWriteAndReportsDatabaseStats(t *testing.T) {
	dir := t.TempDir()
	status := probeMediaService(context.Background(), dir, func(context.Context) (runtimeMediaStats, error) {
		return runtimeMediaStats{ObjectCount: 17, TotalBytes: 4096}, nil
	})
	if status.State != runtimeStateHealthy || !status.Writable || status.ObjectCount != 17 || status.TotalBytes != 4096 {
		t.Fatalf("status = %+v", status)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("health probe left files behind: %+v", entries)
	}
}

func TestProbePushServiceReportsConfiguredAPNSAndQueue(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(t.TempDir(), "AuthKey.p8")
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: raw}), 0o600); err != nil {
		t.Fatal(err)
	}

	status := probePushService(context.Background(), uiConfig{
		PushEnabled:        true,
		APNSTopic:          "com.hsgram.app",
		APNSTeamID:         "TEAM",
		APNSKeyID:          "KEY",
		APNSPrivateKeyPath: keyPath,
	}, func(context.Context) (runtimePushStats, error) {
		return runtimePushStats{RegisteredDevices: 3, Pending: 1}, nil
	})
	if status.State != runtimeStateHealthy || len(status.Providers) != 1 || status.Providers[0] != "apns" ||
		status.RegisteredDevices != 3 || status.Pending != 1 {
		t.Fatalf("status = %+v", status)
	}
}

func TestLocalProbeAddressRewritesWildcardListener(t *testing.T) {
	if got, want := localProbeAddress("0.0.0.0:2398"), "127.0.0.1:2398"; got != want {
		t.Fatalf("localProbeAddress = %q, want %q", got, want)
	}
}
