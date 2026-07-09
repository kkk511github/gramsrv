package layerwire

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

// loadLayerModel parses a vendored historical schema (_schema/layer-N.tl) into a
// schemaModel used as an independent oracle: downgraded bytes must parse cleanly
// against the actual target-layer schema.
func loadLayerModel(t *testing.T, layer int) *schemaModel {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("_schema", fmt.Sprintf("layer-%d.tl", layer)))
	if err != nil {
		t.Fatalf("read layer %d schema: %v", layer, err)
	}
	m, err := parseSchemaModel(string(src))
	if err != nil {
		t.Fatalf("parse layer %d schema: %v", layer, err)
	}
	return m
}

// TestTranscodeIdentity verifies that targeting the canonical layer (or above)
// is a pure passthrough — the transcoder must never mutate 227 bytes.
func TestTranscodeIdentity(t *testing.T) {
	for _, o := range canonicalCorpus() {
		raw := mustEncode(t, o)
		out, err := Transcode(raw, CanonicalLayer)
		if err != nil {
			t.Fatalf("%T: identity transcode: %v", o, err)
		}
		if !bytes.Equal(out, raw) {
			t.Errorf("%T: identity transcode changed bytes", o)
		}
	}
}

// TestTranscodeDowngradeValid downgrades the corpus to every supported layer and
// asserts the result parses cleanly (full byte consumption) against that layer's
// own schema. This is the core correctness oracle for the transcoder.
func TestTranscodeDowngradeValid(t *testing.T) {
	for layer := SupportedFloor; layer < CanonicalLayer; layer++ {
		model := loadLayerModel(t, layer)
		for _, o := range canonicalCorpus() {
			raw := mustEncode(t, o)
			out, err := Transcode(raw, layer)
			if err != nil {
				t.Errorf("layer %d %T: transcode: %v", layer, o, err)
				continue
			}
			b := &bin.Buffer{Buf: append([]byte(nil), out...)}
			if err := model.skipObject(b); err != nil {
				t.Errorf("layer %d %T: result invalid at target: %v", layer, o, err)
				continue
			}
			if b.Len() != 0 {
				t.Errorf("layer %d %T: %d trailing bytes in downgraded output", layer, o, b.Len())
			}
		}
	}
}

// TestTranscodeMessageGolden checks that a message downgraded to 220 carries the
// 220 constructor id and is strictly shorter (dropped trailing fields).
func TestTranscodeMessageGolden(t *testing.T) {
	const message220CRC = 0xb92f76cf
	raw := mustEncode(t, canonicalCorpus()[1]) // the rich message
	out, err := Transcode(raw, 220)
	if err != nil {
		t.Fatalf("transcode message->220: %v", err)
	}
	b := &bin.Buffer{Buf: append([]byte(nil), out...)}
	id, err := b.PeekID()
	if err != nil {
		t.Fatalf("peek id: %v", err)
	}
	if id != message220CRC {
		t.Fatalf("message@220 id = %#08x, want %#08x", id, message220CRC)
	}
	if len(out) >= len(raw) {
		t.Errorf("downgraded message not shorter: %d >= %d", len(out), len(raw))
	}
}

func TestTranscodeBelowSupportedFloorClampsToFloor(t *testing.T) {
	raw := mustEncode(t, &tg.AuthAuthorization{
		User: &tg.User{
			Self:       true,
			ID:         1780243200,
			AccessHash: 42,
			FirstName:  "Safe",
			Phone:      "8618052866760",
		},
	})
	out210, err := Transcode(raw, 210)
	if err != nil {
		t.Fatalf("transcode auth.authorization->210: %v", err)
	}
	out220, err := Transcode(raw, SupportedFloor)
	if err != nil {
		t.Fatalf("transcode auth.authorization->floor: %v", err)
	}
	if !bytes.Equal(out210, out220) {
		t.Fatalf("layer below floor should clamp to %d output", SupportedFloor)
	}
	model := loadLayerModel(t, SupportedFloor)
	b := &bin.Buffer{Buf: append([]byte(nil), out210...)}
	if err := model.skipObject(b); err != nil || b.Len() != 0 {
		t.Fatalf("clamped auth.authorization invalid at floor (err=%v left=%d)", err, b.Len())
	}
}

