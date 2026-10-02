package rpc

import (
	"context"
	"testing"

	"github.com/iamxvbaba/td/bin"
	"github.com/iamxvbaba/td/clock"
	"github.com/iamxvbaba/td/tg"
	"github.com/iamxvbaba/td/tlprofile"
	"go.uber.org/zap/zaptest"
	appcontacts "telesrv/internal/app/contacts"
	appstories "telesrv/internal/app/stories"
	appupdates "telesrv/internal/app/updates"
	appusers "telesrv/internal/app/users"
	"telesrv/internal/domain"
	"telesrv/internal/store/memory"
)

func TestSavedContactSafetyPromptIsIndependentOfBlockedState(t *testing.T) {
	ctx := context.Background()
	users := memory.NewUserStore()
	owner, err := users.Create(ctx, domain.User{AccessHash: 11, FirstName: "Owner"})
	if err != nil {
		t.Fatal(err)
	}
	peer, err := users.Create(ctx, domain.User{AccessHash: 22, FirstName: "Peer"})
	if err != nil {
		t.Fatal(err)
	}
	contacts := memory.NewContactStore()
	stories := memory.NewStoryStore()
	events := memory.NewUpdateEventStore()
	rpcTestBlocklistStores(contacts, users, stories, events)
	if _, err := contacts.Upsert(ctx, owner.ID, domain.ContactInput{ContactUserID: peer.ID, FirstName: "Saved"}); err != nil {
		t.Fatal(err)
	}
	r := New(Config{}, Deps{Users: appusers.NewService(users), Contacts: appcontacts.NewService(contacts, users), Stories: appstories.NewService(stories), Updates: appupdates.NewService(memory.NewUpdateStateStore(), events)}, zaptest.NewLogger(t), clock.System)
	ctx = WithUserID(ctx, owner.ID)
	input := &tg.InputPeerUser{UserID: peer.ID, AccessHash: peer.AccessHash}
	assertState := func(wantBlocked bool) {
		t.Helper()
		// Read twice to exercise both cold and cached peer/full-user projections.
		for attempt := 0; attempt < 2; attempt++ {
			full, err := r.onUsersGetFullUser(ctx, &tg.InputUser{UserID: peer.ID, AccessHash: peer.AccessHash})
			if err != nil {
				t.Fatal(err)
			}
			for profile := tlprofile.Profile225; profile <= tlprofile.Profile228; profile++ {
				var wire bin.Buffer
				if err := tlprofile.EncodeObject(profile, full, &wire); err != nil {
					t.Fatal(err)
				}
				decoded, err := tlprofile.DecodeObject(profile, &wire, tlprofile.Limits{})
				if err != nil {
					t.Fatal(err)
				}
				view := decoded.(*tg.UsersUserFull).FullUser
				if view.Blocked != wantBlocked || view.BlockedMyStoriesFrom != wantBlocked || view.Settings.BlockContact {
					t.Fatalf("layer=%v blocked=%v stories=%v prompt=%v; want blocked=%v and no saved-contact prompt", profile, view.Blocked, view.BlockedMyStoriesFrom, view.Settings.BlockContact, wantBlocked)
				}
			}
		}
		list, err := r.onContactsGetBlocked(ctx, &tg.ContactsGetBlockedRequest{Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		if (len(list.(*tg.ContactsBlocked).Blocked) == 1) != wantBlocked {
			t.Fatalf("blacklist and profile disagree: %+v", list)
		}
	}
	assertState(false)
	if ok, err := r.onContactsBlock(ctx, &tg.ContactsBlockRequest{ID: input}); err != nil || !ok {
		t.Fatalf("block: %v %v", ok, err)
	}
	assertState(true)
	if ok, err := r.onContactsUnblock(ctx, &tg.ContactsUnblockRequest{ID: input}); err != nil || !ok {
		t.Fatalf("unblock: %v %v", ok, err)
	}
	assertState(false)
	deliveries, err := events.ListAfter(ctx, owner.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	settingsEvents := 0
	for _, event := range deliveries {
		if event.Type == domain.UpdateEventPeerSettings {
			settingsEvents++
			if event.Settings.BlockContact {
				t.Fatal("block/unblock update reintroduced saved-contact link protection")
			}
		}
	}
	if settingsEvents < 2 {
		t.Fatalf("missing block/unblock settings updates: %d", settingsEvents)
	}
}
