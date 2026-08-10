# SafeLink grammY service bot

This service replaces the former JSON/Python bot with one grammY process and a
transactional SQLite database. It deliberately runs independently from
`cmd/telesrv`; a bot outage cannot stop MTProto.

## Production functionality

- Chinese-first SafeLink service menu and `safelink.chat` referral links;
- SafeLink account ID binding, daily rewards, referrals and a weighted wheel;
- promo codes and button-based giveaways;
- support tickets and owner replies;
- owner-only statistics, broadcasts, Stars/Premium/bonus grants and Stars-rate controls;
- optional delivery of real login codes through authenticated `POST /code`;
- a dedicated, least-privilege Admin API token for Stars, Premium and collectible
  username grants;
- all Bot API calls routed to the local SafeLink Bot API, never to an external
  public Bot API.

The source also contains an invoice-backed Premium, Stars and collectible
username store. Keep `PAYMENTS_ENABLED=false` until SafeLink Bot API implements
durable invoice creation, pre-checkout state, successful-payment updates and
refunds. The public menu hides the store while disabled. Locally generated
anonymous numbers are also intentionally hidden because they do not provision a
real SafeLink account.

## Additional stored capabilities

- account target IDs and three recent recipients;
- language and notification settings;
- payment deduplication and a durable sales journal for the future payment path;
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
`DEFAULT_LANGUAGE=zh` is the production default.

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
