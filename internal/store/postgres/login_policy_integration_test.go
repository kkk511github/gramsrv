package postgres

import (
	"context"
	"crypto/sha256"
	"testing"
	"time"

	appauth "telesrv/internal/app/auth"
	"telesrv/internal/domain"
	"telesrv/internal/store"
	"telesrv/internal/store/memory"
)

func TestLoginPolicyPostgres(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	users, auths := NewUserStore(pool), NewAuthorizationStore(pool)
	t.Cleanup(func() { _ = users.SaveLoginPolicy(ctx, store.DefaultLoginPolicy()) })
	policy := store.LoginPolicy{FutureAuthEnabled: true, FutureAuthDays: 90}
	if err := users.SaveLoginPolicy(ctx, policy); err != nil {
		t.Fatal(err)
	}
	u := futureTestUser(t, pool)
	key := futureTestKey(t, pool)
	next := futureTestKey(t, pool)
	if err := auths.Bind(ctx, domain.Authorization{AuthKeyID: key, UserID: u.ID}); err != nil {
		t.Fatal(err)
	}
	svc := appauth.NewService(users, auths, memory.NewCodeStore(), nil, nil, "12345", appauth.WithFutureAuthTokens(auths))
	token, err := svc.IssueFutureAuthToken(ctx, key)
	if err != nil || len(token) != 32 {
		t.Fatalf("issue %d %v", len(token), err)
	}
	digest := sha256.Sum256(token)
	var expires, created time.Time
	readExpiry := func() {
		t.Helper()
		if err := pool.QueryRow(ctx, `SELECT created_at,expires_at FROM future_auth_tokens WHERE token_hash=$1`, digest[:]).Scan(&created, &expires); err != nil {
			t.Fatal(err)
		}
	}
	readExpiry()
	if expires.Sub(created) < 89*24*time.Hour {
		t.Fatal("configured 90 days not applied")
	}
	policy.FutureAuthDays = 7
	if err := users.SaveLoginPolicy(ctx, policy); err != nil {
		t.Fatal(err)
	}
	readExpiry()
	shortened := expires
	if expires.Sub(created) > 7*24*time.Hour {
		t.Fatal("existing token not shortened")
	}
	policy.FutureAuthDays = 180
	if err := users.SaveLoginPolicy(ctx, policy); err != nil {
		t.Fatal(err)
	}
	readExpiry()
	if !expires.Equal(shortened) {
		t.Fatal("extending policy resurrected/extended old token")
	}
	policy.FutureAuthEnabled = false
	if err := users.SaveLoginPolicy(ctx, policy); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := svc.TryFutureAuthToken(ctx, domain.Authorization{AuthKeyID: next}, u.Phone, [][]byte{token}); err != nil || ok {
		t.Fatal("disabled policy accepted token", err)
	}
	if token, err := svc.LogOutWithFutureToken(ctx, key); err != nil || len(token) != 0 {
		t.Fatal("disabled logout minted token", err)
	}
	if _, ok, err := auths.ByAuthKey(ctx, key); err != nil || ok {
		t.Fatal("disabled policy prevented logout", err)
	}
	policy.FutureAuthEnabled = true
	if err := users.SaveLoginPolicy(ctx, policy); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := svc.TryFutureAuthToken(ctx, domain.Authorization{AuthKeyID: next}, u.Phone, [][]byte{token}); err != nil || ok {
		t.Fatal("re-enabled policy resurrected old token", err)
	}
}
