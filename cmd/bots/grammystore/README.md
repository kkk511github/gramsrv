# SafeLink grammY service bot

This service replaces the former JSON/Python bot with one grammY process and a
transactional SQLite database. It deliberately runs independently from
`cmd/telesrv`; a bot outage cannot stop MTProto.

## Production functionality

- delivery and storage of real login codes through authenticated `POST /code`;
- Premium, SafeLink Stars and collectible username product workflows;
- arbitrary Stars invoices, payment deduplication and a durable sales journal;
- compensated refunds that revoke the exact Stars, Premium entitlement,
  collectible username or paid number before returning payment Stars;
- account target IDs and three recent recipients;
- daily bonuses, referrals and a weighted wheel;
- promo codes and button-based giveaways;
- support tickets;
- complete Chinese, Russian and English localization for menus, keyboards, invoices,
  errors, login codes and Bot API command descriptions;
- per-user language and notification settings; broadcasts skip disabled and
  stale recipients;
- owner-only statistics, broadcasts, Stars/Premium/bonus grants, invoices,
  payment refunds, login-code access, support replies, sales and Stars-rate controls;
- optional required-channel membership gate.

The bot token and Admin API token must never be committed. `.env.example`
contains names and safe local defaults only.

## Local run

Requirements: Node.js 22.13 or newer and a running gramsrv Admin API.

```bash
cd cmd/bots/grammystore
cp .env.example .env
nano .env
npm ci
npm test
npm start
```

Set `BOT_API_ROOT=http://127.0.0.1:8081` and
`PUBLIC_BASE_URL=https://safelink.chat`. `BOT_PUBLIC_USERNAME` makes referral
links available before the first `getMe`. `OWNER_IDS` accepts comma-separated
bot user IDs. Users set their SafeLink account ID under Settings; the bot user
ID is not assumed to equal the SafeLink account ID.

`PRODUCT_NAME` controls user-facing product text and defaults to `SafeLink`.
Deployment-specific branding belongs in the service environment, not in source.
`DEFAULT_LANGUAGE` is used until the client supplies or the user selects a
supported language. The selection is stored in SQLite and is not overwritten by
later client updates. Bot command descriptions are registered separately for
`zh`, `ru` and `en` client locales.

`PAYMENTS_ENABLED=false` is the production-safe default. It hides invoices,
the store and legacy anonymous-number controls while the local SafeLink Bot API
does not implement the full Stars payment and refund methods. The purchase and
compensated-refund code remains available for a future compatible Bot API; do
not enable it until `sendInvoice`, pre-checkout updates, successful-payment
updates and `refundStarPayment` have been verified end to end.

Each paid fulfillment is snapshotted in the sales journal. Refunds are phased and
idempotent: if the Bot API is temporarily unavailable after the server-side product
has been revoked, retrying the same transaction does not revoke it twice. Legacy
Premium and anonymous-number purchases without exact fulfillment metadata fail
safe and require manual review instead of touching unrelated account state.

## Migrating the former Python bot

Never open the legacy database directly with the new service because its table
names overlap but its columns are incompatible. Keep the old service stopped,
copy its database, and create a separate destination:

```bash
npm run migrate:legacy -- /path/to/legacy.sqlite3 /path/to/new.sqlite3
```

The command refuses to overwrite its source or an existing destination. It
preserves users, bonus balances, referrals, numbers, current login codes and
support messages. Keep the legacy database backup for historical orders and
broadcast drafts, which have no equivalent in the new payment journal.

## Login-code webhook

Configure gramsrv's code-delivery webhook for:

```text
TELESRV_PHONE_CODE_DELIVERY_PROVIDER=webhook
TELESRV_OTP_WEBHOOK_URL=http://127.0.0.1:2800/v1/otp/deliveries
TELESRV_OTP_WEBHOOK_SECRET=<same value as CODE_WEBHOOK_SECRET>
```

gramsrv sends its version-1 JSON envelope and signs the exact request body as
`X-Telesrv-Signature: sha256=<HMAC-SHA256(timestamp + "." + body)>`. The bot
checks that signature, a five-minute timestamp window and `Idempotency-Key`.
For example, the body contains:

```json
{"version":"1","delivery_id":"...","purpose":"login_sms","channel":"sms","recipient":"+79991234567","code":"12345","expires_at":"2026-08-09T12:00:00Z","expires_in":300}
```

The endpoint is loopback-only by default. It rejects requests without the
HMAC secret, stores each delivery idempotently and returns HTTP 202 immediately;
SafeLink bot delivery then runs asynchronously for the number owner and explicitly
granted support viewers. A slow Bot API therefore cannot turn `auth.sendCode`
into a server-side timeout. `/healthz` is read-only.

## Linux install

```bash
sudo useradd --system --home /var/lib/safelink-grammy-bot --shell /usr/sbin/nologin safelink-bot || true
sudo install -d -o safelink-bot -g safelink-bot -m 0750 /opt/safelink-grammy-bot /var/lib/safelink-grammy-bot
sudo cp -a package.json package-lock.json src /opt/safelink-grammy-bot/
cd /opt/safelink-grammy-bot
/opt/safelink-node/bin/npm ci --omit=dev
sudo cp .env.example /etc/safelink-grammy-bot.env
sudo chmod 0600 /etc/safelink-grammy-bot.env
sudo cp deploy/safelink-grammy-bot.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now safelink-grammy-bot
sudo journalctl -u safelink-grammy-bot -f
```

Use `BOT_DB_PATH=/var/lib/safelink-grammy-bot/bot.sqlite3` in the production env.
Back up the database with SQLite's online backup command or while the service is
stopped; include `/etc/safelink-grammy-bot.env` in a separate encrypted secret
backup.

The systemd unit uses `/opt/safelink-node/bin/node`; deploy a supported Node.js
runtime there before enabling the service. The unit intentionally has no
hosting-specific values.
