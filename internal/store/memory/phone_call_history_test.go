package memory

import (
	"context"
	"testing"

	"telesrv/internal/domain"
)

func TestDeletePhoneCallHistoryDeviceExclusionAndPermissions(t *testing.T) {
	ctx := context.Background()
	messages := NewMessageStore(NewDialogStore())
	seed, err := messages.SendPrivateText(ctx, domain.SendPrivateTextRequest{
		SenderUserID: 9, RecipientUserID: 10, RandomID: 11, Date: 1700000000,
		Media: &domain.MessageMedia{Kind: domain.MessageMediaKindService, ServiceAction: &domain.MessageServiceAction{
			Kind: domain.MessageServiceActionPhoneCall, Call: &domain.MessagePhoneCallAction{CallID: 11, Reason: "missed"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result, err := messages.DeletePhoneCallHistory(ctx, 12, domain.DeletePhoneCallHistoryRequest{Revoke: true}); err != nil || result.Changed() {
		t.Fatalf("unrelated owner changed call: result=%+v err=%v", result, err)
	}
	if result, err := messages.DeletePhoneCallHistory(ctx, 9, domain.DeletePhoneCallHistoryRequest{OriginSessionID: 1}); err == nil || result.Changed() {
		t.Fatalf("unbound origin accepted: result=%+v err=%v", result, err)
	}
	key := [8]byte{1, 2, 3}
	result, err := messages.DeletePhoneCallHistory(ctx, 10, domain.DeletePhoneCallHistoryRequest{
		Revoke: true, Date: 1700000001, OriginAuthKeyID: key, OriginSessionID: 4,
	})
	if err != nil || len(result.Self().MessageIDs) != 1 || result.Self().MessageIDs[0] != seed.RecipientMessage.ID {
		t.Fatalf("recipient revoke=%+v err=%v", result, err)
	}
	for _, owner := range []int64{9, 10} {
		dispatches := messages.updateEvents.dispatches[owner]
		last := dispatches[len(dispatches)-1]
		wantKey, wantSession := [8]byte{}, int64(0)
		if owner == 10 {
			wantKey, wantSession = key, 4
		}
		if last.ExcludeAuthKeyID != wantKey || last.ExcludeSessionID != wantSession {
			t.Fatalf("owner=%d exclusion=%+v", owner, last)
		}
		calls, err := messages.ListByUser(ctx, owner, domain.MessageFilter{PhoneCallsOnly: true, CountOnly: true})
		if err != nil || calls.Count != 0 {
			t.Fatalf("owner=%d calls=%+v err=%v", owner, calls, err)
		}
	}
}
