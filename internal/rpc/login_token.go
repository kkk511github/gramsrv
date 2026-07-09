package rpc

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"telesrv/internal/domain"
)

const (
	loginTokenTTL        = 30 * time.Second
	loginTokenBytes      = 32
	loginTokenMaxRecords = 2048
)

type loginTokenTarget struct {
	rawAuthKeyID [8]byte
	authKeyID    [8]byte
	sessionID    int64
}

type loginTokenExport struct {
	token        []byte
	expires      time.Time
	accepted     bool
	acceptedAuth domain.Authorization
}

type loginTokenAcceptStart struct {
	target loginTokenTarget
	authz  domain.Authorization
}

type loginTokenRecord struct {
	token     []byte
	expires   time.Time
	target    loginTokenTarget
	authz     domain.Authorization
	exceptIDs map[int64]struct{}

	accepting      bool
	accepted       bool
	acceptedUserID int64
	acceptedAuth   domain.Authorization
	acceptedAt     time.Time
}

type loginTokenRegistry struct {
	mu       sync.Mutex
	byToken  map[string]*loginTokenRecord
	byTarget map[loginTokenTarget]*loginTokenRecord
}

func newLoginTokenRegistry() *loginTokenRegistry {
	return &loginTokenRegistry{
		byToken:  make(map[string]*loginTokenRecord),
		byTarget: make(map[loginTokenTarget]*loginTokenRecord),
	}
}

