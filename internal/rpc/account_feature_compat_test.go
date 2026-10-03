package rpc

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/iamxvbaba/td/bin"
	"github.com/iamxvbaba/td/clock"
	"github.com/iamxvbaba/td/tg"
	"github.com/iamxvbaba/td/tgerr"
	"github.com/iamxvbaba/td/tlprofile"
	"go.uber.org/zap/zaptest"
	appaccount "telesrv/internal/app/account"
	appusers "telesrv/internal/app/users"
	"telesrv/internal/domain"
	"telesrv/internal/store/memory"
)

type accountFeatureFixture struct {
	router                      *Router
	users                       *memory.UserStore
	state                       *memory.PasswordStore
	owner, other, bot, otherBot domain.User
}

func newAccountFeatureFixture(t *testing.T) accountFeatureFixture {
	t.Helper()
	ctx := context.Background()
	f := accountFeatureFixture{users: memory.NewUserStore(), state: memory.NewPasswordStore()}
	for i, target := range []*domain.User{&f.owner, &f.other, &f.bot, &f.otherBot} {
		user, err := f.users.Create(ctx, domain.User{Phone: fmt.Sprintf("+1999000%d", i), AccessHash: int64(1200 + i), FirstName: fmt.Sprintf("Feature%d", i), Bot: i >= 2})
		if err != nil {
			t.Fatal(err)
		}
		*target = user
	}
	f.router = f.newRouter(t)
	return f
}

func (f accountFeatureFixture) newRouter(t *testing.T) *Router {
	return New(Config{}, Deps{Users: appusers.NewService(f.users), Account: appaccount.NewService(f.state, appaccount.WithUsers(f.users), appaccount.WithBusinessAutomation(f.state), appaccount.WithAccountSettings(f.state))}, zaptest.NewLogger(t), clock.System)
}

func standardProfileTabs() []tg.ProfileTabClass {
	return []tg.ProfileTabClass{&tg.ProfileTabPosts{}, &tg.ProfileTabGifts{}, &tg.ProfileTabMedia{}, &tg.ProfileTabFiles{}, &tg.ProfileTabMusic{}, &tg.ProfileTabVoice{}, &tg.ProfileTabLinks{}, &tg.ProfileTabGifs{}}
}

