package rpc

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/iamxvbaba/td/bin"
	"github.com/iamxvbaba/td/clock"
	"github.com/iamxvbaba/td/tg"
	"github.com/iamxvbaba/td/tgerr"
	"github.com/iamxvbaba/td/tlprofile"
	"go.uber.org/zap/zaptest"
	aiapp "telesrv/internal/app/ai"
	translationapp "telesrv/internal/app/translation"
	"telesrv/internal/domain"
	"telesrv/internal/store/memory"
)

type richComposeProvider struct {
	fail  bool
	calls int
	seen  aiapp.ProviderRequest
}

func (p *richComposeProvider) Name() string { return "rich-test" }
func (p *richComposeProvider) Compose(_ context.Context, req aiapp.ProviderRequest) (domain.AIComposeText, error) {
	p.calls++
	p.seen = req
	if p.fail {
		return domain.AIComposeText{}, domain.ErrAIComposeProviderUnavailable
	}
	return domain.AIComposeText{Text: "edited:" + req.Request.Text.Text}, nil
}

func richTransformFixture() *tg.InputRichMessage {
	cell := tg.PageTableCell{Header: true, AlignCenter: true, Text: &tg.TextItalic{Text: &tg.TextPlain{Text: "cell"}}}
	cell.SetColspan(2)
	return &tg.InputRichMessage{Rtl: true, Blocks: []tg.PageBlockClass{
		&tg.PageBlockParagraph{Text: &tg.TextConcat{Texts: []tg.RichTextClass{&tg.TextBold{Text: &tg.TextPlain{Text: "hello "}}, &tg.TextURL{URL: "https://example.com/path", Text: &tg.TextPlain{Text: "link"}}}}},
		&tg.PageBlockTable{Bordered: true, Title: &tg.TextPlain{Text: "table"}, Rows: []tg.PageTableRow{{Cells: []tg.PageTableCell{cell}}}},
		&tg.PageBlockBlockquote{Collapsed: true, Text: &tg.TextPlain{Text: "quote"}, Caption: &tg.TextPlain{Text: "credit"}},
		&tg.PageBlockPreformatted{Text: &tg.TextPlain{Text: "fmt.Println(1)"}, Language: "go"},
		&tg.PageBlockPhoto{PhotoID: 9, Caption: tg.PageCaption{Text: &tg.TextPlain{Text: "caption"}, Credit: &tg.TextEmpty{}}},
		&tg.PageBlockVideo{VideoID: 10, Caption: tg.PageCaption{Text: &tg.TextPlain{Text: "video"}, Credit: &tg.TextEmpty{}}},
	}}
}

