package contacts

import (
	"context"
	"testing"

	"telesrv/internal/domain"
	"telesrv/internal/store"
	"telesrv/internal/store/memory"
)

func TestPeerSettingsBlockPromptOnlyForUnknownUnblockedPeer(t *testing.T) {
	for _, tc := range []struct {
		name                                 string
		contact, mutual, blocked, wantPrompt bool
	}{
		{"stranger", false, false, false, true},
		{"saved contact", true, false, false, false},
		{"mutual contact", true, true, false, false},
		{"blocked stranger", false, false, true, false},
		{"blocked contact", true, true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			users := memory.NewUserStore()
			owner, err := users.Create(ctx, domain.User{FirstName: "Owner"})
			if err != nil {
				t.Fatal(err)
			}
			peer, err := users.Create(ctx, domain.User{FirstName: "Peer"})
			if err != nil {
				t.Fatal(err)
			}
			contacts := memory.NewContactStore()
			if tc.contact {
				if _, err := contacts.Upsert(ctx, owner.ID, domain.ContactInput{ContactUserID: peer.ID, FirstName: "Saved"}); err != nil {
					t.Fatal(err)
				}
			}
			if tc.mutual {
				if _, err := contacts.Upsert(ctx, peer.ID, domain.ContactInput{ContactUserID: owner.ID, FirstName: "Saved"}); err != nil {
					t.Fatal(err)
				}
			}
			if tc.blocked {
				contacts.AttachBlocklistStores(memory.NewStoryStore(), memory.NewPrivacyStore(), users, memory.NewChannelStore())
				if _, err := contacts.MutateBlocklist(ctx, store.BlocklistMutation{Kind: store.BlocklistBlock, OwnerUserID: owner.ID, PeerIDs: []int64{peer.ID}, Date: contactMutationTestDate}, store.BlocklistDeliveryEffects); err != nil {
					t.Fatal(err)
				}
			}
			settings, err := NewService(contacts, users).GetPeerSettings(ctx, owner.ID, domain.Peer{Type: domain.PeerTypeUser, ID: peer.ID})
			if err != nil {
				t.Fatal(err)
			}
			if settings.BlockContact != tc.wantPrompt {
				t.Fatalf("BlockContact=%v, want %v; saved contacts must not lose incoming link actions", settings.BlockContact, tc.wantPrompt)
			}
			if settings.AddContact == tc.contact {
				t.Fatalf("AddContact=%v for saved=%v", settings.AddContact, tc.contact)
			}
			blocked, err := contacts.IsBlocked(ctx, owner.ID, peer.ID)
			if err != nil || blocked != tc.blocked {
				t.Fatalf("block relationship changed: %v %v", blocked, err)
			}
		})
	}
}
