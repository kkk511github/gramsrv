# SafeLink 生产部署排坑记录

本文记录 2026-07-06 新服务器部署 `slerv`、`web.safelink.chat` 时实际踩到的坑。重点不是“怎么从零部署”，而是把容易导致 Web 登录失败、客户端连不上、品牌名回退、贴纸/表情丢失的点写清楚，方便以后换机器或重装时对照。

## 部署目标

- 服务端进程：`slerv`
- 服务端目录：`/www/safelink/slerv`
- Web 静态目录：`/www/safelink/web`
- Redis 数据目录：`/www/safelink/redis`
- PostgreSQL 数据目录：`/www/safelink/postgres`
- MTProto / WebSocket：`0.0.0.0:2398`
- Admin：`127.0.0.1:2399`
- Sticker link helper：`127.0.0.1:2401`
- 主域名：`safelink.chat`
- Web 域名：`web.safelink.chat`

生产机器尽量把运行数据放到 `/www` 这类数据盘，不要塞系统盘。迁移到新机器时先确认挂载：

```sh
df -h
df -i
```

## 推荐部署顺序

1. 配好 DNS，让 `safelink.chat` 和 `web.safelink.chat` 指到目标服务器。
2. 准备 `/www/safelink/{slerv,web,redis,postgres,logs}`。
3. 配 PostgreSQL、Redis，先确认它们能写入数据盘。
4. 部署并启动 `slerv`，确认 `/apiws` 能被 Nginx 反代到 `2398`。
5. 取当前服务端 `server_rsa.pem` 的公钥和 fingerprint，同步到 Web 端内置 RSA key。
6. 构建并部署 Web 静态文件。
7. 处理 service worker 旧缓存，必要时提供 `/reset-web`。
8. 用 Web 手机号登录跑一次真实链路，最后看服务端日志。

## 关键环境变量

生产环境不要靠代码里散落的字符串改品牌名，统一由服务端环境变量下发：

```sh
TELESRV_APP_NAME=SafeLink
TELESRV_LISTEN=0.0.0.0:2398
TELESRV_ADVERTISE_IP=<server-public-ip>
TELESRV_POSTGRES_DSN=<postgres-dsn>
TELESRV_REDIS_ADDR=127.0.0.1:6399
TELESRV_STICKER_SEED_DIR=/www/safelink/slerv/data/sticker-seed
TELESRV_PUBLIC_BASE_URL=https://safelink.chat
TELESRV_PUBLIC_LINK_WEB_ADDR=127.0.0.1:2401
TELESRV_PUBLIC_LINK_APP_SCHEME=safelink
TELESRV_WEBSOCKET_ORIGINS=https://safelink.chat,http://safelink.chat,https://web.safelink.chat,http://web.safelink.chat
```

`TELESRV_DEV_AUTH_CODE` 只适合测试环境。生产真上线前要换成真实短信/验证码链路，或者至少不要把固定测试码暴露给外部用户。

## 坑 1：Redis 被 systemd 沙箱限制，登录一直卡住

### 症状

Web 能打开，`/apiws` 也返回 `101 Switching Protocols`，但是手机号登录提交后卡在：

```text
PLEASE WAIT...
```

服务端日志里反复出现：

```text
auth.sendCode ... rpc error code 500: INTERNAL_SERVER_ERROR
Persist session failed ... MISCONF Redis is configured to save RDB snapshots,
but it's currently unable to persist to disk. Commands that may modify the data
set are disabled
```

Redis 日志里真正原因是：

```text
Failed opening the temp RDB file temp-xxxxx.rdb
(in server root dir /www/safelink/redis) for saving: Read-only file system
```

### 原因

Ubuntu/Debian 默认 `redis-server.service` 带了 systemd 沙箱：

```text
ProtectSystem=strict
ReadWritePaths=-/var/lib/redis -/var/log/redis -/var/run/redis -/etc/redis
```

我们把 Redis `dir` 改到 `/www/safelink/redis` 后，目录权限虽然是 `redis:redis`，但 systemd 仍然把它当只读路径。Redis RDB 保存失败后，又因为：

```text
stop-writes-on-bgsave-error yes
```

所以 Redis 会拒绝所有写命令，服务端写登录会话/验证码失败，最终 `auth.sendCode` 返回 500。

### 修复

给 Redis 加 systemd override：

```sh
sudo install -d -m 0755 /etc/systemd/system/redis-server.service.d
sudo tee /etc/systemd/system/redis-server.service.d/safelink-www.conf >/dev/null <<'EOF'
[Service]
ReadWritePaths=-/www/safelink/redis
EOF

sudo chown -R redis:redis /www/safelink/redis
sudo systemctl daemon-reload
sudo systemctl restart redis-server
```

