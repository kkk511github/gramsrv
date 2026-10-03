package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"telesrv/internal/domain"
	"telesrv/internal/store"
)

var _ store.AccountFeatureStore = (*PasswordStore)(nil)

func (s *PasswordStore) GetMainProfileTab(ctx context.Context, userID int64) (domain.ProfileTab, bool, error) {
	var tab domain.ProfileTab
	err := s.db.QueryRow(ctx, `SELECT tab FROM account_profile_tabs WHERE user_id = $1`, userID).Scan(&tab)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get main profile tab: %w", err)
	}
	if !tab.Valid() {
		return "", false, domain.ErrProfileTabInvalid
	}
	return tab, true, nil
}

func (s *PasswordStore) SetMainProfileTab(ctx context.Context, userID int64, tab domain.ProfileTab) error {
	if userID <= 0 || !tab.Valid() {
		return domain.ErrProfileTabInvalid
	}
	_, err := s.db.Exec(ctx, `INSERT INTO account_profile_tabs (user_id, tab) VALUES ($1,$2)
ON CONFLICT (user_id) DO UPDATE SET tab = EXCLUDED.tab, updated_at = now()`, userID, string(tab))
	if err != nil {
		return fmt.Errorf("set main profile tab: %w", err)
	}
	return nil
}

func (s *PasswordStore) ConfirmBotConnection(ctx context.Context, ownerID, botID, confirmedAt int64) error {
	if ownerID <= 0 || botID <= 0 || confirmedAt <= 0 {
		return domain.ErrBotBusinessMissing
	}
	tag, err := s.db.Exec(ctx, `UPDATE business_connected_bots SET confirmed_at = COALESCE(confirmed_at, $3)
WHERE owner_user_id = $1 AND bot_user_id = $2`, ownerID, botID, time.Unix(confirmedAt, 0).UTC())
	if err != nil {
		return fmt.Errorf("confirm bot connection: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrBotBusinessMissing
	}
	return nil
}