func TestTranscodeTelegramSwift211AuthorizationUsesLegacyUser(t *testing.T) {
	const (
		canonicalUserCRC     = 0x31774388
		telegramSwiftUserCRC = 0x020b1422
	)
	raw := mustEncode(t, &tg.AuthAuthorization{
		User: &tg.User{
			Self:       true,
			ID:         1780243200,
			AccessHash: 42,
			FirstName:  "Safe",
			Phone:      "8618052866760",
		},
	})
	out, err := Transcode(raw, 211)
	if err != nil {
		t.Fatalf("transcode auth.authorization->TelegramSwift 211: %v", err)
	}
	if bytes.Contains(out, crcLE(canonicalUserCRC)) {
		t.Fatalf("TelegramSwift 211 output leaked canonical user constructor")
	}
	if !bytes.Contains(out, crcLE(telegramSwiftUserCRC)) {
		t.Fatalf("TelegramSwift 211 output missing legacy user constructor")
	}
	if bytes.Equal(out, raw) {
		t.Fatalf("TelegramSwift 211 output unexpectedly unchanged")
	}
}

func TestTranscodeTelegramSwift211DialogsUsesLegacyCoreConstructors(t *testing.T) {
	const (
		canonicalUserCRC        = 0x31774388
		telegramSwiftUserCRC    = 0x020b1422
		canonicalMessageCRC     = 0x7600b9d3
		telegramSwiftMessageCRC = 0x9815cec8
	)
	raw := mustEncode(t, canonicalCorpus()[11]) // messages.dialogs with a simple Message + User.
	out, err := Transcode(raw, 211)
	if err != nil {
		t.Fatalf("transcode messages.dialogs->TelegramSwift 211: %v", err)
	}
	for _, crc := range []uint32{canonicalUserCRC, canonicalMessageCRC} {
		if bytes.Contains(out, crcLE(crc)) {
			t.Fatalf("TelegramSwift 211 dialogs output leaked canonical constructor %#08x", crc)
		}
	}
	for _, crc := range []uint32{telegramSwiftUserCRC, telegramSwiftMessageCRC} {
		if !bytes.Contains(out, crcLE(crc)) {
			t.Fatalf("TelegramSwift 211 dialogs output missing legacy constructor %#08x", crc)
		}
	}
}

func TestTranscodeTelegramSwift211ProfileUsesLegacyCoreConstructors(t *testing.T) {
	for _, tc := range []struct {
		name      string
		raw       []byte
		canonical uint32
		legacy    uint32
	}{
		{name: "userFull", raw: mustEncode(t, canonicalCorpus()[5]), canonical: 0x06cbe645, legacy: 0x7e63ce1f},
		{name: "channel", raw: mustEncode(t, canonicalCorpus()[6]), canonical: 0x1c32b11c, legacy: 0xfe685355},
	} {
		out, err := Transcode(tc.raw, 211)
		if err != nil {
			t.Fatalf("%s: transcode->TelegramSwift 211: %v", tc.name, err)
		}
		if bytes.Contains(out, crcLE(tc.canonical)) {
			t.Fatalf("%s: TelegramSwift 211 output leaked canonical constructor %#08x", tc.name, tc.canonical)
		}
		if !bytes.Contains(out, crcLE(tc.legacy)) {
			t.Fatalf("%s: TelegramSwift 211 output missing legacy constructor %#08x", tc.name, tc.legacy)
		}
	}
}

func TestTranscodeTelegramSwift211InheritsFloorRules(t *testing.T) {
	for _, tc := range []struct {
		name      string
		raw       []byte
		canonical uint32
		legacy    uint32
	}{
		{
			name:      "dialog",
			raw:       mustEncode(t, canonicalCorpus()[8]),
			canonical: 0xfc89f7f3,
			legacy:    0xd58a08c6,
		},
		{
			name: "reactionsNotifySettings",
			raw: mustEncode(t, &tg.ReactionsNotifySettings{
				Sound:        &tg.NotificationSoundDefault{},
				ShowPreviews: true,
			}),
			canonical: 0x71e4ea58,
			legacy:    0x56e34970,
		},
	} {
		out, err := Transcode(tc.raw, 211)
		if err != nil {
			t.Fatalf("%s: transcode->TelegramSwift 211: %v", tc.name, err)
		}
		if bytes.Contains(out, crcLE(tc.canonical)) {
			t.Fatalf("%s: TelegramSwift 211 output leaked canonical constructor %#08x", tc.name, tc.canonical)
		}
		if !bytes.Contains(out, crcLE(tc.legacy)) {
			t.Fatalf("%s: TelegramSwift 211 output missing floor legacy constructor %#08x", tc.name, tc.legacy)
		}
	}
}

