package memory

import (
	"context"
	"fmt"
	"time"

	"telesrv/internal/domain"
	"telesrv/internal/store"
)

var _ store.PhoneCallHistoryStore = (*MessageStore)(nil)

func (s *MessageStore) DeletePhoneCallHistory(_ context.Context, userID int64, req domain.DeletePhoneCallHistoryRequest) (domain.DeleteMessagesResult, error) {
	if userID <= 0 {
		return domain.DeleteMessagesResult{}, domain.ErrMessageIDInvalid
	}
	if (req.OriginAuthKeyID == ([8]byte{})) != (req.OriginSessionID == 0) {
		return domain.DeleteMessagesResult{}, fmt.Errorf("call history delete: invalid origin device")
	}
	if req.Date == 0 {
		req.Date = int(time.Now().Unix())
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	events := s.updateEvents
	if events == nil {
		return domain.DeleteMessagesResult{}, store.ErrDeliveryOutboxRequired
	}
	events.mu.Lock()
	defer events.mu.Unlock()
	deleted, uids, more := s.deleteMemoryMessagesLocked(userID, domain.MaxDeletePhoneCallHistoryBatch, func(message domain.Message) bool {
		return message.Peer.Type == domain.PeerTypeUser && message.Media != nil && message.Media.ServiceAction != nil &&
			message.Media.ServiceAction.Kind == domain.MessageServiceActionPhoneCall
	})
	if req.Revoke {
		deleted = append(deleted, s.deleteMemoryMessagesByUIDLocked(uids, userID)...)
	}
	owners := make(map[int64]struct{}, len(deleted))
	for _, row := range deleted {
		owners[row.userID] = struct{}{}
	}
	for owner := range owners {
		for _, event := range events.events[owner] {
			if event.Pts > s.nextPts[owner] {
				s.nextPts[owner] = event.Pts
			}
		}
	}
	result := s.finishMemoryDeleteLocked(domain.DeleteMessagesResult{OwnerUserID: userID}, deleted, req.Date, nil)
	for _, owner := range result.Deleted {
		auth, session := [8]byte{}, int64(0)
		if owner.UserID == userID {
			auth, session = req.OriginAuthKeyID, req.OriginSessionID
		}
		for _, event := range owner.Events {
			appendMemorySendEventLocked(events, owner.UserID, event, auth, session)
		}
	}
	if more {
		result.Offset = 1
	}
	return result, nil
}