如果 `systemctl restart redis-server` 卡住，是因为 Redis 停止前也在尝试保存旧 RDB。可以强制结束后再拉起：

```sh
sudo systemctl kill -s KILL redis-server || true
sudo systemctl reset-failed redis-server || true
sudo systemctl start redis-server
```

### 验证

```sh
systemctl is-active redis-server
systemctl show redis-server -p ReadWritePaths
redis-cli -p 6399 SET safelink:redis:writecheck ok
redis-cli -p 6399 BGSAVE
sleep 1
redis-cli -p 6399 INFO persistence | grep -E 'rdb_last_bgsave_status|rdb_changes_since_last_save|rdb_last_save_time'
ls -la /www/safelink/redis
```

必须看到：

```text
rdb_last_bgsave_status:ok
```

然后重启服务端：

```sh
sudo systemctl restart slerv
sudo systemctl is-active slerv
```

再次 Web 登录时，服务端日志应变为：

```text
auth.sendCode ... RPC handled ... db_errors: 0
auth.signIn ... RPC handled ... db_errors: 0
```

## 坑 2：Web 内置 RSA 公钥和新服务器不一致

### 症状

Web 登录页能打开，但二维码登录/手机号登录初始化失败，浏览器控制台出现：

```text
[MT] No public key found
SignQRCard: default error: Error: [MT] No public key found
```

### 原因

`slerv` 第一次启动会生成 `data/server_rsa.pem`。换服务器、清数据或重建服务目录后，RSA key 可能变了。

Telegram Web 的 MTProto 握手会拿服务端返回的 RSA fingerprint 去本地内置 key 表里找。如果 Web 端 `src/lib/mtproto/rsaKeysManager.ts` 还是旧 fingerprint，就会直接报 `No public key found`。

### 修复原则

优先保持生产服务器 `server_rsa.pem` 稳定，不要每次部署重新生成。确实换了 key 时，必须同步 Web 端 RSA key：

1. 从服务器取当前 public key。
2. 计算 modulus 和 fingerprint alias。
3. 更新 Web 的 `src/lib/mtproto/rsaKeysManager.ts`。
4. 重新构建并部署 Web。

服务端日志会打印 fingerprint，例如：

```text
rsa_fingerprint=-7080122280541503745
```

Web 端代码里常用的是无符号 hex alias，例如：

```text
9dbe5a4c45febeff
```

### 验证

重新打开 Web 登录页后，控制台不应再出现：

```text
No public key found
```

服务端应能看到：

```text
auth.exportLoginToken ... RPC handled
```

或手机号登录链路里的：

```text
auth.sendCode ... RPC handled
```

## 坑 3：旧 service worker 缓存导致 Web 一直跑旧包

### 症状

已经重新部署 Web，但浏览器仍然加载旧 bundle，例如控制台里还是旧文件名：

```text
index-xxxx.js
index.worker-yyyy.js
```

或者页面出现：

```text
UPDATE
Updating...
```

也可能出现已经修过的旧错误仍然存在，例如旧包里的：

```text
Cannot read properties of undefined (reading 'classList')
```

### 原因

Telegram Web 自带 service worker。部署时使用 `rsync --delete` 替换 `dist/` 后，用户浏览器里旧 service worker 仍可能继续控制页面，导致用户拿到旧 JS、旧 worker 或旧缓存。

### 修复

Nginx 保留一个清缓存入口，例如：

```nginx
location = /reset-web {
    add_header Cache-Control "no-store, no-cache, must-revalidate" always;
    add_header Clear-Site-Data "\"cache\", \"storage\"" always;
    default_type text/html;
    return 200 '<!doctype html><meta http-equiv="refresh" content="1;url=/"><body>Resetting SafeLink Web cache...</body>';
}
```

同时对旧 service worker 文件名兜底，例如所有 `sw-*.js` 如果找不到，都回落到一个 rescue worker：

```nginx
location = /sw-rescue.js {
    root /www/safelink/web;
    add_header Cache-Control "no-store, no-cache, must-revalidate" always;
    add_header Clear-Site-Data "\"cache\", \"storage\"" always;
    add_header Service-Worker-Allowed "/" always;
}

location ~ ^/sw-[A-Za-z0-9_-]+\.js$ {
    root /www/safelink/web;
    try_files $uri /sw-rescue.js =404;
    add_header Cache-Control "no-store, no-cache, must-revalidate" always;
    add_header Clear-Site-Data "\"cache\", \"storage\"" always;
    add_header Service-Worker-Allowed "/" always;
}
```

