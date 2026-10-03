package rpc

import (
	"bytes"
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/iamxvbaba/td/bin"
	"github.com/iamxvbaba/td/tg"
	"github.com/iamxvbaba/td/tgerr"
	"github.com/iamxvbaba/td/tlprofile"

	"telesrv/internal/domain"
	"telesrv/internal/store/memory"
)

func TestForwardEphemeralDoesNotResolveOrdinaryHistory(t *testing.T) {
	request := &tg.MessagesForwardMessagesRequest{
		FromEphemeral: true,
		FromPeer:      &tg.InputPeerSelf{},
		ToPeer:        &tg.InputPeerSelf{},
		ID:            []int{1},
		RandomID:      []int64{123},
	}
	var wire bin.Buffer
	if err := tlprofile.EncodeObject(tlprofile.Profile229, request, &wire); err != nil {
		t.Fatal(err)
	}
	object, err := tlprofile.DecodeObject(tlprofile.Profile229, &wire, tlprofile.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	decoded, ok := object.(*tg.MessagesForwardMessagesRequest)
	if !ok {
		t.Fatalf("unexpected decoded request: %T", object)
	}
	if !decoded.FromEphemeral {
		t.Fatal("Layer 229 lost the ephemeral source flag")
	}
	// No dependencies: the guard must run before authentication, replay lookup or history reads.
	r := &Router{}
	result, err := r.onMessagesForwardMessages(context.Background(), decoded)
	if result != nil || !tgerr.Is(err, "MESSAGE_ID_INVALID") {
		t.Fatalf("ephemeral ID could reach ordinary lookup: %v, %v", result, err)
	}
}

func TestForwardEphemeralRejectsSourceModesBeforeLookup(t *testing.T) {
	for _, test := range []struct {
		name     string
		from     tg.InputPeerClass
		to       tg.InputPeerClass
		schedule int
	}{
		{"self", &tg.InputPeerSelf{}, &tg.InputPeerSelf{}, 0},
		{"inferred", &tg.InputPeerEmpty{}, &tg.InputPeerSelf{}, 0},
		{"private", &tg.InputPeerUser{UserID: 9, AccessHash: 10}, &tg.InputPeerSelf{}, 0},
		{"channel", &tg.InputPeerChannel{ChannelID: 77, AccessHash: 88}, &tg.InputPeerSelf{}, 0},
		{"channel-target", &tg.InputPeerChannel{ChannelID: 77, AccessHash: 88}, &tg.InputPeerChannel{ChannelID: 99, AccessHash: 100}, 0},
		{"scheduled", &tg.InputPeerChannel{ChannelID: 77, AccessHash: 88}, &tg.InputPeerSelf{}, int(time.Now().Add(time.Hour).Unix())},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := &tg.MessagesForwardMessagesRequest{
				FromEphemeral: true, DropAuthor: true, DropMediaCaptions: true,
				FromPeer: test.from, ToPeer: test.to, ScheduleDate: test.schedule,
				ID: []int{1}, RandomID: []int64{123},
			}
			// No services/clock: reaching replay, source lookup, scheduling or
			// monoforum dispatch would fail instead of returning the guard error.
			result, err := (&Router{}).onMessagesForwardMessages(context.Background(), request)
			if result != nil || !tgerr.Is(err, "MESSAGE_ID_INVALID") {
				t.Fatalf("result=%v err=%v", result, err)
			}
		})
	}
}