func TestAccountMainProfileTabPersistedAndProjectedAfterCache(t *testing.T) {
	f := newAccountFeatureFixture(t)
	ctx := WithUserID(context.Background(), f.owner.ID)
	viewerCtx := WithUserID(context.Background(), f.other.ID)
	input := inputUser(f.owner)
	// Populate both self and other-viewer caches before the first write.
	for _, c := range []context.Context{ctx, viewerCtx} {
		if _, err := f.router.onUsersGetFullUser(c, input); err != nil {
			t.Fatal(err)
		}
	}
	for _, tab := range standardProfileTabs() {
		if ok, err := f.router.onAccountSetMainProfileTab(ctx, &tg.AccountSetMainProfileTabRequest{Tab: tab}); err != nil || !ok {
			t.Fatalf("set %T = %v/%v", tab, ok, err)
		}
		fresh := f.newRouter(t)
		for _, c := range []context.Context{ctx, viewerCtx} {
			full, err := fresh.onUsersGetFullUser(c, input)
			if err != nil {
				t.Fatal(err)
			}
			got, found := full.FullUser.GetMainTab()
			if !found || reflect.TypeOf(got) != reflect.TypeOf(tab) {
				t.Fatalf("profile = %T/%v, want %T", got, found, tab)
			}
		}
	}
	// A write through another router must be visible on the already cached one.
	otherRouter := f.newRouter(t)
	if _, err := f.router.onUsersGetFullUser(viewerCtx, input); err != nil {
		t.Fatal(err)
	}
	if _, err := otherRouter.onAccountSetMainProfileTab(ctx, &tg.AccountSetMainProfileTabRequest{Tab: &tg.ProfileTabPosts{}}); err != nil {
		t.Fatal(err)
	}
	full, err := f.router.onUsersGetFullUser(viewerCtx, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := full.FullUser.MainTab.(*tg.ProfileTabPosts); !ok {
		t.Fatalf("stale cached tab = %T", full.FullUser.MainTab)
	}
	if _, found, err := f.state.GetMainProfileTab(ctx, f.other.ID); err != nil || found {
		t.Fatalf("other owner's tab changed: %v/%v", found, err)
	}
}

func TestAccountConfirmBotConnectionOwnershipAndPersistence(t *testing.T) {
	f := newAccountFeatureFixture(t)
	ctx := WithUserID(context.Background(), f.owner.ID)
	want := domain.ConnectedBusinessBot{OwnerUserID: f.owner.ID, BotUserID: f.bot.ID, Rights: domain.BusinessBotRights{Reply: true, ReadMessages: true}, Recipients: domain.BusinessBotRecipients{Users: []int64{f.other.ID}}, CreatedAtUnix: 100, UpdatedAtUnix: 200}
	if _, err := f.state.SaveConnectedBusinessBot(ctx, want); err != nil {
		t.Fatal(err)
	}
	f.router.clock = fixedClock{now: time.Unix(300, 0)}
	req := &tg.AccountConfirmBotConnectionRequest{BotID: inputUser(f.bot)}
	if ok, err := f.router.onAccountConfirmBotConnection(ctx, req); err != nil || !ok {
		t.Fatalf("confirm = %v/%v", ok, err)
	}
	got, found, err := f.state.GetConnectedBusinessBot(ctx, f.owner.ID)
	if err != nil || !found {
		t.Fatal(err)
	}
	want.ConfirmedAtUnix = 300
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("confirmation changed permissions: %#v", got)
	}
	fresh := f.newRouter(t)
	fresh.clock = fixedClock{now: time.Unix(400, 0)}
	if ok, err := fresh.onAccountConfirmBotConnection(ctx, req); err != nil || !ok {
		t.Fatalf("idempotent confirm = %v/%v", ok, err)
	}
	got, _, _ = f.state.GetConnectedBusinessBot(ctx, f.owner.ID)
	if got.ConfirmedAtUnix != 300 {
		t.Fatal("retry changed confirmation time")
	}
	for _, test := range []struct {
		ctx context.Context
		bot tg.InputUserClass
	}{
		{WithUserID(context.Background(), f.other.ID), inputUser(f.bot)},
		{ctx, inputUser(f.otherBot)},
		{ctx, inputUser(f.other)},
		{ctx, &tg.InputUser{UserID: f.bot.ID, AccessHash: f.bot.AccessHash + 1}},
	} {
		if ok, err := f.router.onAccountConfirmBotConnection(test.ctx, &tg.AccountConfirmBotConnectionRequest{BotID: test.bot}); ok || !tgerr.Is(err, "BOT_BUSINESS_MISSING") {
			t.Fatalf("unowned/bad bot = %v/%v", ok, err)
		}
	}
	got, _, _ = f.state.GetConnectedBusinessBot(ctx, f.owner.ID)
	if !reflect.DeepEqual(got, want) {
		t.Fatal("rejected confirmation mutated connection")
	}
	got.BotUserID = f.otherBot.ID
	if _, err := f.state.SaveConnectedBusinessBot(ctx, got); err != nil {
		t.Fatal(err)
	}
	got, _, _ = f.state.GetConnectedBusinessBot(ctx, f.owner.ID)
	if got.ConfirmedAtUnix != 0 {
		t.Fatal("replacement inherited old confirmation")
	}
	if ok, err := f.router.onAccountConfirmBotConnection(ctx, req); ok || !tgerr.Is(err, "BOT_BUSINESS_MISSING") {
		t.Fatalf("replaced connection = %v/%v", ok, err)
	}
	if _, err := f.state.DeleteConnectedBusinessBot(ctx, f.owner.ID, f.otherBot.ID); err != nil {
		t.Fatal(err)
	}
	if ok, err := f.router.onAccountConfirmBotConnection(ctx, &tg.AccountConfirmBotConnectionRequest{BotID: inputUser(f.otherBot)}); ok || !tgerr.Is(err, "BOT_BUSINESS_MISSING") {
		t.Fatalf("removed connection = %v/%v", ok, err)
	}
}

