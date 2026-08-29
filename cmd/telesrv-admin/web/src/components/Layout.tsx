import {
  AtSign,
  BadgeCheck,
  Bell,
  Bot,
  ChevronDown,
  CircleCheck,
  Database,
  Film,
  Gift,
  Gavel,
  LayoutDashboard,
  LogOut,
  Menu,
  MessageSquareText,
  Megaphone,
  PanelLeftClose,
  PanelLeftOpen,
  Phone,
  BadgeDollarSign,
  RefreshCw,
  Send,
  ShieldAlert,
  ShieldCheck,
  Smile,
  Stamp,
  Sticker,
  Trophy,
  UserRound,
  Users,
  X
} from "lucide-react";
import { useEffect, useMemo, useState, type ReactNode } from "react";
import { api } from "../api";
import { LanguageSwitch, useI18n } from "../i18n";
import { permissionBotVerificationReview, permissionPremiumManage, permissionVerificationReview, useCan } from "../permissions";
import { type Navigate, type RouteState, routeSubtitle, routeTitle } from "../routing";
import { ThemeSwitch } from "../theme";
import { AppLink } from "./AppLink";

export function BootScreen() {
  const { t } = useI18n();
  return (
    <div className="boot-screen">
      <div className="brand compact brand-elevated">
        <span className="brand-mark"><ShieldCheck size={23} /></span>
        <span>
          <strong>SafeLink</strong>
          <small>{t("app.adminConsole")}</small>
        </span>
      </div>
      <div className="loader-bar" />
    </div>
  );
}

