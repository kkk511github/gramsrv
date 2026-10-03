package ai

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"go.uber.org/zap"
	"telesrv/internal/domain"
)

// ComposeBatch transforms independently formatted runs under one account quota.
// It is atomic to the caller: no partially transformed rich message is returned.
func (s *Service) ComposeBatch(ctx context.Context, req domain.AIComposeRequest, texts []domain.AIComposeText) ([]domain.AIComposeText, error) {
	if !s.ready() || !s.enabled {
		return nil, domain.ErrAIComposeDisabled
	}
	if len(texts) == 0 || len(texts) > 256 {
		return nil, domain.ErrAIComposeInvalid
	}
	total := 0
	for _, text := range texts {
		req.Text = text
		if err := validateComposeRequest(req); err != nil {
			return nil, err
		}
		total += utf8.RuneCountInString(text.Text)
	}
	if total > domain.MaxAIComposeTextLength {
		return nil, domain.ErrAIComposeInvalid
	}
	tone, err := s.resolveComposeTone(ctx, req.UserID, req.Tone)
	if err != nil {
		return nil, err
	}
	if err := s.consumeRateLimit(ctx, fmt.Sprintf("ai:compose:%d", req.UserID)); err != nil {
		return nil, err
	}
	batchCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	out := make([]domain.AIComposeText, len(texts))
	outputBytes := 0
	for i, text := range texts {
		req.Text = text
		result, err := s.composeWithProviders(batchCtx, req, tone, composeInstruction(req, tone), ProviderPurposeCompose, []zap.Field{zap.Int64("user_id", req.UserID), zap.Int("run", i)})
		if err != nil {
			return nil, err
		}
		outputBytes += len(result.Text)
		if !utf8.ValidString(result.Text) || outputBytes > domain.MaxTranslationOutputBytes {
			return nil, domain.ErrAIComposeProviderUnavailable
		}
		out[i] = result
	}
	return out, nil
}

func (s *Service) resolveComposeTone(ctx context.Context, userID int64, ref domain.AIComposeToneRef) (domain.AIComposeTone, error) {
	if ref.Kind == domain.AIComposeToneRefSingleUse {
		prompt := strings.TrimSpace(ref.CustomPrompt)
		if !utf8.ValidString(prompt) || !validToneText(prompt, domain.MaxAIComposeTonePromptLength) {
			return domain.AIComposeTone{}, domain.ErrAIComposeToneInvalid
		}
		// No ID, slug, owner, persistence, or logging of the custom prompt.
		return domain.AIComposeTone{Prompt: prompt}, nil
	}
	tone, found, err := s.resolveTone(ctx, userID, ref)
	if err != nil {
		return domain.AIComposeTone{}, err
	}
	if !ref.Empty() && !found {
		return domain.AIComposeTone{}, domain.ErrAIComposeToneNotFound
	}
	return tone, nil
}
