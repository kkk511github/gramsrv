package rpc

import (
	"context"
	"testing"

	"github.com/iamxvbaba/td/clock"
	"github.com/iamxvbaba/td/tg"
	"go.uber.org/zap/zaptest"

	"telesrv/internal/domain"
)

func TestNotifyAccountFreezeChangedPushesUpdateConfig(t *testing.T) {
	const userID = int64(1001)
	sessions := &captureSessions{}
	router := New(Config{}, Deps{Sessions: sessions}, zaptest.NewLogger(t), clock.System)

	if err := router.NotifyAccountFreezeChanged(context.Background(), domain.AccountFreeze{
		UserID:  userID,
		Frozen:  true,
		Version: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if got := sessions.pushedUserIDs(); len(got) != 1 || got[0] != userID {
		t.Fatalf("pushed users = %v, want [%d]", got, userID)
	}
	updates, ok := sessions.lastUserPush().(*tg.Updates)
	if !ok || len(updates.Updates) != 1 {
		t.Fatalf("push = %#v, want one updateConfig", sessions.lastUserPush())
	}
	if _, ok := updates.Updates[0].(*tg.UpdateConfig); !ok {
		t.Fatalf("update = %T, want *tg.UpdateConfig", updates.Updates[0])
	}
}