`/www/safelink/web/sw-rescue.js`：

```js
self.addEventListener('install', event => {
  self.skipWaiting();
});

self.addEventListener('activate', event => {
  event.waitUntil((async () => {
    try {
      const keys = await caches.keys();
      await Promise.all(keys.map(key => caches.delete(key)));
    } catch (error) {}

    try {
      await self.registration.unregister();
    } catch (error) {}

    const clients = await self.clients.matchAll({ type: 'window', includeUncontrolled: true });
    for (const client of clients) {
      client.navigate(client.url);
    }
  })());
});
```

注意：如果 Web 部署命令是：

```sh
rsync -az --delete dist/ server:/www/safelink/web/
```

那么 `sw-rescue.js` 如果不是 `dist/` 的一部分，会被删掉。部署脚本里必须在 `rsync --delete` 之后重新写入，或者把它纳入 Web 构建产物。

### 验证

```sh
curl -I https://web.safelink.chat/reset-web
curl -I https://web.safelink.chat/sw-any-old-name.js
```

应能看到 `Clear-Site-Data` 和 `Cache-Control: no-store`。

用户侧仍旧异常时，让用户打开一次：

```text
https://web.safelink.chat/reset-web
```

然后再回到首页。

## 坑 4：Nginx `/apiws` 不是普通 HTTP，要确认 101

### 症状

Web 页面能加载，但登录页二维码转圈或手机号提交无反应。

### 原因

Web 通过 `/apiws` 走 WebSocket 连接 MTProto。Nginx 配置必须带 Upgrade headers，否则请求会被当普通 HTTP 代理。

### 参考配置

```nginx
location = /apiws {
    proxy_pass http://127.0.0.1:2398;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_read_timeout 3600s;
    proxy_send_timeout 3600s;
}
```

### 验证

```sh
curl -i -N \
  -H 'Connection: Upgrade' \
  -H 'Upgrade: websocket' \
  -H 'Sec-WebSocket-Version: 13' \
  -H 'Sec-WebSocket-Key: SGVsbG8sIHdvcmxkIQ==' \
  https://web.safelink.chat/apiws
```

应返回：

```text
HTTP/1.1 101 Switching Protocols
```

Nginx access log 里也应看到：

```text
"GET /apiws HTTP/1.1" 101
```

## 坑 5：WebSocket Origin 白名单漏域名

### 症状

`/apiws` 能连到 Nginx，但服务端拒绝 WebSocket，或浏览器控制台出现连接被关闭。

### 修复

服务端环境变量至少包含当前 Web 域名和主域名：

```sh
TELESRV_WEBSOCKET_ORIGINS=https://safelink.chat,http://safelink.chat,https://web.safelink.chat,http://web.safelink.chat
```

如果临时用 IP 测试，也把 IP origin 加进去，例如：

```sh
http://154.201.73.55
```

生产确认域名可用后，尽量不要长期保留无关 origin。

## 坑 6：品牌名不是只改 Web 文案

### 症状

刚打开是 SafeLink，过一会儿又变成 Telegram；或系统账号、登录验证码消息、贴纸机器人文案仍然出现 `Telegram`、`telesrv`。

### 原因

客户端很多文案来自服务端语言包、系统账号 seed、bot seed，不全是 Web 静态文本。只改 Web 仓库会漏。

### 修复原则

1. 应用展示名统一走 `TELESRV_APP_NAME=SafeLink`。
2. 服务端语言包在启动时根据 `TELESRV_APP_NAME` 做品牌替换和版本提升。
3. 系统账号、贴纸机器人、邀请链接、公开链接等 seed 数据里不要残留 `telesrv`。
4. 域名统一用 `safelink.chat` / `web.safelink.chat`。

### 验证

新账号登录后，系统消息应显示：

```text
Do not give this code to anyone, even if they say they are from SafeLink!
```

贴纸机器人帮助文案里不应出现：

```text
telesrv
Telegram
```

## 坑 7：贴纸、表情 seed 不是只拷贝元数据

### 症状

PC 能看到集合但右下角缺图，Android/PC 贴纸或表情空白，或者发送后先显示文件再变成贴纸。

### 原因

`slerv` 运行时读的是生成后的 `data/sticker-seed`。这个目录里既有 set 元数据，也有文档、缩略图、reaction 动画等多层文件。只拷贝 JSON 或顶层目录会导致客户端拿不到完整文档。

### 修复

从素材仓库生成完整 seed：

```sh
go run ./cmd/stickerseeddeploy \
  -source /path/to/HSgram_-premium-promo \
  -dest data/sticker-seed
```

