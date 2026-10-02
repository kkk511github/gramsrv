// Package clientconfig publishes the public, TLS-authenticated native-client bootstrap.
package clientconfig

import (
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/iamxvbaba/td/exchange"
)

const Path = "/.well-known/safelink-client.json"

type Descriptor struct {
	Version        int    `json:"version"`
	ServerID       string `json:"server_id"`
	Name           string `json:"name"`
	Host           string `json:"host"`
	Port           int    `json:"port"`
	DCID           int    `json:"dc_id"`
	RSAPublicKey   string `json:"rsa_public_key"`
	RSAFingerprint string `json:"rsa_fingerprint"`
}

// The identity is the SHA-256 of canonical PKCS#1 public-key DER, not an address
// or a display name. Moving a deployment must preserve its key and account data.
func New(key *rsa.PublicKey, host string, port, dc int, name func() string) (http.Handler, error) {
	ip := net.ParseIP(strings.TrimSpace(host))
	if ip == nil || ip.IsUnspecified() || ip.IsMulticast() || port < 1 || port > 65535 || dc < 1 || dc > 1000 {
		return nil, fmt.Errorf("invalid client discovery DC endpoint")
	}
	if key == nil || key.N == nil || key.N.BitLen() != 2048 || key.E != 65537 {
		return nil, fmt.Errorf("client discovery requires an RSA-2048 public key with exponent 65537")
	}
	der := x509.MarshalPKCS1PublicKey(key)
	id := sha256.Sum256(der)
	descriptor := Descriptor{
		Version: 1, ServerID: hex.EncodeToString(id[:]), Host: ip.String(), Port: port, DCID: dc,
		RSAPublicKey:   string(pem.EncodeToMemory(&pem.Block{Type: "RSA PUBLIC KEY", Bytes: der})),
		RSAFingerprint: fmt.Sprintf("%016x", uint64((exchange.PublicKey{RSA: key}).Fingerprint())),
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		value := descriptor
		value.Name = "SafeLink"
		if name != nil {
			candidate := strings.TrimSpace(name())
			if candidate != "" && utf8.ValidString(candidate) && utf8.RuneCountInString(candidate) <= 80 && !strings.ContainsAny(candidate, "\r\n\x00") {
				value.Name = candidate
			}
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method != http.MethodHead {
			_ = json.NewEncoder(w).Encode(value)
		}
	}), nil
}
