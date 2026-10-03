package postgres

import (
	"context"
	"errors"
	"testing"

	"telesrv/internal/domain"
)

func TestAccountFeaturesPostgresPersistenceAndOwnership(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	users := NewUserStore(pool)
	owner, err := users.Create(ctx, domain.User{Phone: "+1884" + randomSuffix(t), FirstName: "FeatureOwner"})
	if err != nil {
		t.Fatal(err)
	}
	bot, err := users.Create(ctx, domain.User{Phone: "+1885" + randomSuffix(t), FirstName: "FeatureBot", Bot: true})
	if err != nil {
		t.Fatal(err)
	}
	otherBot, err := users.Create(ctx, domain.User{Phone: "+1886" + randomSuffix(t), FirstName: "OtherBot", Bot: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM business_connected_bots WHERE owner_user_id = $1", owner.ID)
		_, _ = pool.Exec(ctx, "DELETE FROM users WHERE id = $1", owner.ID)
		_, _ = pool.Exec(ctx, "DELETE FROM users WHERE id = $1", bot.ID)
		_, _ = pool.Exec(ctx, "DELETE FROM users WHERE id = $1", otherBot.ID)
	})
	st := NewPasswordStore(pool)
	if err := st.SetMainProfileTab(ctx, owner.ID, domain.ProfileTabGifts); err != nil {
		t.Fatal(err)
	}
	fresh := NewPasswordStore(pool)
	if tab, found, err := fresh.GetMainProfileTab(ctx, owner.ID); err != nil || !found || tab != domain.ProfileTabGifts {
		t.Fatalf("persisted tab = %s/%v/%v", tab, found, err)
	}
	if err := fresh.SetMainProfileTab(ctx, owner.ID, "unknown"); !errors.Is(err, domain.ErrProfileTabInvalid) {
		t.Fatalf("invalid tab = %v", err)
	}
	if err := st.ConfirmBotConnection(ctx, owner.ID, bot.ID, 123); !errors.Is(err, domain.ErrBotBusinessMissing) {
		t.Fatalf("arbitrary connection = %v", err)
	}
	if _, err := st.SaveConnectedBusinessBot(ctx, domain.ConnectedBusinessBot{OwnerUserID: owner.ID, BotUserID: bot.ID, Rights: domain.BusinessBotRights{Reply: true}}); err != nil {
		t.Fatal(err)
	}
	if err := st.ConfirmBotConnection(ctx, bot.ID, bot.ID, 123); !errors.Is(err, domain.ErrBotBusinessMissing) {
		t.Fatalf("wrong owner = %v", err)
	}
	if err := st.ConfirmBotConnection(ctx, owner.ID, otherBot.ID, 123); !errors.Is(err, domain.ErrBotBusinessMissing) {
		t.Fatalf("wrong bot = %v", err)
	}
	if err := st.ConfirmBotConnection(ctx, owner.ID, bot.ID, 123); err != nil {
		t.Fatal(err)
	}
	if err := fresh.ConfirmBotConnection(ctx, owner.ID, bot.ID, 456); err != nil {
		t.Fatal(err)
	}
	connection, found, err := fresh.GetConnectedBusinessBot(ctx, owner.ID)
	if err != nil || !found || connection.ConfirmedAtUnix != 123 || !connection.Rights.Reply {
		t.Fatalf("confirmed connection = %#v/%v/%v", connection, found, err)
	}
	if _, err := fresh.SaveConnectedBusinessBot(ctx, connection); err != nil {
		t.Fatal(err)
	}
	connection, _, err = fresh.GetConnectedBusinessBot(ctx, owner.ID)
	if err != nil || connection.ConfirmedAtUnix != 0 {
		t.Fatalf("new review retained confirmation = %#v/%v", connection, err)
	}
}
