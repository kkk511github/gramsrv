package memory

import (
	"context"
	"telesrv/internal/domain"
	"telesrv/internal/store"
)

var _ store.AccountFeatureStore = (*PasswordStore)(nil)

func (s *PasswordStore) GetMainProfileTab(_ context.Context, userID int64) (domain.ProfileTab, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tab, found := s.mainProfileTabs[userID]
	return tab, found, nil
}

func (s *PasswordStore) SetMainProfileTab(_ context.Context, userID int64, tab domain.ProfileTab) error {
	if userID <= 0 || !tab.Valid() {
		return domain.ErrProfileTabInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mainProfileTabs == nil {
		s.mainProfileTabs = make(map[int64]domain.ProfileTab)
	}
	s.mainProfileTabs[userID] = tab
	return nil
}

func (s *PasswordStore) ConfirmBotConnection(_ context.Context, ownerID, botID, confirmedAt int64) error {
	if ownerID <= 0 || botID <= 0 || confirmedAt <= 0 {
		return domain.ErrBotBusinessMissing
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	bot, found := s.connectedBusinessBots[ownerID]
	if !found || bot.BotUserID != botID {
		return domain.ErrBotBusinessMissing
	}
	if bot.ConfirmedAtUnix == 0 {
		bot.ConfirmedAtUnix = confirmedAt
		s.connectedBusinessBots[ownerID] = bot
	}
	return nil
}