func TestTranscodeTelegramSwift211MessagesUsesLegacyShape(t *testing.T) {
	const (
		canonicalMessagesCRC     = 0x1d73e7ea
		telegramSwiftMessagesCRC = 0x8c718e87
		canonicalMessageCRC      = 0x7600b9d3
		telegramSwiftMessageCRC  = 0x9815cec8
	)
	raw := mustEncode(t, canonicalCorpus()[12]) // messages.messages with topics vector in canonical.
	out, err := Transcode(raw, 211)
	if err != nil {
		t.Fatalf("transcode messages.messages->TelegramSwift 211: %v", err)
	}
	for _, crc := range []uint32{canonicalMessagesCRC, canonicalMessageCRC} {
		if bytes.Contains(out, crcLE(crc)) {
			t.Fatalf("TelegramSwift 211 messages output leaked canonical constructor %#08x", crc)
		}
	}
	for _, crc := range []uint32{telegramSwiftMessagesCRC, telegramSwiftMessageCRC} {
		if !bytes.Contains(out, crcLE(crc)) {
			t.Fatalf("TelegramSwift 211 messages output missing legacy constructor %#08x", crc)
		}
	}
	if len(out) >= len(raw) {
		t.Fatalf("TelegramSwift 211 messages output did not drop canonical topics vector")
	}
}

func TestTranscodeTelegramSwift211StarGiftUsesLegacyConstructor(t *testing.T) {
	const (
		canonicalStarGiftCRC     = 0x313a9547
		telegramSwiftStarGiftCRC = 0x00bcff5b
	)
	raw := mustEncode(t, &tg.StarGift{
		Limited:             true,
		LimitedPerUser:      true,
		ID:                  1,
		Sticker:             &tg.DocumentEmpty{ID: 2},
		Stars:               10,
		AvailabilityRemains: 5,
		AvailabilityTotal:   10,
		ConvertStars:        10,
		Title:               "SafeLink",
		PerUserTotal:        1,
		PerUserRemains:      1,
		LockedUntilDate:     123,
	})
	out, err := Transcode(raw, 211)
	if err != nil {
		t.Fatalf("transcode starGift->TelegramSwift 211: %v", err)
	}
	if bytes.Contains(out, crcLE(canonicalStarGiftCRC)) {
		t.Fatalf("TelegramSwift 211 starGift output leaked canonical constructor")
	}
	if !bytes.Contains(out, crcLE(telegramSwiftStarGiftCRC)) {
		t.Fatalf("TelegramSwift 211 starGift output missing legacy constructor")
	}
	b := &bin.Buffer{Buf: append([]byte(nil), out...)}
	if err := b.ConsumeID(telegramSwiftStarGiftCRC); err != nil {
		t.Fatalf("consume legacy starGift id: %v", err)
	}
	flags, err := b.Uint32()
	if err != nil {
		t.Fatalf("read legacy starGift flags: %v", err)
	}
	if flags&(1<<9) != 0 {
		t.Fatalf("TelegramSwift 211 starGift flags leaked dropped locked_until_date bit: %#x", flags)
	}
}

