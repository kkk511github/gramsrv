package clientconfig

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDescriptorIdentityAndPublicOnly(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	name := "SafeLink"
	h, err := New(&key.PublicKey, "192.0.2.20", 2398, 2, func() string { return name })
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, Path, nil))
	var got Descriptor
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(x509.MarshalPKCS1PublicKey(&key.PublicKey))
	if got.ServerID != hex.EncodeToString(hash[:]) || got.Version != 1 || got.DCID != 2 || got.Port != 2398 || got.Host != "192.0.2.20" || got.Name != name {
		t.Fatalf("unexpected descriptor: %+v", got)
	}
	if !strings.Contains(got.RSAPublicKey, "BEGIN RSA PUBLIC KEY") || strings.Contains(w.Body.String(), "PRIVATE") || len(got.RSAFingerprint) != 16 {
		t.Fatal("invalid public key document")
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("discovery must not be cached")
	}
	name = "Updated instance"
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, Path, nil))
	if !strings.Contains(w.Body.String(), name) {
		t.Fatal("server identity changes must be visible")
	}
	for _, method := range []string{http.MethodHead, http.MethodPost} {
		w = httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, Path, nil))
		if method == http.MethodHead && (w.Code != 200 || w.Body.Len() != 0) {
			t.Fatal("HEAD response")
		}
		if method == http.MethodPost && w.Code != 405 {
			t.Fatal("POST must be rejected")
		}
	}
}

func TestRejectInvalidConfiguration(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		host     string
		port, dc int
	}{
		{"0.0.0.0", 2398, 2}, {"::", 2398, 2}, {"224.0.0.1", 2398, 2},
		{"https://example.com", 2398, 2}, {"192.0.2.1", 0, 2}, {"192.0.2.1", 65536, 2}, {"192.0.2.1", 2398, 0},
	} {
		if _, err := New(&key.PublicKey, tc.host, tc.port, tc.dc, nil); err == nil {
			t.Fatalf("accepted %+v", tc)
		}
	}
	if _, err := New(nil, "192.0.2.1", 2398, 2, nil); err == nil {
		t.Fatal("accepted nil key")
	}
}
