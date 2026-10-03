package messages

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"telesrv/internal/domain"
	"telesrv/internal/store"
)

type phoneCallHistoryFaultStore struct {
	store.MessageStore
	list                             domain.MessageList
	listErr, deleteErr, remainingErr error
	listCalls                        int
	filter                           domain.MessageFilter
	deleteReq                        domain.DeleteMessagesRequest
	deleteCalls                      int
}

func (s *phoneCallHistoryFaultStore) ListByUser(_ context.Context, _ int64, filter domain.MessageFilter) (domain.MessageList, error) {
	s.listCalls++
	if s.listCalls > 1 {
		return s.list, s.remainingErr
	}
	s.filter = filter
	return s.list, s.listErr
}

func (s *phoneCallHistoryFaultStore) DeleteMessages(_ context.Context, req domain.DeleteMessagesRequest) (domain.DeleteMessagesResult, error) {
	s.deleteCalls++
	s.deleteReq = req
	return domain.DeleteMessagesResult{OwnerUserID: req.OwnerUserID}, s.deleteErr
}

func TestDeletePhoneCallHistoryPropagatesPageFailures(t *testing.T) {
	failure := errors.New("transaction failed")
	call := domain.Message{ID: 7, OwnerUserID: 9, Peer: domain.Peer{Type: domain.PeerTypeUser, ID: 10},
		Media: &domain.MessageMedia{Kind: domain.MessageMediaKindService, ServiceAction: &domain.MessageServiceAction{Kind: domain.MessageServiceActionPhoneCall}},
	}
	for _, failurePhase := range []string{"list", "delete", "remaining"} {
		storage := &phoneCallHistoryFaultStore{list: domain.MessageList{Messages: []domain.Message{call}, Count: 501}}
		switch failurePhase {
		case "list":
			storage.listErr = failure
		case "delete":
			storage.deleteErr = failure
		case "remaining":
			storage.remainingErr = failure
		}
		service := NewService(storage, nil)
		result, err := service.DeletePhoneCallHistory(context.Background(), 9, domain.DeletePhoneCallHistoryRequest{
			Revoke: true, Date: 1700000000, OriginAuthKeyID: [8]byte{3}, OriginSessionID: 4,
		})
		if !errors.Is(err, failure) || result.Changed() || result.Offset != 0 {
			t.Fatalf("failure acknowledged as completed page: result=%+v err=%v", result, err)
		}
		if !storage.filter.PhoneCallsOnly || !storage.filter.NeedTotalCount || storage.filter.Limit != domain.MaxDeletePhoneCallHistoryBatch {
			t.Fatalf("filter=%+v", storage.filter)
		}
		if failurePhase == "list" && storage.deleteCalls != 0 {
			t.Fatal("delete ran after failed source query")
		}
		if failurePhase != "list" && (!storage.deleteReq.Revoke || storage.deleteReq.OwnerUserID != 9 || !reflect.DeepEqual(storage.deleteReq.IDs, []int{7}) || storage.deleteReq.OriginAuthKeyID != ([8]byte{3}) || storage.deleteReq.OriginSessionID != 4) {
			t.Fatalf("delete request=%+v", storage.deleteReq)
		}
	}
}

func TestDeletePhoneCallHistoryRejectsWrongOwnerAndNonCalls(t *testing.T) {
	for _, message := range []domain.Message{
		{ID: 1, OwnerUserID: 10, Peer: domain.Peer{Type: domain.PeerTypeUser, ID: 9}},
		{ID: 1, OwnerUserID: 9, Peer: domain.Peer{Type: domain.PeerTypeUser, ID: 10}, Body: "not a call"},
	} {
		storage := &phoneCallHistoryFaultStore{list: domain.MessageList{Count: 1, Messages: []domain.Message{message}}}
		if _, err := NewService(storage, nil).DeletePhoneCallHistory(context.Background(), 9, domain.DeletePhoneCallHistoryRequest{}); err == nil || storage.deleteCalls != 0 {
			t.Fatalf("invalid source accepted: message=%+v err=%v", message, err)
		}
	}
}