func TestTranscodeTelegramSwift211ReadHistoryInboxUsesLegacyConstructor(t *testing.T) {
	const (
		canonicalReadHistoryInboxCRC     = 0x9e84bc99
		telegramSwiftReadHistoryInboxCRC = 0x9c974fdf
	)
	raw := mustEncode(t, &tg.UpdateReadHistoryInbox{
		FolderID:         1,
		Peer:             &tg.PeerUser{UserID: 1780243204},
		TopMsgID:         88,
		MaxID:            77,
		StillUnreadCount: 0,
		Pts:              12,
		PtsCount:         1,
	})
	out, err := Transcode(raw, 211)
	if err != nil {
		t.Fatalf("transcode updateReadHistoryInbox->TelegramSwift 211: %v", err)
	}
	if bytes.Contains(out, crcLE(canonicalReadHistoryInboxCRC)) {
		t.Fatalf("TelegramSwift 211 readHistoryInbox output leaked canonical constructor")
	}
	if !bytes.Contains(out, crcLE(telegramSwiftReadHistoryInboxCRC)) {
		t.Fatalf("TelegramSwift 211 readHistoryInbox output missing legacy constructor")
	}
	b := &bin.Buffer{Buf: append([]byte(nil), out...)}
	if err := b.ConsumeID(telegramSwiftReadHistoryInboxCRC); err != nil {
		t.Fatalf("consume legacy readHistoryInbox id: %v", err)
	}
	flags, err := b.Uint32()
	if err != nil {
		t.Fatalf("read legacy readHistoryInbox flags: %v", err)
	}
	if flags&(1<<1) != 0 {
		t.Fatalf("TelegramSwift 211 readHistoryInbox flags leaked dropped top_msg_id bit: %#x", flags)
	}
	if flags&(1<<0) == 0 {
		t.Fatalf("TelegramSwift 211 readHistoryInbox flags dropped folder_id bit: %#x", flags)
	}
	if len(out) >= len(raw) {
		t.Fatalf("TelegramSwift 211 readHistoryInbox output did not drop top_msg_id")
	}
}

func TestTranscodeTelegramSwift211StarGiftActionUsesLegacyConstructor(t *testing.T) {
	const (
		canonicalStarGiftActionCRC     = 0xea2c31d3
		telegramSwiftStarGiftActionCRC = 0x4717e8a4
		telegramSwiftStarGiftCRC       = 0x00bcff5b
	)
	raw := mustEncode(t, &tg.MessageActionStarGift{
		NameHidden:         true,
		Saved:              true,
		Converted:          true,
		CanUpgrade:         true,
		PrepaidUpgrade:     true,
		UpgradeSeparate:    true,
		AuctionAcquired:    true,
		Gift:               &tg.StarGift{ID: 1, Sticker: &tg.DocumentEmpty{ID: 2}, Stars: 10, ConvertStars: 10, Title: "SafeLink"},
		Message:            tg.TextWithEntities{Text: "gift"},
		ConvertStars:       10,
		FromID:             &tg.PeerUser{UserID: 1780243204},
		Peer:               &tg.PeerUser{UserID: 1780243200},
		SavedID:            33,
		PrepaidUpgradeHash: "hash",
		GiftMsgID:          44,
		ToID:               &tg.PeerUser{UserID: 1780243201},
		GiftNum:            55,
	})
	out, err := Transcode(raw, 211)
	if err != nil {
		t.Fatalf("transcode messageActionStarGift->TelegramSwift 211: %v", err)
	}
	if bytes.Contains(out, crcLE(canonicalStarGiftActionCRC)) {
		t.Fatalf("TelegramSwift 211 starGift action output leaked canonical constructor")
	}
	if !bytes.Contains(out, crcLE(telegramSwiftStarGiftActionCRC)) {
		t.Fatalf("TelegramSwift 211 starGift action output missing legacy constructor")
	}
	if !bytes.Contains(out, crcLE(telegramSwiftStarGiftCRC)) {
		t.Fatalf("TelegramSwift 211 starGift action output missing legacy nested starGift constructor")
	}
	b := &bin.Buffer{Buf: append([]byte(nil), out...)}
	if err := b.ConsumeID(telegramSwiftStarGiftActionCRC); err != nil {
		t.Fatalf("consume legacy starGift action id: %v", err)
	}
	flags, err := b.Uint32()
	if err != nil {
		t.Fatalf("read legacy starGift action flags: %v", err)
	}
	for _, bit := range []uint{13, 14, 15, 16, 17, 18, 19} {
		if flags&(1<<bit) != 0 {
			t.Fatalf("TelegramSwift 211 starGift action flags leaked dropped bit %d: %#x", bit, flags)
		}
	}
	for _, bit := range []uint{0, 1, 2, 3, 4, 10, 11, 12} {
		if flags&(1<<bit) == 0 {
			t.Fatalf("TelegramSwift 211 starGift action flags dropped retained bit %d: %#x", bit, flags)
		}
	}
	if len(out) >= len(raw) {
		t.Fatalf("TelegramSwift 211 starGift action output did not drop new fields")
	}
}

