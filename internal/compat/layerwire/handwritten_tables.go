package layerwire

// handwrittenTables covers client-private TL constructor drift that does not
// exist in the official historical schemas used by tables_gen.go.
var handwrittenTables = map[int]layerRaw{
	211: telegramSwift211,
}

// telegramSwift211 targets the TelegramSwift 11.15 runtime schema observed in
// SafeLink macOS builds. The client negotiates Layer 211, but its generated
// TelegramApi uses a few constructor ids that differ from both canonical 227 and
// the official 220 floor. Without these mappings, login succeeds server-side but
// the client aborts while decoding auth.Authorization.user.
var telegramSwift211 = layerRaw{
	rules: map[uint32]ruleRaw{
		0x31774388: {target: 0x020b1422, keep: []string{
			"flags", "self", "contact", "mutual_contact", "deleted", "bot", "bot_chat_history", "bot_nochats", "verified", "restricted", "min", "bot_inline_geo", "support", "scam", "apply_min_photo", "fake", "bot_attach_menu", "premium", "attach_menu_enabled",
			"flags2", "bot_can_edit", "close_friend", "stories_hidden", "stories_unavailable", "contact_require_premium", "bot_business", "bot_has_main_app",
			"id", "access_hash", "first_name", "last_name", "username", "phone", "photo", "status", "bot_info_version", "restriction_reason", "bot_inline_placeholder", "lang_code", "emoji_status", "usernames", "color", "profile_color", "bot_active_users", "bot_verification_icon", "send_paid_messages_stars",
		}}, // user
		0x1c32b11c: {target: 0xfe685355, keep: []string{
			"flags", "creator", "left", "broadcast", "verified", "megagroup", "restricted", "signatures", "min", "scam", "has_link", "has_geo", "slowmode_enabled", "call_active", "call_not_empty", "fake", "gigagroup", "noforwards", "join_to_send", "join_request", "forum",
			"flags2", "stories_hidden", "stories_hidden_min", "stories_unavailable", "signature_profiles", "autotranslation",
			"id", "access_hash", "title", "username", "photo", "date", "restriction_reason", "admin_rights", "banned_rights", "default_banned_rights", "participants_count", "usernames", "color", "profile_color", "emoji_status", "level", "subscription_until_date", "bot_verification_icon", "send_paid_messages_stars", "linked_monoforum_id",
		}}, // channel
		0x7600b9d3: {target: 0x9815cec8, keep: []string{
			"flags", "out", "mentioned", "media_unread", "silent", "post", "from_scheduled", "legacy", "edit_hide", "pinned", "noforwards", "invert_media",
			"flags2", "offline", "video_processing_pending",
			"id", "from_id", "from_boosts_applied", "peer_id", "saved_peer_id", "fwd_from", "via_bot_id", "via_business_bot_id", "reply_to", "date", "message", "media", "reply_markup", "entities", "views", "forwards", "replies", "edit_date", "post_author", "grouped_id", "reactions", "restriction_reason", "ttl_period", "quick_reply_shortcut_id", "effect", "factcheck", "report_delivery_until_date", "paid_message_stars", "suggested_post",
		}}, // message
		0x06cbe645: {target: 0x7e63ce1f, keep: []string{
			"flags", "blocked", "phone_calls_available", "phone_calls_private", "can_pin_message", "has_scheduled", "video_calls_available", "voice_messages_forbidden", "translations_disabled", "stories_pinned_available", "blocked_my_stories_from", "wallpaper_overridden", "contact_require_premium", "read_dates_private",
			"flags2", "sponsored_enabled", "can_view_revenue", "bot_can_manage_emoji_status", "display_gifts_button",
			"id", "about", "settings", "personal_photo", "profile_photo", "fallback_photo", "notify_settings", "bot_info", "pinned_msg_id", "common_chats_count", "folder_id", "ttl_period", "theme", "private_forward_name", "bot_group_admin_rights", "bot_broadcast_admin_rights", "wallpaper", "stories", "business_work_hours", "business_location", "business_greeting_message", "business_away_message", "business_intro", "birthday", "personal_channel_id", "personal_channel_message", "stargifts_count", "starref_program", "bot_verification", "send_paid_messages_stars", "disallowed_gifts", "stars_rating", "stars_my_pending_rating", "stars_my_pending_rating_date",
		}}, // userFull
		0x1d73e7ea: {target: 0x8c718e87, keep: []string{
			"messages", "chats", "users",
		}}, // messages.messages
		0x5f206716: {target: 0x762b263d, keep: []string{
			"flags", "inexact", "count", "next_rate", "offset_id_offset", "search_flood", "messages", "chats", "users",
		}}, // messages.messagesSlice
		0x313a9547: {target: 0x00bcff5b, keep: []string{
			"flags", "limited", "sold_out", "birthday", "limited_per_user",
			"id", "sticker", "stars", "availability_remains", "availability_total", "availability_resale", "convert_stars", "first_sale_date", "last_sale_date", "upgrade_stars", "resell_min_stars", "title", "released_by", "per_user_total", "per_user_remains",
		}}, // starGift
		0x9e84bc99: {target: 0x9c974fdf, keep: []string{
			"flags", "folder_id", "peer", "max_id", "still_unread_count", "pts", "pts_count",
		}}, // updateReadHistoryInbox
		0xea2c31d3: {target: 0x4717e8a4, keep: []string{
			"flags", "name_hidden", "saved", "converted", "upgraded", "refunded", "can_upgrade",
			"gift", "message", "convert_stars", "upgrade_msg_id", "upgrade_stars", "from_id", "peer", "saved_id",
		}}, // messageActionStarGift
	},
}
