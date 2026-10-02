package rpc

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/iamxvbaba/td/bin"
	"github.com/iamxvbaba/td/clock"
	"github.com/iamxvbaba/td/tg"
	"github.com/iamxvbaba/td/tlprofile"
	"go.uber.org/zap/zaptest"

	appchannels "telesrv/internal/app/channels"
	appusers "telesrv/internal/app/users"
	"telesrv/internal/domain"
	"telesrv/internal/store/memory"
)

func TestSafeLinkGroupPrivateChatForbiddenRPC(t *testing.T) {
	ctx := context.Background()
	userStore := memory.NewUserStore()
	owner, err := userStore.Create(ctx, domain.User{AccessHash: 7101, Phone: "15550007101", FirstName: "Owner"})
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	member, err := userStore.Create(ctx, domain.User{AccessHash: 7102, Phone: "15550007102", FirstName: "Member"})
	if err != nil {
		t.Fatalf("create member: %v", err)
	}
	channelStore := memory.NewChannelStore()
	channelService := appchannels.NewService(channelStore)
	r := New(Config{}, Deps{
		Users:    appusers.NewService(userStore),
		Channels: channelService,
	}, zaptest.NewLogger(t), clock.System)

	created, err := r.onMessagesCreateChat(WithUserID(ctx, owner.ID), &tg.MessagesCreateChatRequest{
		Users: []tg.InputUserClass{&tg.InputUser{UserID: member.ID, AccessHash: member.AccessHash}},
		Title: "Private Chat Policy",
	})
	if err != nil {
		t.Fatalf("create chat: %v", err)
	}
	channel := created.Updates.(*tg.Updates).Chats[0].(*tg.Channel)
	input := &tg.InputChannel{ChannelID: channel.ID, AccessHash: channel.AccessHash}

	if got := safeLinkGetPrivateChatForbidden(t, r, WithUserID(ctx, member.ID), input); got {
		t.Fatalf("initial private chat forbidden = true, want false")
	}

	if _, _, err := r.trySafeLinkGroupPrivateChatRPC(WithUserID(ctx, member.ID), safeLinkTogglePrivateChatBuffer(t, input, true), safeLinkToggleGroupPrivateChatForbiddenTypeID); err == nil || !strings.Contains(err.Error(), "CHAT_ADMIN_REQUIRED") {
		t.Fatalf("member toggle err = %v, want CHAT_ADMIN_REQUIRED", err)
	}

	enc, handled, err := r.trySafeLinkGroupPrivateChatRPC(WithUserID(ctx, owner.ID), safeLinkTogglePrivateChatBuffer(t, input, true), safeLinkToggleGroupPrivateChatForbiddenTypeID)
	if err != nil || !handled {
		t.Fatalf("owner toggle on handled=%v err=%v", handled, err)
	}
	updates, ok := enc.(*tg.Updates)
	if !ok {
		t.Fatalf("toggle response = %T, want *tg.Updates", enc)
	}
	if len(updates.Updates) == 0 {
		t.Fatalf("toggle response updates empty, want UpdateChannel refresh signal")
	}
	if refresh, ok := updates.Updates[0].(*tg.UpdateChannel); !ok || refresh.ChannelID != channel.ID {
		t.Fatalf("toggle response first update = %T %#v, want UpdateChannel(%d)", updates.Updates[0], updates.Updates[0], channel.ID)
	}
	if got := safeLinkGetPrivateChatForbidden(t, r, WithUserID(ctx, member.ID), input); !got {
		t.Fatalf("private chat forbidden after owner toggle = false, want true")
	}
	view, err := channelService.GetChannel(ctx, owner.ID, channel.ID)
	if err != nil {
		t.Fatalf("get channel after toggle: %v", err)
	}
	if !view.Channel.PrivateChatForbidden {
		t.Fatalf("stored private chat forbidden = false, want true")
	}

	if _, _, err := r.trySafeLinkGroupPrivateChatRPC(WithUserID(ctx, owner.ID), safeLinkTogglePrivateChatBuffer(t, input, false), safeLinkToggleGroupPrivateChatForbiddenTypeID); err != nil {
		t.Fatalf("owner toggle off: %v", err)
	}
	if got := safeLinkGetPrivateChatForbidden(t, r, WithUserID(ctx, member.ID), input); got {
		t.Fatalf("private chat forbidden after owner toggle off = true, want false")
	}
	for _, profile := range []tlprofile.Profile{tlprofile.Profile225, tlprofile.Profile226, tlprofile.Profile227, tlprofile.Profile228, tlprofile.Profile229} {
		for _, wrapped := range []bool{false, true} {
			t.Run(fmt.Sprintf("exact_%d_wrapped_%v", profile, wrapped), func(t *testing.T) {
				run := func(ctx context.Context, body *bin.Buffer) (tlprofile.Result, error) {
					wire := body.Raw()
					if wrapped {
						var envelope bin.Buffer
						if err := (&tg.InvokeWithLayerRequest{Layer: int(profile), Query: &rawObject{data: wire}}).Encode(&envelope); err != nil {
							t.Fatal(err)
						}
						wire = envelope.Raw()
					}
					buffer := &bin.Buffer{Buf: wire}
					var admitted tlprofile.Admission
					var err error
					if wrapped {
						admitted, err = r.AdmitUnprofiled(buffer, tlprofile.Limits{})
					} else {
						admitted, err = r.AdmitDefaultLayer(profile, buffer, tlprofile.Limits{})
					}
					if err != nil {
						t.Fatalf("production admission: %v", err)
					}
					if buffer.Len() != 0 {
						t.Fatal("unconsumed request bytes")
					}
					result, method, err := r.DispatchAdmitted(ctx, [8]byte{}, 0, 0, 0, admitted)
					if !strings.HasPrefix(method, "safelink.") {
						t.Fatalf("unexpected method %s", method)
					}
					return result, err
				}
				for _, enabled := range []bool{true, false} {
					if _, err := run(WithUserID(ctx, member.ID), safeLinkTogglePrivateChatBuffer(t, input, enabled)); err == nil || !strings.Contains(err.Error(), "CHAT_ADMIN_REQUIRED") {
						t.Fatalf("member toggle: %v", err)
					}
					result, err := run(WithUserID(ctx, owner.ID), safeLinkTogglePrivateChatBuffer(t, input, enabled))
					if err != nil {
						t.Fatal(err)
					}
					var encoded bin.Buffer
					if err := result.Encode(&encoded); err != nil {
						t.Fatalf("encode exact updates: %v", err)
					}
					if id, _ := encoded.PeekID(); id != tg.UpdatesTypeID {
						t.Fatalf("result id %x", id)
					}
					get := &bin.Buffer{}
					get.PutID(safeLinkGetGroupPrivateChatForbiddenTypeID)
					if err := input.Encode(get); err != nil {
						t.Fatal(err)
					}
					result, err = run(WithUserID(ctx, member.ID), get)
					if err != nil {
						t.Fatal(err)
					}
					encoded = bin.Buffer{}
					if err := result.Encode(&encoded); err != nil {
						t.Fatal(err)
					}
					value, err := tg.DecodeBool(&encoded)
					if err != nil {
						t.Fatal(err)
					}
					_, actual := value.(*tg.BoolTrue)
					if actual != enabled || encoded.Len() != 0 {
						t.Fatalf("readback=%v want=%v", actual, enabled)
					}
				}
			})
		}
	}
	t.Run("admin_hidden_members", func(t *testing.T) {
		// Warm the same views clients already opened before the operator edits the group.
		for _, enabled := range []bool{true, false} {
			if _, err := r.onChannelsGetFullChannel(WithUserID(ctx, member.ID), input); err != nil {
				t.Fatal(err)
			}
			updated, err := channelService.AdminSetSettings(ctx, channel.ID, domain.ChannelAdminSettings{ParticipantsHidden: &enabled})
			if err != nil {
				t.Fatal(err)
			}
			if err := r.NotifyChannelChanged(ctx, updated); err != nil {
				t.Fatal(err)
			}
			for _, viewer := range []int64{owner.ID, member.ID} {
				full, err := r.onChannelsGetFullChannel(WithUserID(ctx, viewer), input)
				if err != nil {
					t.Fatal(err)
				}
				info := full.FullChat.(*tg.ChannelFull)
				wantVisible := viewer == owner.ID || !enabled
				if info.ParticipantsHidden != enabled || info.CanViewParticipants != wantVisible {
					t.Fatalf("viewer %d hidden=%v visible=%v, want hidden=%v visible=%v", viewer, info.ParticipantsHidden, info.CanViewParticipants, enabled, wantVisible)
				}
				participants, err := r.onChannelsGetParticipants(WithUserID(ctx, viewer), &tg.ChannelsGetParticipantsRequest{Channel: input, Filter: &tg.ChannelParticipantsRecent{}, Limit: 100})
				if err != nil {
					t.Fatal(err)
				}
				count := len(participants.(*tg.ChannelsChannelParticipants).Participants)
				if (count > 0) != wantVisible {
					t.Fatalf("viewer %d returned %d members, visible=%v", viewer, count, wantVisible)
				}
			}
		}
	})
}

