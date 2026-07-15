package rpc

import (
	"context"

	"github.com/iamxvbaba/td/bin"
	"github.com/iamxvbaba/td/tg"
)

const helpTestID = 0xc0e202f7

// tryHelpCompatRPC handles official client help methods missing from gotd's
// current server dispatcher schema.
func (r *Router) tryHelpCompatRPC(_ context.Context, b *bin.Buffer) (bin.Encoder, bool, error) {
	id, err := b.PeekID()
	if err != nil {
		return nil, false, nil
	}
	switch id {
	case helpTestID:
		return &tg.BoolTrue{}, true, nil
	default:
		return nil, false, nil
	}
}