func (r *loginTokenRegistry) export(now time.Time, target loginTokenTarget, authz domain.Authorization, exceptIDs []int64) (loginTokenExport, error) {
	if r == nil {
		return loginTokenExport{}, fmt.Errorf("login token registry is nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cleanupExpiredLocked(now)

	if rec := r.byTarget[target]; rec != nil && rec.expires.After(now) {
		if rec.accepted {
			return loginTokenExport{accepted: true, acceptedAuth: rec.acceptedAuth}, nil
		}
		return loginTokenExport{token: append([]byte(nil), rec.token...), expires: rec.expires}, nil
	}
	if len(r.byToken) >= loginTokenMaxRecords {
		r.evictOldestLocked()
	}

	token, err := randomLoginTokenLocked(r.byToken)
	if err != nil {
		return loginTokenExport{}, err
	}
	rec := &loginTokenRecord{
		token:     token,
		expires:   now.Add(loginTokenTTL),
		target:    target,
		authz:     authz,
		exceptIDs: loginTokenExceptSet(exceptIDs),
	}
	r.byToken[string(token)] = rec
	r.byTarget[target] = rec
	return loginTokenExport{token: append([]byte(nil), token...), expires: rec.expires}, nil
}

func (r *loginTokenRegistry) lookup(now time.Time, token []byte) (loginTokenExport, error) {
	if r == nil || len(token) == 0 {
		return loginTokenExport{}, authTokenInvalidErr()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, err := r.findLocked(now, token)
	if err != nil {
		return loginTokenExport{}, err
	}
	if !rec.expires.After(now) {
		r.deleteLocked(rec)
		return loginTokenExport{}, authTokenExpiredErr()
	}
	r.cleanupExpiredLocked(now)
	if rec.accepted {
		return loginTokenExport{accepted: true, acceptedAuth: rec.acceptedAuth}, nil
	}
	return loginTokenExport{token: append([]byte(nil), rec.token...), expires: rec.expires}, nil
}

func (r *loginTokenRegistry) beginAccept(now time.Time, token []byte, userID int64) (loginTokenAcceptStart, error) {
	if r == nil || len(token) == 0 {
		return loginTokenAcceptStart{}, authTokenInvalidErr()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, err := r.findLocked(now, token)
	if err != nil {
		return loginTokenAcceptStart{}, err
	}
	if !rec.expires.After(now) {
		r.deleteLocked(rec)
		return loginTokenAcceptStart{}, authTokenExpiredErr()
	}
	r.cleanupExpiredLocked(now)
	if rec.accepted || rec.accepting {
		return loginTokenAcceptStart{}, authTokenAlreadyAcceptedErr()
	}
	if _, denied := rec.exceptIDs[userID]; denied {
		return loginTokenAcceptStart{}, authTokenAlreadyAcceptedErr()
	}
	rec.accepting = true
	return loginTokenAcceptStart{target: rec.target, authz: rec.authz}, nil
}

func (r *loginTokenRegistry) findLocked(now time.Time, token []byte) (*loginTokenRecord, error) {
	for _, candidate := range loginTokenCandidates(token) {
		if rec := r.byToken[string(candidate)]; rec != nil {
			return rec, nil
		}
	}
	r.cleanupExpiredLocked(now)
	return nil, authTokenInvalidErr()
}

func (r *loginTokenRegistry) finishAccept(now time.Time, token []byte, userID int64, acceptedAuth domain.Authorization) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, _ := r.findLocked(now, token)
	if rec == nil {
		return
	}
	rec.accepting = false
	rec.accepted = true
	rec.acceptedUserID = userID
	rec.acceptedAuth = acceptedAuth
	rec.acceptedAt = now
	r.byTarget[rec.target] = rec
}

func (r *loginTokenRegistry) failAccept(token []byte) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if rec := r.byToken[string(token)]; rec != nil {
		rec.accepting = false
	}
}

func (r *loginTokenRegistry) cleanupExpiredLocked(now time.Time) {
	for _, rec := range r.byToken {
		if !rec.expires.After(now) {
			r.deleteLocked(rec)
		}
	}
}

func (r *loginTokenRegistry) deleteLocked(rec *loginTokenRecord) {
	delete(r.byToken, string(rec.token))
	if r.byTarget[rec.target] == rec {
		delete(r.byTarget, rec.target)
	}
}

func (r *loginTokenRegistry) evictOldestLocked() {
	var oldest *loginTokenRecord
	for _, rec := range r.byToken {
		if oldest == nil || rec.expires.Before(oldest.expires) {
			oldest = rec
		}
	}
	if oldest != nil {
		r.deleteLocked(oldest)
	}
}

func randomLoginTokenLocked(existing map[string]*loginTokenRecord) ([]byte, error) {
	for i := 0; i < 4; i++ {
		token := make([]byte, loginTokenBytes)
		if _, err := rand.Read(token); err != nil {
			return nil, fmt.Errorf("generate login token: %w", err)
		}
		if existing[string(token)] == nil {
			return token, nil
		}
	}
	return nil, fmt.Errorf("generate login token: collision")
}

func loginTokenExceptSet(ids []int64) map[int64]struct{} {
	if len(ids) == 0 {
		return nil
	}
	out := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id != 0 {
			out[id] = struct{}{}
		}
	}
	return out
}

func loginTokenCandidates(token []byte) [][]byte {
	seen := map[string]struct{}{}
	out := make([][]byte, 0, 4)
	add := func(candidate []byte) {
		if len(candidate) == 0 {
			return
		}
		key := string(candidate)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, append([]byte(nil), candidate...))
	}
	add(token)

	text := strings.TrimSpace(string(token))
	if text == "" {
		return out
	}
	addEncodedLoginTokenCandidates(text, add)
	if u, err := url.Parse(text); err == nil {
		if v := u.Query().Get("token"); v != "" {
			addEncodedLoginTokenCandidates(v, add)
		}
	}
	if i := strings.Index(text, "token="); i >= 0 {
		values, err := url.ParseQuery(strings.TrimLeft(text[i:], "?&"))
		if err == nil {
			if v := values.Get("token"); v != "" {
				addEncodedLoginTokenCandidates(v, add)
			}
		}
	}
	return out
}

func addEncodedLoginTokenCandidates(raw string, add func([]byte)) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return
	}
	for _, value := range []string{raw, strings.ReplaceAll(raw, " ", "+")} {
		for _, encoding := range []*base64.Encoding{
			base64.RawURLEncoding,
			base64.URLEncoding,
			base64.RawStdEncoding,
			base64.StdEncoding,
		} {
			if decoded, err := encoding.DecodeString(value); err == nil && len(decoded) == loginTokenBytes {
				add(decoded)
			}
		}
	}
}