func TestRichTransformsPreserveStructureAndMedia(t *testing.T) {
	for _, mode := range []string{"ai", "translation"} {
		t.Run(mode, func(t *testing.T) {
			r, owner, _ := newMediaTestRouter(t)
			files := r.deps.Files.(*fakeFiles)
			photo := domain.Photo{ID: 9, AccessHash: 99, FileReference: []byte{1, 2}, Sizes: []domain.PhotoSize{{Kind: domain.PhotoSizeKindDefault, Type: "x", W: 12, H: 12}}}
			doc := domain.Document{ID: 10, AccessHash: 100, FileReference: []byte{3}, MimeType: "video/mp4", Size: 42}
			files.photos[9] = photo
			files.docs[10] = doc
			input := richTransformFixture()
			input.Photos = []tg.InputPhotoClass{&tg.InputPhoto{ID: 9, AccessHash: 99, FileReference: []byte{1, 2}}}
			input.Documents = []tg.InputDocumentClass{&tg.InputDocument{ID: 10, AccessHash: 100, FileReference: []byte{3}}}
			before, err := encodeRichBlocks(tlprofile.ProfileCanonical, input.Blocks)
			if err != nil {
				t.Fatal(err)
			}
			ctx := WithUserID(context.Background(), owner.ID)
			var got tg.RichMessage
			prefix := "translated:"
			if mode == "ai" {
				prefix = "edited:"
				provider := &richComposeProvider{}
				r.deps.AICompose = aiapp.NewService(memory.NewAIComposeStore(), aiapp.WithProvider(provider))
				req := &tg.MessagesComposeRichMessageWithAIRequest{Proofread: true}
				req.SetText(input)
				req.SetTone(&tg.InputAiComposeToneSingleUse{CustomPrompt: "Make this concise"})
				result, err := r.onMessagesComposeRichMessageWithAI(ctx, req)
				if err != nil {
					t.Fatal(err)
				}
				got = result.Result
				if provider.seen.Tone.Prompt != "Make this concise" {
					t.Fatal("single-use prompt lost")
				}
			} else {
				r.deps.Translation = &captureTranslationService{}
				req := &tg.MessagesTranslateRichMessageRequest{ToLang: "zh"}
				req.SetText([]tg.InputRichMessageClass{input})
				result, err := r.onMessagesTranslateRichMessage(ctx, req)
				if err != nil {
					t.Fatal(err)
				}
				got = result.Result[0]
			}
			if !got.Rtl || len(got.Blocks) != len(input.Blocks) {
				t.Fatal("structure or RTL lost")
			}
			paragraph := got.Blocks[0].(*tg.PageBlockParagraph).Text.(*tg.TextConcat)
			if paragraph.Texts[0].(*tg.TextBold).Text.(*tg.TextPlain).Text != prefix+"hello " {
				t.Fatalf("bold/whitespace = %#v", paragraph)
			}
			if paragraph.Texts[1].(*tg.TextURL).URL != "https://example.com/path" {
				t.Fatal("URL changed")
			}
			table := got.Blocks[1].(*tg.PageBlockTable)
			if !table.Bordered || table.Rows[0].Cells[0].Colspan != 2 || !table.Rows[0].Cells[0].AlignCenter || table.Rows[0].Cells[0].Text.(*tg.TextItalic).Text.(*tg.TextPlain).Text != prefix+"cell" {
				t.Fatalf("table = %#v", table)
			}
			quote := got.Blocks[2].(*tg.PageBlockBlockquote)
			if !quote.Collapsed || quote.Caption.(*tg.TextPlain).Text != prefix+"credit" {
				t.Fatal("quote/caption changed")
			}
			if got.Blocks[3].(*tg.PageBlockPreformatted).Text.(*tg.TextPlain).Text != "fmt.Println(1)" {
				t.Fatal("code transformed")
			}
			if got.Blocks[4].(*tg.PageBlockPhoto).Caption.Text.(*tg.TextPlain).Text != prefix+"caption" {
				t.Fatal("photo caption lost")
			}
			if !reflect.DeepEqual(got.Photos[0], tgPhoto(photo)) || !reflect.DeepEqual(got.Documents[0], tgDocument(doc)) {
				t.Fatal("media metadata changed")
			}
			after, _ := encodeRichBlocks(tlprofile.ProfileCanonical, input.Blocks)
			if string(before) != string(after) {
				t.Fatal("caller input mutated")
			}
		})
	}
}

func TestRichTransformMediaCapabilities(t *testing.T) {
	r, owner, _ := newMediaTestRouter(t)
	r.deps.Translation = &captureTranslationService{}
	files := r.deps.Files.(*fakeFiles)
	files.photos[9] = domain.Photo{ID: 9, AccessHash: 99, FileReference: []byte{1}, Sizes: []domain.PhotoSize{{Type: "x", W: 12, H: 12}}}
	for _, photo := range []tg.InputPhotoClass{nil, &tg.InputPhoto{ID: 9, AccessHash: 98, FileReference: []byte{1}}, &tg.InputPhoto{ID: 9, AccessHash: 99, FileReference: []byte{2}}} {
		input := &tg.InputRichMessage{Blocks: []tg.PageBlockClass{&tg.PageBlockPhoto{PhotoID: 9, Caption: tg.PageCaption{Text: &tg.TextPlain{Text: "caption"}, Credit: &tg.TextEmpty{}}}}}
		if photo != nil {
			input.Photos = []tg.InputPhotoClass{photo}
		}
		req := &tg.MessagesTranslateRichMessageRequest{ToLang: "en"}
		req.SetText([]tg.InputRichMessageClass{input})
		if _, err := r.onMessagesTranslateRichMessage(WithUserID(context.Background(), owner.ID), req); err == nil {
			t.Fatal("bad media capability admitted")
		}
	}
}

