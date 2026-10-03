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
	expires := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Microsecond)
	seed, err := users.CreateRegistrationInvites(ctx, 1, 1, expires, "")
	if err != nil {
		t.Fatal(err)
	}
	code := seed[0]
	if _, err := pool.Exec(ctx, `DELETE FROM registration_invites WHERE code=$1`, code); err != nil {
		t.Fatal(err)
	}
	codes, err := users.CreateRegistrationInvites(ctx, 1, 2, expires, code)
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
	if len(items) == 0 || items[0].Code != code || items[0].UsedCount != 2 || !items[0].ExpiresAt.Equal(expires) {
		t.Fatalf("stored limits: %+v", items)
	}
	codes, err = users.CreateRegistrationInvites(ctx, 10, 1, expires, "")
	if err != nil {
		t.Fatal(err)
	}
	unique := make(map[string]bool)
	for _, code := range codes {
		if len(code) != 5 || strings.Trim(code, "0123456789") != "" {
			t.Fatalf("not a five-digit code: %q", code)
		}
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
		{1, 100, "01234", true}, {1, 100, "00000", true}, {1, 100, " 12345 ", true}, {100, 1, "", true},
		{2, 100, "FIXED-CODE", false}, {1, 0, "", false},
		{101, 1, "", false}, {1, 10001, "", false}, {1, 1, "short", false},
		{1, 1, "bad code", false},
		{1, 1, "1234", false}, {1, 1, "123456", false}, {1, 1, "12A45", false},
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

func TestRegistrationInviteRandomDigits(t *testing.T) {
	for i := 0; i < 1000; i++ {
		code, err := randomRegistrationInvite()
		if err != nil || len(code) != 5 || strings.Trim(code, "0123456789") != "" {
			t.Fatalf("generated %q: %v", code, err)
		}
	}
}

func TestRegistrationInviteCollisionAndLegacyPostgres(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	users := NewUserStore(pool)
	expires := time.Now().Add(time.Hour)
	seed, err := users.CreateRegistrationInvites(ctx, 2, 1, expires, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM registration_invites WHERE code=$1`, seed[1]); err != nil {
		t.Fatal(err)
	}
	calls := 0
	codes, err := users.createRegistrationInvites(ctx, 1, 1, expires, "", func() (string, error) {
		calls++
		if calls == 1 {
			return seed[0], nil
		}
		return seed[1], nil
	})
	if err != nil || calls != 2 || len(codes) != 1 || codes[0] != seed[1] {
		t.Fatalf("collision retry: %v %d %v", codes, calls, err)
	}
	// A failed batch must not return or persist an earlier candidate.
	if _, err := pool.Exec(ctx, `DELETE FROM registration_invites WHERE code=$1`, seed[1]); err != nil {
		t.Fatal(err)
	}
	codes, err = users.createRegistrationInvites(ctx, 2, 1, expires, "", func() (string, error) { return seed[1], nil })
	if err == nil || codes != nil {
		t.Fatalf("partial batch leaked: %v %v", codes, err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM registration_invites WHERE code=$1`, seed[1]).Scan(&count); err != nil || count != 0 {
		t.Fatalf("batch did not roll back: %d %v", count, err)
	}
	legacy := fmt.Sprintf("LEGACY-%d", time.Now().UnixNano())
	hash := registrationInviteHash(legacy)
	var legacyID int64
	if err := pool.QueryRow(ctx, `INSERT INTO registration_invites(code_hash,prefix,max_uses,expires_at) VALUES($1,$2,1,$3) RETURNING id`, hash[:], legacy[:3], expires).Scan(&legacyID); err != nil {
		t.Fatal(err)
	}
	if err := users.SetRegistrationInviteRequired(ctx, true); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = users.SetRegistrationInviteRequired(ctx, false) })
	if err := users.ValidateRegistrationInvite(ctx, legacy); err != nil {
		t.Fatalf("legacy invite invalidated: %v", err)
	}
	_, items, err := NewUserStore(pool).RegistrationInvites(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range items {
		if item.ID == legacyID {
			found = true
			if item.Code != "" {
				t.Fatal("legacy hash fabricated a code")
			}
		}
		if item.Code == seed[0] {
			if item.Prefix != seed[0][:3] {
				t.Fatal("visible code changed")
			}
		}
	}
	if !found {
		t.Fatal("legacy entry missing")
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
