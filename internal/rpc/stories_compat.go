package rpc

import (
	"context"

	"github.com/iamxvbaba/td/bin"
	"github.com/iamxvbaba/td/tg"

	"telesrv/internal/domain"
)

const legacyStoriesGetPeerMaxIDsIntID = 0x535983c3

// legacyStoryMaxIDs encodes the TelegramSwift 11.15 shape of
// stories.getPeerMaxIDs#535983c3, whose result is Vector<int>. Canonical 227
// returns Vector<RecentStory> for the same semantic query.
type legacyStoryMaxIDs []int

func (v legacyStoryMaxIDs) Encode(b *bin.Buffer) error {
	b.PutVectorHeader(len(v))
	for _, id := range v {
		b.PutInt(id)
	}
	return nil
}

// tryLegacyStoriesRPC handles client-private stories constructors whose result
// shape differs from gotd's canonical schema. These cannot use layerwire's
// generic id/body upgrade because the rpc_result needs a legacy type.
func (r *Router) tryLegacyStoriesRPC(ctx context.Context, b *bin.Buffer) (enc bin.Encoder, handled bool, err error) {
	id, perr := b.PeekID()
	if perr != nil || id != legacyStoriesGetPeerMaxIDsIntID {
		return nil, false, nil
	}
	enc, err = r.legacyStoriesGetPeerMaxIDs(ctx, b)
	return enc, true, err
}

func (r *Router) legacyStoriesGetPeerMaxIDs(ctx context.Context, b *bin.Buffer) (bin.Encoder, error) {
	if _, err := b.ID(); err != nil {
		return nil, inputConstructorInvalidErr()
	}
	n, err := b.VectorHeader()
	if err != nil {
		return nil, inputConstructorInvalidErr()
	}
	if n > domain.MaxStoryIDs {
		return nil, storyIDInvalidErr()
	}
	peers := make([]tg.InputPeerClass, 0, n)
	for i := 0; i < n; i++ {
		peer, err := tg.DecodeInputPeer(b)
		if err != nil {
			return nil, inputConstructorInvalidErr()
		}
		peers = append(peers, peer)
	}
	if b.Len() != 0 {
		return nil, inputConstructorInvalidErr()
	}
	recent, err := r.onStoriesGetPeerMaxIDs(ctx, peers)
	if err != nil {
		return nil, err
	}
	out := make(legacyStoryMaxIDs, len(recent))
	for i, item := range recent {
		if maxID, ok := item.GetMaxID(); ok {
			out[i] = maxID
		}
	}
	return out, nil
}
