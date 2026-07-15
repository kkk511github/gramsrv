package rpc

import (
	"context"

	"github.com/iamxvbaba/td/bin"
	"github.com/iamxvbaba/td/tg"
)

const helpTestID = tg.HelpTestRequestTypeID

// tryHelpCompatRPC retains help.test support for the legacy unprofiled router.
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
