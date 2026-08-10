export const KINDS = Object.freeze({ premium: "premium", stars: "stars", number: "number", username: "username" });

const fixed = Object.freeze([
  { kind: KINDS.premium, code: "premium_1m", title: "SafeLink Premium - 1 个月", description: "为指定 SafeLink 账号开通 1 个月 Premium", starsPrice: 20, months: 1 },
  { kind: KINDS.premium, code: "premium_3m", title: "SafeLink Premium - 3 个月", description: "为指定 SafeLink 账号开通 3 个月 Premium", starsPrice: 40, months: 3 },
  { kind: KINDS.username, code: "uname_10", title: "收藏用户名 - 10 TON", description: "铸造 SafeLink 收藏用户名", starsPrice: 10, bid: 10 },
  { kind: KINDS.username, code: "uname_100", title: "收藏用户名 - 100 TON", description: "铸造 SafeLink 收藏用户名", starsPrice: 20, bid: 100 },
  { kind: KINDS.username, code: "uname_1000", title: "收藏用户名 - 1000 TON", description: "铸造 SafeLink 收藏用户名", starsPrice: 40, bid: 1000 },
]);

export function catalog(starsRate = 20) {
  const starPackages = [1, 5, 10, 25, 50, 100].map((price) => ({
    kind: KINDS.stars,
    code: `stars_${price}`,
    title: `${price * starsRate} SafeLink Stars`,
    description: `支付 ${price} Stars，获得 ${price * starsRate} SafeLink Stars`,
    starsPrice: price,
    starsAmount: price * starsRate,
  }));
  return [...fixed, ...starPackages];
}

export function findProduct(code, starsRate = 20) {
  const fixedProduct = catalog(starsRate).find((product) => product.code === code);
  if (fixedProduct) return fixedProduct;
  const match = String(code).match(/^stars_([1-9]\d{0,4})$/);
  if (!match) return null;
  const starsPrice = Number(match[1]);
  return {
    kind: KINDS.stars,
    code: `stars_${starsPrice}`,
    title: `${starsPrice * starsRate} SafeLink Stars`,
    description: `支付 ${starsPrice} Stars，获得 ${starsPrice * starsRate} SafeLink Stars`,
    starsPrice,
    starsAmount: starsPrice * starsRate,
  };
}

export function productsOfKind(kind, starsRate = 20) {
  return catalog(starsRate).filter((product) => product.kind === kind);
}

export function normalizeUsername(value) {
  const username = String(value ?? "").trim().replace(/^@/, "").toLowerCase();
  return /^[a-z][a-z0-9_]{4,31}$/.test(username) ? username : "";
}

export function buildPayload(productCode, targetUserID = 0, extra = "") {
  const encoded = Buffer.from(extra, "utf8").toString("base64url");
  return `store|${productCode}|${targetUserID}|${encoded}`;
}

export function parsePayload(payload) {
  const [scope, code, target, encoded = ""] = String(payload).split("|", 4);
  const targetUserID = Number(target);
  if (scope !== "store" || !code || !Number.isSafeInteger(targetUserID) || targetUserID < 0) throw new Error("invalid invoice payload");
  return { code, targetUserID, extra: Buffer.from(encoded, "base64url").toString("utf8") };
}
