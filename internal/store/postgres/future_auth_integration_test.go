package postgres

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	appauth "telesrv/internal/app/auth"
	"telesrv/internal/domain"
	"telesrv/internal/store"
	"telesrv/internal/store/memory"
)

func futureTestKey(t *testing.T, pool *pgxpool.Pool) [8]byte {
	t.Helper()
	var key store.AuthKeyData
	if _, err := rand.Read(key.ID[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := rand.Read(key.Value[:]); err != nil {
		t.Fatal(err)
	}
	if err := NewAuthKeyStore(pool).Save(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM auth_keys WHERE auth_key_id=$1`, authKeyIDToInt64(key.ID))
	})
	return key.ID
}

func futureTestUser(t *testing.T, pool *pgxpool.Pool) domain.User {
	t.Helper()
	u, err := NewUserStore(pool).Create(context.Background(), domain.User{Phone: fmt.Sprintf("155%08d", time.Now().UnixNano()%100000000), FirstName: "Future", AccessHash: time.Now().UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM registration_invite_uses WHERE user_id=$1`, u.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, u.ID)
	})
	return u
}

func TestFutureAuthTokenPostgres(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	users := NewUserStore(pool)
	auths := NewAuthorizationStore(pool)
	passwords := NewPasswordStore(pool)
	u := futureTestUser(t, pool)
	old := futureTestKey(t, pool)
	next := futureTestKey(t, pool)
	if err := passwords.Save(ctx, u.ID, domain.PasswordSettings{HasPassword: true, SRPVerifier: []byte{1}}); err != nil {
		t.Fatal(err)
	}
	if err := auths.Bind(ctx, domain.Authorization{AuthKeyID: old, UserID: u.ID}); err != nil {
		t.Fatal(err)
	}
	svc := appauth.NewService(users, auths, memory.NewCodeStore(), NewAuthKeyStore(pool), nil, "12345", appauth.WithPasswords(passwords), appauth.WithFutureAuthTokens(auths))
	loginToken, err := svc.IssueFutureAuthToken(ctx, old)
	if err != nil || len(loginToken) != 32 {
		t.Fatalf("issue %d %v", len(loginToken), err)
	}
	token, err := svc.LogOutWithFutureToken(ctx, old)
	if err != nil || len(token) != 32 {
		t.Fatalf("logout %d %v", len(token), err)
	}
	if _, ok, _ := auths.ByAuthKey(ctx, old); ok {
		t.Fatal("logout kept authorization")
	}
	if _, ok, err := svc.TryFutureAuthToken(ctx, domain.Authorization{AuthKeyID: next}, u.Phone, [][]byte{loginToken}); err != nil || ok {
		t.Fatalf("old token survived logout: %v %v", ok, err)
	}
	if _, ok, err := svc.TryFutureAuthToken(ctx, domain.Authorization{AuthKeyID: next}, "15500009999", [][]byte{token}); err != nil || ok {
		t.Fatal("wrong phone accepted")
	}
	got, ok, err := svc.TryFutureAuthToken(ctx, domain.Authorization{AuthKeyID: next}, u.Phone, [][]byte{token})
	if !ok || got.ID != u.ID || !errors.Is(err, domain.ErrSessionPasswordNeeded) {
		t.Fatalf("password admission %v %v", ok, err)
	}
	if _, ok, _ := svc.UserID(ctx, next); ok {
		t.Fatal("pending password authorized business RPC")
	}
	if pending, ok, _ := svc.PendingPasswordUserID(ctx, next); !ok || pending != u.ID {
		t.Fatal("missing password state")
	}
	if issued, err := svc.IssueFutureAuthToken(ctx, next); err != nil || len(issued) != 0 {
		t.Fatal("pending session minted token")
	}
	third := futureTestKey(t, pool)
	if _, ok, err := svc.TryFutureAuthToken(ctx, domain.Authorization{AuthKeyID: third}, u.Phone, [][]byte{token}); err != nil || ok {
		t.Fatal("token replay accepted")
	}
	if err := svc.CompletePasswordSignIn(ctx, next, u.ID); err != nil {
		t.Fatal(err)
	}
	token, err = svc.IssueFutureAuthToken(ctx, next)
	if err != nil || len(token) != 32 {
		t.Fatal(err)
	}
	if err := passwords.Save(ctx, u.ID, domain.PasswordSettings{HasPassword: true, SRPVerifier: []byte{2}}); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := svc.TryFutureAuthToken(ctx, domain.Authorization{AuthKeyID: third}, u.Phone, [][]byte{token}); err != nil || ok {
		t.Fatal("password change kept token")
	}
}

func TestFutureAuthTokenReplayExpiryRevocationPostgres(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	auths := NewAuthorizationStore(pool)
	u := futureTestUser(t, pool)
	key := futureTestKey(t, pool)
	if err := auths.Bind(ctx, domain.Authorization{AuthKeyID: key, UserID: u.ID}); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("test-expired"))
	if _, err := auths.IssueFutureAuthToken(ctx, key, digest, time.Now().Add(-time.Second), false); err != nil {
		t.Fatal(err)
	}
	newKey := futureTestKey(t, pool)
	if _, ok, err := auths.RedeemFutureAuthToken(ctx, u.Phone, [][32]byte{digest}, domain.Authorization{AuthKeyID: newKey}); err != nil || ok {
		t.Fatal("expired token accepted")
	}
	digest = sha256.Sum256([]byte("test-concurrent"))
	if _, err := auths.IssueFutureAuthToken(ctx, key, digest, time.Now().Add(time.Hour), false); err != nil {
		t.Fatal(err)
	}
	keys := make([][8]byte, 8)
	for i := range keys {
		keys[i] = futureTestKey(t, pool)
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for _, k := range keys {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, ok, err := auths.RedeemFutureAuthToken(ctx, u.Phone, [][32]byte{digest}, domain.Authorization{AuthKeyID: k})
			if err != nil {
				t.Error(err)
			}
			if ok {
				successes.Add(1)
				if a.PasswordPending {
					t.Error("unexpected 2FA")
				}
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("redemptions=%d", successes.Load())
	}
	digest = sha256.Sum256([]byte("test-revoke"))
	if _, err := auths.IssueFutureAuthToken(ctx, key, digest, time.Now().Add(time.Hour), true); err != nil {
		t.Fatal(err)
	}
	if _, err := auths.RevokeByUserExcept(ctx, u.ID, newKey); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := auths.RedeemFutureAuthToken(ctx, u.Phone, [][32]byte{digest}, domain.Authorization{AuthKeyID: newKey}); err != nil || ok {
		t.Fatal("terminate others kept logged-out token")
	}
}
