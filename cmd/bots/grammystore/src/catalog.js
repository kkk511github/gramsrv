export const KINDS = Object.freeze({ premium: "premium", stars: "stars", number: "number", username: "username" });

const fixed = Object.freeze([
  { kind: KINDS.premium, code: "premium_1m", title: "SafeLink Premium — 1 month", titleZh: "SafeLink Premium - 1 个月", titleRu: "SafeLink Premium — 1 месяц", description: "SafeLink Premium subscription for one month", descriptionZh: "为指定 SafeLink 账号开通 1 个月 Premium", descriptionRu: "Подписка SafeLink Premium на один месяц", starsPrice: 20, months: 1 },
  { kind: KINDS.premium, code: "premium_3m", title: "SafeLink Premium — 3 months", titleZh: "SafeLink Premium - 3 个月", titleRu: "SafeLink Premium — 3 месяца", description: "SafeLink Premium subscription for three months", descriptionZh: "为指定 SafeLink 账号开通 3 个月 Premium", descriptionRu: "Подписка SafeLink Premium на три месяца", starsPrice: 40, months: 3 },
  { kind: KINDS.number, code: "num_short", title: "Anonymous +888 8 XXX", titleZh: "匿名号码 +888 8 XXX", titleRu: "Анонимный +888 8 XXX", description: "Short collectible anonymous number", descriptionZh: "短位收藏匿名号码", descriptionRu: "Короткий коллекционный анонимный номер", starsPrice: 50, numberFormat: "short" },
  { kind: KINDS.number, code: "num_long", title: "Anonymous +888 0XXX XXXX", titleZh: "匿名号码 +888 0XXX XXXX", titleRu: "Анонимный +888 0XXX XXXX", description: "Anonymous +888 number", descriptionZh: "SafeLink 匿名 +888 号码", descriptionRu: "Анонимный номер +888", starsPrice: 25, numberFormat: "long" },
  { kind: KINDS.username, code: "uname_10", title: "SafeLink collectible username — 10 TON", titleZh: "SafeLink 收藏用户名 - 10 TON", titleRu: "Коллекционный username SafeLink — 10 TON", description: "Mint a SafeLink collectible username", descriptionZh: "铸造 SafeLink 收藏用户名", descriptionRu: "Выпустить коллекционный username SafeLink", starsPrice: 10, bid: 10 },
  { kind: KINDS.username, code: "uname_100", title: "SafeLink collectible username — 100 TON", titleZh: "SafeLink 收藏用户名 - 100 TON", titleRu: "Коллекционный username SafeLink — 100 TON", description: "Mint a SafeLink collectible username", descriptionZh: "铸造 SafeLink 收藏用户名", descriptionRu: "Выпустить коллекционный username SafeLink", starsPrice: 20, bid: 100 },
  { kind: KINDS.username, code: "uname_1000", title: "SafeLink collectible username — 1000 TON", titleZh: "SafeLink 收藏用户名 - 1000 TON", titleRu: "Коллекционный username SafeLink — 1000 TON", description: "Mint a SafeLink collectible username", descriptionZh: "铸造 SafeLink 收藏用户名", descriptionRu: "Выпустить коллекционный username SafeLink", starsPrice: 40, bid: 1000 },
]);

export function catalog(starsRate = 20) {
  const starPackages = [1, 5, 10, 25, 50, 100].map((price) => ({
    kind: KINDS.stars,
    code: `stars_${price}`,
    title: `${price * starsRate} SafeLink Stars`,
    titleZh: `${price * starsRate} SafeLink Stars`,
    titleRu: `${price * starsRate} SafeLink Stars`,
    description: `Receive ${price * starsRate} SafeLink Stars for ${price} payment Stars`,
    descriptionZh: `支付 ${price} Stars，获得 ${price * starsRate} SafeLink Stars`,
    descriptionRu: `${price * starsRate} Stars SafeLink за ${price} платёжных Stars`,
    starsPrice: price,
    starsAmount: price * starsRate,
  }));
  return [...fixed, ...starPackages];
}

export function findProduct(code, starsRate = 20) {
  const fixedProduct = catalog(starsRate).find((product) => product.code === code);
  if (fixedProduct) return fixedProduct;
  const match = String(code).match(/^stars_([1-9]\d{0,5})$/);
  if (!match) return null;
  const starsPrice = Number(match[1]);
  if (starsPrice > 100000) return null;
  return {
    kind: KINDS.stars,
    code: `stars_${starsPrice}`,
    title: `${starsPrice * starsRate} SafeLink Stars`,
    titleZh: `${starsPrice * starsRate} SafeLink Stars`,
    titleRu: `${starsPrice * starsRate} SafeLink Stars`,
    description: `Receive ${starsPrice * starsRate} SafeLink Stars for ${starsPrice} payment Stars`,
    descriptionZh: `支付 ${starsPrice} Stars，获得 ${starsPrice * starsRate} SafeLink Stars`,
    descriptionRu: `${starsPrice * starsRate} Stars SafeLink за ${starsPrice} платёжных Stars`,
    starsPrice,
    starsAmount: starsPrice * starsRate,
  };
}

export function productsOfKind(kind, starsRate = 20) {
  return catalog(starsRate).filter((product) => product.kind === kind);
}

export function localizeProduct(product, language = "en") {
  if (!product) return null;
  if (language === "zh") return { ...product, title: product.titleZh ?? product.title, description: product.descriptionZh ?? product.description };
  if (language === "ru") return { ...product, title: product.titleRu ?? product.title, description: product.descriptionRu ?? product.description };
  return product;
}

export function normalizeUsername(value) {
  const username = String(value ?? "").trim().replace(/^@/, "").toLowerCase();
  return /^[a-z][a-z0-9_]{4,31}$/.test(username) ? username : "";
}

export function buildPayload(productCode, targetUserID = 0, extra = "", starsAmount = 0) {
  const encoded = Buffer.from(extra, "utf8").toString("base64url");
  return `store|${productCode}|${targetUserID}|${encoded}|${starsAmount}`;
}

export function parsePayload(payload) {
  const [scope, code, target, encoded = "", rawStarsAmount = "0"] = String(payload).split("|", 5);
  const targetUserID = Number(target);
  const starsAmount = Number(rawStarsAmount || 0);
  if (scope !== "store" || !code || !Number.isSafeInteger(targetUserID) || targetUserID < 0 ||
      !Number.isSafeInteger(starsAmount) || starsAmount < 0) throw new Error("invalid invoice payload");
  return { code, targetUserID, extra: Buffer.from(encoded, "base64url").toString("utf8"), starsAmount };
}
