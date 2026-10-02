package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	appauth "telesrv/internal/app/auth"
	"telesrv/internal/domain"
	"telesrv/internal/store"
	"telesrv/internal/store/memory"
)

func TestRegistrationInviteSharedAndGeneratedPostgres(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	users := NewUserStore(pool)
	t.Cleanup(func() { _ = users.SetRegistrationInviteRequired(ctx, false) })
	if err := users.SetRegistrationInviteRequired(ctx, true); err != nil {
		t.Fatal(err)
	}
	code := fmt.Sprintf("SHARED-%d", time.Now().UnixNano())
	expires := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Microsecond)
	codes, err := users.CreateRegistrationInvites(ctx, 1, 2, expires, strings.ToLower(code))
	if err != nil || len(codes) != 1 || codes[0] != code {
		t.Fatalf("fixed code: %v %v", codes, err)
	}
	if _, err := users.CreateRegistrationInvites(ctx, 1, 2, expires, code); err == nil {
		t.Fatal("duplicate fixed code accepted")
	}
	for i := 0; i < 2; i++ {
		_, err := users.CreateRegistration(ctx, domain.User{Phone: fmt.Sprintf("158%08d", time.Now().UnixNano()%100000000), FirstName: "Shared", AccessHash: time.Now().UnixNano()}, strings.ToLower(code))
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := users.ValidateRegistrationInvite(ctx, code); !errors.Is(err, store.ErrRegistrationInviteInvalid) {
		t.Fatalf("shared quota: %v", err)
	}
	_, items, err := users.RegistrationInvites(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 || items[0].UsedCount != 2 || !items[0].ExpiresAt.Equal(expires) {
		t.Fatalf("stored limits: %+v", items)
	}
	codes, err = users.CreateRegistrationInvites(ctx, 10, 1, expires, "")
	if err != nil {
		t.Fatal(err)
	}
	unique := make(map[string]bool)
	for _, code := range codes {
		unique[code] = true
	}
	if len(unique) != 10 {
		t.Fatalf("generated unique codes: %d", len(unique))
	}
}

func TestRegistrationInviteOptions(t *testing.T) {
	expires := time.Now().Add(time.Hour)
	for _, tc := range []struct {
		count, uses int
		code        string
		valid       bool
	}{
		{1, 100, "SafeLink-2026", true}, {100, 1, "", true},
		{2, 100, "FIXED-CODE", false}, {1, 0, "", false},
		{101, 1, "", false}, {1, 10001, "", false}, {1, 1, "short", false},
		{1, 1, "bad code", false},
	} {
		err := ValidateRegistrationInviteOptions(tc.count, tc.uses, expires, tc.code)
		if (err == nil) != tc.valid {
			t.Errorf("%+v: %v", tc, err)
		}
	}
	if ValidateRegistrationInviteOptions(1, 1, time.Now().Add(-time.Second), "") == nil {
		t.Fatal("expired date accepted")
	}
}

func TestRegistrationInvitesPostgres(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	users := NewUserStore(pool)
	t.Cleanup(func() { _ = users.SetRegistrationInviteRequired(ctx, false) })
	if err := users.SetRegistrationInviteRequired(ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := users.ValidateRegistrationInvite(ctx, ""); !errors.Is(err, store.ErrRegistrationInviteRequired) {
		t.Fatal(err)
	}
	if err := users.ValidateRegistrationInvite(ctx, "BAD"); !errors.Is(err, store.ErrRegistrationInviteInvalid) {
		t.Fatal(err)
	}
	codes, err := users.GenerateRegistrationInvites(ctx, 1, 1, 30)
	if err != nil {
		t.Fatal(err)
	}
	var count atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := users.CreateRegistration(ctx, domain.User{Phone: fmt.Sprintf("156%d%d", time.Now().UnixNano(), i), FirstName: "Invite", AccessHash: time.Now().UnixNano()}, codes[0])
			if err == nil {
				count.Add(1)
			} else if !errors.Is(err, store.ErrRegistrationInviteInvalid) {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	if count.Load() != 1 {
		t.Fatalf("single-use code created %d users", count.Load())
	}
	if err := users.ValidateRegistrationInvite(ctx, codes[0]); !errors.Is(err, store.ErrRegistrationInviteInvalid) {
		t.Fatalf("exhausted %v", err)
	}
	if err := users.SetRegistrationInviteRequired(ctx, false); err != nil {
		t.Fatal(err)
	}
	if err := users.ValidateRegistrationInvite(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if err := users.SetRegistrationInviteRequired(ctx, true); err != nil {
		t.Fatal(err)
	}
	codes, err = users.GenerateRegistrationInvites(ctx, 1, 1, 30)
	if err != nil {
		t.Fatal(err)
	}
	hash := registrationInviteHash(codes[0])
	if _, err := pool.Exec(ctx, `UPDATE registration_invites SET expires_at=now()-interval '1 second' WHERE code_hash=$1`, hash[:]); err != nil {
		t.Fatal(err)
	}
	if err := users.ValidateRegistrationInvite(ctx, codes[0]); !errors.Is(err, store.ErrRegistrationInviteInvalid) {
		t.Fatalf("expired %v", err)
	}
	codes, err = users.GenerateRegistrationInvites(ctx, 1, 1, 30)
	if err != nil {
		t.Fatal(err)
	}
	hash = registrationInviteHash(codes[0])
	if _, err := pool.Exec(ctx, `UPDATE registration_invites SET disabled=true WHERE code_hash=$1`, hash[:]); err != nil {
		t.Fatal(err)
	}
	if err := users.ValidateRegistrationInvite(ctx, codes[0]); !errors.Is(err, store.ErrRegistrationInviteInvalid) {
		t.Fatalf("disabled %v", err)
	}
}

func TestRegistrationInviteSignUpRetryPostgres(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	users := NewUserStore(pool)
	t.Cleanup(func() { _ = users.SetRegistrationInviteRequired(ctx, false) })
	if err := users.SetRegistrationInviteRequired(ctx, true); err != nil {
		t.Fatal(err)
	}
	codes, err := users.GenerateRegistrationInvites(ctx, 1, 1, 30)
	if err != nil {
		t.Fatal(err)
	}
	svc := appauth.NewService(users, NewAuthorizationStore(pool), memory.NewCodeStore(), nil, nil, "12345")
	phone := fmt.Sprintf("157%08d", time.Now().UnixNano()%100000000)
	key := futureTestKey(t, pool)
	hash, err := svc.SendCode(ctx, phone)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, signup, err := svc.SignIn(ctx, domain.Authorization{AuthKeyID: key}, phone, hash, "12345"); err != nil || !signup {
		t.Fatalf("verify %v %v", signup, err)
	}
	if _, _, err := svc.SignUp(ctx, domain.Authorization{AuthKeyID: key}, phone, hash, "Invite", ""); !errors.Is(err, store.ErrRegistrationInviteRequired) {
		t.Fatalf("old client bypass: %v", err)
	}
	if _, _, err := svc.SignUp(ctx, domain.Authorization{AuthKeyID: key}, phone, hash+store.RegistrationInviteSeparator+"BAD", "Invite", ""); !errors.Is(err, store.ErrRegistrationInviteInvalid) {
		t.Fatal(err)
	}
	if _, _, err := svc.SignUp(ctx, domain.Authorization{AuthKeyID: key}, phone, hash+store.RegistrationInviteSeparator+codes[0], "Invite", ""); err != nil {
		t.Fatalf("retry consumed phone proof: %v", err)
	}
}
