package rpc

import (
	"context"

	"github.com/iamxvbaba/td/tg"

	"telesrv/internal/domain"
)

type phoneCallHistoryService interface {
	DeletePhoneCallHistory(context.Context, int64, domain.DeletePhoneCallHistoryRequest) (domain.DeleteMessagesResult, error)
}

func (r *Router) onMessagesDeletePhoneCallHistory(ctx context.Context, req *tg.MessagesDeletePhoneCallHistoryRequest) (*tg.MessagesAffectedFoundMessages, error) {
	userID, authorized, err := r.currentUserID(ctx)
	if err != nil {
		return nil, internalErr()
	}
	if !authorized || userID <= 0 {
		return nil, authKeyUnregisteredErr()
	}
	if req == nil {
		return nil, inputRequestInvalidErr()
	}
	if r.deps.Users == nil {
		return nil, internalErr()
	}
	self, err := r.deps.Users.Self(ctx, userID)
	if err != nil || self.ID != userID {
		return nil, internalErr()
	}
	if self.Bot {
		return nil, botMethodInvalidErr()
	}
	service, ok := r.deps.Messages.(phoneCallHistoryService)
	if !ok || service == nil {
		return nil, internalErr()
	}
	sessionID, _ := SessionIDFrom(ctx)
	result, err := service.DeletePhoneCallHistory(ctx, userID, domain.DeletePhoneCallHistoryRequest{
		Revoke: req.GetRevoke(), Date: int(r.clock.Now().Unix()),
		OriginAuthKeyID: rawAuthKeyIDForOrigin(ctx), OriginSessionID: sessionID,
	})
	if err != nil {
		return nil, internalErr()
	}
	deleted := result.Self()
	pts, ptsCount := deleted.AffectedPts()
	if pts == 0 {
		authKeyID, _ := AuthKeyIDFrom(ctx)
		state, err := r.affectedMessages(ctx, authKeyID, userID)
		if err != nil {
			return nil, err
		}
		pts = state.Pts
	}
	return &tg.MessagesAffectedFoundMessages{
		Pts: pts, PtsCount: ptsCount, Offset: result.Offset,
		Messages: append([]int{}, deleted.MessageIDs...),
	}, nil
}
