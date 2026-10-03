package postgres

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"telesrv/internal/domain"
	"telesrv/internal/store"
	"telesrv/internal/store/postgres/sqlcgen"
)

type RegistrationInvite struct {
	ID        int64     `json:"id"`
	Prefix    string    `json:"prefix"`
	Code      string    `json:"code,omitempty"`
	MaxUses   int       `json:"max_uses"`
	UsedCount int       `json:"used_count"`
	Disabled  bool      `json:"disabled"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

func registrationInviteHash(code string) [32]byte {
	return sha256.Sum256([]byte(strings.ToUpper(strings.TrimSpace(code))))
}

func checkRegistrationInvite(ctx context.Context, db sqlcgen.DBTX, code string, lock bool) (int64, error) {
	var required bool
	query := `SELECT invite_required FROM registration_policy WHERE singleton`
	if lock {
		query += ` FOR SHARE`
	}
	if err := db.QueryRow(ctx, query).Scan(&required); err != nil {
		return 0, err
	}
	if !required {
		return 0, nil
	}
	if strings.TrimSpace(code) == "" {
		return 0, store.ErrRegistrationInviteRequired
	}
	if len(code) > 64 {
		return 0, store.ErrRegistrationInviteInvalid
	}
	hash := registrationInviteHash(code)
	query = `SELECT id FROM registration_invites WHERE code_hash=$1 AND NOT disabled AND expires_at>now() AND used_count<max_uses`
	if lock {
		query += ` FOR UPDATE`
	}
	var id int64
	err := db.QueryRow(ctx, query, hash[:]).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, store.ErrRegistrationInviteInvalid
	}
	return id, err
}

func (s *UserStore) ValidateRegistrationInvite(ctx context.Context, code string) error {
	_, err := checkRegistrationInvite(ctx, s.db, code, false)
	return err
}

func (s *UserStore) CreateRegistration(ctx context.Context, u domain.User, code string) (domain.User, error) {
	var result domain.User
	err := withAuthIdentityTx(ctx, s.db, "create invited account", func(tx pgx.Tx) error {
		id, err := checkRegistrationInvite(ctx, tx, code, true)
		if err != nil {
			return err
		}
		result, err = NewUserStore(tx).Create(ctx, u)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO registration_password_pending(user_id)
SELECT $1 FROM registration_policy WHERE singleton AND registration_password_required`, result.ID); err != nil {
			return err
		}
		if id == 0 {
			return nil
		}
		if _, err := tx.Exec(ctx, `UPDATE registration_invites SET used_count=used_count+1 WHERE id=$1`, id); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO registration_invite_uses(user_id,invite_id) VALUES($1,$2)`, result.ID, id)
		return err
	})
	return result, err
}

func (s *UserStore) RegistrationInviteRequired(ctx context.Context) (bool, error) {
	var enabled bool
	err := s.db.QueryRow(ctx, `SELECT invite_required FROM registration_policy WHERE singleton`).Scan(&enabled)
	return enabled, err
}

func (s *UserStore) RegistrationInvites(ctx context.Context) (bool, []RegistrationInvite, error) {
	enabled, err := s.RegistrationInviteRequired(ctx)
	if err != nil {
		return false, nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT id,prefix,COALESCE(code,''),max_uses,used_count,disabled,expires_at,created_at FROM registration_invites ORDER BY id DESC LIMIT 200`)
	if err != nil {
		return false, nil, err
	}
	defer rows.Close()
	list := make([]RegistrationInvite, 0)
	for rows.Next() {
		var item RegistrationInvite
		if err := rows.Scan(&item.ID, &item.Prefix, &item.Code, &item.MaxUses, &item.UsedCount, &item.Disabled, &item.ExpiresAt, &item.CreatedAt); err != nil {
			return false, nil, err
		}
		list = append(list, item)
	}
	return enabled, list, rows.Err()
}

func (s *UserStore) SetRegistrationInviteRequired(ctx context.Context, enabled bool) error {
	_, err := s.db.Exec(ctx, `UPDATE registration_policy SET invite_required=$1 WHERE singleton`, enabled)
	return err
}

func (s *UserStore) DisableRegistrationInvite(ctx context.Context, id int64) error {
	tag, err := s.db.Exec(ctx, `UPDATE registration_invites SET disabled=true WHERE id=$1`, id)
	if err == nil && tag.RowsAffected() != 1 {
		return fmt.Errorf("invite not found")
	}
	return err
}

func (s *UserStore) GenerateRegistrationInvites(ctx context.Context, count, uses, days int) ([]string, error) {
	if count < 1 || count > 100 || uses < 1 || uses > 10000 || days < 1 || days > 365 {
		return nil, fmt.Errorf("invalid invite limits")
	}
	return s.CreateRegistrationInvites(ctx, count, uses, time.Now().Add(time.Duration(days)*24*time.Hour), "")
}

func ValidateRegistrationInviteOptions(count, uses int, expires time.Time, code string) error {
	if count < 1 || count > 100 || uses < 1 || uses > 10000 || !expires.After(time.Now()) || expires.After(time.Now().Add(366*24*time.Hour)) {
		return fmt.Errorf("邀请码数量、次数或到期时间超出范围")
	}
	code = strings.ToUpper(strings.TrimSpace(code))
	if code != "" {
		if count != 1 || len(code) != 5 {
			return fmt.Errorf("固定邀请码需为 5 位数字，生成数量须为 1")
		}
		for _, c := range code {
			if c < '0' || c > '9' {
				return fmt.Errorf("邀请码仅支持 5 位数字")
			}
		}
	}
	return nil
}

func (s *UserStore) CreateRegistrationInvites(ctx context.Context, count, uses int, expires time.Time, fixedCode string) ([]string, error) {
	return s.createRegistrationInvites(ctx, count, uses, expires, fixedCode, randomRegistrationInvite)
}

func randomRegistrationInvite() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(100000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%05d", n.Int64()), nil
}

func (s *UserStore) createRegistrationInvites(ctx context.Context, count, uses int, expires time.Time, fixedCode string, generate func() (string, error)) ([]string, error) {
	if err := ValidateRegistrationInviteOptions(count, uses, expires, fixedCode); err != nil {
		return nil, err
	}
	fixedCode = strings.ToUpper(strings.TrimSpace(fixedCode))
	codes := make([]string, 0, count)
	err := withAuthIdentityTx(ctx, s.db, "generate registration invites", func(tx pgx.Tx) error {
		codes = codes[:0]
		// Serialize batches so colliding codes cannot deadlock two transactions.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('safelink.registration.invite.allocation',0))`); err != nil {
			return err
		}
		for i := 0; i < count; i++ {
			inserted := false
			for attempt := 0; attempt < 1000; attempt++ {
				code := fixedCode
				if code == "" {
					var err error
					code, err = generate()
					if err != nil {
						return err
					}
				}
				hash := registrationInviteHash(code)
				tag, err := tx.Exec(ctx, `INSERT INTO registration_invites(code_hash,prefix,code,max_uses,expires_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT(code_hash) DO NOTHING`, hash[:], code[:3], code, uses, expires)
				if err != nil {
					return err
				}
				if tag.RowsAffected() == 1 {
					codes = append(codes, code)
					inserted = true
					break
				}
				if fixedCode != "" {
					return fmt.Errorf("此邀请码已存在，请使用其他邀请码")
				}
			}
			if !inserted {
				return fmt.Errorf("可用的 5 位邀请码不足，请减少生成数量或使用其他固定码")
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return codes, nil
}
