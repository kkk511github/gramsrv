import { Bot, GrammyError, HttpError, InlineKeyboard } from "grammy";
import { randomInt } from "node:crypto";
import { buildPayload, findProduct, KINDS, normalizeUsername, parsePayload, productsOfKind } from "./catalog.js";

const spinPrizes = [{ amount: 50, weight: 250 }, { amount: 100, weight: 130 }, { amount: 500, weight: 50 }, { amount: 1000, weight: 30 }, { amount: 10000, weight: 10 }, { amount: 15, weight: 530 }];
const text = {
  zh: {
    hello: "👋 <b>欢迎使用 SafeLink 服务助手</b>\n\n这里可以领取活动奖励、联系客服并管理 SafeLink 账号关联。",
    menu: "🏠 <b>SafeLink 服务中心</b>",
    shop: "🛒 <b>SafeLink 商店</b>\n请选择商品类型：", bonuses: "🎁 <b>我的奖励</b>", settings: "⚙️ <b>设置</b>",
    support: "💬 请用一条消息说明问题，客服会在此回复。", target: "请输入接收方的 {product} 数字 ID。",
    account: "请输入你的 {product} 数字 ID，用于领取奖励和账号服务。",
  },
  en: {
    hello: "👋 <b>Welcome to SafeLink Services</b>\n\nClaim rewards, contact support, and manage your linked SafeLink account.",
    menu: "🏠 <b>SafeLink services</b>",
    shop: "🛒 <b>Store</b>\nChoose a section:", bonuses: "🎁 <b>Bonuses</b>", settings: "⚙️ <b>Settings</b>",
    support: "💬 Send your support question in one message.", target: "Enter the numeric {product} user ID.",
    account: "Enter your numeric {product} account ID. It is used for bonuses and purchases for yourself.",
  },
};

