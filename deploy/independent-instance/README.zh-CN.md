# SafeLink 87 独立实例

## 部署边界

服务端协议依赖固定到 `github.com/kkk511github/td` 的 `safelink/private-chat-rpc` 分支提交 `f195e248dfb2`，具体伪版本写在 `go.mod` 的 replace 中。该版本包含现有三端禁止私聊 RPC；不要恢复成未打补丁的上游模块，也不要提交 `../td-safelink` 等本机绝对或相对路径依赖。常规构建直接运行 `go build ./cmd/slerv`，无需手工复制协议库。

- 主机：`212.189.31.87`；应用目录：`/home/safelink-chat`。
- 这是新实例，不连接或迁移已停用的 154 实例。
- MTProto：TCP `2398`；公开页面：`https://safelink.chat`。
- 客户端发现：`https://safelink.chat/.well-known/safelink-client.json`。
- 保留 IP 兼容入口 `https://212.189.31.87:8443` 和 IP HTTPS 443 的配置发现路由。
- IP 的 HTTPS 443 原签名门户继续保留；新配置发现只占用一个精确路由。
- PostgreSQL、Redis、Admin API、后台 UI 源站仅绑定回环地址；后台和 Web 聊天由 Nginx 提供 HTTPS 入口。

## 域名入口

`safelink.chat` 指向 `212.189.31.87`，TLS 证书由服务器现有 Certbot 配置续期。
在现有域名 Nginx 虚拟主机中，仅将原先返回 404 的 `location /` 改为反代
`http://127.0.0.1:2401`，保留其他应用的精确路径、ACME 验证路径和签名门户。
不要新增同名冲突虚拟主机，也不要覆盖已有其他服务。

```dotenv
TELESRV_PUBLIC_BASE_URL=https://safelink.chat
TELESRV_PUBLIC_WEB_BASE_URL=https://web.safelink.chat
TELESRV_WEBSOCKET_ENABLE=true
TELESRV_WEBSOCKET_ALLOWED_ORIGINS=https://web.safelink.chat
```

Web 聊天客户端已部署到 `https://web.safelink.chat`，`/apiws` 经 Nginx 升级并转发到 `127.0.0.1:2398`。
域名切换不改变 MTProto IP、DC、公钥或账号数据库，旧客户端不需要更换公钥。
发现接口里的 `host` 仍为 MTProto 的数字 IP，不能直接改成域名绕过客户端校验。

设备列表响应对当前设备返回 `hash=0`；其他设备保留真实撤销 hash。
修复只改变返回投影，不改数据库中的授权标识。

## 后台与 Web 运维

### 后台域名配置

- 入口：服务器设置 → 域名与链接。公开链接域名和网页版地址仅接受 HTTPS 根地址。
- 服务和后台都配置 `TELESRV_PUBLIC_LINK_SETTINGS_FILE=/data/link-settings.json`，通过共享 `/data` 保存；无需开放 `.env` 写入权限或 Docker socket。
- 保存包含格式校验、预演与确认，写入原子替换的私有 JSON 文件。**保存不等于上线**：主服务启动时读取覆盖配置，保存后需要重启主服务和后台。
- 切换前先配置新域名 DNS、证书和 Nginx。新公开域名反代 2401，新 Web 域名部署 `current` 静态目录、`/apiws` 反代 2398。后台不能代替域名提供商或自动签发证书，表单不宣称已验证 DNS/TLS。
- 应用层自动同步公开链接根地址、品牌链接配置、Web 入口和 WebSocket Origin；客户端协议保持 `safelink://`。不要修改公钥、DC、IP 或账号数据库来切换公开域名。
- Web 传输使用同源 `/apiws`，同一实例换域名无需重编 Web。浏览器按域名隔离登录数据，新 Web 域名首次访问需要重新登录。
- 原生客户端需先安装注册 `safelink://` 的兼容版本；已安装且只注册 `tg://` 的旧包无法由服务端补注册。新域名的普通 HTTPS 链接经落地页点击“打开 SafeLink”，不依赖逐个域名写入 iOS Universal Links/Android App Links。
- 保留旧域名的有效证书及路由，旧分享链接才能继续访问。第三方 OIDC issuer/回调、Bot webhook、外部下载地址属于独立契约，不随此表单自动迁移。
- 回退：在后台保存原域名后重启，或恢复之前的 `link-settings.json`。缺省没有该文件时仍使用环境变量。文件损坏时启动报错，不默默回退旧域名。

