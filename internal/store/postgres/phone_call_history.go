package postgres

import (
	"context"

	"telesrv/internal/domain"
	"telesrv/internal/store"
)

var _ store.PhoneCallHistoryStore = (*MessageStore)(nil)

func (s *MessageStore) DeletePhoneCallHistory(ctx context.Context, userID int64, req domain.DeletePhoneCallHistoryRequest) (domain.DeleteMessagesResult, error) {
	return store.DeletePhoneCallHistoryPage(ctx, s, userID, req)
}