function escapeHTML(value) { return String(value ?? "").replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;").replaceAll('"', "&quot;"); }
function language(db, id, fallback) { const value = db.user(id)?.language ?? fallback; return text[value] ? value : "zh"; }
function t(db, id, fallback, key, productName = "SafeLink") { return text[language(db, id, fallback)][key].replaceAll("{product}", productName); }
function isOwner(config, id) { return config.ownerIDs.has(id); }
function userName(from) { return from.username ? `@${from.username}` : [from.first_name, from.last_name].filter(Boolean).join(" "); }
function initialLanguage(from, fallback) { const code = String(from?.language_code ?? "").toLowerCase(); return code.startsWith("en") ? "en" : code.startsWith("zh") ? "zh" : fallback; }

function mainKeyboard(config, admin = false) {
  const kb = new InlineKeyboard();
  if (config.paymentsEnabled) kb.text("🛒 SafeLink 商店", "menu:shop").row();
  kb.text("🎁 奖励中心", "menu:bonuses").text("👥 邀请好友", "menu:referrals").row()
    .text("💬 联系客服", "menu:support").text("⚙️ 设置", "menu:settings");
  if (admin) kb.row().text("🛡 管理员", "admin:menu");
  return kb;
}
function backKeyboard(target = "menu:home") { return new InlineKeyboard().text("‹ 返回", target); }
function shopKeyboard() { return new InlineKeyboard().text("👑 Premium", "shop:premium").text("⭐ SafeLink Stars", "shop:stars").row().text("💎 收藏用户名", "shop:username").row().text("‹ 返回", "menu:home"); }
function settingsKeyboard(user) { return new InlineKeyboard().text("🆔 SafeLink 账号 ID", "settings:account").row().text("🌐 中文", "settings:lang:zh").text("🌐 English", "settings:lang:en").row().text(user.notifications ? "🔔 通知：已开启" : "🔕 通知：已关闭", "settings:notifications").row().text("‹ 返回", "menu:home"); }
function adminKeyboard() { return new InlineKeyboard().text("📊 统计", "admin:stats").text("📣 群发", "admin:broadcast").row().text("⭐ 发放 Stars", "admin:stars").text("💎 发放 Premium", "admin:premium").row().text("🎟 优惠码", "admin:promo").text("🎁 抽奖活动", "admin:giveaway").row().text("🎁 发放奖励", "admin:bonus").text("💬 回复工单", "admin:reply").row().text("📈 发放记录", "admin:sales").text("⚙️ Stars 汇率", "admin:rate").row().text("‹ 返回", "menu:home"); }

async function editOrReply(ctx, message, keyboard = undefined) {
  const options = { parse_mode: "HTML", link_preview_options: { is_disabled: true }, reply_markup: keyboard };
  if (ctx.callbackQuery?.message) {
    try { return await ctx.editMessageText(message, options); } catch (error) { if (!String(error.description ?? error).includes("message is not modified")) throw error; }
  }
  return ctx.reply(message, options);
}

async function subscribed(ctx, config) {
  if (!config.requiredChannel || isOwner(config, ctx.from.id)) return true;
  try { const member = await ctx.api.getChatMember(config.requiredChannel, ctx.from.id); return ["creator", "administrator", "member", "restricted"].includes(member.status); }
  catch { return false; }
}

async function subscriptionGate(ctx, config) {
  const kb = new InlineKeyboard();
  if (config.requiredChannelURL) kb.url("📣 打开频道", config.requiredChannelURL).row();
  kb.text("✅ 我已加入", "subscription:check");
  await editOrReply(ctx, "🔒 请先加入指定频道，然后点击下方按钮验证。", kb);
}

function parseStartRef(ctx) {
  const match = String(ctx.match ?? "").match(/^ref_(\d+)$/); return match ? Number(match[1]) : 0;
}

function productText(product) {
  const extra = product.kind === KINDS.username ? `\n报价：<b>${product.bid} TON</b>` : product.kind === KINDS.stars ? `\n到账：<b>${product.starsAmount} SafeLink Stars</b>` : "";
  return `<b>${escapeHTML(product.title)}</b>\n\n${escapeHTML(product.description)}\n\n价格：<b>${product.starsPrice} ⭐</b>${extra}`;
}

function productKeyboard(product, db, buyerID) {
  const kb = new InlineKeyboard();
  const selfID = db.user(buyerID)?.server_user_id ?? 0;
  if (selfID > 0) kb.text("给自己", `buy:${product.code}:${selfID}`).row();
  kb.text("赠送给其他 ID", `target:${product.code}`).row();
  for (const id of db.recentRecipients(buyerID)) kb.text(`ID ${id}`, `buy:${product.code}:${id}`);
  return kb.row().text("‹ 返回", `shop:${product.kind}`);
}

async function sendInvoice(ctx, product, targetUserID, extra = "") {
  await ctx.api.sendInvoice(ctx.chat.id, product.title, product.description, buildPayload(product.code, targetUserID, extra), "XTR", [{ label: product.title, amount: product.starsPrice }]);
}

function rollPrize() {
  const total = spinPrizes.reduce((sum, value) => sum + value.weight, 0); let value = randomInt(total);
  for (const prize of spinPrizes) { value -= prize.weight; if (value < 0) return prize.amount; }
  return 15;
}

export function createBot({ config, db, gramsrv }) {
  const bot = new Bot(config.botToken, { client: { apiRoot: config.botApiRoot } });

  bot.use(async (ctx, next) => {
    if (ctx.from && ctx.chat) db.upsertUser(ctx.from, ctx.chat.id, initialLanguage(ctx.from, config.defaultLanguage));
    await next();
  });

  bot.command("start", async (ctx) => {
    const referrer = parseStartRef(ctx);
    const existed = db.user(ctx.from.id);
    db.upsertUser(ctx.from, ctx.chat.id, initialLanguage(ctx.from, config.defaultLanguage), referrer, config.referralBonus);
    if (!(await subscribed(ctx, config))) return subscriptionGate(ctx, config);
    const referralNotice = !existed && referrer > 0 ? "\n\n✅ 邀请关系已记录。" : "";
    await ctx.reply(`${t(db, ctx.from.id, config.defaultLanguage, "hello")}${referralNotice}`, { parse_mode: "HTML", reply_markup: mainKeyboard(config, isOwner(config, ctx.from.id)) });
  });

  bot.command("menu", async (ctx) => editOrReply(ctx, t(db, ctx.from.id, config.defaultLanguage, "menu"), mainKeyboard(config, isOwner(config, ctx.from.id))));
  bot.command("admin", async (ctx) => { if (isOwner(config, ctx.from.id)) await editOrReply(ctx, "🛡 <b>SafeLink 管理员</b>", adminKeyboard()); });
  bot.command("promo_code", async (ctx) => {
    const [code, rawID] = String(ctx.match ?? "").trim().split(/\s+/); const serverID = Number(rawID || db.user(ctx.from.id)?.server_user_id);
    if (!code || !Number.isSafeInteger(serverID) || serverID <= 0) return ctx.reply(`格式：/promo_code 优惠码 ${config.productName.toUpperCase()}_ID`);
    try {
      const promo = db.claimPromo(code, ctx.from.id);
      try { await gramsrv.grantStars(serverID, promo.stars_amount, `Promo ${code}`, `promo:${code.toLowerCase()}:${ctx.from.id}`); }
      catch (error) { db.releaseCampaignClaim("promo", code.toLowerCase(), ctx.from.id); throw error; }
      await ctx.reply(`✅ 已发放 ${promo.stars_amount} SafeLink Stars。`);
    } catch (error) { await ctx.reply(`⚠️ ${escapeHTML(error.message)}`, { parse_mode: "HTML" }); }
  });

  bot.on("pre_checkout_query", async (ctx) => {
    try {
      if (!config.paymentsEnabled) throw new Error("payments are disabled");
      if (ctx.preCheckoutQuery.currency !== "XTR") throw new Error("unsupported currency");
      if (!ctx.preCheckoutQuery.invoice_payload.startsWith("custom|")) {
        const parsed = parsePayload(ctx.preCheckoutQuery.invoice_payload); const product = findProduct(parsed.code, db.starsRate());
        if (!product || product.starsPrice !== ctx.preCheckoutQuery.total_amount) throw new Error("product price changed");
      }
      await ctx.answerPreCheckoutQuery(true);
    }
    catch { await ctx.answerPreCheckoutQuery(false, { error_message: "订单无效、已过期或支付通道尚未开放。" }); }
  });

  async function fulfill(product, recipientID, buyer, chatID, chargeID, extra = "") {
    if (product.kind !== KINDS.number && (!Number.isSafeInteger(recipientID) || recipientID <= 0)) throw new Error(`recipient ${config.productName} ID is invalid`);
    if (db.saleByCharge(chargeID)) return;
    const key = `payment:${chargeID}:${product.code}`;
    let number = null;
    if (product.kind === KINDS.premium) await gramsrv.grantPremium(recipientID, product.months, "SafeLink bot purchase", key);
    else if (product.kind === KINDS.stars) await gramsrv.grantStars(recipientID, product.starsAmount, "SafeLink bot purchase", key);
    else if (product.kind === KINDS.username) {
      const username = normalizeUsername(extra); if (!username) throw new Error("collectible username is invalid");
      await gramsrv.mintUsername(recipientID, username, product.bid, key);
    } else if (product.kind === KINDS.number) {
      number = db.createNumber(buyer.id, chatID, product.numberFormat, "ANON", true);
    }
    db.addSale({ product: product.code, title: product.title, starsPrice: product.starsPrice, recipientID, buyerID: buyer.id, buyerName: userName(buyer), chargeID });
    if (number) await bot.api.sendMessage(chatID, `✅ 号码已预留：<code>${escapeHTML(number.display)}</code>`, { parse_mode: "HTML" }).catch(() => {});
    else await bot.api.sendMessage(chatID, `✅ <b>${escapeHTML(product.title)}</b> 已发放给 SafeLink ID <code>${recipientID}</code>。`, { parse_mode: "HTML" }).catch(() => {});
  }

  bot.on("message:successful_payment", async (ctx) => {
    const payment = ctx.message.successful_payment;
    if (!db.beginPayment(payment.telegram_payment_charge_id, ctx.from.id, payment.invoice_payload, payment.total_amount)) return;
    try {
      if (payment.invoice_payload.startsWith("custom|")) {
        const title = Buffer.from(payment.invoice_payload.slice(7), "base64url").toString("utf8");
        db.addSale({ product: "custom", title, starsPrice: payment.total_amount, recipientID: ctx.from.id, buyerID: ctx.from.id, buyerName: userName(ctx.from), chargeID: payment.telegram_payment_charge_id });
        await ctx.reply(`✅ 已收到「${escapeHTML(title)}」付款。`, { parse_mode: "HTML" });
      } else {
        const parsed = parsePayload(payment.invoice_payload);
        const product = findProduct(parsed.code, db.starsRate()); if (!product) throw new Error("product no longer exists");
        if (payment.currency !== "XTR" || payment.total_amount !== product.starsPrice) throw new Error("paid amount does not match the product");
        const recipient = parsed.targetUserID || db.user(ctx.from.id)?.server_user_id || 0;
        await fulfill(product, recipient, ctx.from, ctx.chat.id, payment.telegram_payment_charge_id, parsed.extra);
      }
      db.finishPayment(payment.telegram_payment_charge_id);
    } catch (error) {
      db.failPayment(payment.telegram_payment_charge_id, error);
      await ctx.reply(`⚠️ 已收到付款，但自动发放未完成。请向客服提供凭证 <code>${escapeHTML(payment.telegram_payment_charge_id)}</code>。`, { parse_mode: "HTML" });
      for (const owner of config.ownerIDs) await bot.api.sendMessage(owner, `⚠️ 发放失败 ${escapeHTML(payment.telegram_payment_charge_id)}：${escapeHTML(error.message)}`, { parse_mode: "HTML" }).catch(() => {});
    }
  });

  bot.callbackQuery(/^subscription:check$/, async (ctx) => { await ctx.answerCallbackQuery(); if (await subscribed(ctx, config)) await editOrReply(ctx, t(db, ctx.from.id, config.defaultLanguage, "menu"), mainKeyboard(config, isOwner(config, ctx.from.id))); else await subscriptionGate(ctx, config); });
  bot.callbackQuery(/^menu:(.+)$/, async (ctx) => {
    await ctx.answerCallbackQuery(); const page = ctx.match[1]; const user = db.user(ctx.from.id);
    if (page === "home") return editOrReply(ctx, t(db, ctx.from.id, config.defaultLanguage, "menu"), mainKeyboard(config, isOwner(config, ctx.from.id)));
    if (page === "numbers") {
      return editOrReply(ctx, "手机号码与登录验证码仍按 SafeLink 现有邮箱验证流程处理。", backKeyboard());
    }
    if (page === "shop") return config.paymentsEnabled ? editOrReply(ctx, t(db, ctx.from.id, config.defaultLanguage, "shop"), shopKeyboard()) : editOrReply(ctx, "SafeLink 商店支付通道尚未开放。", backKeyboard());
    if (page === "bonuses") return editOrReply(ctx, `${t(db, ctx.from.id, config.defaultLanguage, "bonuses")}\n\n奖励余额：<b>${user.bonus}</b>\n已邀请：<b>${user.referral_count}</b>`, new InlineKeyboard().text("🎁 每日奖励", "bonus:daily").text("🎡 幸运转盘", "bonus:spin").row().text("‹ 返回", "menu:home"));
    if (page === "referrals") {
      const username = config.publicUsername || bot.botInfo?.username || "bot"; const link = `${config.publicBaseURL}/${username}?start=ref_${ctx.from.id}`;
      return editOrReply(ctx, `👥 <b>邀请好友</b>\n\n已邀请：<b>${user.referral_count}</b>\n每位好友奖励：<b>${config.referralBonus}</b>\n\n<code>${link}</code>`, backKeyboard());
    }
    if (page === "support") { db.setPending(ctx.from.id, "support"); return editOrReply(ctx, t(db, ctx.from.id, config.defaultLanguage, "support"), backKeyboard()); }
    if (page === "settings") return editOrReply(ctx, t(db, ctx.from.id, config.defaultLanguage, "settings"), settingsKeyboard(user));
  });

  bot.callbackQuery(/^numbers:new(?::(?:RU|US))?$/, async (ctx) => { await ctx.answerCallbackQuery(); await editOrReply(ctx, "SafeLink 不会在机器人中生成虚假登录号码。", backKeyboard()); });
  bot.callbackQuery(/^shop:(premium|stars|username)$/, async (ctx) => { await ctx.answerCallbackQuery(); if (!config.paymentsEnabled) return editOrReply(ctx, "SafeLink 商店支付通道尚未开放。", backKeyboard()); const kind = ctx.match[1]; const kb = new InlineKeyboard(); for (const product of productsOfKind(kind, db.starsRate())) kb.text(`${product.title} · ${product.starsPrice}⭐`, `product:${product.code}`).row(); if (kind === KINDS.stars) kb.text("✍️ 自定义数量", "stars:custom").row(); kb.text("‹ 返回", "menu:shop"); await editOrReply(ctx, "请选择商品：", kb); });
  bot.callbackQuery(/^stars:custom$/, async (ctx) => { await ctx.answerCallbackQuery(); if (!config.paymentsEnabled) return editOrReply(ctx, "SafeLink 商店支付通道尚未开放。", backKeyboard()); db.setPending(ctx.from.id, "stars_amount"); await editOrReply(ctx, "请输入希望支付的 Stars 数量（1-99999）。", backKeyboard("shop:stars")); });
  bot.callbackQuery(/^product:(.+)$/, async (ctx) => { await ctx.answerCallbackQuery(); const product = findProduct(ctx.match[1], db.starsRate()); if (!product) return; await editOrReply(ctx, productText(product), productKeyboard(product, db, ctx.from.id)); });
  bot.callbackQuery(/^target:(.+)$/, async (ctx) => { await ctx.answerCallbackQuery(); db.setPending(ctx.from.id, "target", { productCode: ctx.match[1] }); await editOrReply(ctx, t(db, ctx.from.id, config.defaultLanguage, "target", config.productName), backKeyboard(`product:${ctx.match[1]}`)); });
  bot.callbackQuery(/^buy:([^:]+):(\d+)$/, async (ctx) => {
    await ctx.answerCallbackQuery(); if (!config.paymentsEnabled) return editOrReply(ctx, "SafeLink 商店支付通道尚未开放。", backKeyboard()); const product = findProduct(ctx.match[1], db.starsRate()); const targetID = Number(ctx.match[2]); if (!product) return;
    if (product.kind === KINDS.username) { db.setPending(ctx.from.id, "username", { productCode: product.code, targetID }); return editOrReply(ctx, "请输入希望获得的用户名（不含 @）：", backKeyboard(`product:${product.code}`)); }
    if (targetID > 0 && targetID !== db.user(ctx.from.id)?.server_user_id) db.rememberRecipient(ctx.from.id, targetID);
    if (isOwner(config, ctx.from.id)) { await fulfill(product, targetID, ctx.from, ctx.chat.id, `owner-${Date.now()}`); return; }
    await sendInvoice(ctx, product, targetID);
  });

  bot.callbackQuery(/^settings:lang:(zh|en)$/, async (ctx) => { db.setLanguage(ctx.from.id, ctx.match[1]); await ctx.answerCallbackQuery({ text: "OK" }); await editOrReply(ctx, t(db, ctx.from.id, config.defaultLanguage, "settings"), settingsKeyboard(db.user(ctx.from.id))); });
  bot.callbackQuery(/^settings:notifications$/, async (ctx) => { const enabled = db.toggleNotifications(ctx.from.id); await ctx.answerCallbackQuery({ text: enabled ? "已开启" : "已关闭" }); await editOrReply(ctx, t(db, ctx.from.id, config.defaultLanguage, "settings"), settingsKeyboard(db.user(ctx.from.id))); });
  bot.callbackQuery(/^settings:account$/, async (ctx) => { await ctx.answerCallbackQuery(); db.setPending(ctx.from.id, "account"); await editOrReply(ctx, t(db, ctx.from.id, config.defaultLanguage, "account", config.productName), backKeyboard("menu:settings")); });
  bot.callbackQuery(/^bonus:daily$/, async (ctx) => { const result = db.claimDaily(ctx.from.id, config.dailyBonus); await ctx.answerCallbackQuery({ text: result.claimed ? `+${config.dailyBonus}` : "今日已领取" }); await editOrReply(ctx, `${t(db, ctx.from.id, config.defaultLanguage, "bonuses")}\n\n奖励余额：<b>${result.balance}</b>`, backKeyboard("menu:bonuses")); });
  bot.callbackQuery(/^bonus:spin$/, async (ctx) => {
    await ctx.answerCallbackQuery(); const user = db.user(ctx.from.id); if (!user.server_user_id) { db.setPending(ctx.from.id, "account"); return editOrReply(ctx, t(db, ctx.from.id, config.defaultLanguage, "account", config.productName), backKeyboard("menu:bonuses")); }
    try { const award = db.reserveSpin(ctx.from.id, user.server_user_id, rollPrize()); await gramsrv.grantStars(user.server_user_id, award.prize, "SafeLink daily reward", `spin:${ctx.from.id}:${award.day}`); db.finishSpin(ctx.from.id, award.day); await editOrReply(ctx, `🎉 恭喜获得 <b>${award.prize} SafeLink Stars</b>！`, backKeyboard("menu:bonuses")); }
    catch (error) { await editOrReply(ctx, `⚠️ ${escapeHTML(error.message)}`, backKeyboard("menu:bonuses")); }
  });

  bot.callbackQuery(/^giveaway:([a-f0-9]+)$/, async (ctx) => { await ctx.answerCallbackQuery(); const user = db.user(ctx.from.id); if (!user.server_user_id) return ctx.reply(`请先在设置中填写 ${config.productName} ID。`); const id = ctx.match[1]; try { const item = db.claimGiveaway(id, ctx.from.id); try { await gramsrv.grantStars(user.server_user_id, item.stars_amount, `SafeLink giveaway ${id}`, `giveaway:${id}:${ctx.from.id}`); } catch (error) { db.releaseCampaignClaim("giveaway", id, ctx.from.id); throw error; } await ctx.reply(`✅ 已获得 ${item.stars_amount} SafeLink Stars。`); } catch (error) { await ctx.reply(`⚠️ ${error.message}`); } });

  bot.callbackQuery(/^admin:(.+)$/, async (ctx) => {
    await ctx.answerCallbackQuery(); if (!isOwner(config, ctx.from.id)) return; const action = ctx.match[1];
    if (action === "menu") return editOrReply(ctx, "🛡 <b>SafeLink 管理员</b>", adminKeyboard());
    if (action === "stats") { const stats = db.stats(); return editOrReply(ctx, `📊 使用者：<b>${stats.users}</b>\n发放记录：<b>${stats.sales}</b>`, backKeyboard("admin:menu")); }
    if (action === "sales") { const lines = db.recentSales().map((sale) => `${sale.id}. ${escapeHTML(sale.product)} → <code>${sale.recipient_id}</code> · ${sale.stars_price}⭐`).join("\n"); return editOrReply(ctx, `📈 <b>最近发放记录</b>\n\n${lines || "—"}`, backKeyboard("admin:menu")); }
    const prompts = {
      broadcast: "请输入群发内容。",
      stars: `格式：${config.productName.toUpperCase()}_ID 数量`,
      premium: `格式：${config.productName.toUpperCase()}_ID 月数`,
      promo: "格式：优惠码 Stars数量 可领取人数",
      giveaway: "格式：Stars数量 可领取人数 活动文案",
      bonus: "格式：机器人用户ID 数量",
      reply: "格式：工单ID 回复内容",
      rate: "请输入新汇率：每 1 Stars 兑换的 SafeLink Stars 数量。",
    };
    if (prompts[action]) { db.setPending(ctx.from.id, `admin_${action}`, { operationID: `admin:${ctx.from.id}:${Date.now()}:${randomInt(1_000_000)}` }); return editOrReply(ctx, prompts[action], backKeyboard("admin:menu")); }
  });

  bot.on("message:text", async (ctx) => {
    if (ctx.message.text.startsWith("/")) return; const pending = db.pending(ctx.from.id); if (!pending) return;
    const input = ctx.message.text.trim();
    try {
      if (pending.kind === "account") { const id = Number(input); if (!Number.isSafeInteger(id) || id <= 0) throw new Error("SafeLink ID 无效"); db.setServerUserID(ctx.from.id, id); db.clearPending(ctx.from.id); return ctx.reply(`✅ ${config.productName} ID: ${id}`, { reply_markup: mainKeyboard(config, isOwner(config, ctx.from.id)) }); }
      if (pending.kind === "stars_amount") { const stars = Number(input); if (!Number.isSafeInteger(stars) || stars <= 0 || stars > 99999) throw new Error("amount must be from 1 to 99999"); const product = findProduct(`stars_${stars}`, db.starsRate()); db.clearPending(ctx.from.id); return ctx.reply(productText(product), { parse_mode: "HTML", reply_markup: productKeyboard(product, db, ctx.from.id) }); }
      if (pending.kind === "target") { if (!config.paymentsEnabled) throw new Error("SafeLink 商店支付通道尚未开放"); const id = Number(input); if (!Number.isSafeInteger(id) || id <= 0) throw new Error("SafeLink ID 无效"); const product = findProduct(pending.payload.productCode, db.starsRate()); if (!product) throw new Error("商品不存在"); db.rememberRecipient(ctx.from.id, id); db.clearPending(ctx.from.id); if (product.kind === KINDS.username) { db.setPending(ctx.from.id, "username", { productCode: product.code, targetID: id }); return ctx.reply("请输入希望获得的用户名（不含 @）："); } if (isOwner(config, ctx.from.id)) return fulfill(product, id, ctx.from, ctx.chat.id, `owner-${Date.now()}`); await sendInvoice(ctx, product, id); return; }
      if (pending.kind === "username") { const username = normalizeUsername(input); if (!username) throw new Error("username must be 5-32 latin characters and start with a letter"); const product = findProduct(pending.payload.productCode, db.starsRate()); db.clearPending(ctx.from.id); if (isOwner(config, ctx.from.id)) return fulfill(product, pending.payload.targetID, ctx.from, ctx.chat.id, `owner-${Date.now()}`, username); await sendInvoice(ctx, product, pending.payload.targetID, username); return; }
      if (pending.kind === "support") { const ticket = db.addSupportMessage(ctx.from.id, ctx.chat.id, input); db.clearPending(ctx.from.id); for (const owner of config.ownerIDs) await bot.api.sendMessage(owner, `💬 工单 #${ticket}\n来自：${escapeHTML(userName(ctx.from))} (<code>${ctx.from.id}</code>)\n\n${escapeHTML(input)}`, { parse_mode: "HTML" }); return ctx.reply(`✅ 工单 #${ticket} 已提交。`); }
      if (!isOwner(config, ctx.from.id)) return;
      if (pending.kind === "admin_broadcast") { db.clearPending(ctx.from.id); let ok = 0, failed = 0; for (const user of db.users()) { try { await bot.api.sendMessage(user.chat_id, input, { parse_mode: "HTML" }); ok++; } catch { failed++; } } return ctx.reply(`群发完成：成功 ${ok}，失败 ${failed}`); }
      if (pending.kind === "admin_stars") { const [id, amount] = input.split(/\s+/).map(Number); if (!Number.isSafeInteger(id) || id <= 0 || !Number.isSafeInteger(amount) || amount <= 0) throw new Error(`${config.productName} ID 或数量无效`); await gramsrv.grantStars(id, amount, "SafeLink bot administrator grant", pending.payload.operationID); db.clearPending(ctx.from.id); return ctx.reply(`✅ 已发放 ${amount} SafeLink Stars → ${config.productName} ID ${id}`); }
      if (pending.kind === "admin_premium") { const [id, months] = input.split(/\s+/).map(Number); if (!Number.isSafeInteger(id) || id <= 0 || !Number.isSafeInteger(months) || months <= 0) throw new Error(`${config.productName} ID 或月数无效`); await gramsrv.grantPremium(id, months, "SafeLink bot administrator grant", pending.payload.operationID); db.clearPending(ctx.from.id); return ctx.reply(`✅ 已发放 ${months} 个月 Premium → ${config.productName} ID ${id}`); }
      if (pending.kind === "admin_promo") { const [code, stars, limit] = input.split(/\s+/); db.createPromo(code, Number(stars), Number(limit)); db.clearPending(ctx.from.id); return ctx.reply(`✅ 优惠码 ${code} 已创建。`); }
      if (pending.kind === "admin_giveaway") { const [stars, limit, ...words] = input.split(/\s+/); const item = db.createGiveaway(words.join(" "), Number(stars), Number(limit)); db.clearPending(ctx.from.id); return ctx.reply(`🎁 ${escapeHTML(item.text)}`, { parse_mode: "HTML", reply_markup: new InlineKeyboard().text("领取奖励", `giveaway:${item.id}`) }); }
      if (pending.kind === "admin_bonus") { const [id, amount] = input.split(/\s+/).map(Number); const balance = db.addBonus(id, amount); db.clearPending(ctx.from.id); return ctx.reply(`✅ 奖励余额 ${id}：${balance}`); }
      if (pending.kind === "admin_invoice" || pending.kind === "admin_access") { throw new Error("该功能尚未开放"); }
      if (pending.kind === "admin_refund") { throw new Error("SafeLink 商店支付通道尚未开放"); }
      if (pending.kind === "admin_reply") { const [ticketRaw, ...words] = input.split(/\s+/); const ticketID = Number(ticketRaw), answer = words.join(" "); const ticket = db.supportMessage(ticketID); if (!ticket || !answer) throw new Error("工单不存在或回复为空"); await bot.api.sendMessage(ticket.chat_id, `💬 <b>SafeLink 客服回复 #${ticketID}</b>\n\n${escapeHTML(answer)}`, { parse_mode: "HTML" }); db.closeSupportMessage(ticketID); db.clearPending(ctx.from.id); return ctx.reply(`✅ 工单 #${ticketID} 已回复。`); }
      if (pending.kind === "admin_rate") { const rate = Number(input); if (!Number.isSafeInteger(rate) || rate <= 0) throw new Error("汇率无效"); db.setSetting("stars_rate", rate); db.clearPending(ctx.from.id); return ctx.reply(`✅ 汇率：1⭐ = ${rate} SafeLink Stars`); }
    } catch (error) { await ctx.reply(`⚠️ ${escapeHTML(error.message)}`, { parse_mode: "HTML" }); }
  });

  bot.catch(({ error, ctx }) => {
    if (error instanceof GrammyError) console.error("SafeLink Bot API error", error.description);
    else if (error instanceof HttpError) console.error("SafeLink Bot API network error", error);
    else console.error(`Bot update ${ctx.update.update_id} failed`, error);
  });
  return bot;
}
