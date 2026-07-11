package push

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"telesrv/internal/domain"
)

type fcmServiceAccount struct {
	ProjectID   string `json:"project_id"`
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

type fcmSender struct {
	projectID   string
	clientEmail string
	privateKey  *rsa.PrivateKey
	tokenURI    string
	client      *http.Client

	mu          sync.Mutex
	accessToken string
	tokenExpiry time.Time
}

func newFCMSender(cfg Config) (*fcmSender, error) {
	var account fcmServiceAccount
	rawAccount := []byte(strings.TrimSpace(cfg.FCMServiceAccountJSON))
	if err := json.Unmarshal(rawAccount, &account); err != nil {
		decoded, decodeErr := base64.StdEncoding.DecodeString(string(rawAccount))
		if decodeErr != nil {
			return nil, fmt.Errorf("parse FCM service account JSON: %w", err)
		}
		if err := json.Unmarshal(decoded, &account); err != nil {
			return nil, fmt.Errorf("parse base64 FCM service account JSON: %w", err)
		}
	}
	projectID := strings.TrimSpace(cfg.FCMProjectID)
	if projectID == "" {
		projectID = strings.TrimSpace(account.ProjectID)
	}
	if projectID == "" || account.ClientEmail == "" || account.PrivateKey == "" {
		return nil, fmt.Errorf("incomplete FCM service account configuration")
	}
	block, _ := pem.Decode([]byte(account.PrivateKey))
	if block == nil {
		return nil, fmt.Errorf("decode FCM private key PEM")
	}
	var key *rsa.PrivateKey
	if parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		key, _ = parsed.(*rsa.PrivateKey)
	} else if parsed, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		key = parsed
	}
	if key == nil {
		return nil, fmt.Errorf("parse FCM RSA private key")
	}
	tokenURI := strings.TrimSpace(account.TokenURI)
	if tokenURI == "" {
		tokenURI = "https://oauth2.googleapis.com/token"
	}
	return &fcmSender{
		projectID: projectID, clientEmail: account.ClientEmail, privateKey: key,
		tokenURI: tokenURI, client: &http.Client{},
	}, nil
}

func (s *fcmSender) Send(ctx context.Context, device domain.PushDevice, job domain.PushNotificationJob) (bool, error) {
	accessToken, err := s.oauthToken(ctx, time.Now())
	if err != nil {
		return false, err
	}
	payload, err := json.Marshal(map[string]any{
		"message": map[string]any{
			"token":        device.Token,
			"notification": map[string]string{"title": job.Title, "body": job.Body},
			"android": map[string]any{
				"priority":     "high",
				"collapse_key": fmt.Sprintf("%d-%d", job.TargetUserID, job.Pts),
				"notification": map[string]string{"sound": "default"},
			},
			"data": map[string]string{"kind": "message", "pts": strconv.Itoa(job.Pts)},
		},
	})
	if err != nil {
		return false, err
	}
	endpoint := "https://fcm.googleapis.com/v1/projects/" + url.PathEscape(s.projectID) + "/messages:send"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return false, err
	}
	req.Header.Set("authorization", "Bearer "+accessToken)
	req.Header.Set("content-type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("send FCM request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return false, nil
	}
	text := string(body)
	// A project/endpoint error can also be HTTP 404. Delete only when FCM's
	// response explicitly identifies the registration token as invalid.
	if strings.Contains(text, "UNREGISTERED") || strings.Contains(text, "registration-token-not-registered") {
		return true, nil
	}
	return false, fmt.Errorf("FCM status %d: %s", resp.StatusCode, strings.TrimSpace(text))
}

func (s *fcmSender) oauthToken(ctx context.Context, now time.Time) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.accessToken != "" && now.Before(s.tokenExpiry) {
		return s.accessToken, nil
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claimsJSON, _ := json.Marshal(map[string]any{
		"iss":   s.clientEmail,
		"scope": "https://www.googleapis.com/auth/firebase.messaging",
		"aud":   s.tokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	})
	unsigned := header + "." + base64.RawURLEncoding.EncodeToString(claimsJSON)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, s.privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign FCM JWT: %w", err)
	}
	assertion := unsigned + "." + base64.RawURLEncoding.EncodeToString(signature)
	form := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {assertion},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.tokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("content-type", "application/x-www-form-urlencoded")
	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request FCM access token: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("FCM token status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var tokenResponse struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tokenResponse); err != nil || tokenResponse.AccessToken == "" {
		return "", fmt.Errorf("parse FCM access token response: %w", err)
	}
	expiresIn := tokenResponse.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	s.accessToken = tokenResponse.AccessToken
	s.tokenExpiry = now.Add(time.Duration(expiresIn)*time.Second - 5*time.Minute)
	return s.accessToken, nil
}
