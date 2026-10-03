package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	appmessages "telesrv/internal/app/messages"
	"telesrv/internal/domain"
)

var errCallHistoryCommit = errors.New("injected call-history commit failure")

type callHistoryRollbackDB struct{ *pgxpool.Pool }
type callHistoryRollbackTx struct{ pgx.Tx }

func (db callHistoryRollbackDB) Begin(ctx context.Context) (pgx.Tx, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return callHistoryRollbackTx{tx}, nil
}

func (callHistoryRollbackTx) Commit(context.Context) error { return errCallHistoryCommit }

func TestDeletePhoneCallHistoryPostgresRevokeAndRollback(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	for _, rollback := range []bool{false, true} {
		name := "revoke"
		if rollback {
			name = "rollback"
		}
		t.Run(name, func(t *testing.T) {
			suffix := randomSuffix(t)
			users := NewUserStore(pool)
			a, err := users.Create(ctx, domain.User{AccessHash: 61, Phone: "+16681" + suffix, FirstName: "CallA"})
			if err != nil {
				t.Fatal(err)
			}
			b, err := users.Create(ctx, domain.User{AccessHash: 62, Phone: "+16682" + suffix, FirstName: "CallB"})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _, _ = pool.Exec(ctx, "DELETE FROM users WHERE id = ANY($1::bigint[])", []int64{a.ID, b.ID}) })
			messages := NewMessageStore(pool)
			for i := range 2 {
				media := (*domain.MessageMedia)(nil)
				body := "keep ordinary message"
				if i == 1 {
					body = ""
					media = &domain.MessageMedia{Kind: domain.MessageMediaKindService, ServiceAction: &domain.MessageServiceAction{
						Kind: domain.MessageServiceActionPhoneCall, Call: &domain.MessagePhoneCallAction{CallID: 100, Reason: "missed"},
					}}
				}
				if _, err := messages.SendPrivateText(ctx, domain.SendPrivateTextRequest{
					SenderUserID: a.ID, RecipientUserID: b.ID, RandomID: int64(i + 1),
					Message: body, Media: media, Date: 1700000000,
				}); err != nil {
					t.Fatal(err)
				}
			}
			var before int
			if err := pool.QueryRow(ctx, "SELECT count(*) FROM user_update_events WHERE user_id = ANY($1::bigint[])", []int64{a.ID, b.ID}).Scan(&before); err != nil {
				t.Fatal(err)
			}
			deleting := messages
			if rollback {
				deleting = NewMessageStore(callHistoryRollbackDB{pool})
			}
			result, err := appmessages.NewService(deleting, nil).DeletePhoneCallHistory(ctx, b.ID, domain.DeletePhoneCallHistoryRequest{Revoke: true, Date: 1700000001})
			if rollback {
				if !errors.Is(err, errCallHistoryCommit) || result.Changed() || result.Offset != 0 {
					t.Fatalf("commit failure acknowledged: result=%+v err=%v", result, err)
				}
			} else if err != nil || result.Offset != 0 || len(result.Self().MessageIDs) != 1 || len(result.Deleted) != 2 {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			for _, owner := range []int64{a.ID, b.ID} {
				list, err := messages.ListByUser(ctx, owner, domain.MessageFilter{PhoneCallsOnly: true, CountOnly: true})
				want := 0
				if rollback {
					want = 1
				}
				if err != nil || list.Count != want {
					t.Fatalf("owner=%d calls=%+v err=%v", owner, list, err)
				}
				ordinary, err := messages.ListByUser(ctx, owner, domain.MessageFilter{Query: "keep ordinary", CountOnly: true})
				if err != nil || ordinary.Count != 1 {
					t.Fatalf("ordinary history changed: %+v err=%v", ordinary, err)
				}
			}
			var after int
			if err := pool.QueryRow(ctx, "SELECT count(*) FROM user_update_events WHERE user_id = ANY($1::bigint[])", []int64{a.ID, b.ID}).Scan(&after); err != nil {
				t.Fatal(err)
			}
			if (rollback && after != before) || (!rollback && after <= before) {
				t.Fatalf("delete events before=%d after=%d rollback=%v", before, after, rollback)
			}
		})
	}
}
