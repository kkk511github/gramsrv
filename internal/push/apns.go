package push

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"telesrv/internal/domain"
)

type apnsSender struct {
	topic  string
	teamID string
	keyID  string
	key    *ecdsa.PrivateKey
	client *http.Client

	mu        sync.Mutex
	jwt       string
	jwtExpiry time.Time
}

func newAPNSSender(cfg Config) (*apnsSender, error) {
	if strings.TrimSpace(cfg.APNSTopic) == "" || strings.TrimSpace(cfg.APNSTeamID) == "" || strings.TrimSpace(cfg.APNSKeyID) == "" || strings.TrimSpace(cfg.APNSPrivateKeyPath) == "" {
		return nil, fmt.Errorf("incomplete APNs configuration")
	}
	raw, err := os.ReadFile(cfg.APNSPrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("read APNs private key: %w", err)
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("decode APNs private key PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse APNs private key: %w", err)
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("APNs private key is not ECDSA")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ForceAttemptHTTP2 = true
	return &apnsSender{
		topic:  strings.TrimSpace(cfg.APNSTopic),
		teamID: strings.TrimSpace(cfg.APNSTeamID),
		keyID:  strings.TrimSpace(cfg.APNSKeyID),
		key:    key,
		client: &http.Client{Transport: transport},
	}, nil
}

func (s *apnsSender) Send(ctx context.Context, device domain.PushDevice, job domain.PushNotificationJob) (bool, error) {
	payload, err := json.Marshal(map[string]any{
		"aps": map[string]any{
			"alert": map[string]string{"title": job.Title, "body": job.Body},
			"sound": "default",
		},
		"safelink": "message",
	})
	if err != nil {
		return false, err
	}
	host := "https://api.push.apple.com"
	if device.AppSandbox {
		host = "https://api.sandbox.push.apple.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, host+"/3/device/"+strings.TrimSpace(device.Token), bytes.NewReader(payload))
	if err != nil {
		return false, err
	}
	token, err := s.authToken(time.Now())
	if err != nil {
		return false, err
	}
	req.Header.Set("authorization", "bearer "+token)
	req.Header.Set("apns-topic", s.topic)
	req.Header.Set("apns-push-type", "alert")
	req.Header.Set("apns-priority", "10")
	req.Header.Set("apns-expiration", fmt.Sprintf("%d", time.Now().Add(time.Hour).Unix()))
	req.Header.Set("apns-collapse-id", fmt.Sprintf("%d-%d", job.TargetUserID, job.Pts))
	req.Header.Set("content-type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("send APNs request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode == http.StatusOK {
		return false, nil
	}
	var failure struct {
		Reason string `json:"reason"`
	}
	_ = json.Unmarshal(body, &failure)
	switch failure.Reason {
	case "BadDeviceToken", "DeviceTokenNotForTopic", "Unregistered":
		return true, nil
	default:
		return false, fmt.Errorf("APNs status %d: %s", resp.StatusCode, strings.TrimSpace(failure.Reason))
	}
}

func (s *apnsSender) authToken(now time.Time) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.jwt != "" && now.Before(s.jwtExpiry) {
		return s.jwt, nil
	}
	header, _ := json.Marshal(map[string]string{"alg": "ES256", "kid": s.keyID})
	claims, _ := json.Marshal(map[string]any{"iss": s.teamID, "iat": now.Unix()})
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(unsigned))
	r, ss, err := ecdsa.Sign(rand.Reader, s.key, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign APNs JWT: %w", err)
	}
	signature := append(paddedBigInt(r, 32), paddedBigInt(ss, 32)...)
	s.jwt = unsigned + "." + base64.RawURLEncoding.EncodeToString(signature)
	s.jwtExpiry = now.Add(50 * time.Minute)
	return s.jwt, nil
}

func paddedBigInt(value *big.Int, size int) []byte {
	out := make([]byte, size)
	raw := value.Bytes()
	copy(out[size-len(raw):], raw)
	return out
}