func TestRichTranslationStoredReadAuthorization(t *testing.T) {
	r, owner, friend := newMediaTestRouter(t)
	r.deps.Translation = &captureTranslationService{}
	ctx := WithUserID(context.Background(), owner.ID)
	peer := &tg.InputPeerUser{UserID: friend.ID, AccessHash: friend.AccessHash}
	updates, err := r.onMessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{Peer: peer, RandomID: 9911, RichMessage: &tg.InputRichMessage{Blocks: []tg.PageBlockClass{&tg.PageBlockParagraph{Text: &tg.TextBold{Text: &tg.TextPlain{Text: "draft"}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	id := newMessageFromUpdates(t, updates).ID
	req := &tg.MessagesTranslateRichMessageRequest{ToLang: "en"}
	req.SetPeer(peer)
	req.SetID([]int{id, id})
	got, err := r.onMessagesTranslateRichMessage(ctx, req)
	if err != nil || len(got.Result) != 2 {
		t.Fatalf("stored = %v/%v", got, err)
	}
	req.SetPeer(&tg.InputPeerSelf{})
	if _, err := r.onMessagesTranslateRichMessage(ctx, req); !tgerr.Is(err, "MSG_ID_INVALID") {
		t.Fatalf("wrong peer = %v", err)
	}
	req.SetPeer(&tg.InputPeerUser{UserID: friend.ID, AccessHash: friend.AccessHash + 1})
	if _, err := r.onMessagesTranslateRichMessage(ctx, req); !tgerr.Is(err, "PEER_ID_INVALID") {
		t.Fatalf("wrong access hash = %v", err)
	}
	req.SetPeer(peer)
	req.SetID([]int{id + 1000})
	if _, err := r.onMessagesTranslateRichMessage(ctx, req); !tgerr.Is(err, "MSG_ID_INVALID") {
		t.Fatalf("missing message = %v", err)
	}
}

func TestRichTransformErrorsAndSourceFormats(t *testing.T) {
	r := New(Config{}, Deps{Translation: &captureTranslationService{}, AICompose: aiapp.NewService(memory.NewAIComposeStore())}, zaptest.NewLogger(t), clock.System)
	ctx := WithUserID(context.Background(), 1)
	for _, input := range []tg.InputRichMessageClass{&tg.InputRichMessageHTML{HTML: "<p><b>Hello</b></p>"}, &tg.InputRichMessageMarkdown{Markdown: "**Hello**"}} {
		req := &tg.MessagesTranslateRichMessageRequest{ToLang: "zh"}
		req.SetText([]tg.InputRichMessageClass{input})
		if result, err := r.onMessagesTranslateRichMessage(ctx, req); err != nil || len(result.Result[0].Blocks) == 0 {
			t.Fatalf("source = %v/%v", result, err)
		}
	}
	input := &tg.InputRichMessage{Blocks: []tg.PageBlockClass{&tg.PageBlockParagraph{Text: &tg.TextPlain{Text: "hello"}}}}
	req := &tg.MessagesComposeRichMessageWithAIRequest{}
	req.SetText(input)
	req.SetTranslateToLang("zh")
	if _, err := r.onMessagesComposeRichMessageWithAI(ctx, req); !tgerr.Is(err, "AICOMPOSE_FAILED") {
		t.Fatalf("local fake translation = %v", err)
	}
	if _, err := r.onMessagesComposeRichMessageWithAI(context.Background(), req); !tgerr.Is(err, "AUTH_KEY_UNREGISTERED") {
		t.Fatalf("unauthenticated = %v", err)
	}
	cycle := &tg.TextFixed{}
	cycle.Text = cycle
	if _, err := richProseSlots([]tg.PageBlockClass{&tg.PageBlockParagraph{Text: cycle}}); !tgerr.Is(err, "RICH_MESSAGE_TOO_LONG") {
		t.Fatalf("cyclic protected node = %v", err)
	}
	trans := &tg.MessagesTranslateRichMessageRequest{ToLang: "zh"}
	trans.SetText([]tg.InputRichMessageClass{input})
	trans.SetPeer(&tg.InputPeerSelf{})
	trans.SetID([]int{1})
	if _, err := r.onMessagesTranslateRichMessage(ctx, trans); !tgerr.Is(err, "INPUT_TEXT_EMPTY") {
		t.Fatalf("mixed modes = %v", err)
	}
	trans = &tg.MessagesTranslateRichMessageRequest{ToLang: "zh"}
	trans.SetText([]tg.InputRichMessageClass{&tg.InputRichMessage{Blocks: []tg.PageBlockClass{&tg.PageBlockParagraph{Text: &tg.TextPlain{Text: strings.Repeat("x", richMessageLengthLimit+1)}}}}})
	if _, err := r.onMessagesTranslateRichMessage(ctx, trans); !tgerr.Is(err, "RICH_MESSAGE_TOO_LONG") {
		t.Fatalf("size = %v", err)
	}
	r.deps.Translation = nil
	if _, err := r.onMessagesTranslateRichMessage(ctx, trans); !tgerr.Is(err, "TRANSLATIONS_DISABLED") {
		t.Fatalf("disabled = %v", err)
	}
}

func TestNewRichRPCExactLayerDispatch(t *testing.T) {
	for _, profile := range []tlprofile.Profile{tlprofile.Profile228, tlprofile.Profile229} {
		t.Run(fmt.Sprint(profile), func(t *testing.T) {
			f := newInlineBotRPCTestFixture(t)
			r := f.router
			r.deps.AICompose = aiapp.NewService(memory.NewAIComposeStore(), aiapp.WithProvider(&richComposeProvider{}))
			r.deps.Translation = &captureTranslationService{}
			ctx := WithUserID(context.Background(), f.owner.ID)
			if _, err := f.bots.SetBotMenuButton(ctx, f.bot.ID, domain.BotMenuButton{Type: domain.BotMenuButtonWebView, Text: "Verify", URL: "https://local.example/verify"}); err != nil {
				t.Fatal(err)
			}
			session := r.webviews.registerContext(ctx, r.clock.Now(), storeJoinSession(f.owner.ID, f.bot.ID))
			aiReq := &tg.MessagesComposeRichMessageWithAIRequest{Proofread: true}
			aiReq.SetText(&tg.InputRichMessage{Blocks: []tg.PageBlockClass{&tg.PageBlockBlockquote{Text: &tg.TextBold{Text: &tg.TextPlain{Text: "draft"}}, Caption: &tg.TextEmpty{}}}})
			if profile == tlprofile.Profile229 {
				aiReq.SetTone(&tg.InputAiComposeToneSingleUse{CustomPrompt: "Keep it concise"})
			}
			trReq := &tg.MessagesTranslateRichMessageRequest{ToLang: "zh"}
			trReq.SetText([]tg.InputRichMessageClass{aiReq.Text})
			for _, test := range []struct {
				request  bin.Object
				method   string
				semantic tlprofile.SemanticID
			}{
				{aiReq, "messages.composeRichMessageWithAI", tlprofile.SemanticMethodMessagesComposeRichMessageWithAI},
				{trReq, "messages.translateRichMessage", tlprofile.SemanticMethodMessagesTranslateRichMessage},
				{&tg.MessagesRequestChatJoinWebViewRequest{QueryID: session.QueryID, Platform: "ios"}, "messages.requestChatJoinWebView", tlprofile.SemanticMethodMessagesRequestChatJoinWebView},
			} {
				body := encodeExactLayerRPC(t, profile, test.request)
				admitted, err := r.AdmitLayer(profile, &body, tlprofile.Limits{})
				if err != nil {
					t.Fatal(err)
				}
				if body.Len() != 0 || admitted.Call().Profile() != profile || admitted.Call().Method() != test.semantic {
					t.Fatalf("bad admission for %s", test.method)
				}
				result, method, err := r.DispatchAdmitted(ctx, [8]byte{}, 0, 0, 0, admitted)
				if err != nil || method != test.method || result == nil {
					t.Fatalf("dispatch %s = %s/%v", test.method, method, err)
				}
				var wire bin.Buffer
				if err := result.Encode(&wire); err != nil {
					t.Fatalf("encode %s: %v", method, err)
				}
				decoded, err := tlprofile.DecodeObject(profile, &wire, tlprofile.Limits{})
				if err != nil || wire.Len() != 0 {
					t.Fatalf("decode %s: %v", method, err)
				}
				if test.method == "messages.composeRichMessageWithAI" {
					composed, ok := decoded.(*tg.MessagesComposedRichMessageWithAI)
					if !ok || composed.Result.Blocks[0].(*tg.PageBlockBlockquote).Text.(*tg.TextBold).Text.(*tg.TextPlain).Text != "edited:draft" {
						t.Fatalf("composed = %#v", decoded)
					}
				}
			}
		})
	}
}

func TestRichTranslationProductionNoProviderIsDisabled(t *testing.T) {
	r := New(Config{}, Deps{Translation: translationapp.NewService(nil, nil, nil)}, zaptest.NewLogger(t), clock.System)
	req := &tg.MessagesTranslateRichMessageRequest{ToLang: "zh"}
	req.SetText([]tg.InputRichMessageClass{&tg.InputRichMessage{Blocks: []tg.PageBlockClass{&tg.PageBlockParagraph{Text: &tg.TextPlain{Text: "hello"}}}}})
	ctx := WithUserID(context.Background(), 1)
	if result, err := r.onMessagesTranslateRichMessage(ctx, req); result != nil || !tgerr.Is(err, "TRANSLATIONS_DISABLED") {
		t.Fatalf("no-provider result = %v/%v", result, err)
	}
	for _, profile := range []tlprofile.Profile{tlprofile.Profile228, tlprofile.Profile229} {
		body := encodeExactLayerRPC(t, profile, req)
		admitted, err := r.AdmitLayer(profile, &body, tlprofile.Limits{})
		if err != nil {
			t.Fatal(err)
		}
		if result, method, err := r.DispatchAdmitted(ctx, [8]byte{}, 0, 0, 0, admitted); result != nil || method != "messages.translateRichMessage" || !tgerr.Is(err, "TRANSLATIONS_DISABLED") {
			t.Fatalf("layer %d no-provider dispatch = %s/%v/%v", profile, method, result, err)
		}
	}
}

type failedRichTranslation struct {
	captureTranslationService
	err        error
	wrongCount bool
}

func (s *failedRichTranslation) Translate(context.Context, domain.TranslationRequest) (domain.TranslationResult, error) {
	if s.wrongCount {
		return domain.TranslationResult{}, nil
	}
	return domain.TranslationResult{}, s.err
}

func TestRichTransformFailuresAreAtomic(t *testing.T) {
	ctx := WithUserID(context.Background(), 1)
	input := &tg.InputRichMessage{Blocks: []tg.PageBlockClass{&tg.PageBlockParagraph{Text: &tg.TextBold{Text: &tg.TextPlain{Text: "draft"}}}}}
	before, _ := encodeRichBlocks(tlprofile.ProfileCanonical, input.Blocks)
	for _, test := range []struct {
		cause error
		rpc   string
	}{
		{domain.ErrTranslationDisabled, "TRANSLATIONS_DISABLED"},
		{domain.ErrTranslationRateLimited, "TRANSLATE_REQ_QUOTA_EXCEEDED"},
		{domain.ErrTranslationTimeout, "TRANSLATION_TIMEOUT"},
		{domain.ErrTranslationProviderUnavailable, "TRANSLATE_REQ_FAILED"},
	} {
		r := New(Config{}, Deps{Translation: &failedRichTranslation{err: test.cause}}, zaptest.NewLogger(t), clock.System)
		req := &tg.MessagesTranslateRichMessageRequest{ToLang: "zh"}
		req.SetText([]tg.InputRichMessageClass{input})
		if result, err := r.onMessagesTranslateRichMessage(ctx, req); result != nil || !tgerr.Is(err, test.rpc) {
			t.Fatalf("%v => %v/%v", test.cause, result, err)
		}
	}
	r := New(Config{}, Deps{Translation: &failedRichTranslation{wrongCount: true}, AICompose: aiapp.NewService(memory.NewAIComposeStore(), aiapp.WithProvider(&richComposeProvider{fail: true}))}, zaptest.NewLogger(t), clock.System)
	trReq := &tg.MessagesTranslateRichMessageRequest{ToLang: "zh"}
	trReq.SetText([]tg.InputRichMessageClass{input})
	if result, err := r.onMessagesTranslateRichMessage(ctx, trReq); result != nil || !tgerr.Is(err, "TRANSLATE_REQ_FAILED") {
		t.Fatalf("missing translated run => %v/%v", result, err)
	}
	aiReq := &tg.MessagesComposeRichMessageWithAIRequest{Proofread: true}
	aiReq.SetText(input)
	if result, err := r.onMessagesComposeRichMessageWithAI(ctx, aiReq); result != nil || !tgerr.Is(err, "AICOMPOSE_FAILED") {
		t.Fatalf("AI unavailable => %v/%v", result, err)
	}
	after, _ := encodeRichBlocks(tlprofile.ProfileCanonical, input.Blocks)
	if string(before) != string(after) {
		t.Fatal("failure mutated source")
	}
	if !errors.Is(r.deps.AICompose.(*aiapp.Service).SaveTone(context.Background(), 1, domain.AIComposeToneRef{Kind: domain.AIComposeToneRefSingleUse, CustomPrompt: "prompt"}, false), domain.ErrAIComposeToneInvalid) {
		t.Fatal("ephemeral prompt could be saved")
	}
}

func TestRichTranslationChannelReadAuthorization(t *testing.T) {
	r, owner, channel := newRichChannelTestRouter(t)
	r.deps.Translation = &captureTranslationService{}
	ctx := WithUserID(context.Background(), owner.ID)
	peer := &tg.InputPeerChannel{ChannelID: channel.ID, AccessHash: channel.AccessHash}
	updates, err := r.onMessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{Peer: peer, RandomID: 9944, RichMessage: &tg.InputRichMessage{Blocks: []tg.PageBlockClass{&tg.PageBlockParagraph{Text: &tg.TextPlain{Text: "draft"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	req := &tg.MessagesTranslateRichMessageRequest{ToLang: "zh"}
	req.SetPeer(peer)
	req.SetID([]int{newMessageFromUpdates(t, updates).ID})
	if result, err := r.onMessagesTranslateRichMessage(ctx, req); err != nil || len(result.Result) != 1 {
		t.Fatalf("channel translation = %v/%v", result, err)
	}
	req.SetPeer(&tg.InputPeerChannel{ChannelID: channel.ID, AccessHash: channel.AccessHash + 1})
	if _, err := r.onMessagesTranslateRichMessage(ctx, req); !tgerr.Is(err, "PEER_ID_INVALID") {
		t.Fatalf("channel hash = %v", err)
	}
	req.SetPeer(peer)
	r.deps.Users = nil
	if _, err := r.onMessagesTranslateRichMessage(WithUserID(context.Background(), owner.ID+1000), req); !tgerr.Is(err, "PEER_ID_INVALID") {
		t.Fatalf("channel outsider = %v", err)
	}
}

func TestRichTransformsRejectBots(t *testing.T) {
	f := newInlineBotRPCTestFixture(t)
	f.router.deps.Translation = &captureTranslationService{}
	f.router.deps.AICompose = aiapp.NewService(memory.NewAIComposeStore())
	ctx := WithUserID(context.Background(), f.bot.ID)
	trReq := &tg.MessagesTranslateRichMessageRequest{ToLang: "zh"}
	if _, err := f.router.onMessagesTranslateRichMessage(ctx, trReq); !tgerr.Is(err, "BOT_METHOD_INVALID") {
		t.Fatalf("translation bot = %v", err)
	}
	if _, err := f.router.onMessagesComposeRichMessageWithAI(ctx, &tg.MessagesComposeRichMessageWithAIRequest{}); !tgerr.Is(err, "BOT_METHOD_INVALID") {
		t.Fatalf("AI bot = %v", err)
	}
}
