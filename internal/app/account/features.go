package account

import (
	"context"
	"telesrv/internal/domain"
	"telesrv/internal/store"
)

func (s *Service) featureStore() (store.AccountFeatureStore, bool) {
	if s == nil {
		return nil, false
	}
	st, ok := s.passwords.(store.AccountFeatureStore)
	return st, ok
}

func (s *Service) requireFeatureOwner(ctx context.Context, userID int64) error {
	if s == nil || s.users == nil {
		return domain.ErrAccountFeatureUnavailable
	}
	user, found, err := s.users.ByID(ctx, userID)
	if err != nil {
		return err
	}
	if userID <= 0 || !found || user.ID != userID || user.Bot || user.Deleted {
		return domain.ErrAccountFeatureOwnerInvalid
	}
	return nil
}

func (s *Service) GetMainProfileTab(ctx context.Context, userID int64) (domain.ProfileTab, error) {
	st, ok := s.featureStore()
	if !ok {
		return "", nil
	}
	tab, found, err := st.GetMainProfileTab(ctx, userID)
	if err != nil || !found {
		return "", err
	}
	if !tab.Valid() {
		return "", domain.ErrProfileTabInvalid
	}
	return tab, nil
}

func (s *Service) SetMainProfileTab(ctx context.Context, userID int64, tab domain.ProfileTab) error {
	if !tab.Valid() {
		return domain.ErrProfileTabInvalid
	}
	if err := s.requireFeatureOwner(ctx, userID); err != nil {
		return err
	}
	st, ok := s.featureStore()
	if !ok {
		return domain.ErrAccountFeatureUnavailable
	}
	return st.SetMainProfileTab(ctx, userID, tab)
}

func (s *Service) ConfirmBotConnection(ctx context.Context, ownerID, botID, confirmedAt int64) error {
	if err := s.requireFeatureOwner(ctx, ownerID); err != nil {
		return err
	}
	bot, found, err := s.users.ByID(ctx, botID)
	if err != nil {
		return err
	}
	if !found || !bot.Bot || bot.Deleted || bot.ID == ownerID || bot.ID == domain.BotFatherUserID {
		return domain.ErrBotBusinessMissing
	}
	st, ok := s.featureStore()
	if !ok {
		return domain.ErrAccountFeatureUnavailable
	}
	// The store atomically checks the existing owner+bot row, including races
	// with replacement or removal. No recipients or rights are ever changed.
	return st.ConfirmBotConnection(ctx, ownerID, botID, confirmedAt)
}
