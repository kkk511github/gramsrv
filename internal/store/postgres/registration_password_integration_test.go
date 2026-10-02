package postgres

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"
	"time"

	"telesrv/internal/domain"
	"telesrv/internal/store"
)

func TestRegistrationPasswordPostgres(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	users := NewUserStore(pool)
	policy := store.DefaultLoginPolicy()
	t.Cleanup(func() { _ = users.SaveLoginPolicy(ctx, store.DefaultLoginPolicy()) })
	if err := users.SetRegistrationInviteRequired(ctx, false); err != nil {
		t.Fatal(err)
	}
	create := func() domain.User {
		t.Helper()
		u, err := users.CreateRegistration(ctx, domain.User{Phone: fmt.Sprintf("159%08d", time.Now().UnixNano()%100000000), FirstName: "Password", AccessHash: time.Now().UnixNano()}, "")
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	check := func(u domain.User, want bool) {
		t.Helper()
		got, err := users.RegistrationPasswordPending(ctx, u.ID)
		if err != nil || got != want {
			t.Fatalf("pending %v want %v: %v", got, want, err)
		}
	}
	if err := users.SaveLoginPolicy(ctx, policy); err != nil {
		t.Fatal(err)
	}
	old := create()
	policy.RegistrationPasswordRequired = true
	if err := users.SaveLoginPolicy(ctx, policy); err != nil {
		t.Fatal(err)
	}
	check(old, false)
	u := create()
	check(u, true)
	key := futureTestKey(t, pool)
	auths := NewAuthorizationStore(pool)
	if err := auths.Bind(ctx, domain.Authorization{AuthKeyID: key, UserID: u.ID}); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("registration-test"))
	if ok, err := auths.IssueFutureAuthToken(ctx, key, digest, time.Now().Add(time.Hour), false); err != nil || ok {
		t.Fatal("pending minted token", err)
	}
	passwords := NewPasswordStore(pool)
	settings := domain.PasswordSettings{HasPassword: true, EmailUnconfirmedPattern: "t***@example.com"}
	if err := passwords.Save(ctx, u.ID, settings); err != nil {
		t.Fatal(err)
	}
	check(u, true)
	settings.EmailUnconfirmedPattern = ""
	if err := passwords.Save(ctx, u.ID, settings); err != nil {
		t.Fatal(err)
	}
	check(u, false)
	if ok, err := auths.IssueFutureAuthToken(ctx, key, digest, time.Now().Add(time.Hour), true); err != nil || !ok {
		t.Fatal("completed logout missing token", err)
	}
	settings.HasPassword = false
	if err := passwords.Save(ctx, u.ID, settings); err != nil {
		t.Fatal(err)
	}
	check(u, false)
	uncompleted := create()
	policy.RegistrationPasswordRequired = false
	if err := users.SaveLoginPolicy(ctx, policy); err != nil {
		t.Fatal(err)
	}
	check(uncompleted, false)
	policy.RegistrationPasswordRequired = true
	if err := users.SaveLoginPolicy(ctx, policy); err != nil {
		t.Fatal(err)
	}
	check(uncompleted, false)
}
