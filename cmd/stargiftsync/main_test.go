package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iamxvbaba/td/telegram/auth/qrlogin"
)

func TestSyncOneGiftValidatesThenImports(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/v1/gifts/import" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("request = %s auth=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		var metadata importMetadata
		if err := json.Unmarshal([]byte(r.FormValue("metadata")), &metadata); err != nil {
			t.Fatal(err)
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		data, _ := io.ReadAll(file)
		if header.Filename != "telegram-star-gift-7001.tgs" || string(data) != "tgs" {
			t.Fatalf("file = %q %q", header.Filename, data)
		}
		if requests == 1 && !metadata.DryRun {
			t.Fatal("first request must validate")
		}
		if requests == 2 && metadata.DryRun {
			t.Fatal("second request must import")
		}
		_ = json.NewEncoder(w).Encode(commandResult{Status: "completed", Details: map[string]any{"gift_id": 9001}})
	}))
	defer server.Close()

	gift := sourceGift{ID: 7001, Title: "Cake", Stars: 50, ConvertStars: 25, TGS: []byte("tgs"), SHA256: "0123456789abcdef"}
	id, err := syncOneGift(context.Background(), server.Client(), syncConfig{
		AdminURL: server.URL, AdminToken: "secret", Confirm: true,
	}, gift, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if id != 9001 || requests != 2 {
		t.Fatalf("id=%d requests=%d", id, requests)
	}
}

func TestManifestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "manifest.json")
	want := syncManifest{
		UpdatedAt: time.Unix(1_700_000_000, 0).UTC(),
		Gifts: map[string]manifestGift{
			"7001": {SafeLinkGiftID: 9001, Title: "Cake", SHA256: "abc", Stars: 50, ConvertStars: 25},
		},
	}
	if err := saveManifest(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := loadManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Gifts["7001"].SafeLinkGiftID != 9001 || !got.UpdatedAt.Equal(want.UpdatedAt) {
		t.Fatalf("manifest = %+v", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("manifest mode = %o", info.Mode().Perm())
	}
}

func TestPhoneLoginStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth", "pending.json")
	want := phoneLoginState{Phone: "+447000000000", PhoneCodeHash: "secret-hash"}
	if err := savePhoneLoginState(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := loadPhoneLoginState(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("login state = %+v", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("login state mode = %o", info.Mode().Perm())
	}
}

func TestResultGiftIDUsesJSONNumber(t *testing.T) {
	id, err := resultGiftID(commandResult{Details: map[string]any{"gift_id": json.Number("9000000000000001")}})
	if err != nil || id != 9000000000000001 {
		t.Fatalf("id=%d err=%v", id, err)
	}
}

func TestWriteQRUsesPrivatePNG(t *testing.T) {
	path := filepath.Join(t.TempDir(), "qr", "login.png")
	if err := writeQR(path, qrlogin.NewToken([]byte("one-time-token"), int(time.Now().Add(time.Minute).Unix()))); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 8 || string(raw[:8]) != "\x89PNG\r\n\x1a\n" {
		t.Fatalf("QR is not PNG: %x", raw)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("QR mode = %o", info.Mode().Perm())
	}
}