特别注意：消息发送特效不是 `stickerseeddeploy` 自动生成的目录，必须额外保留或生成：

```text
data/sticker-seed/telegram_effects_export/effects.json
data/sticker-seed/telegram_effects_export/documents/
```

缺这个目录时，`messages.getAvailableEffects` 会返回空，iOS/PC 表情特效选择器会像“没了”一样。
有已授权 Telegram session 时用下面命令刷新官方特效 seed：

```sh
SESSION=/tmp/appearance.session go run ./cmd/stickerfetch data/sticker-seed effects
```

部署时完整同步目录树：

```sh
rsync -az data/sticker-seed/ server:/www/safelink/slerv/data/sticker-seed/
```

环境变量：

```sh
TELESRV_STICKER_SEED_DIR=/www/safelink/slerv/data/sticker-seed
TELESRV_STICKER_PUBLIC_URL=https://safelink.chat
```

重启后看日志里 seed 导入是否完成。

尤其要看 effects 阶段，数量必须大于 0：

```sh
journalctl -u slerv.service --since "5 minutes ago" --no-pager | grep '"phase": "effects"'
```

## 坑 8：部署 Web 后文件 owner 不是重点，但文件必须完整

Mac 上 `rsync` 到服务器后，静态文件 owner 可能显示成类似：

```text
501 staff
```

Nginx 只读静态文件时通常没问题。真正要确认的是：

```sh
ls -l /www/safelink/web/index-*.js /www/safelink/web/index.worker-*.js
grep -o 'index-[^"<]*\.js' /www/safelink/web/index.html | sort -u
```

如果 Web 静态目录用 `rsync --delete`，不要忘了重新生成或保留 `sw-rescue.js`。

## 本次问题的最终判断链路

这次 Web 登录不进去，按时间线实际经历了三层问题：

1. 旧 service worker 让浏览器继续跑旧包。
2. Web 内置 RSA key 还是旧服务器的，导致 `[MT] No public key found`。
3. RSA 修好后，`auth.sendCode` 仍然 500，最终定位到 Redis 因 systemd 只读路径导致 RDB 保存失败，进而禁止写入。

修完后的关键日志：

```text
auth.sendCode ... RPC handled ... db_errors: 0
auth.signIn ... RPC handled ... db_errors: 0
auth.signUp ... RPC handled ... db_errors: 0
```

Redis 验证：

```text
rdb_last_bgsave_status:ok
```

Web 验证：

```text
SafeLink Web
Log in by QR Code
Scan with SafeLink app on your phone
```

登录后系统消息里也应是 `SafeLink`。

## 一键排查清单

### 服务是否活着

```sh
systemctl is-active slerv redis-server nginx
ss -ltnp | grep -E '2398|2399|2401|6399'
```

### Nginx 配置

```sh
nginx -t
tail -120 /var/log/nginx/access.log | grep apiws
tail -80 /var/log/nginx/error.log
```

### Redis 写入

```sh
redis-cli -p 6399 SET safelink:redis:writecheck ok
redis-cli -p 6399 BGSAVE
sleep 1
redis-cli -p 6399 INFO persistence | grep -E 'rdb_last_bgsave_status|rdb_changes_since_last_save'
```

### 服务端登录链路

```sh
journalctl -u slerv --since '15 minutes ago' --no-pager \
  | grep -E 'auth.sendCode|auth.signIn|auth.signUp|RPC error|INTERNAL_SERVER_ERROR|Persist session failed|MISCONF'
```

### Web 缓存

```sh
curl -I https://web.safelink.chat/
curl -I https://web.safelink.chat/reset-web
curl -I https://web.safelink.chat/sw-old-test.js
```

### WebSocket

```sh
curl -i -N \
  -H 'Connection: Upgrade' \
  -H 'Upgrade: websocket' \
  -H 'Sec-WebSocket-Version: 13' \
  -H 'Sec-WebSocket-Key: SGVsbG8sIHdvcmxkIQ==' \
  https://web.safelink.chat/apiws
```

必须返回 `101 Switching Protocols`。

## 长期建议

- 把 `sw-rescue.js` 纳入 Web 构建产物或部署脚本，避免每次 `rsync --delete` 后手工补。
- 生产 `server_rsa.pem` 要备份并稳定复用；换 key 必须同步所有客户端和 Web。
- Redis/PostgreSQL 如果放到 `/www`，同时检查 systemd 沙箱写路径，不只看 Linux 文件权限。
- 品牌名统一从服务端配置和 seed 入口维护，不要三端、Web、服务端到处手改字符串。
- 新机器部署完必须跑一次真实 Web 登录链路，而不是只看首页能打开。