func TestSafeLinkPrivateChatExactAdmissionRejectsMalformed(t *testing.T) {
	r := New(Config{}, Deps{}, zaptest.NewLogger(t), clock.System)
	valid := safeLinkTogglePrivateChatBuffer(t, &tg.InputChannel{ChannelID: 1, AccessHash: 2}, true).Raw()
	for _, data := range [][]byte{valid[:len(valid)-1], append(append([]byte(nil), valid...), 0, 0, 0, 0), append(append([]byte(nil), valid[:len(valid)-4]...), 0, 0, 0, 0)} {
		original := append([]byte(nil), data...)
		body := &bin.Buffer{Buf: data}
		if _, err := r.AdmitLayer(tlprofile.Profile227, body, tlprofile.Limits{}); err == nil {
			t.Fatal("malformed private RPC accepted")
		}
		if !bytes.Equal(body.Raw(), original) {
			t.Fatal("failed admission consumed input")
		}
	}
	body := &bin.Buffer{Buf: valid}
	if _, err := r.AdmitLayer(tlprofile.Profile227, body, tlprofile.Limits{MaxWireBytes: len(valid) - 1}); err == nil {
		t.Fatal("request size limit bypassed")
	}
}

func TestSafeLinkPrivateChatExactAdmissionRequiresLogin(t *testing.T) {
	r := New(Config{}, Deps{Auth: &captureAuthService{}}, zaptest.NewLogger(t), clock.System)
	channel := &tg.InputChannel{ChannelID: 1, AccessHash: 2}
	get := &bin.Buffer{}
	get.PutID(safeLinkGetGroupPrivateChatForbiddenTypeID)
	if err := channel.Encode(get); err != nil {
		t.Fatal(err)
	}
	for _, body := range []*bin.Buffer{get, safeLinkTogglePrivateChatBuffer(t, channel, true)} {
		admitted, err := r.AdmitDefaultLayer(tlprofile.Profile227, body, tlprofile.Limits{})
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = r.DispatchAdmitted(context.Background(), [8]byte{}, 0, 0, 0, admitted)
		if err == nil || !strings.Contains(err.Error(), "AUTH_KEY_UNREGISTERED") {
			t.Fatalf("guest private RPC: %v", err)
		}
	}
}

