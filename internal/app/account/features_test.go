package account

import (
	"context"
	"errors"
	"testing"

	"telesrv/internal/domain"
	"telesrv/internal/store/memory"
)

func TestAccountFeatureServicePersistsAndValidatesOwner(t *testing.T) {
	ctx := context.Background()
	users := memory.NewUserStore()
	owner, err := users.Create(ctx, domain.User{Phone: "+1888222", FirstName: "Owner"})
	if err != nil {
		t.Fatal(err)
	}
	bot, err := users.Create(ctx, domain.User{Phone: "+1888223", FirstName: "Bot", Bot: true})
	if err != nil {
		t.Fatal(err)
	}
	state := memory.NewPasswordStore()
	svc := NewService(state, WithUsers(users))
	if err := svc.SetMainProfileTab(ctx, owner.ID, domain.ProfileTabMusic); err != nil {
		t.Fatal(err)
	}
	otherSvc := NewService(state, WithUsers(users))
	if tab, err := otherSvc.GetMainProfileTab(ctx, owner.ID); err != nil || tab != domain.ProfileTabMusic {
		t.Fatalf("persisted = %s/%v", tab, err)
	}
	if err := svc.SetMainProfileTab(ctx, bot.ID, domain.ProfileTabGifts); !errors.Is(err, domain.ErrAccountFeatureOwnerInvalid) {
		t.Fatalf("bot owner = %v", err)
	}
	if err := svc.SetMainProfileTab(ctx, owner.ID+999, domain.ProfileTabGifts); !errors.Is(err, domain.ErrAccountFeatureOwnerInvalid) {
		t.Fatalf("missing owner = %v", err)
	}
	if err := svc.SetMainProfileTab(ctx, owner.ID, "unknown"); !errors.Is(err, domain.ErrProfileTabInvalid) {
		t.Fatalf("invalid enum = %v", err)
	}
	if err := svc.ConfirmBotConnection(ctx, owner.ID, bot.ID, 123); !errors.Is(err, domain.ErrBotBusinessMissing) {
		t.Fatalf("arbitrary bot created connection = %v", err)
	}
	if _, found, _ := state.GetConnectedBusinessBot(ctx, owner.ID); found {
		t.Fatal("confirmation created a connection")
	}
	if err := NewService(state).SetMainProfileTab(ctx, owner.ID, domain.ProfileTabPosts); !errors.Is(err, domain.ErrAccountFeatureUnavailable) {
		t.Fatalf("missing auth dependency = %v", err)
	}
}
