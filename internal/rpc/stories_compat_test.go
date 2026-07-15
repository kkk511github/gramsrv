package rpc

import (
	"context"
	"testing"
	"time"

	"github.com/iamxvbaba/td/bin"
	"github.com/iamxvbaba/td/tg"
	"go.uber.org/zap/zaptest"

	appstories "telesrv/internal/app/stories"
	"telesrv/internal/domain"
	"telesrv/internal/store/memory"
)

func TestLegacyStoriesGetPeerMaxIDsReturnsIntVector(t *testing.T) {
	ctx := context.Background()
	userID := int64(1000000001)
	owner := domain.Peer{Type: domain.PeerTypeUser, ID: userID}
	storyStore := memory.NewStoryStore()
	for _, storyID := range []int{1, 4} {
		if _, err := storyStore.UpsertStory(ctx, domain.UpsertStoryRequest{Story: domain.Story{
			Owner:      owner,
			ID:         storyID,
			Date:       1700000000 + storyID,
			ExpireDate: 1700003600,
			Public:     true,
		}}); err != nil {
			t.Fatalf("upsert story %d: %v", storyID, err)
		}
	}
	r := New(Config{}, Deps{
		Stories: appstories.NewService(storyStore),
	}, zaptest.NewLogger(t), fixedClock{now: time.Unix(1700000100, 0)})

	var req bin.Buffer
	req.PutID(legacyStoriesGetPeerMaxIDsIntID)
	req.PutVectorHeader(2)
	_ = (&tg.InputPeerSelf{}).Encode(&req)
	_ = (&tg.InputPeerSelf{}).Encode(&req)

	enc, handled, err := r.tryLegacyStoriesRPC(WithUserID(ctx, userID), &req)
	if err != nil || !handled {
		t.Fatalf("legacy stories.getPeerMaxIDs: handled=%v err=%v", handled, err)
	}
	var out bin.Buffer
	if err := enc.Encode(&out); err != nil {
		t.Fatalf("encode legacy result: %v", err)
	}
	n, err := out.VectorHeader()
	if err != nil {
		t.Fatalf("decode vector header: %v", err)
	}
	if n != 2 {
		t.Fatalf("result vector len = %d, want 2", n)
	}
	for i := 0; i < n; i++ {
		got, err := out.Int()
		if err != nil {
			t.Fatalf("decode result[%d]: %v", i, err)
		}
		if got != 4 {
			t.Fatalf("result[%d] = %d, want max id 4", i, got)
		}
	}
	if out.Len() != 0 {
		t.Fatalf("legacy result has %d trailing bytes", out.Len())
	}
}