func safeLinkGetPrivateChatForbidden(t *testing.T, r *Router, ctx context.Context, channel tg.InputChannelClass) bool {
	t.Helper()
	b := &bin.Buffer{}
	b.PutID(safeLinkGetGroupPrivateChatForbiddenTypeID)
	if err := channel.Encode(b); err != nil {
		t.Fatalf("encode get channel: %v", err)
	}
	enc, handled, err := r.trySafeLinkGroupPrivateChatRPC(ctx, b, safeLinkGetGroupPrivateChatForbiddenTypeID)
	if err != nil || !handled {
		t.Fatalf("get private chat forbidden handled=%v err=%v", handled, err)
	}
	_, ok := enc.(*tg.BoolTrue)
	return ok
}

func safeLinkTogglePrivateChatBuffer(t *testing.T, channel tg.InputChannelClass, enabled bool) *bin.Buffer {
	t.Helper()
	b := &bin.Buffer{}
	b.PutID(safeLinkToggleGroupPrivateChatForbiddenTypeID)
	if err := channel.Encode(b); err != nil {
		t.Fatalf("encode toggle channel: %v", err)
	}
	var boolValue tg.BoolClass = &tg.BoolFalse{}
	if enabled {
		boolValue = &tg.BoolTrue{}
	}
	if err := boolValue.Encode(b); err != nil {
		t.Fatalf("encode toggle enabled: %v", err)
	}
	return b
}
