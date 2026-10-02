package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"telesrv/internal/domain"
	"telesrv/internal/store"
)

var _ store.FutureAuthTokenStore = (*AuthorizationStore)(nil)

func lockFutureAuthUser(ctx context.Context, tx pgx.Tx, userID int64) (bool, error) {
	if domain.IsSystemUserID(userID) {
		return false, nil
	}
	if err := lockUsersForUpdate(ctx, tx, userID); err != nil {
		return false, err
	}
	var active bool
	err := tx.QueryRow(ctx, `SELECT deleted_at IS NULL AND NOT is_bot FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&active)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return active, err
}

func lockFutureAuthPassword(ctx context.Context, tx pgx.Tx, userID int64) (bool, error) {
	var needed bool
	err := tx.QueryRow(ctx, `SELECT has_password FROM account_passwords WHERE user_id=$1 FOR SHARE`, userID).Scan(&needed)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return needed, err
}

func (s *AuthorizationStore) IssueFutureAuthToken(ctx context.Context, key [8]byte, digest [32]byte, expires time.Time, logout bool) (bool, error) {
	issued := false
	err := withAuthIdentityTx(ctx, s.db, "issue future auth token", func(tx pgx.Tx) error {
		policy, err := readLoginPolicy(ctx, tx, true)
		if err != nil {
			return err
		}
		keyID := authKeyIDToInt64(key)
		var userID int64
		err = tx.QueryRow(ctx, `SELECT user_id FROM authorizations WHERE auth_key_id=$1`, keyID).Scan(&userID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		active, err := lockFutureAuthUser(ctx, tx, userID)
		if err != nil {
			return err
		}
		if _, err := lockFutureAuthPassword(ctx, tx, userID); err != nil {
			return err
		}
		if err := lockPermanentAuthIdentities(ctx, tx, []int64{keyID}); err != nil {
			return err
		}
		var current int64
		var pending bool
		err = tx.QueryRow(ctx, `SELECT user_id,password_pending FROM authorizations WHERE auth_key_id=$1 FOR UPDATE`, keyID).Scan(&current, &pending)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if current != userID {
			return store.ErrAuthorizationStateChanged
		}
		if logout {
			if _, err := tx.Exec(ctx, `DELETE FROM authorizations WHERE auth_key_id=$1`, keyID); err != nil {
				return err
			}
		}
		if !active || pending || !policy.FutureAuthEnabled {
			return nil
		}
		if setupPending, err := NewUserStore(tx).RegistrationPasswordPending(ctx, userID); err != nil || setupPending {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM future_auth_tokens WHERE user_id=$1 AND expires_at <= now()`, userID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO future_auth_tokens(token_hash,user_id,source_auth_key_id,expires_at)
VALUES($1,$2,$3,LEAST($4::timestamptz,now()+$5 * interval '1 day')) ON CONFLICT(user_id,source_auth_key_id) DO UPDATE SET token_hash=EXCLUDED.token_hash,expires_at=EXCLUDED.expires_at,created_at=now()`, digest[:], userID, keyID, expires, policy.FutureAuthDays); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM future_auth_tokens WHERE token_hash IN
(SELECT token_hash FROM future_auth_tokens WHERE user_id=$1 ORDER BY created_at DESC,token_hash DESC OFFSET 20)`, userID); err != nil {
			return err
		}
		issued = true
		return nil
	})
	return issued && err == nil, err
}

func (s *AuthorizationStore) RedeemFutureAuthToken(ctx context.Context, phone string, digests [][32]byte, a domain.Authorization) (domain.Authorization, bool, error) {
	if len(digests) == 0 || len(digests) > 20 {
		return domain.Authorization{}, false, nil
	}
	found := false
	err := withAuthIdentityTx(ctx, s.db, "redeem future auth token", func(tx pgx.Tx) error {
		policy, err := readLoginPolicy(ctx, tx, true)
		if err != nil || !policy.FutureAuthEnabled {
			return err
		}
		var userID int64
		err = tx.QueryRow(ctx, `SELECT id FROM users WHERE phone=$1 AND deleted_at IS NULL AND NOT is_bot`, phone).Scan(&userID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		active, err := lockFutureAuthUser(ctx, tx, userID)
		if err != nil || !active {
			return err
		}
		// Re-read after the user lock: a phone transfer must invalidate old proof.
		var currentPhone string
		if err := tx.QueryRow(ctx, `SELECT phone FROM users WHERE id=$1`, userID).Scan(&currentPhone); err != nil {
			return err
		}
		if currentPhone != phone {
			return nil
		}
		pending, err := lockFutureAuthPassword(ctx, tx, userID)
		if err != nil {
			return err
		}
		if err := lockPermanentAuthIdentities(ctx, tx, []int64{authKeyIDToInt64(a.AuthKeyID)}); err != nil {
			return err
		}
		var boundUser int64
		var boundPending bool
		err = tx.QueryRow(ctx, `SELECT user_id,password_pending FROM authorizations WHERE auth_key_id=$1`, authKeyIDToInt64(a.AuthKeyID)).Scan(&boundUser, &boundPending)
		if err == nil {
			if boundUser != userID {
				return store.ErrAuthorizationStateChanged
			}
			// A transport retry on the same key reuses its existing authorization.
			a.UserID, a.PasswordPending, found = boundUser, boundPending, true
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var hash []byte
		for _, digest := range digests {
			err = tx.QueryRow(ctx, `SELECT token_hash FROM future_auth_tokens WHERE token_hash=$1 AND user_id=$2 AND expires_at>now() FOR UPDATE`, digest[:], userID).Scan(&hash)
			if err == nil {
				break
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		if len(hash) == 0 {
			return nil
		}
		a.UserID = userID
		a.PasswordPending = pending
		if a.Hash == 0 {
			a.Hash = authorizationHash(a.AuthKeyID)
		}
		if err := bindAuthorization(ctx, tx, a); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM future_auth_tokens WHERE token_hash=$1`, hash); err != nil {
			return err
		}
		found = true
		return nil
	})
	return a, found && err == nil, err
}