### 管理与发布

- 后台入口：`https://admin.hsgram.cloud`，源站 `127.0.0.1:2600`，镜像 `safelink-admin:local`。
- 在 `/home/safelink-chat` 使用 `docker compose -f compose.yaml -f safelink-admin87-compose.yaml` 管理完整服务。
- 后台凭据保存在服务器 `admin.env`，权限 600，不提交仓库；管理员用户名为 `admin`。
- 后台与主服务共享数据库和 `/data`，无 Docker socket 权限；环境文件只读挂载，因此容器内不能保存环境配置或重启宿主机服务。服务器环境配置仍由 SSH 修改后重新创建容器。
- Web 发布物在 `/home/safelink-web/releases/20260930-links-ui`，`current` 指向当前版本；后续更新使用新目录并原子切换链接。
- Web 已固定本实例 RSA 公钥，IndexedDB、localStorage 和媒体缓存使用 `safelink-5cc5b7bffe3c4275-` 前缀。旧实例数据不删除，也不作为当前实例登录凭据复用。
- Nginx 活动配置是 `/etc/nginx/sites-enabled/safelink-web-admin-http` 和 `safelink-web-admin-https`；本机现有活动配置为普通文件，不要假设它们是 sites-available 的符号链接。
- 证书名 `safelink-web-admin` 包含 Web 和后台域名，使用 Certbot webroot `/var/www/html` 续期。现有 safelink.chat 和 IP 签名门户证书保持不变。
- 2026-09-30 Web 生产构建及 TypeScript 检查通过，HTTPS 登录页实际生成二维码，未进行真实账号登录；后台 HTTPS 登录、读取接口、退出及未认证 401 均需在发布后复查。

## 语音视频通话检查

2026-09-30 检查本实例：

- 私聊 TURN/STUN 为 UDP `212.189.31.87:12400`，媒体中继分配范围 UDP `12500-12999`；群通话 SFU 为 UDP `12399`。
- 服务实际监听和下发地址均为 87；主机 INPUT 为 ACCEPT，UFW 未启用。若以后启用防火墙或安全组，必须保留以上 UDP 端口。
- 从外网完成 STUN Binding、短期凭据认证、两路 TURN allocation 和双向中继 UDP 数据测试，均通过；主密钥不写入测试输出。
- 呼叫状态、权限、信令、视频生命周期和 SFU Opus/视频基础层转发回归测试通过。iOS 原生调用链读取服务端下发的 STUN/TURN 地址、端口、用户名和密码，无需另写固定 IP。
- 当前 TURN 只监听 UDP，没有 TCP/TLS 中继后备；禁 UDP 的网络尚不能保证通话。
- **未完成：iOS VoIP/PushKit 来电唤醒。** `phone_push.go` 只向在线 MTProto 会话发送来电；现有 APNs 发送器为普通 alert 通知，不能替代 VoIP 来电。App 被挂起、锁屏或退出后的接听不能标为正常。
- **待真机验收：** 两台设备跨 Wi-Fi/蜂窝双向拨打语音、视频，检查铃声、音频双向、摄像头、静音、切换前后摄像头、挂断和后台接听。自动化链路测试不等同于真机验收。

## 身份与登录

### 服务器头像持久化

服务端和后台的 Compose 都必须显式设置 `TELESRV_IDENTITY_DIR=/data/identity`，
并共享宿主机 `data/server:/data`。目录归 UID/GID `10001:10001`，不关闭容器的
`read_only`。默认相对路径 `data/identity` 在 `/app` 下不可写，会导致
`identity: mkdir: mkdir data: read-only file system`。修改环境配置后需要重建容器，
仅 `restart` 不会更新容器环境。2026-09-30 已检查后台用户写入、原子重命名以及
主服务读取目录；失败的头像上传不会自动重传，需要在后台重新选择图片上传。

