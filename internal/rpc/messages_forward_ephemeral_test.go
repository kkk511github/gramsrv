package rpc

import (
	"context"
	"testing"

	"github.com/iamxvbaba/td/bin"
	"github.com/iamxvbaba/td/tg"
	"github.com/iamxvbaba/td/tgerr"
	"github.com/iamxvbaba/td/tlprofile"
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
