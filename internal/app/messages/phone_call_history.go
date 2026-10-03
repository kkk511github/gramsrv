package messages

import (
	"context"

	"telesrv/internal/domain"
	"telesrv/internal/store"
)

// DeletePhoneCallHistory commits one bounded page through the existing atomic
// delete/PTS/outbox path. Offset asks the client to repeat, not skip older calls.
func (s *Service) DeletePhoneCallHistory(ctx context.Context, userID int64, req domain.DeletePhoneCallHistoryRequest) (domain.DeleteMessagesResult, error) {
	if s == nil || s.messages == nil || userID <= 0 {
		return domain.DeleteMessagesResult{}, domain.ErrMessageIDInvalid
	}
	if backend, ok := s.messages.(store.PhoneCallHistoryStore); ok {
		return backend.DeletePhoneCallHistory(ctx, userID, req)
	}
	return store.DeletePhoneCallHistoryPage(ctx, s.messages, userID, req)
}
