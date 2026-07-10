package rpc

import (
	"context"
	"testing"
	"time"

	"github.com/gotd/td/tg"
	"go.uber.org/zap/zaptest"

	"telesrv/internal/domain"
)

type appCodeIssueAuthService struct {
	*captureAuthService
	issue domain.AuthCodeIssue
}

func (s *appCodeIssueAuthService) IssueCode(context.Context, string) (domain.AuthCodeIssue, error) {
	return s.issue, nil
}

func (s *appCodeIssueAuthService) ResendCodeIssueForAuthKey(context.Context, [8]byte, string, string) (domain.AuthCodeIssue, error) {
	return s.issue, nil
}

func TestAuthSendCodePublishesAppCodeBeforeReturning(t *testing.T) {
	msg := domain.Message{
		ID:          91,
		OwnerUserID: 1000000001,
		Peer:        domain.Peer{Type: domain.PeerTypeUser, ID: domain.OfficialSystemUserID},
		From:        domain.Peer{Type: domain.PeerTypeUser, ID: domain.OfficialSystemUserID},
		Date:        1700000000,
		Body:        "Login code: 54321",
	}
	authService := &appCodeIssueAuthService{
		captureAuthService: &captureAuthService{},
		issue: domain.AuthCodeIssue{
			PhoneCodeHash: "app-code-hash",
			Delivery:      domain.AuthCodeDelivery{Kind: domain.AuthCodeDeliveryApp, Length: 5},
			AppMessage:    msg,
		},
	}
	updates := &captureUpdates{state: domain.UpdateState{Pts: 7, Date: 1700000000}}
	r := New(Config{}, Deps{Auth: authService, Updates: updates}, zaptest.NewLogger(t), fixedClock{now: time.Unix(1700000000, 0)})

	sent, err := r.onAuthSendCode(context.Background(), &tg.AuthSendCodeRequest{PhoneNumber: "+15550004313", APIID: 1, APIHash: "hash"})
	if err != nil {
		t.Fatalf("onAuthSendCode: %v", err)
	}
	code, ok := sent.(*tg.AuthSentCode)
	if !ok {
		t.Fatalf("sent = %T, want *tg.AuthSentCode", sent)
	}
	appType, ok := code.Type.(*tg.AuthSentCodeTypeApp)
	if !ok || appType.Length != 5 || code.PhoneCodeHash != "app-code-hash" {
		t.Fatalf("sent code = %+v type=%T", code, code.Type)
	}
	if len(updates.events) != 1 || updates.events[0].Type != domain.UpdateEventNewMessage || updates.events[0].Message.ID != msg.ID {
		t.Fatalf("published events = %+v, want app login message", updates.events)
	}
}

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