export function Shell({
  actor,
  route,
  navigate,
  onLogout,
  children
}: {
  actor: string;
  route: RouteState;
  navigate: Navigate;
  onLogout: () => void;
  children: ReactNode;
}) {
  const { t, lang } = useI18n();
  // The verification queue is hidden for a session without verification.review:
  // the entry would only lead to a 403 (and the route itself is gated as well).
  const canReviewVerification = useCan(permissionVerificationReview);
  // Same reasoning for the third-party queue, which has its own right: the two
  // sections are granted independently, so one entry can be visible without the other.
  const canReviewBotVerification = useCan(permissionBotVerificationReview);
  const canManagePremium = useCan(permissionPremiumManage);
  const messagesActive = route.path.startsWith("/messages");
  const [messagesOpen, setMessagesOpen] = useState(messagesActive);
  const [collapsed, setCollapsed] = useState(false);
  const [mobileOpen, setMobileOpen] = useState(false);
  const loadedAt = useMemo(() => new Date(), []);
  const refreshTime = new Intl.DateTimeFormat(lang === "zh" ? "zh-CN" : "en-US", {
    hour: "2-digit",
    minute: "2-digit"
  }).format(loadedAt);

  useEffect(() => {
    if (messagesActive) {
      setMessagesOpen(true);
    }
    setMobileOpen(false);
  }, [messagesActive, route.href]);

  async function logout() {
    await api.logout().catch(() => undefined);
    onLogout();
  }

  return (
    <div className={`shell ${collapsed ? "sidebar-collapsed" : ""} ${mobileOpen ? "sidebar-mobile-open" : ""}`}>
      <button
        className="sidebar-scrim"
        type="button"
        aria-label={t("layout.closeNav")}
        onClick={() => setMobileOpen(false)}
      />
      <aside className="sidebar">
        <div className="sidebar-head">
          <AppLink className="brand" href="/" navigate={navigate}>
            <span className="brand-mark"><ShieldCheck size={23} /></span>
            <span className="brand-copy">
              <strong>SafeLink</strong>
              <small>{t("app.adminConsole")}</small>
            </span>
          </AppLink>
          <button
            className="icon-btn sidebar-close"
            type="button"
            aria-label={t("layout.closeNav")}
            title={t("layout.closeNav")}
            onClick={() => setMobileOpen(false)}
          >
            <X size={18} />
          </button>
        </div>
        <nav className="nav-list" aria-label={t("layout.primaryNav")}>
          <NavLink icon={<LayoutDashboard size={18} />} href="/" route={route} navigate={navigate} title={t("layout.dashboard")}>{t("layout.dashboard")}</NavLink>
          <NavLink icon={<Users size={18} />} href="/accounts" route={route} navigate={navigate} title={t("layout.accounts")}>{t("layout.accounts")}</NavLink>
          <NavLink icon={<ShieldCheck size={18} />} href="/channels" route={route} navigate={navigate} title={t("layout.channels")}>{t("layout.channels")}</NavLink>
          <NavLink icon={<Bot size={18} />} href="/bots" route={route} navigate={navigate} title={t("layout.bots")}>{t("layout.bots")}</NavLink>
          <NavLink icon={<Megaphone size={18} />} href="/broadcasts" route={route} navigate={navigate} title={t("layout.broadcasts")}>{t("layout.broadcasts")}</NavLink>
          {canManagePremium && (
            <NavLink icon={<BadgeDollarSign size={18} />} href="/monetization" route={route} navigate={navigate} title={t("layout.premium")}
              activeWhen={(path) => path.startsWith("/monetization") || path.startsWith("/premium")}>
              {t("layout.premium")}
            </NavLink>
          )}
          <NavLink icon={<ShieldAlert size={18} />} href="/moderation" route={route} navigate={navigate} title={t("layout.moderation")}>{t("layout.moderation")}</NavLink>
          {canReviewVerification && (
            <NavLink icon={<BadgeCheck size={18} />} href="/verification" route={route} navigate={navigate} title={t("layout.verification")}>{t("layout.verification")}</NavLink>
          )}
          {canReviewBotVerification && (
            <NavLink icon={<Stamp size={18} />} href="/bot-verification" route={route} navigate={navigate} title={t("layout.botVerification")}>{t("layout.botVerification")}</NavLink>
          )}
          <NavLink icon={<AtSign size={18} />} href="/collectible-usernames" route={route} navigate={navigate} title={t("layout.collectibleUsernames")}>{t("layout.collectibleUsernames")}</NavLink>
          <NavLink icon={<Phone size={18} />} href="/collectible-phones" route={route} navigate={navigate} title={t("layout.collectiblePhones")}>{t("layout.collectiblePhones")}</NavLink>
          <NavLink icon={<Trophy size={18} />} href="/account-ratings" route={route} navigate={navigate} title={t("layout.accountRatings")}>{t("layout.accountRatings")}</NavLink>
          <NavLink icon={<Database size={18} />} href="/storage" route={route} navigate={navigate} title={t("layout.storage")}>{t("layout.storage")}</NavLink>
          <NavLink icon={<Gift size={18} />} href="/gifts" route={route} navigate={navigate} title={t("layout.gifts")}>{t("layout.gifts")}</NavLink>
          <NavLink icon={<Send size={18} />} href="/give-gifts" route={route} navigate={navigate} title={t("layout.giveGifts")}>{t("layout.giveGifts")}</NavLink>
          <NavLink icon={<Gavel size={18} />} href="/auctions" route={route} navigate={navigate} title={t("layout.auctions")}>{t("layout.auctions")}</NavLink>
          <NavLink icon={<Sticker size={18} />} href="/stickers" route={route} navigate={navigate} title={t("layout.stickers")}>{t("layout.stickers")}</NavLink>
          <NavLink icon={<Smile size={18} />} href="/emoji" route={route} navigate={navigate} title={t("layout.emoji")}>{t("layout.emoji")}</NavLink>
          <NavLink icon={<Film size={18} />} href="/gif-catalog" route={route} navigate={navigate} title={t("layout.gifCatalog")}>{t("layout.gifCatalog")}</NavLink>
          <div className={`nav-section ${messagesActive ? "active" : ""} ${messagesOpen ? "open" : ""}`}>
            <button
              className="nav-section-toggle"
              type="button"
              aria-expanded={messagesOpen}
              title={t("layout.messages")}
              onClick={() => setMessagesOpen((open) => !open)}
            >
              <MessageSquareText size={18} />
              <span>{t("layout.messages")}</span>
              <ChevronDown className="nav-section-chevron" size={15} />
            </button>
            {messagesOpen && (
              <div className="nav-children">
                <NavLink
                  href="/messages/private"
                  route={route}
                  navigate={navigate}
                  title={t("layout.privateMessages")}
                  activeWhen={(path) => path === "/messages" || path === "/messages/detail" || path.startsWith("/messages/private")}
                >
                  {t("layout.privateMessages")}
                </NavLink>
                <NavLink
                  href="/messages/groups"
                  route={route}
                  navigate={navigate}
                  title={t("layout.groupMessages")}
                  activeWhen={(path) => path.startsWith("/messages/groups")}
                >
                  {t("layout.groupMessages")}
                </NavLink>
              </div>
            )}
          </div>
        </nav>
        <div className="sidebar-footer">
          <div className="sidebar-system" title={t("layout.connected")}>
            <CircleCheck size={17} />
            <span>
              <strong>{t("layout.production")}</strong>
              <small>{t("layout.connected")}</small>
            </span>
          </div>
          <button
            className="sidebar-collapse"
            type="button"
            title={collapsed ? t("layout.expandNav") : t("layout.collapseNav")}
            onClick={() => setCollapsed((value) => !value)}
          >
            {collapsed ? <PanelLeftOpen size={17} /> : <PanelLeftClose size={17} />}
            <span>{collapsed ? t("layout.expandNav") : t("layout.collapseNav")}</span>
          </button>
        </div>
      </aside>
      <div className="workspace">
        <header className="topbar">
          <div className="topbar-title-group">
            <button
              className="icon-btn topbar-menu"
              type="button"
              aria-label={t("layout.openNav")}
              title={t("layout.openNav")}
              onClick={() => setMobileOpen(true)}
            >
              <Menu size={19} />
            </button>
            <div className="topbar-title">
              <h1>{routeTitle(route.path, t)}</h1>
              <span>{routeSubtitle(route.path, t)}</span>
            </div>
          </div>
          <div className="topbar-actions">
            <span className="environment-chip"><i />{t("layout.production")}</span>
            <span className="refresh-stamp">{t("layout.lastRefresh", { time: refreshTime })}</span>
            <button className="icon-btn" type="button" title={t("common.refresh")} aria-label={t("common.refresh")} onClick={() => window.location.reload()}>
              <RefreshCw size={16} />
            </button>
            <ThemeSwitch />
            <LanguageSwitch />
            <button className="icon-btn notification-button" type="button" title={t("layout.notifications")} aria-label={t("layout.notifications")}>
              <Bell size={17} />
            </button>
            <span className="actor-pill"><UserRound size={16} /><span>{actor}</span><i /></span>
            <button className="icon-btn logout-button" type="button" onClick={logout} title={t("layout.logout")} aria-label={t("layout.logout")}>
              <LogOut size={17} />
            </button>
          </div>
        </header>
        <main className="content">{children}</main>
      </div>
    </div>
  );
}

function NavLink({
  href,
  route,
  navigate,
  icon,
  children,
  activeWhen,
  title
}: {
  href: string;
  route: RouteState;
  navigate: Navigate;
  icon?: ReactNode;
  children: ReactNode;
  activeWhen?: (path: string) => boolean;
  title?: string;
}) {
  const active = activeWhen ? activeWhen(route.path) : href === "/" ? route.path === "/" : route.path.startsWith(href);
  return (
    <AppLink className={`nav-item ${active ? "active" : ""}`} href={href} navigate={navigate} title={title}>
      {icon ?? <span aria-hidden="true" className="nav-dot" />}
      <span>{children}</span>
    </AppLink>
  );
}
