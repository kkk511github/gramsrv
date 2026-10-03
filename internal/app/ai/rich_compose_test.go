package ai

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"telesrv/internal/domain"
	"telesrv/internal/store/memory"
)

type countComposeLimiter struct{ calls int }

func (l *countComposeLimiter) Allow(context.Context, string, int, time.Duration) (bool, int, error) {
	l.calls++
	return true, 0, nil
}

func TestComposeSingleUseIsEphemeralAndComposeOnly(t *testing.T) {
	ctx := context.Background()
	st := memory.NewAIComposeStore()
	provider := &fakeProvider{text: "Rewritten"}
	svc := NewService(st, WithProvider(provider))
	ref := domain.AIComposeToneRef{Kind: domain.AIComposeToneRefSingleUse, CustomPrompt: "  Make it friendly  "}
	_, err := svc.Compose(ctx, domain.AIComposeRequest{UserID: 1, Text: domain.AIComposeText{Text: "draft"}, Tone: ref})
	if err != nil {
		t.Fatal(err)
	}
	if provider.seen.Tone.Prompt != "Make it friendly" || provider.seen.Tone.ID != 0 || !strings.Contains(provider.seen.Instruction, "Make it friendly") {
		t.Fatalf("provider request = %#v", provider.seen)
	}
	tones, err := st.ListAIComposeTonesForUser(ctx, 1)
	if err != nil || len(tones) != 0 {
		t.Fatalf("persisted tones = %v/%v", tones, err)
	}
	if _, err := svc.GetTone(ctx, 1, ref); !errors.Is(err, domain.ErrAIComposeToneInvalid) {
		t.Fatalf("get = %v", err)
	}
	if err := svc.SaveTone(ctx, 1, ref, false); !errors.Is(err, domain.ErrAIComposeToneInvalid) {
		t.Fatalf("save = %v", err)
	}
	for _, prompt := range []string{"", "   ", strings.Repeat("a", domain.MaxAIComposeTonePromptLength+1), string([]byte{255})} {
		ref.CustomPrompt = prompt
		if _, err := svc.Compose(ctx, domain.AIComposeRequest{UserID: 1, Text: domain.AIComposeText{Text: "draft"}, Tone: ref}); !errors.Is(err, domain.ErrAIComposeToneInvalid) {
			t.Fatalf("invalid prompt err = %v", err)
		}
	}
}

func TestComposeBatchQuotaAndErrors(t *testing.T) {
	ctx := context.Background()
	limiter := &countComposeLimiter{}
	provider := &fakeProvider{text: "Rewritten"}
	svc := NewService(memory.NewAIComposeStore(), WithProvider(provider), WithRateLimiter(limiter, 20, time.Minute))
	texts := []domain.AIComposeText{{Text: "one"}, {Text: "two"}}
	out, err := svc.ComposeBatch(ctx, domain.AIComposeRequest{UserID: 1, Proofread: true}, texts)
	if err != nil || len(out) != 2 || limiter.calls != 1 {
		t.Fatalf("batch = %v/%v quota = %d", out, err, limiter.calls)
	}
	provider.err = domain.ErrAIComposeProviderUnavailable
	if out, err := svc.ComposeBatch(ctx, domain.AIComposeRequest{UserID: 1, Proofread: true}, texts); out != nil || !errors.Is(err, domain.ErrAIComposeProviderUnavailable) {
		t.Fatalf("provider failure = %v/%v", out, err)
	}
	denied := NewService(memory.NewAIComposeStore(), WithProvider(provider), WithRateLimiter(denyLimiter{}, 1, time.Minute))
	if _, err := denied.ComposeBatch(ctx, domain.AIComposeRequest{UserID: 1, Proofread: true}, texts); !errors.Is(err, domain.ErrAIComposeRateLimited) {
		t.Fatalf("quota failure = %v", err)
	}
	if _, err := svc.ComposeBatch(ctx, domain.AIComposeRequest{UserID: 1, Proofread: true}, []domain.AIComposeText{{Text: strings.Repeat("x", domain.MaxAIComposeTextLength)}, {Text: "y"}}); !errors.Is(err, domain.ErrAIComposeInvalid) {
		t.Fatalf("total limit = %v", err)
	}
	disabled := NewService(memory.NewAIComposeStore(), WithEnabled(false))
	if _, err := disabled.ComposeBatch(ctx, domain.AIComposeRequest{UserID: 1, Proofread: true}, texts); !errors.Is(err, domain.ErrAIComposeDisabled) {
		t.Fatalf("disabled = %v", err)
	}
}

func TestLocalComposeDoesNotPretendToTranslateOrApplyCustomPrompt(t *testing.T) {
	svc := NewService(memory.NewAIComposeStore())
	for _, req := range []domain.AIComposeRequest{
		{UserID: 1, TranslateToLang: "zh"},
		{UserID: 1, Emojify: true},
		{UserID: 1, Tone: domain.AIComposeToneRef{Kind: domain.AIComposeToneRefSingleUse, CustomPrompt: "rewrite"}},
	} {
		req.Text.Text = "hello"
		if _, err := svc.Compose(context.Background(), req); !errors.Is(err, domain.ErrAIComposeProviderUnavailable) {
			t.Fatalf("local unsupported operation = %v", err)
		}
	}
}