func crcLE(crc uint32) []byte {
	return []byte{byte(crc), byte(crc >> 8), byte(crc >> 16), byte(crc >> 24)}
}

func TestTranscodeFormattedDateEntityLayerBoundary(t *testing.T) {
	const formattedDateEntityCRC = 0x904ac7c7
	entityCRC := func(crc uint32) []byte {
		return []byte{byte(crc), byte(crc >> 8), byte(crc >> 16), byte(crc >> 24)}
	}
	msg := &tg.Message{
		ID:      7,
		PeerID:  &tg.PeerUser{UserID: 2},
		Date:    100,
		Message: "Meet soon",
		Entities: []tg.MessageEntityClass{
			&tg.MessageEntityFormattedDate{Offset: 5, Length: 4, Date: 1773436800, ShortDate: true, ShortTime: true},
		},
	}
	raw := mustEncode(t, msg)

	out222, err := Transcode(raw, 222)
	if err != nil {
		t.Fatalf("transcode message->222: %v", err)
	}
	if bytes.Contains(out222, entityCRC(formattedDateEntityCRC)) {
		t.Fatalf("layer 222 output leaked formatted-date entity")
	}
	if !bytes.Contains(out222, entityCRC(messageEntityUnknownID)) {
		t.Fatalf("layer 222 output missing messageEntityUnknown fallback")
	}
	m222 := loadLayerModel(t, 222)
	b222 := &bin.Buffer{Buf: append([]byte(nil), out222...)}
	if err := m222.skipObject(b222); err != nil || b222.Len() != 0 {
		t.Fatalf("layer 222 formatted-date fallback does not parse cleanly (err=%v left=%d)", err, b222.Len())
	}

	out223, err := Transcode(raw, 223)
	if err != nil {
		t.Fatalf("transcode message->223: %v", err)
	}
	if !bytes.Contains(out223, entityCRC(formattedDateEntityCRC)) {
		t.Fatalf("layer 223 output did not preserve formatted-date entity")
	}
	m223 := loadLayerModel(t, 223)
	b223 := &bin.Buffer{Buf: append([]byte(nil), out223...)}
	if err := m223.skipObject(b223); err != nil || b223.Len() != 0 {
		t.Fatalf("layer 223 formatted-date output does not parse cleanly (err=%v left=%d)", err, b223.Len())
	}
}

// TestTranscodePassthroughNonAPI verifies that a top-level constructor absent
// from the tg schema (an MTProto control object such as rpc_error) passes
// through untouched at any layer.
func TestTranscodePassthroughNonAPI(t *testing.T) {
	var b bin.Buffer
	b.PutID(0xc4b9f9bb) // rpc_error#c4b9f9bb (mt.*), not a tg API constructor
	b.PutInt(420)
	b.PutString("FLOOD_WAIT")
	raw := b.Copy()
	out, err := Transcode(raw, 220)
	if err != nil {
		t.Fatalf("passthrough transcode: %v", err)
	}
	if !bytes.Equal(out, raw) {
		t.Errorf("non-API object was modified by transcode")
	}
}

