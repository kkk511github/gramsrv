# SafeLink 五端更新与兼容检查（2026-10-03）

## 客户端更新

| 客户端 | 官方基线 | SafeLink 本地提交 | 说明 |
| --- | --- | --- | --- |
| iOS | 12.9.2 / Layer 228，`6ad963e5b6` | `4498038aca` | 合并 305 个上游提交，保留原生 UI、服务器与账号、邀请码、密码登录、禁止私聊；修复企业聊天链接复制地址。 |
| Android | 12.10.6 (7112) / Layer 229，`f2908b141` | `56828bb4b` | 合并 27 个上游提交，修复合并后的中文覆盖与资源生成兼容。 |
| PC | 7.2.10 beta / Layer 229，`d8594c0117` | `9a58a053c6` | 合并官方 dev（官方 master 较旧），处理 14 处冲突，保留 SafeLink 定制。 |
| Mac | 官方 master/release/appstore 当前提交均已包含 | `9b6f83b26` | 无需重复合并；同步账号对应服务器的公共链接，覆盖业务、频道、消息分享、邀请、贴纸、礼物等复制入口。 |
| Web | 2.2 (676) / Layer 229，官方 master `e55552763` | `87f873597`，合并提交 `3577d38ab` | 合并 309 个上游提交，保留 SafeLink 传输、账号隔离、登录策略和重复气泡修复；能力开关补丁验证和生产构建成功。 |

主要新增与改进：

- Community：将多个群组或频道组织为社区，提供成员、关联申请、审批、搜索与折叠管理。
- 富文本：文章式编辑、列表、表格、媒体布局、草稿、链接编辑与富文本消息显示。具体入口随客户端版本而异。
- 临时消息及媒体、欢迎消息、礼物与媒体界面更新；并非每个客户端都提供相同入口。
- Android 的主题文本解析、震动及数字解析修复；iOS 的聊天预览、手势与富文本编辑细节修复。
- 复制和显示的公共链接使用当前服务器配置，避免显示 SafeLink、复制却是 Telegram 的不一致。

## 服务端兼容

服务端既有精确协议支持覆盖 Layer 225–229；不能仅放开层号而不验证各层构造器和字段。