### 禁止私聊与隐藏成员

- iOS、安卓、PC 共用 `safelink.getGroupPrivateChatForbidden#06ebdea1` 与
  `safelink.toggleGroupPrivateChatForbidden#94ba7b67`。它们必须进入生成的 exact-layer
  协议目录和 `DispatchAdmitted`；仅在旧 `Router.Dispatch` 中添加处理函数不够。
  否则生产会在业务处理前返回 `CONNECTION_LAYER_INVALID`，iOS 会捕获错误并回退开关。
- 当前服务端 `go.mod` 使用相邻 `../td-safelink`，基于 TD `v1.3.4`，附加
  `_schema/safelink.tl`、manifest 和生成的 `tg` / `tlprofile` 文件。旧 `td-layer211`
  保持不变。**不能用移除 replace 的临时 modfile 构建发布包**，否则会丢失私有接口。
  后续推送或换机器构建时必须一起保存、发布该依赖的 schema、manifest、生成代码；
  当前依赖仍为本地分支，并未发布到远端。
- 回归测试覆盖 layer 225–229 的裸请求、invokeWithLayer、开启/关闭与读取、普通成员
  无权修改、未登录拒绝和非法请求。生产探测以未登录会话验证真实加密链路已进入鉴权，
  不会修改真实群开关；这不等同于三端真机操作验收。
- 隐藏成员与禁止私聊是两个独立开关。隐藏成员不会对群主和管理员隐藏名单，也不会
  删除已有消息中的发言者或本地联系人。2026-09-30 对 87 的群 ID 1 做只读检查：
  `participants_hidden=true`，群主仍返回 3 人，两个普通成员均返回 0 人，保留总人数 3。
- `policycheck` 可检查指定生产群的持久化权限，连接强制 `default_transaction_read_only=on`，
  不创建账号、不修改群或会话。需用 Compose 的 env_file 解析启动，避免 Docker 原生
  `--env-file` 将 DSN 外层引号作为值的一部分。
- 本次服务器及头像目录配置备份：`/home/safelink-chat/backups/20260930-151318-private-chat`。

客户端默认公钥属于本实例，服务端 ID 为
`5cc5b7bffe3c42758a7d4a44c168feb59039b8099ae6a759740d90d5e0393507`，
MTProto 公钥指纹为 `4be27a5bb0fc10c4`。

RSA 私钥保存在 `data/server/server_rsa.pem`，使用 PKCS#1 格式，权限 600。
不要重新生成此密钥来修复连接问题，否则现有客户端绑定会失效。
客户端发现接口仅包含公钥，不得暴露私钥、SMTP、APNs 或数据库凭据。

邮箱验证码为 5 位，启用真实 SMTP 和首次设置邮箱流程。
SMTP TLS 和认证已验证；实际收信与登录仍需真机验证。
APNs 使用 `com.hsgram.app`，私钥在 `data/server/apns-key.p8`，权限 600。
无效设备令牌的 APNs 测试未返回提供方认证错误，不等同于真实设备送达验证。
FCM 尚未启用。开发支付保持关闭。

## 贴纸和表情素材

完整素材挂载 `data/sticker-seed:/seed/stickers:ro`：

```dotenv
TELESRV_STICKER_SEED_DIR=/seed/stickers
TELESRV_STICKER_SEED_MAX_SETS=0
```

2026-09-30 已导入 336 组集合与 74 种消息反应，包括贴纸及动态表情。
导入后的目录含 231 组贴纸、100 组自定义表情和系统集合；含服务自动生成的集合共 337 组。
数据库全部 localfs 记录对应的 13,919 个去重文件已验证存在、大小与 SHA-256；无缺失或不匹配。
此数量包含系统素材，不应直接当成可见贴纸数量。
另已验证 13,381 个 TGS 文件可解压和解析、374 个 WebP 文件头有效；基础格式检查无失败。
这些检查不能替代真机渲染测试。服务器 `verify-blobs.py` 可复查文件及格式。