// TestTranscodeChangedTypeNestedInUnchangedContainer is the case raised in
// review: an outer constructor whose CRC is IDENTICAL across 227 and the target
// layer (so a naive "same CRC ⇒ copy verbatim" would be wrong) but which nests a
// CHANGED type (message). The dirty closure must mark the outer container dirty
// purely because it can transitively reach a changed type, so the transcoder
// keeps the outer CRC yet recurses and rewrites the inner message to the target.
func TestTranscodeChangedTypeNestedInUnchangedContainer(t *testing.T) {
	const (
		message227CRC = 0x7600b9d3
		message220CRC = 0xb92f76cf
	)
	// updates#... nests Vector<Update> → updateNewMessage → message:Message.
	updates := &tg.Updates{
		Updates: []tg.UpdateClass{
			&tg.UpdateNewMessage{
				Message: &tg.Message{ID: 7, PeerID: &tg.PeerUser{UserID: 2}, Date: 1, Message: "nested"},
				Pts:     1, PtsCount: 1,
			},
		},
		Users: []tg.UserClass{&tg.User{ID: 2, AccessHash: 5, FirstName: "A"}},
		Chats: []tg.ChatClass{},
		Date:  100, Seq: 1,
	}

	// Premise of the question: the OUTER container's CRC is unchanged at 220.
	m220 := loadLayerModel(t, 220)
	if canonical.byName["updates"].crc != m220.byName["updates"].crc {
		t.Skip("updates CRC differs 220<->227; premise no longer holds")
	}

	raw := mustEncode(t, updates)
	out, err := Transcode(raw, 220)
	if err != nil {
		t.Fatalf("transcode updates->220: %v", err)
	}

	// Outer CRC preserved (it really is unchanged).
	if id, _ := (&bin.Buffer{Buf: out}).PeekID(); id != canonical.byName["updates"].crc {
		t.Fatalf("outer updates id changed to %#08x", id)
	}
	// Inner message rewritten to the 220 constructor; the 227 one must be gone.
	le := func(crc uint32) []byte { return []byte{byte(crc), byte(crc >> 8), byte(crc >> 16), byte(crc >> 24)} }
	if bytes.Contains(out, le(message227CRC)) {
		t.Errorf("downgraded output still contains the 227 message constructor")
	}
	if !bytes.Contains(out, le(message220CRC)) {
		t.Errorf("downgraded output missing the 220 message constructor")
	}
	// Rigorous: the whole thing must parse cleanly against the real 220 schema —
	// impossible if a 227-only nested constructor leaked through.
	b := &bin.Buffer{Buf: append([]byte(nil), out...)}
	if err := m220.skipObject(b); err != nil || b.Len() != 0 {
		t.Fatalf("downgraded updates invalid at 220 (err=%v left=%d)", err, b.Len())
	}
}

// TestTranscodePollResults exercises the pollAnswerVoters structural transform.
func TestTranscodePollResults(t *testing.T) {
	raw := mustEncode(t, canonicalCorpus()[10]) // PollResults
	for layer := SupportedFloor; layer < CanonicalLayer; layer++ {
		out, err := Transcode(raw, layer)
		if err != nil {
			t.Fatalf("layer %d: pollResults transcode: %v", layer, err)
		}
		model := loadLayerModel(t, layer)
		b := &bin.Buffer{Buf: append([]byte(nil), out...)}
		if err := model.skipObject(b); err != nil || b.Len() != 0 {
			t.Errorf("layer %d: pollResults invalid (err=%v left=%d)", layer, err, b.Len())
		}
	}
}

// TestTranscodePollAnswerVotersAbsentFlag exercises the structural transform's
// flag-bit-2-unset path: a pollAnswerVoters whose voters field is absent in 227
// (flags.2 clear) must still emit voters:0 (unconditional int) at older layers.
func TestTranscodePollAnswerVotersAbsentFlag(t *testing.T) {
	// voters absent (flag bit 2 unset): Voters=0 ⇒ gotd SetFlags leaves flags.2 clear.
	pr := &tg.PollResults{
		Results:     []tg.PollAnswerVoters{{Option: []byte{0}, Chosen: true}},
		TotalVoters: 0,
	}
	raw := mustEncode(t, pr)
	for layer := SupportedFloor; layer < CanonicalLayer; layer++ {
		out, err := Transcode(raw, layer)
		if err != nil {
			t.Fatalf("layer %d: transcode: %v", layer, err)
		}
		model := loadLayerModel(t, layer)
		b := &bin.Buffer{Buf: append([]byte(nil), out...)}
		if err := model.skipObject(b); err != nil || b.Len() != 0 {
			t.Errorf("layer %d: voters-absent pollResults invalid (err=%v left=%d)", layer, err, b.Len())
		}
	}
}
