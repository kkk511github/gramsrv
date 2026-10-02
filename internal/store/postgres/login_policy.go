package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"telesrv/internal/store"
	"telesrv/internal/store/postgres/sqlcgen"
)

func readLoginPolicy(ctx context.Context, db sqlcgen.DBTX, lock bool) (store.LoginPolicy, error) {
	var p store.LoginPolicy
	query := `SELECT future_auth_enabled,future_auth_days,registration_password_required FROM registration_policy WHERE singleton`
	if lock {
		query += ` FOR SHARE`
	}
	err := db.QueryRow(ctx, query).Scan(&p.FutureAuthEnabled, &p.FutureAuthDays, &p.RegistrationPasswordRequired)
	return p, err
}

func (s *AuthorizationStore) LoginPolicy(ctx context.Context) (store.LoginPolicy, error) {
	return readLoginPolicy(ctx, s.db, false)
}

func (s *UserStore) LoginPolicy(ctx context.Context) (store.LoginPolicy, error) {
	return readLoginPolicy(ctx, s.db, false)
}

func (s *UserStore) SaveLoginPolicy(ctx context.Context, p store.LoginPolicy) error {
	if err := p.Validate(); err != nil {
		return err
	}
	return withAuthIdentityTx(ctx, s.db, "save login policy", func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE registration_policy SET future_auth_enabled=$1,future_auth_days=$2,registration_password_required=$3 WHERE singleton`, p.FutureAuthEnabled, p.FutureAuthDays, p.RegistrationPasswordRequired); err != nil {
			return err
		}
		if !p.RegistrationPasswordRequired {
			if _, err := tx.Exec(ctx, `DELETE FROM registration_password_pending`); err != nil {
				return err
			}
		}
		if !p.FutureAuthEnabled {
			_, err := tx.Exec(ctx, `DELETE FROM future_auth_tokens`)
			return err
		}
		// Shortening is permanent for existing tokens; extending only affects new ones.
		_, err := tx.Exec(ctx, `UPDATE future_auth_tokens SET expires_at=LEAST(expires_at,created_at + $1 * interval '1 day')`, p.FutureAuthDays)
		return err
	})
}

func (s *UserStore) RegistrationPasswordPending(ctx context.Context, userID int64) (bool, error) {
	var pending bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM registration_password_pending WHERE user_id=$1)`, userID).Scan(&pending)
	return pending, err
}
