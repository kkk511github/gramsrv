package rpc

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/iamxvbaba/td/tg"
	"github.com/iamxvbaba/td/tgerr"
	"github.com/iamxvbaba/td/tlprofile"
	"telesrv/internal/domain"
)

// Visit the generated TL object graph, not a flattened projection. Only prose
// slots are writable; code, math, automatic entities and all media stay opaque.
func richProseSlots(blocks []tg.PageBlockClass) ([]*string, error) {
	if err := checkRichGraphBounds(reflect.ValueOf(blocks), 0, new(int)); err != nil {
		return nil, err
	}
	var slots []*string
	count := 0
	var walk func(reflect.Value, int) error
	walk = func(v reflect.Value, depth int) error {
		count++
		if depth > 64 || count > 20000 {
			return richMessageTooLongErr()
		}
		if !v.IsValid() {
			return nil
		}
		if v.Kind() == reflect.Interface {
			if v.IsNil() {
				return nil
			}
			return walk(v.Elem(), depth+1)
		}
		if v.Kind() == reflect.Pointer {
			if v.IsNil() {
				return richMessageInvalidErr()
			}
			switch x := v.Interface().(type) {
			case *tg.TextPlain:
				if !utf8.ValidString(x.Text) {
					return richMessageInvalidErr()
				}
				if strings.TrimSpace(x.Text) != "" {
					slots = append(slots, &x.Text)
				}
				return nil
			case *tg.PageBlockPreformatted, *tg.PageBlockMath, *tg.TextFixed, *tg.TextMath,
				*tg.TextMention, *tg.TextHashtag, *tg.TextBotCommand, *tg.TextCashtag,
				*tg.TextAutoURL, *tg.TextAutoEmail, *tg.TextAutoPhone, *tg.TextBankCard,
				*tg.TextCustomEmoji, *tg.TextMentionName, *tg.TextDate:
				return nil
			}
			return walk(v.Elem(), depth+1)
		}
		switch v.Kind() {
		case reflect.Struct:
			if v.Type() == reflect.TypeOf(tg.PageRelatedArticle{}) {
				for _, name := range []string{"Title", "Description"} {
					field := v.FieldByName(name)
					if field.CanAddr() && strings.TrimSpace(field.String()) != "" {
						slots = append(slots, field.Addr().Interface().(*string))
					}
				}
			}
			for i := 0; i < v.NumField(); i++ {
				if err := walk(v.Field(i), depth+1); err != nil {
					return err
				}
			}
		case reflect.Slice:
			for i := 0; i < v.Len(); i++ {
				if err := walk(v.Index(i), depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(reflect.ValueOf(blocks), 0); err != nil {
		return nil, err
	}
	return slots, nil
}

// Preflight even protected subtrees before the existing recursive validators.
func checkRichGraphBounds(v reflect.Value, depth int, count *int) error {
	*count++
	if depth > 64 || *count > 20000 {
		return richMessageTooLongErr()
	}
	if !v.IsValid() {
		return nil
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			return checkRichGraphBounds(v.Elem(), depth+1, count)
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if err := checkRichGraphBounds(v.Field(i), depth+1, count); err != nil {
				return err
			}
		}
	case reflect.Slice:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return nil
		}
		for i := 0; i < v.Len(); i++ {
			if err := checkRichGraphBounds(v.Index(i), depth+1, count); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Router) richTransformInput(ctx context.Context, input tg.InputRichMessageClass) (*tg.RichMessage, error) {
	if input == nil {
		return nil, richMessageInvalidErr()
	}
	if in, ok := input.(*tg.InputRichMessage); ok {
		if in == nil {
			return nil, richMessageInvalidErr()
		}
		for _, value := range in.Photos {
			if photo, ok := value.(*tg.InputPhoto); !ok || photo == nil {
				return nil, photoInvalidErr()
			}
		}
		for _, value := range in.Documents {
			if document, ok := value.(*tg.InputDocument); !ok || document == nil {
				return nil, mediaInvalidErr()
			}
		}
		if _, err := richProseSlots(in.Blocks); err != nil {
			return nil, err
		}
		if err := validateRichMessageBlocks(in.Blocks); err != nil {
			return nil, err
		}
		wire, err := encodeRichBlocks(tlprofile.ProfileCanonical, in.Blocks)
		if err != nil {
			return nil, richMessageInvalidErr()
		}
		blocks, err := decodeRichBlocks(tlprofile.ProfileCanonical, wire)
		if err != nil {
			return nil, richMessageInvalidErr()
		}
		clone := *in
		clone.Blocks = blocks
		input = &clone
	}
	rich, err := r.domainRichMessageFromInput(ctx, input)
	if err != nil {
		return nil, err
	}
	if rich.IsZero() {
		return nil, richMessageInvalidErr()
	}
	if in, ok := input.(*tg.InputRichMessage); ok {
		// IDs alone are not capabilities. Every referenced file must have a
		// matching supplied access hash and (when stored) file reference.
		if len(in.Photos) != len(rich.Photos) || len(in.Documents) != len(rich.Documents) {
			return nil, mediaInvalidErr()
		}
		photos := make(map[int64]*tg.InputPhoto, len(in.Photos))
		for _, value := range in.Photos {
			p, ok := value.(*tg.InputPhoto)
			if !ok || p == nil || photos[p.ID] != nil {
				return nil, photoInvalidErr()
			}
			photos[p.ID] = p
		}
		for _, photo := range rich.Photos {
			p := photos[photo.ID]
			if p == nil || p.AccessHash == 0 || p.AccessHash != photo.AccessHash || !bytes.Equal(p.FileReference, photo.FileReference) {
				return nil, photoInvalidErr()
			}
		}
		docs := make(map[int64]*tg.InputDocument, len(in.Documents))
		for _, value := range in.Documents {
			d, ok := value.(*tg.InputDocument)
			if !ok || d == nil || docs[d.ID] != nil {
				return nil, mediaInvalidErr()
			}
			docs[d.ID] = d
		}
		for _, doc := range rich.Documents {
			d := docs[doc.ID]
			if d == nil || d.AccessHash == 0 || d.AccessHash != doc.AccessHash || !bytes.Equal(d.FileReference, doc.FileReference) {
				return nil, mediaInvalidErr()
			}
		}
	}
	out, err := tgRichMessage(rich)
	if err != nil {
		return nil, richMessageInvalidErr()
	}
	return out, nil
}

func replaceRichProse(slots []*string, results []string) error {
	if len(slots) != len(results) {
		return translateReqFailedErr()
	}
	total := 0
	for i, result := range results {
		if !utf8.ValidString(result) || strings.TrimSpace(result) == "" {
			return translateReqFailedErr()
		}
		total += len(result)
		if total > domain.MaxTranslationOutputBytes {
			return richMessageTooLongErr()
		}
		original := *slots[i]
		left := len(original) - len(strings.TrimLeftFunc(original, unicode.IsSpace))
		right := len(strings.TrimRightFunc(original, unicode.IsSpace))
		*slots[i] = original[:left] + strings.TrimSpace(result) + original[right:]
	}
	return nil
}

type richAIComposeService interface {
	ComposeBatch(context.Context, domain.AIComposeRequest, []domain.AIComposeText) ([]domain.AIComposeText, error)
}

func (r *Router) onMessagesComposeRichMessageWithAI(ctx context.Context, req *tg.MessagesComposeRichMessageWithAIRequest) (*tg.MessagesComposedRichMessageWithAI, error) {
	userID, err := r.currentAIComposeUserID(ctx)
	if err != nil {
		return nil, err
	}
	if err := r.requireTranslationUser(ctx, userID); err != nil {
		return nil, err
	}
	svc, ok := r.deps.AICompose.(richAIComposeService)
	if !ok {
		return nil, tgerr.New(500, "AICOMPOSE_FAILED")
	}
	if req == nil {
		return nil, richMessageInvalidErr()
	}
	ref, err := domainAIComposeOptionalToneRef(req.Tone)
	if err != nil {
		return nil, err
	}
	rich, err := r.richTransformInput(ctx, req.Text)
	if err != nil {
		return nil, err
	}
	slots, err := richProseSlots(rich.Blocks)
	if err != nil {
		return nil, err
	}
	if len(slots) == 0 {
		return nil, richMessageInvalidErr()
	}
	texts := make([]domain.AIComposeText, len(slots))
	for i, slot := range slots {
		texts[i].Text = strings.TrimSpace(*slot)
	}
	result, err := svc.ComposeBatch(ctx, domain.AIComposeRequest{UserID: userID, Proofread: req.Proofread, Emojify: req.Emojify, TranslateToLang: req.TranslateToLang, Tone: ref}, texts)
	if err != nil {
		return nil, aiComposeErr(err)
	}
	values := make([]string, len(result))
	for i := range result {
		values[i] = result[i].Text
	}
	if err := replaceRichProse(slots, values); err != nil {
		return nil, tgerr.New(500, "AICOMPOSE_FAILED")
	}
	if err := validateRichMessageBlocks(rich.Blocks); err != nil {
		return nil, err
	}
	return &tg.MessagesComposedRichMessageWithAI{Result: *rich}, nil
}

func (r *Router) onMessagesTranslateRichMessage(ctx context.Context, req *tg.MessagesTranslateRichMessageRequest) (*tg.MessagesTranslatedRichMessage, error) {
	userID, err := r.currentAIComposeUserID(ctx)
	if err != nil {
		return nil, err
	}
	if r.deps.Translation == nil {
		return nil, translationsDisabledErr()
	}
	if err := r.requireTranslationUser(ctx, userID); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, inputTextEmptyErr()
	}
	inputPeer, peerSet := req.GetPeer()
	ids, idsSet := req.GetID()
	inputs, textSet := req.GetText()
	if peerSet != idsSet || peerSet == textSet {
		return nil, inputTextEmptyErr()
	}
	var rich []*tg.RichMessage
	if peerSet {
		peer, err := r.checkedTranslationPeer(ctx, userID, inputPeer)
		if err != nil {
			return nil, peerIDInvalidErr()
		}
		rich, err = r.storedRichTranslationInputs(ctx, userID, peer, ids)
		if err != nil {
			return nil, err
		}
	} else {
		if len(inputs) == 0 {
			return nil, inputTextEmptyErr()
		}
		if len(inputs) > domain.MaxTranslationTexts {
			return nil, inputTextTooLongErr()
		}
		for _, input := range inputs {
			value, err := r.richTransformInput(ctx, input)
			if err != nil {
				return nil, err
			}
			rich = append(rich, value)
		}
	}
	var slots []*string
	for _, value := range rich {
		prose, err := richProseSlots(value.Blocks)
		if err != nil {
			return nil, err
		}
		if len(prose) == 0 {
			return nil, inputTextEmptyErr()
		}
		slots = append(slots, prose...)
	}
	if len(slots) > domain.MaxTranslationTexts {
		return nil, inputTextTooLongErr()
	}
	texts := make([]domain.TranslationText, len(slots))
	for i, slot := range slots {
		texts[i].Text = strings.TrimSpace(*slot)
	}
	result, err := r.deps.Translation.Translate(ctx, domain.TranslationRequest{UserID: userID, Texts: texts, ToLang: req.ToLang, Tone: req.Tone})
	if err != nil {
		return nil, translationRPCErr(err)
	}
	values := make([]string, len(result.Texts))
	for i := range result.Texts {
		values[i] = result.Texts[i].Text
	}
	if err := replaceRichProse(slots, values); err != nil {
		return nil, err
	}
	out := &tg.MessagesTranslatedRichMessage{Result: make([]tg.RichMessage, len(rich))}
	for i, value := range rich {
		if err := validateRichMessageBlocks(value.Blocks); err != nil {
			return nil, err
		}
		out.Result[i] = *value
	}
	return out, nil
}

func (r *Router) storedRichTranslationInputs(ctx context.Context, userID int64, peer domain.Peer, ids []int) ([]*tg.RichMessage, error) {
	if len(ids) == 0 {
		return nil, inputTextEmptyErr()
	}
	if len(ids) > domain.MaxTranslationTexts {
		return nil, inputTextTooLongErr()
	}
	for _, id := range ids {
		if id <= 0 || id > domain.MaxMessageBoxID {
			return nil, msgIDInvalidErr()
		}
	}
	byID := make(map[int]*domain.MessageRichMessage, len(ids))
	switch peer.Type {
	case domain.PeerTypeUser:
		if r.deps.Messages == nil {
			return nil, translateReqFailedErr()
		}
		list, err := r.deps.Messages.GetMessages(ctx, userID, ids)
		if err != nil {
			return nil, translationRPCErr(err)
		}
		for _, message := range list.Messages {
			if message.Peer == peer {
				byID[message.ID] = message.RichMessage
			}
		}
	case domain.PeerTypeChannel:
		if r.deps.Channels == nil {
			return nil, translateReqFailedErr()
		}
		list, err := r.deps.Channels.GetMessages(ctx, userID, peer.ID, ids)
		if err != nil {
			return nil, translationRPCErr(err)
		}
		for _, message := range list.Messages {
			if message.ChannelID == peer.ID && !message.Deleted {
				byID[message.ID] = message.RichMessage
			}
		}
	default:
		return nil, peerIDInvalidErr()
	}
	out := make([]*tg.RichMessage, 0, len(ids))
	for _, id := range ids {
		stored := byID[id]
		if stored.IsZero() {
			return nil, msgIDInvalidErr()
		}
		value, err := tgRichMessage(stored)
		if err != nil {
			return nil, richMessageInvalidErr()
		}
		out = append(out, value)
	}
	return out, nil
}