func TestForwardEphemeralDoesNotReplayOrdinaryForward(t *testing.T) {
	router, messages, events, user, other := savedForwardFixture(t)
	ctx := WithUserID(context.Background(), user.ID)
	seed, err := messages.SendPrivateText(ctx, domain.SendPrivateTextRequest{
		SenderUserID: user.ID, RecipientUserID: user.ID, RandomID: 1,
		Message: "ordinary history must not substitute for transient content", Date: 1700000000,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := &tg.MessagesForwardMessagesRequest{
		FromPeer: &tg.InputPeerSelf{}, ToPeer: &tg.InputPeerUser{UserID: other.ID, AccessHash: other.AccessHash},
		ID: []int{seed.SenderMessage.ID}, RandomID: []int64{123},
	}
	if _, err := router.onMessagesForwardMessages(ctx, request); err != nil {
		t.Fatal(err)
	}
	ordinaryFingerprint, err := forwardMessagesItemIdempotencyFingerprint(request, request.ID[0], request.RandomID[0])
	if err != nil {
		t.Fatal(err)
	}
	request.FromEphemeral = true
	ephemeralFingerprint, err := forwardMessagesItemIdempotencyFingerprint(request, request.ID[0], request.RandomID[0])
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(ordinaryFingerprint, ephemeralFingerprint) {
		t.Fatal("forward replay fingerprint lost the source namespace")
	}
	before := savedForwardEvents(t, events, user.ID, other.ID)
	for _, randomID := range []int64{123, 124} {
		request.RandomID = []int64{randomID}
		result, err := router.onMessagesForwardMessages(ctx, request)
		if result != nil || !tgerr.Is(err, "MESSAGE_ID_INVALID") {
			t.Fatalf("ordinary ID/replay substituted: result=%v err=%v", result, err)
		}
		if !reflect.DeepEqual(before, savedForwardEvents(t, events, user.ID, other.ID)) {
			t.Fatal("rejected transient forward wrote ordinary history/update events")
		}
	}
}

func TestForwardEphemeralSourceKindsHaveIndistinguishableWireIDs(t *testing.T) {
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)
	peer := domain.Peer{Type: domain.PeerTypeChannel, ID: 77}
	botMessage := domain.EphemeralMessage{
		ID: 1, Peer: peer, SenderUserID: 20, ReceiverUserID: 9,
		Date: int(now.Unix()), RandomID: 100, Version: 1, PayloadHash: [32]byte{1},
		Content:      domain.EphemeralContent{Message: "private bot reply"},
		OriginDevice: domain.EphemeralDevice{UserID: 9, BusinessAuthKeyID: [8]byte{1}, SessionID: 2},
		CreatedAt:    now, ExpiresAt: now.Add(domain.EphemeralMessageRetention),
	}
	stored, _, err := memory.NewEphemeralMessageStore().CreateEphemeralMessage(ctx, botMessage)
	if err != nil {
		t.Fatal(err)
	}
	template := domain.WelcomeMessage{
		ID: 1, Peer: peer, CreatorUserID: 9, Date: int(now.Unix()), RandomID: 101,
		Content: domain.WelcomeMessageContent{Message: "admin template", NoForwards: true},
		Version: 1, CreateFingerprint: [32]byte{2},
	}
	if err := template.ValidateStored(); err != nil {
		t.Fatal(err)
	}
	delivery := domain.WelcomeMessageDelivery{
		ID: 1, JoinEventID: 1, ChannelID: peer.ID, TargetUserID: 9,
		TemplateID: template.ID, EphemeralID: 1, JoinedAt: int(now.Unix()),
		Content:      domain.WelcomeMessageContent{Message: "frozen join greeting"},
		AttemptCount: 1, ExpiresAt: now.Add(domain.WelcomeMessageDeliveryTTL),
	}
	if err := delivery.ValidateStored(now); err != nil {
		t.Fatal(err)
	}
	// All three valid namespaces admit ID 1 for the same peer/user. Source
	// sender, template flag, device and join epoch are absent from forwarding.
	var first []byte
	for _, id := range []int{stored.ID, template.ID, delivery.EphemeralID} {
		request := &tg.MessagesForwardMessagesRequest{
			FromEphemeral: true, FromPeer: &tg.InputPeerChannel{ChannelID: peer.ID, AccessHash: 88},
			ToPeer: &tg.InputPeerSelf{}, ID: []int{id}, RandomID: []int64{123},
		}
		var wire bin.Buffer
		if err := tlprofile.EncodeObject(tlprofile.Profile229, request, &wire); err != nil {
			t.Fatal(err)
		}
		if first == nil {
			first = append([]byte(nil), wire.Buf...)
		} else if !bytes.Equal(first, wire.Buf) {
			t.Fatal("source kinds unexpectedly have different forward wire identities")
		}
		result, err := (&Router{}).onMessagesForwardMessages(ctx, request)
		if result != nil || !tgerr.Is(err, "MESSAGE_ID_INVALID") {
			t.Fatalf("ambiguous source accepted: result=%v err=%v", result, err)
		}
	}
}