独立发送消息特效的 `telegram_effects_export` 尚缺，不能把消息反应动画当作发送特效。
补充官方素材需要合法的已登录 Telegram 会话，使用仓库 `cmd/stickerfetch` 的 `effects` 模式导出。
不要提交会话文件或以空占位数据宣称特效已就绪。

## 更新检查

1. 先备份数据库、环境文件与持久化目录；不覆盖本实例密钥。
2. 二进制、语言包和素材先上传至临时文件，等待传输退出，校验 SHA-256 后再原子替换。
3. 语言包必须完整上传后启动；不要用半份目录初始化种子版本。
4. 检查 `docker compose ps`、服务健康检查、种子导入日志和真实 blob 文件。
5. 从外部验证 HTTPS 配置、公钥身份及加密 MTProto `help.getConfig`。
6. 用已登录测试账号验证贴纸列表、反应、文件下载，再做实际发送与接收验证。

2026-09-30 已完成外部 HTTPS、公钥及加密 MTProto 配置链路测试。
贴纸、反应、Emoji、文件服务相关测试以及配置发现、公开页面、推送模块测试通过。
未登录会话访问贴纸列表返回认证错误是正常权限行为，不应为测试而取消认证。

## iOS 打包

- Bundle ID：`com.hsgram.app`，Team：`J5427JC9Y3`。
- Ad Hoc 描述文件：本机 `~/.config/safelink-signing/ipa-87/profiles`，7 个目标均包含 5 台已启用设备。
- 分发证书到期日：2027-05-12；安装有效期受描述文件、证书与设备状态共同约束。
- 新版使用 `safelink-data-v1` 数据目录，与旧实例本地数据隔离，首次使用需要重新登录。
- 签名后验证主程序和每个扩展的签名、Bundle ID、设备列表及有效期。
- 2026-09-30 构建号 `2026093002` 已包含服务器账号列表头像修复；7 个签名目标、5 台设备及实例身份校验通过。安装包和验证 JSON 在本机 `IPA-87/2026093002`。
- 构建号 `2026093004` 增加服务器分组、账号用户名与当前标记、稳定行 ID，以及独立添加服务器入口。使用原生 ItemListUI 和每个账号自己的头像上下文。
- `2026093004` 最终 IPA 已验证 `safelink`、`hsgramapp` URL scheme 注册、7 个签名目标、5 台设备、公钥身份与 APNs production entitlement。旧包缺少 `safelink` 注册，服务端无法替已安装旧包补注册；需安装新版。
- 2026-09-30 域名管理、公开页面和 Web 修复已部署到 87，备份在 `/home/safelink-chat/backups/20260930-links-ui`。后台登录、受权限保护的域名读取与预演已实测；未修改生产域名值。邀请页面不再自动尝试 `tg://`，提供显式 App / Web 两个入口。
- 当前没有已连接真机，不能把编译或签名成功等同于真机登录、推送和素材下载已通过。

## 2026-10-01 链接无响应修复

- 邀请页仍使用 `safelink://join?invite=...`，用户名使用 `safelink://resolve?domain=...`；不要改回 `tg://`，以免唤起 Telegram。
- iOS 内置浏览器之前只把 `tg://` 交给原生导航，`safelink://` 误入外部打开路径后被自身协议保护直接忽略。BrowserUI 的两个导航代理及新窗口入口现统一识别 SafeLink 和应用自身 scheme。
- Android 普通浏览器先处理原生内部链接并收起浏览页面，再由已有导航解析邀请；Bot WebApp 继续遵守服务端协议白名单。
- 服务端 `help.getAppConfig` 新增 `web_app_allowed_protocols=["http","https","safelink"]`，默认配置 hash 从 30 升至 31。不要只修改配置内容而不提升 hash；客户端需重新获取配置才能刷新白名单。
- 87 服务器仅重建并重启 server 服务，未改变密钥、数据库或域名配置。二进制 SHA-256：`200fa97839b2ce0cd06bb5b9f97ad6e5e6845ca131b08edf6b1ce096894f05a4`；回滚备份：`/home/safelink-chat/backups/link-navigation-20260930T163858Z`。
- iOS Ad Hoc 构建号 `2026100101`，文件在本机 `IPA-87/2026100101`；正式编译与 7 个目标签名验证通过，支持现有 5 台设备。此前 `2026093005` 包不含这次浏览器修复。
- iOS 修复提交 `16b77aef8b`，Android 修复提交 `60f79c0ef`，均已推送 SafeLink 仓库 dev。Android 本机缺少 SDK，未编译 APK；旧安装包不会因为源码 push 而自动更新。
- 配置、邀请页与相关 RPC 回归测试通过。尚无已连接的 iOS/Android 真机，仍需实际安装新版本后验证邀请页按钮、用户名链接及群组打开。

