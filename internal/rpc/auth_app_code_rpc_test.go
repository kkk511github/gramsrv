package rpc

import (
	"context"
	"testing"
	"time"

	"github.com/gotd/td/tg"
	"go.uber.org/zap/zaptest"

	"telesrv/internal/domain"
)

func TestAuthCheckPasswordNotifiesOtherDevicesAfterPendingLogin(t *testing.T) {
	user := domain.User{ID: 1000000002, Phone: "15550004314", FirstName: "Alice"}
	authKeyID := [8]byte{7, 8, 9}
	authService := &captureAuthService{
		pendingPasswordUserID: user.ID,
		pendingPassword:       true,
	}
	users := &captureUsersService{user: user}
	sessions := &captureSessions{}
	r := New(Config{}, Deps{
		Auth:     authService,
		Account:  acceptPasswordAccountService{},
		Users:    users,
		Sessions: sessions,
	}, zaptest.NewLogger(t), fixedClock{now: time.Unix(1700000000, 0)})
	ctx := WithClientInfo(
		WithSessionID(WithAuthKeyID(context.Background(), authKeyID), 77),
		ClientInfo{DeviceModel: "SafeLink iOS", AppVersion: "2.2"},
	)

	if _, err := r.onAuthCheckPassword(ctx, &tg.InputCheckPasswordSRP{SRPID: 1, A: []byte{1}, M1: []byte{2}}); err != nil {
		t.Fatalf("onAuthCheckPassword: %v", err)
	}
	if authService.completePasswordCount != 1 || authService.completedPasswordKey != authKeyID {
		t.Fatalf("password completion count/key = %d/%x", authService.completePasswordCount, authService.completedPasswordKey)
	}
	deadline := time.Now().Add(time.Second)
	for len(sessions.pushedUserIDs()) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := sessions.pushedUserIDs(); len(got) == 0 || got[len(got)-1] != user.ID {
		t.Fatalf("notified users = %v, want final login alert for %d", got, user.ID)
	}
	updates, ok := sessions.lastUserPush().(*tg.Updates)
	if !ok || len(updates.Updates) == 0 {
		t.Fatalf("notification = %T, want tg.Updates", sessions.lastUserPush())
	}
}
