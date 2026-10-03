package rpc

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/iamxvbaba/td/bin"
	"github.com/iamxvbaba/td/tg"
	"github.com/iamxvbaba/td/tgerr"
	"github.com/iamxvbaba/td/tlprofile"

	appupdates "telesrv/internal/app/updates"
	"telesrv/internal/domain"
	"telesrv/internal/store/memory"
)

func seedPhoneCallHistory(t *testing.T, messages *memory.MessageStore, from, to, randomID int64) domain.SendPrivateTextResult {
	t.Helper()
	result, err := messages.SendPrivateText(context.Background(), domain.SendPrivateTextRequest{
		SenderUserID: from, RecipientUserID: to, RandomID: randomID, Date: 1700000000,
		Media: &domain.MessageMedia{Kind: domain.MessageMediaKindService, ServiceAction: &domain.MessageServiceAction{
			Kind: domain.MessageServiceActionPhoneCall,
			Call: &domain.MessagePhoneCallAction{CallID: randomID, Reason: "missed"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestDeletePhoneCallHistoryBoundedPagesAndLocalScope(t *testing.T) {
	router, messages, events, a, b := savedForwardFixture(t)
	router.deps.Updates = appupdates.NewService(nil, events)
	ctx := WithUserID(context.Background(), a.ID)
	ordinary, err := messages.SendPrivateText(ctx, domain.SendPrivateTextRequest{
		SenderUserID: a.ID, RecipientUserID: b.ID, RandomID: 1, Message: "keep ordinary message", Date: 1700000000,
	})
	if err != nil {
		t.Fatal(err)
	}
	total := domain.MaxDeletePhoneCallHistoryBatch + 2
	for i := range total {
		from, to := a.ID, b.ID
		if i%2 != 0 {
			from, to = to, from
		}
		seedPhoneCallHistory(t, messages, from, to, int64(i+100))
	}
	before, err := events.MaxContiguousPts(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	for page, size := range []int{domain.MaxDeletePhoneCallHistoryBatch, 2, 0} {
		result, err := router.onMessagesDeletePhoneCallHistory(ctx, &tg.MessagesDeletePhoneCallHistoryRequest{})
		if err != nil {
			t.Fatal(err)
		}
		wantOffset := 0
		if page == 0 {
			wantOffset = 1
		}
		if result.Offset != wantOffset || len(result.Messages) != size || result.PtsCount != size || result.Pts != before+size {
			t.Fatalf("page %d result=%+v want size=%d pts=%d offset=%d", page, result, size, before+size, wantOffset)
		}
		before = result.Pts
		for _, id := range result.Messages {
			if id == ordinary.SenderMessage.ID {
				t.Fatal("ordinary message deleted")
			}
		}
	}
	remaining, err := messages.ListByUser(ctx, a.ID, domain.MessageFilter{Limit: 500})
	if err != nil || len(remaining.Messages) != 1 || remaining.Messages[0].ID != ordinary.SenderMessage.ID {
		t.Fatalf("remaining=%+v err=%v", remaining, err)
	}
	peerCalls, err := messages.ListByUser(ctx, b.ID, domain.MessageFilter{PhoneCallsOnly: true, CountOnly: true})
	if err != nil || peerCalls.Count != total {
		t.Fatalf("local delete changed peer history: %+v err=%v", peerCalls, err)
	}
	deletedEvents, err := events.ListAfter(ctx, a.ID, total+1, 10)
	if err != nil || len(deletedEvents) != 2 {
		t.Fatalf("delete events=%+v err=%v", deletedEvents, err)
	}
	for i, event := range deletedEvents {
		want := domain.MaxDeletePhoneCallHistoryBatch
		if i == 1 {
			want = 2
		}
		if event.Type != domain.UpdateEventDeleteMessages || len(event.MessageIDs) != want || event.PtsCount != want {
			t.Fatalf("event=%+v", event)
		}
	}
}

func TestDeletePhoneCallHistoryRevokeOnlyOwnedCallUIDs(t *testing.T) {
	for _, receiverDeletes := range []bool{false, true} {
		t.Run(fmt.Sprintf("receiver=%v", receiverDeletes), func(t *testing.T) {
			router, messages, events, a, b := savedForwardFixture(t)
			caller := a.ID
			if receiverDeletes {
				caller = b.ID
			}
			seedPhoneCallHistory(t, messages, a.ID, b.ID, 10)
			seedPhoneCallHistory(t, messages, b.ID, a.ID, 11)
			unrelated := seedPhoneCallHistory(t, messages, b.ID, b.ID, 12)
			request := &tg.MessagesDeletePhoneCallHistoryRequest{}
			request.SetRevoke(true)
			result, err := router.onMessagesDeletePhoneCallHistory(WithUserID(context.Background(), caller), request)
			want := 2
			if receiverDeletes {
				want = 3 // The caller also owns its self-call; another account cannot delete it.
			}
			if err != nil || result.Offset != 0 || len(result.Messages) != want {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			for _, owner := range []int64{a.ID, b.ID} {
				left, err := messages.ListByUser(context.Background(), owner, domain.MessageFilter{PhoneCallsOnly: true, Limit: 500})
				wantLeft := 0
				if !receiverDeletes && owner == b.ID {
					wantLeft = 1
				}
				if err != nil || len(left.Messages) != wantLeft {
					t.Fatalf("owner=%d left=%+v err=%v", owner, left, err)
				}
				if wantLeft == 1 && left.Messages[0].ID != unrelated.SenderMessage.ID {
					t.Fatal("revoke crossed caller-owned UID boundary")
				}
				log, err := events.ListAfter(context.Background(), owner, 0, 100)
				if err != nil || log[len(log)-1].Type != domain.UpdateEventDeleteMessages {
					t.Fatalf("owner=%d log=%+v err=%v", owner, log, err)
				}
			}
		})
	}
}

func TestDeletePhoneCallHistoryExactLayerDispatch(t *testing.T) {
	for _, profile := range []tlprofile.Profile{tlprofile.Profile225, tlprofile.Profile227, tlprofile.Profile228, tlprofile.Profile229} {
		t.Run(fmt.Sprint(profile), func(t *testing.T) {
			router, messages, _, a, b := savedForwardFixture(t)
			seed := seedPhoneCallHistory(t, messages, a.ID, b.ID, 10)
			body := encodeExactLayerRPC(t, profile, &tg.MessagesDeletePhoneCallHistoryRequest{Revoke: true})
			admission, err := router.AdmitLayer(profile, &body, tlprofile.Limits{})
			if err != nil || admission.Call().Method() != tlprofile.SemanticMethodMessagesDeletePhoneCallHistory {
				t.Fatalf("admission=%+v err=%v", admission, err)
			}
			result, method, err := router.DispatchAdmitted(WithUserID(context.Background(), a.ID), [8]byte{}, 0, 0, 0, admission)
			if err != nil || method != "messages.deletePhoneCallHistory" {
				t.Fatalf("dispatch method=%s err=%v", method, err)
			}
			var wire bin.Buffer
			if err := result.Encode(&wire); err != nil {
				t.Fatal(err)
			}
			object, err := tlprofile.DecodeObject(profile, &wire, tlprofile.Limits{})
			if err != nil {
				t.Fatal(err)
			}
			affected, ok := object.(*tg.MessagesAffectedFoundMessages)
			if !ok || affected.Offset != 0 || affected.PtsCount != 1 || !reflect.DeepEqual(affected.Messages, []int{seed.SenderMessage.ID}) {
				t.Fatalf("result=%+v", object)
			}
		})
	}
}

func TestDeletePhoneCallHistoryRejectsUnauthenticatedBotsAndMissingService(t *testing.T) {
	router, _, _, a, _ := savedForwardFixture(t)
	if result, err := router.onMessagesDeletePhoneCallHistory(context.Background(), &tg.MessagesDeletePhoneCallHistoryRequest{}); result != nil || !tgerr.Is(err, "AUTH_KEY_UNREGISTERED") {
		t.Fatalf("unauthenticated result=%v err=%v", result, err)
	}
	router.deps.Users = &welcomeRPCUsers{user: domain.User{ID: a.ID, Bot: true}}
	if result, err := router.onMessagesDeletePhoneCallHistory(WithUserID(context.Background(), a.ID), &tg.MessagesDeletePhoneCallHistoryRequest{}); result != nil || !tgerr.Is(err, "BOT_METHOD_INVALID") {
		t.Fatalf("bot result=%v err=%v", result, err)
	}
	router.deps.Users = &welcomeRPCUsers{user: domain.User{ID: a.ID}}
	router.deps.Messages = nil
	if result, err := router.onMessagesDeletePhoneCallHistory(WithUserID(context.Background(), a.ID), &tg.MessagesDeletePhoneCallHistoryRequest{}); result != nil || err == nil {
		t.Fatalf("missing storage returned fake success: result=%v err=%v", result, err)
	}
}