## 2026-10-01 联系人收到链接不可点击

- 该问题与浏览器 scheme 跳转不同：消息本身含完整 URL entity 和已生成的网页预览，但接收方文字仍为黑色，预览也不可点击，顶部显示“屏蔽此用户”。
- 原 `contacts.GetPeerSettings` 把 `block_contact` 设置成 `!blocked`，导致已保存、双向联系人也被 iOS 当作可疑发送方。iOS 会主动移除入站 URL 的点击属性，并禁用网页预览点击。
- 修正为 `!found && !blocked`：只对未保存且未拉黑的陌生人提示，不取消拉黑功能，也不全局关闭客户端安全保护。单向已保存联系人同样不应误标。
- 新增联系人/双向联系人/陌生人/拉黑关系五种回归用例，修复前联系人两例失败，修复后联系人及 RPC 全包测试通过。
- `peercheck` 可只读验证指定两个用户双向关系与实际服务层返回值；强制 PostgreSQL `default_transaction_read_only=on`，不修改聊天或联系人。
- 旧客户端可能缓存错误的 peer settings；重新进入聊天/资料页以刷新。提示条右侧关闭按钮是关闭安全提示，不能与“屏蔽此用户”动作混淆。本修复不需要重新打客户端包。
- 已部署至 87，二进制 SHA-256 `910087b26ef88c51ae3a894e1766ba059a931bb8240ebe36663ee61a025a1900`，备份 `/home/safelink-chat/backups/contact-links-20260930T165658Z`。生产只读服务层探针确认两个指定账号双向 `contact=true mutual=true blocked=false block_prompt=false`；健康检查与外部加密 MTProto 配置探针通过。接收方实际刷新后的点击效果仍待真机确认。

### 关联回归：联系人被误显示为已拉黑

- 上述首版修复遗漏 `users.getFullUser` 中 `blocked = !settings.BlockContact` 的旧推导。安全提示关闭后，资料接口会把联系人误标为已拉黑/禁止查看动态；真实 `contact_blocks` 未因此增加记录。首版不能作为联系人状态正确的回滚目标。
- 资料接口改为调用 owner-scoped `Contacts.IsBlocked`，与安全提示独立；解除拉黑事件的 `PeerSettings` 同步遵守“已保存联系人不显示陌生人屏蔽提示”。
- 回归测试包含真实服务组合、冷/热缓存、保存联系人、主动拉黑、解除拉黑、黑名单列表与 225～228 层编码后的资料字段。首版会在未拉黑联系人用例失败，修正后通过。
- `peercheck` 扩展为同时执行只读资料 RPC，检查 `UserFull.blocked`、`blocked_my_stories_from`、`settings.block_contact` 与持久化关系一致。不批量删除黑名单、不修改真实用户关系。
- 01:19（北京时间）修正版本已部署到 87，SHA-256 `11926675336c39e2e28d05e03bf5c4eab0f197dc5de08e758987e7d669c9e249`；本次部署前版本保存在 `/home/safelink-chat/backups/block-state-20260930T171848Z`，该备份含上述已知回归。健康检查通过；只读生产资料 RPC 确认两个指定账号双向 `blocked=false stories_blocked=false block_prompt=false`。联系人、RPC、store 包测试通过，数据库集成测试仍以各测试的环境依赖和跳过规则为准。