- 既有社区生命周期、欢迎消息、富文本存储与跨层投影有 RPC 测试。
- 本次增加 `safelink_public_link_prefix` AppConfig 字段，来自公共站点基址；基址改变会更新配置 hash。
- 新客户端使用的富文本 AI、富文本翻译与入群 WebView 接口已补齐；对应 RPC/AI/翻译包测试和 vet 通过。格式、媒体引用、单次 prompt、权限与 Layer 228/229 精确派发均有检查。翻译每批最多 20 个文本片段，保留原有 AI 总文本长度限制。
- 入群 WebView 仅接受属于当前用户、未过期的可信 `chat_join` 会话。生产暂无签发挑战的流程，因此没有启用入群挑战；普通 WebView token 不可复用，挑战也不能发消息或延期。
- 生产 AI 使用默认本地 provider；没有配置远程翻译 provider。协议适配不等于开通外部模型，不能把未翻译文本冒充成功翻译。
- iOS 生成协议中的 `channels.getCategories`、`channels.getContactPersonalChannels`、`channels.search` 不属于当前 SDK 的规范 Layer 228/229，且本次客户端没有实际调用入口；未伪造接口响应。
- Firebase PNV 属于 Google 手机号验证体系，不等同于 SafeLink 邮箱验证码。没有调用的 schema 声明不代表生产启用了该登录方式。
- SafeLink 没有 `auth.importWebTokenAuthorization` 的签发/导入链路，SDK 的规范层也没有 `auth.cancelWebTokenAuthorization` 类型。AppConfig 明确下发 `safelink_web_auth_tokens_enabled=false`；Web 对该能力单独处理，不假装撤销成功，也不影响扫码、验证码和密码登录。
- 服务端常用联系人评分本来处于 disabled，没有排名持久化。本次不伪造 `contacts.resetTopPeerRating` 成功，AppConfig 下发 `safelink_top_peers_enabled=false`；Web 清理本地旧提示缓存，不发送未支持的评分重置 RPC。
- PC 新增的 `forwardMessages.from_ephemeral` 暂不支持转发。服务端明确拒绝临时消息来源，防止其独立 ID 被错误解析为普通历史消息；这项限制不能称为完整功能适配。
- `account.setMainProfileTab`：新增八种标准资料页签的持久化与完整资料投影，缓存会正确刷新。用户可选取个人资料默认页签，见 [官方接口说明](https://core.telegram.org/method/account.setMainProfileTab)。
- `account.confirmBotConnection`：只确认当前用户已有的业务机器人连接，检查 bot 和 access hash，持久化确认时间；不创建任意连接，也不扩大机器人权限。
- `messages.deletePhoneCallHistory`：按最多 500 条/页清理调用者的语音/视频通话记录，支持 revoke、真实 PTS/ID 回执及继续删除的 offset，普通消息不受影响，见 [官方接口说明](https://core.telegram.org/method/messages.deletePhoneCallHistory)。
- 新增迁移 `0227_account_feature_compat`：独立账号页签表与业务机器人确认时间字段；sqlc 模型已通过 `/Users/kk/go/bin/sqlc generate` 再生成。

## 验证与部署要求

- iOS 公共链接及服务器绑定测试通过，769 个新增/修改 Swift 文件语法检查通过；Ad Hoc 全量构建与文案增量构建成功。最终使用 5 台设备的新描述文件重新签名，主应用及六个扩展的签名、bundle、生产 APNs、SafeLink scheme、默认服务器与公钥均通过校验。产物为 `IPA-87/2026100301/SafeLink-12.9.2-2026100301-AdHoc.ipa`，SHA-256：`bbf5465e15128097150171c3ef8920a566d4e990ba889b5ed71b2f98b7a6b71f`。
- Android 自定义 Java 语法、资源、令牌环与本地化、窄范围 Kotlin 编译检查通过；缺少完整 JDK/SDK/NDK，未做完整 APK 编译或真机测试。
- PC 协议生成及 9 项源码契约检查通过；缺少 Qt6，未做 C++ 全量编译或运行测试。
- Mac 75 项链接 helper 测试、调用点检查与 24 个 Swift 文件语法检查通过；未做完整 Xcode 构建或真机测试。
- Web 合并版本 4,794 项测试通过、52 项跳过，4 workers；typecheck、重点 lint、325 个生产 bundle chunk 检查通过。桌面/手机登录、模拟注册及密码流程、键盘、Axe opt-in contrast 与四种消息回执场景通过；真实授权账号流程未验证。最终能力开关补丁另有 64 项测试通过，typecheck/lint/生产构建及 324 个 chunk 检查通过，桌面和手机生产页面 smoke 通过。
- 使用服务器独立临时 PostgreSQL 数据库验证社区、富文本、邀请码、注册密码和 future_auth_token；14 项测试通过，测试库已删除。
- 富文本接口提交 `8798bd78` 后，全仓 `go test ./... -count=1` 与 `go vet ./...` 通过，AI 和翻译服务的 race 检查通过。外部 PostgreSQL/Redis/Compose 集成测试未配置时按原规则跳过，独立 PostgreSQL 测试另行运行。
- 最终账号与通话补丁加入后，全仓 `go test ./... -count=1` 和 `go vet ./...` 再次通过。独立测试库完成 226→227 升级，账号持久化/机器人所有权、通话 revoke/事务 rollback、社区、富文本、邀请码及密码策略全部通过，测试库已删除。
- 本次生产目标仅 `212.189.31.87`；备份配置、RSA 身份、数据库和旧二进制，不修改生产登录策略、邀请码、媒体卷或公钥。
- Web 使用独立发布目录与原子软链接切换；验证入口 HTML 与引用资源 SHA-256，失败回滚。
- 本次代码已同步到各组件的 SafeLink 远端：服务端、iOS、Android、PC、Mac 使用 `dev`，Web 使用 `master`；总仓库 `main` 同步固定版本指针。生产部署和 IPA 成果见下方记录。文档提交不改变下方记录的生产二进制源码版本。

## 首次兼容发布结果

2026-10-03 已在 `212.189.31.87` 部署服务端源码 `8ca4d6e3`，发布目录 `/home/safelink-chat/releases/client-sync-20261003`，备份目录 `/home/safelink-chat/backups/client-sync-20261003T094424Z`。服务端和后台容器 healthy，数据库仍为 `226|false`；配置文件、RSA 身份、登录策略和可视邀请码设置保持一致。

## 最终服务端发布

2026-10-03 已部署服务端源码 `102cd3a9`，发布目录 `/home/safelink-chat/releases/client-sync-final-20261003`，备份目录 `/home/safelink-chat/backups/client-sync-final-20261003T102008Z`。服务端和后台容器 healthy，生产数据库升级到 `227|false`；配置、RSA 身份、登录/密码策略和可视邀请码设置不变。

最终 Web 源码为 `87f873597`，针对服务端 `102cd3a9` 的启用功能新增 RPC 审计为零缺口。未支持的 Web-token 登录与评分重置能力明确关闭，不能将其称为已启用功能。

Web 已上线到 `https://web.safelink.chat/`，发布目录 `/home/safelink-web/releases/20261003-official-sync`；旧版 `/home/safelink-web/releases/20261003-pending-message-ack` 保留供回滚。线上 HTML 与 51 个入口资源逐一校验通过，HTTP 200。后续打包静态文件应使用 `COPYFILE_DISABLE=1` 并禁用 macOS xattrs，避免 GNU tar 输出 Apple 扩展属性警告；本次警告未影响内容校验。

最终公网链路检查通过：HTTPS 服务发现返回预期服务器 ID 与公钥指纹；WebSocket 升级为 101 且握手校验正确；TCP 2398 的 MTProto `req_pq_multi/resPQ` 返回正确 nonce 和 RSA 指纹 `4be27a5bb0fc10c4`。该检查未登录账号或发送消息，不代替真机功能验收。
