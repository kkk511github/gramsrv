import {
  Bell,
  ChevronDown,
  CircleCheck,
  Gift,
  LayoutDashboard,
  LogOut,
  Menu,
  MessageSquareText,
  PanelLeftClose,
  PanelLeftOpen,
  RefreshCw,
  ShieldCheck,
  UserRound,
  Users,
  X
} from "lucide-react";
import { useEffect, useMemo, useState, type ReactNode } from "react";
import { api } from "../api";
import { LanguageSwitch, useI18n } from "../i18n";
import { type Navigate, type RouteState, routeSubtitle, routeTitle } from "../routing";
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
          <NavLink icon={<Gift size={18} />} href="/gifts" route={route} navigate={navigate} title={t("layout.gifts")}>{t("layout.gifts")}</NavLink>
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