func TestAccountFeatureAuthenticationAndInvalidInputs(t *testing.T) {
	f := newAccountFeatureFixture(t)
	for _, test := range []struct {
		ctx context.Context
		rpc string
	}{
		{context.Background(), "AUTH_KEY_UNREGISTERED"},
		{WithUserID(context.Background(), f.bot.ID), "BOT_METHOD_INVALID"},
	} {
		if _, err := f.router.onAccountSetMainProfileTab(test.ctx, &tg.AccountSetMainProfileTabRequest{Tab: &tg.ProfileTabPosts{}}); !tgerr.Is(err, test.rpc) {
			t.Fatalf("tab auth = %v", err)
		}
		if _, err := f.router.onAccountConfirmBotConnection(test.ctx, &tg.AccountConfirmBotConnectionRequest{BotID: inputUser(f.bot)}); !tgerr.Is(err, test.rpc) {
			t.Fatalf("bot auth = %v", err)
		}
	}
	ctx := WithUserID(context.Background(), f.owner.ID)
	var nilTab *tg.ProfileTabPosts
	for _, tab := range []tg.ProfileTabClass{nil, nilTab} {
		if _, err := f.router.onAccountSetMainProfileTab(ctx, &tg.AccountSetMainProfileTabRequest{Tab: tab}); !tgerr.Is(err, "INPUT_CONSTRUCTOR_INVALID") {
			t.Fatalf("invalid tab = %v", err)
		}
	}
	f.router.deps.Account = nil
	if ok, err := f.router.onAccountSetMainProfileTab(ctx, &tg.AccountSetMainProfileTabRequest{Tab: &tg.ProfileTabPosts{}}); ok || !tgerr.Is(err, "NOT_IMPLEMENTED") {
		t.Fatalf("no persistence = %v/%v", ok, err)
	}
	if ok, err := f.router.onAccountConfirmBotConnection(ctx, &tg.AccountConfirmBotConnectionRequest{BotID: inputUser(f.bot)}); ok || !tgerr.Is(err, "NOT_IMPLEMENTED") {
		t.Fatalf("no persistence confirm = %v/%v", ok, err)
	}
}

func TestAccountFeatureExactProfileDispatch(t *testing.T) {
	for _, profile := range []tlprofile.Profile{tlprofile.Profile228, tlprofile.Profile229} {
		f := newAccountFeatureFixture(t)
		ctx := WithUserID(context.Background(), f.owner.ID)
		if _, err := f.state.SaveConnectedBusinessBot(ctx, domain.ConnectedBusinessBot{OwnerUserID: f.owner.ID, BotUserID: f.bot.ID}); err != nil {
			t.Fatal(err)
		}
		requests := []bin.Object{&tg.AccountSetMainProfileTabRequest{Tab: &tg.ProfileTabGifts{}}, &tg.AccountConfirmBotConnectionRequest{BotID: inputUser(f.bot)}}
		for _, request := range requests {
			t.Run(fmt.Sprintf("%d_%T", profile, request), func(t *testing.T) {
				expectedMethod := "account.setMainProfileTab"
				expectedSemantic := tlprofile.SemanticMethodAccountSetMainProfileTab
				if _, ok := request.(*tg.AccountConfirmBotConnectionRequest); ok {
					expectedMethod = "account.confirmBotConnection"
					expectedSemantic = tlprofile.SemanticMethodAccountConfirmBotConnection
				}
				body := encodeExactLayerRPC(t, profile, request)
				admitted, err := f.router.AdmitLayer(profile, &body, tlprofile.Limits{})
				if err != nil {
					t.Fatal(err)
				}
				if body.Len() != 0 || admitted.Call().Profile() != profile || admitted.Call().Method() != expectedSemantic {
					t.Fatal("wrong exact admission")
				}
				result, method, err := f.router.DispatchAdmitted(ctx, [8]byte{}, 0, 0, 0, admitted)
				if err != nil || result == nil || method != expectedMethod {
					t.Fatalf("dispatch %s = %v", method, err)
				}
				var encoded bin.Buffer
				if err := result.Encode(&encoded); err != nil {
					t.Fatal(err)
				}
				if id, err := encoded.ID(); err != nil || id != tg.BoolTrueTypeID {
					t.Fatalf("result ID = %#x/%v", id, err)
				}
			})
		}
		full, err := f.router.onUsersGetFullUser(ctx, &tg.InputUserSelf{})
		if err != nil {
			t.Fatal(err)
		}
		var encoded bin.Buffer
		if err := tlprofile.EncodeObject(profile, full, &encoded); err != nil {
			t.Fatal(err)
		}
		object, err := tlprofile.DecodeObject(profile, &encoded, tlprofile.Limits{})
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := object.(*tg.UsersUserFull).FullUser.MainTab.(*tg.ProfileTabGifts); !ok {
			t.Fatalf("exact full profile lost tab at %d", profile)
		}
	}
}
