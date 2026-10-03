package store

import (
	"context"
	"fmt"

	"telesrv/internal/domain"
)

// PhoneCallHistoryStore deletes a bounded page and commits deletion/PTS/events
// together. The returned offset must reflect whether more call history remains.
type PhoneCallHistoryStore interface {
	DeletePhoneCallHistory(context.Context, int64, domain.DeletePhoneCallHistoryRequest) (domain.DeleteMessagesResult, error)
}

// DeletePhoneCallHistoryPage uses the existing transactional private-message
// delete boundary; it never performs one mutation per call or per participant.
func DeletePhoneCallHistoryPage(ctx context.Context, messages MessageStore, userID int64, req domain.DeletePhoneCallHistoryRequest) (domain.DeleteMessagesResult, error) {
	if messages == nil || userID <= 0 {
		return domain.DeleteMessagesResult{}, domain.ErrMessageIDInvalid
	}
	list, err := messages.ListByUser(ctx, userID, domain.MessageFilter{
		PhoneCallsOnly: true, NeedTotalCount: true, Limit: domain.MaxDeletePhoneCallHistoryBatch,
	})
	if err != nil {
		return domain.DeleteMessagesResult{}, fmt.Errorf("list phone call history: %w", err)
	}
	ids := make([]int, 0, len(list.Messages))
	for _, message := range list.Messages {
		if message.OwnerUserID != userID || message.Peer.Type != domain.PeerTypeUser ||
			message.Media == nil || message.Media.ServiceAction == nil ||
			message.Media.ServiceAction.Kind != domain.MessageServiceActionPhoneCall {
			return domain.DeleteMessagesResult{}, domain.ErrMessageIDInvalid
		}
		ids = append(ids, message.ID)
	}
	if len(ids) > domain.MaxDeletePhoneCallHistoryBatch || (list.Count > 0 && len(ids) == 0) {
		return domain.DeleteMessagesResult{}, domain.ErrMessageIDInvalid
	}
	result, err := messages.DeleteMessages(ctx, domain.DeleteMessagesRequest{
		OwnerUserID: userID, IDs: ids, Revoke: req.Revoke, Date: req.Date,
		OriginAuthKeyID: req.OriginAuthKeyID, OriginSessionID: req.OriginSessionID,
	})
	if err != nil {
		return domain.DeleteMessagesResult{}, fmt.Errorf("delete phone call history: %w", err)
	}
	// Recheck after the atomic page commit: concurrent deletions/insertions can
	// make the initial total stale. On a read failure, return an error and let
	// retry/getDifference recover the committed page instead of claiming done.
	remaining, err := messages.ListByUser(ctx, userID, domain.MessageFilter{PhoneCallsOnly: true, CountOnly: true})
	if err != nil {
		return domain.DeleteMessagesResult{}, fmt.Errorf("count remaining phone call history: %w", err)
	}
	if remaining.Count > 0 {
		result.Offset = 1
	}
	return result, nil
}
