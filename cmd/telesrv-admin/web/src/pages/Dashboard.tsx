import {
  Activity,
  AlertTriangle,
  ArrowRight,
  BellRing,
  Check,
  CircleCheckBig,
  Clock3,
  Database,
  HardDrive,
  LineChart,
  MessageSquareText,
  Radio,
  Server,
  ShieldAlert,
  ShieldCheck,
  Snowflake
} from "lucide-react";
import { useEffect, useState, type ReactNode } from "react";
import { api } from "../api";
import { AppLink } from "../components/AppLink";
import { useI18n, type TFunction } from "../i18n";
import type { Navigate } from "../routing";
import type {
  OverviewActivity,
  OverviewAttention,
  OverviewResponse,
  OverviewTrendPoint,
  RuntimeServiceStatus,
  RuntimeStatusResponse
} from "../types";

type Tone = "good" | "pending" | "high" | "medium";

export function Dashboard({ navigate }: { navigate: Navigate }) {
  const { t, lang } = useI18n();
  const [runtime, setRuntime] = useState<RuntimeStatusResponse | null>(null);
  const [runtimeFailed, setRuntimeFailed] = useState(false);
  const [overview, setOverview] = useState<OverviewResponse | null>(null);
  const [overviewFailed, setOverviewFailed] = useState(false);

  useEffect(() => {
    let active = true;
    const load = async () => {
      try {
        const next = await api.runtimeStatus();
        if (active) {
          setRuntime(next);
          setRuntimeFailed(false);
        }
      } catch {
        if (active) setRuntimeFailed(true);
      }
    };
    void load();
    const timer = window.setInterval(() => void load(), 15_000);
    return () => {
      active = false;
      window.clearInterval(timer);
    };
  }, []);

  useEffect(() => {
    let active = true;
    const load = async () => {
      try {
        const next = await api.overview();
        if (active) {
          setOverview(next);
          setOverviewFailed(false);
        }
      } catch {
        if (active) setOverviewFailed(true);
      }
    };
    void load();
    const timer = window.setInterval(() => void load(), 60_000);
    return () => {
      active = false;
      window.clearInterval(timer);
    };
  }, []);

  const status = runtime?.services;
  const services = [
    {
      icon: <Radio />,
      label: "MTProto",
      ...servicePresentation("mtproto", status?.mtproto, runtimeFailed, t)
    },
    {
      icon: <Server />,
      label: "Admin API",
      ...servicePresentation("admin_api", status?.admin_api, runtimeFailed, t)
    },
    {
      icon: <Database />,
      label: "PostgreSQL",
      ...servicePresentation("postgres", status?.postgres, runtimeFailed, t)
    },
    {
      icon: <BellRing />,
      label: t("dashboard.service.push"),
      ...servicePresentation("push", status?.push, runtimeFailed, t)
    },
    {
      icon: <HardDrive />,
      label: t("dashboard.service.media"),
      ...servicePresentation("media", status?.media, runtimeFailed, t)
    }
  ];
  const overall = runtime?.overall ?? (runtimeFailed ? "unavailable" : "loading");
  const checkedAt = runtime?.checked_at
    ? new Intl.DateTimeFormat(localeFor(lang), {
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
        hour12: false
      }).format(new Date(runtime.checked_at))
    : "";
  const attentionCount = overview?.attention.filter((item) => item.count > 0).length ?? 0;

  return (
    <div className="control-dashboard">
      <section className={`health-band ${overall}`} aria-live="polite">
        <div className="health-summary">
          <span className="health-icon">
            {overall === "healthy" ? <CircleCheckBig size={31} /> : overall === "loading" ? <Activity size={29} /> : <AlertTriangle size={29} />}
          </span>
          <div>
            <h2>{overall === "healthy" ? t("dashboard.healthTitle") : overall === "loading" ? t("dashboard.healthChecking") : t("dashboard.healthDegraded")}</h2>
            <p>{checkedAt ? t("dashboard.healthLiveBody", { time: checkedAt }) : runtimeFailed ? t("dashboard.healthProbeFailed") : t("dashboard.healthCheckingBody")}</p>
          </div>
        </div>
        <div className="service-grid">
          {services.map((service) => (
            <div className={`service-status ${service.tone}`} key={service.label}>
              <span className="service-icon">{service.icon}</span>
              <div>
                <strong>{service.label}</strong>
                <span><i />{service.value}</span>
                <small>{service.detail}</small>
              </div>
            </div>
          ))}
        </div>
      </section>

      <section className="dashboard-panel attention-panel">
        <div className="dashboard-panel-head">
          <div>
            <h2>
              {t("dashboard.attentionTitle")}
              <b className={`attention-count ${overview && attentionCount === 0 ? "good" : ""}`}>{overview ? attentionCount : "…"}</b>
            </h2>
            <p>{t("dashboard.attentionBody")}</p>
          </div>
          <AppLink className="panel-link" href="/accounts" navigate={navigate}>
            {t("dashboard.viewAll")}<ArrowRight size={14} />
          </AppLink>
        </div>
        <div className="attention-table" role="table" aria-label={t("dashboard.attentionTitle")}>
          <div className="attention-table-head" role="row">
            <span>{t("dashboard.table.item")}</span>
            <span>{t("dashboard.table.severity")}</span>
            <span>{t("dashboard.table.count")}</span>
            <span>{t("dashboard.table.statusOwner")}</span>
            <span>{t("dashboard.table.discovered")}</span>
            <span>{t("common.operations")}</span>
          </div>
          {overview?.attention.map((item) => {
            const presentation = attentionPresentation(item, t);
            return (
              <AttentionRow
                key={item.id}
                {...presentation}
                count={item.count}
                status={t(`dashboard.queue.state.${item.state}`)}
                discovered={item.discovered_at ? formatDateTime(item.discovered_at, lang) : t("dashboard.queue.none")}
                navigate={navigate}
              />
            );
          })}
          {!overview && (
            <div className={`dashboard-data-state ${overviewFailed ? "failed" : ""}`}>
              {overviewFailed ? t("dashboard.data.failed") : t("dashboard.data.loading")}
            </div>
          )}
        </div>
      </section>

      <div className="dashboard-lower-grid">
        <section className="dashboard-panel activity-panel">
          <div className="dashboard-panel-head compact">
            <div>
              <h2>{t("dashboard.activityTitle")}</h2>
              <p>{t("dashboard.activityBody")}</p>
            </div>
            <span className="panel-meta"><Clock3 size={14} />{t("dashboard.activity.audit")}</span>
          </div>
          <div className="activity-list">
            {overview?.activity.map((item) => (
              <ActivityRow key={item.id} item={item} lang={lang} t={t} />
            ))}
            {overview && overview.activity.length === 0 && (
              <div className="dashboard-data-state">{t("dashboard.activity.empty")}</div>
            )}
            {!overview && (
              <div className={`dashboard-data-state ${overviewFailed ? "failed" : ""}`}>
                {overviewFailed ? t("dashboard.data.failed") : t("dashboard.data.loading")}
              </div>
            )}
          </div>
        </section>

        <section className="dashboard-panel trend-panel">
          <div className="dashboard-panel-head compact">
            <div>
              <h2>{t("dashboard.trendTitle")}</h2>
              <p>{t("dashboard.trendBody")}</p>
            </div>
            <span className="panel-meta">{t("dashboard.last24Hours")}</span>
          </div>
          <RuntimeTrendChart
            points={overview?.trend ?? []}
            loading={!overview && !overviewFailed}
            failed={overviewFailed}
            lang={lang}
            t={t}
          />
        </section>
      </div>

      <section className="dashboard-safety-bar">
        <ShieldAlert size={24} />
        <div>
          <strong>{t("dashboard.safetyTitle")}</strong>
          <span>{t("dashboard.safetyBody")}</span>
        </div>
        <AppLink href="/accounts" navigate={navigate}>{t("dashboard.securitySettings")}<ArrowRight size={15} /></AppLink>
      </section>
    </div>
  );
}

function servicePresentation(
  id: "mtproto" | "admin_api" | "postgres" | "push" | "media",
  status: RuntimeServiceStatus | undefined,
  failed: boolean,
  t: TFunction
): { value: string; detail: string; tone: Tone } {
  if (!status) {
    return {
      value: failed ? t("dashboard.service.unavailable") : t("dashboard.service.checking"),
      detail: failed ? t("dashboard.service.probeFailed") : t("dashboard.service.collecting"),
      tone: failed ? "high" : "pending"
    };
  }
  const value = t(`dashboard.service.state.${status.state}`);
  const tone = status.state === "healthy" ? "good" : status.state === "degraded" ? "medium" : status.state === "unavailable" ? "high" : "pending";
  if (id === "postgres") {
    return {
      value,
      detail: t("dashboard.service.postgresDetail", {
        acquired: status.pool_acquired ?? 0,
        total: status.pool_total ?? 0,
        latency: formatLatency(status.latency_ms)
      }),
      tone
    };
  }
  if (id === "push") {
    if (status.state === "disabled") {
      return { value, detail: t("dashboard.service.pushDisabled"), tone };
    }
    const providers = (status.providers ?? []).map((item) => item === "apns" ? "APNs" : item.toUpperCase()).join(" + ") || t("common.none");
    return {
      value,
      detail: t("dashboard.service.pushDetail", {
        providers,
        devices: status.registered_devices ?? 0,
        pending: status.pending ?? 0,
        retrying: status.retrying ?? 0
      }),
      tone
    };
  }
  if (id === "media") {
    return {
      value,
      detail: t("dashboard.service.mediaDetail", {
        backend: status.backend ?? "localfs",
        objects: status.object_count ?? 0,
        size: formatBytes(status.total_bytes ?? 0)
      }),
      tone
    };
  }
  return {
    value,
    detail: t("dashboard.service.endpointDetail", {
      endpoint: status.endpoint ?? "-",
      latency: formatLatency(status.latency_ms)
    }),
    tone
  };
}

function attentionPresentation(item: OverviewAttention, t: TFunction): {
  icon: ReactNode;
  title: string;
  text: string;
  severity: string;
  owner: string;
  href: string;
  tone: "high" | "medium" | "good";
} {
  const healthy = item.count === 0;
  if (item.id === "frozen_accounts") {
    return {
      icon: <Snowflake />,
      title: t("dashboard.queue.freezeTitle"),
      text: t("dashboard.queue.freezeText"),
      severity: healthy ? t("dashboard.severity.normal") : t("dashboard.severity.high"),
      owner: t("dashboard.queue.riskOwner"),
      href: "/accounts",
      tone: healthy ? "good" : "high"
    };
  }
  if (item.id === "push_retries") {
    return {
      icon: <MessageSquareText />,
      title: t("dashboard.queue.pushTitle"),
      text: t("dashboard.queue.pushText"),
      severity: healthy ? t("dashboard.severity.normal") : t("dashboard.severity.medium"),
      owner: t("dashboard.queue.messageOwner"),
      href: "/messages",
      tone: healthy ? "good" : "medium"
    };
  }
  return {
    icon: <HardDrive />,
    title: t("dashboard.queue.mediaTitle"),
    text: t("dashboard.queue.mediaText"),
    severity: healthy ? t("dashboard.severity.normal") : t("dashboard.severity.medium"),
    owner: t("dashboard.queue.mediaOwner"),
    href: "/messages",
    tone: healthy ? "good" : "medium"
  };
}

function AttentionRow({
  icon,
  title,
  text,
  severity,
  count,
  status,
  owner,
  discovered,
  href,
  navigate,
  tone
}: {
  icon: ReactNode;
  title: string;
  text: string;
  severity: string;
  count: number;
  status: string;
  owner: string;
  discovered: string;
  href: string;
  navigate: Navigate;
  tone: "high" | "medium" | "good";
}) {
  const { t } = useI18n();
  return (
    <div className={`attention-row ${tone}`} role="row">
      <div className="attention-item">
        <span className={`attention-icon ${tone}`}>{icon}</span>
        <div><strong>{title}</strong><small>{text}</small></div>
      </div>
      <span><b className={`severity-badge ${tone}`}>{severity}</b></span>
      <span className="attention-number">{count.toLocaleString()}</span>
      <div className="attention-state"><strong>{status}</strong><small>{owner}</small></div>
      <span className="attention-time">{discovered}</span>
      <AppLink className="queue-action" href={href} navigate={navigate}>{t("dashboard.view")}</AppLink>
    </div>
  );
}

function ActivityRow({ item, lang, t }: { item: OverviewActivity; lang: string; t: TFunction }) {
  const title = activityTitle(item.action, t);
  const target = item.target_user_id
    ? t("dashboard.activity.targetUser", { id: item.target_user_id })
    : item.target_peer_id
      ? t("dashboard.activity.targetPeer", { type: item.target_peer_type || "peer", id: item.target_peer_id })
      : t("dashboard.activity.targetSystem");
  return (
    <div className="activity-row">
      <time dateTime={item.created_at}>{formatDateTime(item.created_at, lang)}</time>
      <span className="activity-node" />
      <span className={`activity-icon ${item.status}`}>
        {item.status === "failed" ? <AlertTriangle /> : <ShieldCheck />}
      </span>
      <div>
        <strong>{item.dry_run ? t("dashboard.activity.dryRunTitle", { action: title }) : title}</strong>
        <small>{t("dashboard.activity.detail", { actor: item.actor, target })}</small>
      </div>
      <span className={`activity-success ${item.status}`}>
        {item.status === "failed" ? <AlertTriangle size={12} /> : <Check size={12} />}
        {t(`dashboard.activity.status.${item.status}`)}
      </span>
    </div>
  );
}

function activityTitle(action: string, t: TFunction): string {
  const keys: Record<string, string> = {
    "set-frozen": "dashboard.activity.action.setFrozen",
    "grant-premium": "dashboard.activity.action.grantPremium",
    "grant-stars": "dashboard.activity.action.grantStars",
    "set-verified": "dashboard.activity.action.setVerified",
    "set-channel-verified": "dashboard.activity.action.setChannelVerified",
    "revoke-sessions": "dashboard.activity.action.revokeSessions",
    "delete-messages": "dashboard.activity.action.deleteMessages",
    "delete-history": "dashboard.activity.action.deleteHistory",
    "import-gift": "dashboard.activity.action.importGift",
    "import-official-gift": "dashboard.activity.action.importOfficialGift",
    "publish-gift-collectibles": "dashboard.activity.action.publishCollectibles",
    "set-gift-enabled": "dashboard.activity.action.setGiftEnabled",
    "set-gift-sort-order": "dashboard.activity.action.setGiftOrder"
  };
  return keys[action] ? t(keys[action]) : action;
}

function RuntimeTrendChart({
  points,
  loading,
  failed,
  lang,
  t
}: {
  points: OverviewTrendPoint[];
  loading: boolean;
  failed: boolean;
  lang: string;
  t: TFunction;
}) {
  if (loading || failed || points.length === 0) {
    return (
      <div className={`trend-empty ${failed ? "failed" : ""}`}>
        {failed ? <AlertTriangle size={28} /> : <LineChart size={28} />}
        <strong>{failed ? t("dashboard.data.failed") : t("dashboard.data.loading")}</strong>
        <span>{failed ? t("dashboard.trend.failedText") : t("dashboard.trend.loadingText")}</span>
      </div>
    );
  }

  const latest = points[points.length - 1];
  const width = 720;
  const height = 226;
  const plot = { left: 44, right: 48, top: 18, bottom: 32 };
  const plotWidth = width - plot.left - plot.right;
  const plotHeight = height - plot.top - plot.bottom;
  const countMax = niceMaximum(Math.max(
    ...points.map((point) => point.messages_per_minute),
    ...points.map((point) => point.rpc_requests_per_minute ?? 0),
    1
  ));
  const x = (index: number) => plot.left + (points.length === 1 ? plotWidth / 2 : (index / (points.length - 1)) * plotWidth);
  const countY = (value: number) => plot.top + plotHeight - (value / countMax) * plotHeight;
  const pushY = (value: number) => plot.top + plotHeight - (Math.max(0, Math.min(100, value)) / 100) * plotHeight;
  const messagePath = seriesPath(points, x, (point) => point.messages_per_minute, countY);
  const rpcPath = seriesPath(points, x, (point) => point.rpc_requests_per_minute, countY);
  const pushPath = seriesPath(points, x, (point) => point.push_success_rate, pushY);
  const labelIndexes = new Set([0, 6, 12, 18, points.length - 1].filter((index) => index < points.length));

  return (
    <>
      <div className="trend-legend">
        <span><i className="cyan" />{t("dashboard.trend.messages")}<b>{formatMetric(latest.messages_per_minute, lang)}</b></span>
        <span><i className="blue" />{t("dashboard.trend.api")}<b>{formatMetric(latest.rpc_requests_per_minute, lang)}</b></span>
        <span><i className="gray" />{t("dashboard.trend.push")}<b>{latest.push_success_rate == null ? "—" : `${formatMetric(latest.push_success_rate, lang)}%`}</b></span>
      </div>
      <div className="trend-chart-wrap">
        <svg className="trend-chart" viewBox={`0 0 ${width} ${height}`} role="img" aria-label={t("dashboard.trend.chartLabel")}>
          <title>{t("dashboard.trend.chartLabel")}</title>
          {Array.from({ length: 5 }, (_, index) => {
            const ratio = index / 4;
            const y = plot.top + ratio * plotHeight;
            return (
              <g key={index}>
                <line className="trend-grid-line" x1={plot.left} x2={width - plot.right} y1={y} y2={y} />
                <text className="trend-axis-label" x={plot.left - 8} y={y + 3} textAnchor="end">{formatAxis(countMax * (1 - ratio), lang)}</text>
                <text className="trend-axis-label push" x={width - plot.right + 8} y={y + 3}>{Math.round(100 * (1 - ratio))}%</text>
              </g>
            );
          })}
          {points.map((point, index) => labelIndexes.has(index) ? (
            <text className="trend-axis-label time" key={point.bucket_at} x={x(index)} y={height - 9} textAnchor="middle">
              {formatHour(point.bucket_at, lang)}
            </text>
          ) : null)}
          <path className="trend-series messages" d={messagePath} />
          <path className="trend-series rpc" d={rpcPath} />
          <path className="trend-series push" d={pushPath} />
          {points.map((point, index) => (
            <circle className="trend-hit" key={`hit-${point.bucket_at}`} cx={x(index)} cy={plot.top + plotHeight / 2} r={9}>
              <title>{trendTooltip(point, lang, t)}</title>
            </circle>
          ))}
          <circle className="trend-latest messages" cx={x(points.length - 1)} cy={countY(latest.messages_per_minute)} r={3.5} />
          {latest.rpc_requests_per_minute != null && <circle className="trend-latest rpc" cx={x(points.length - 1)} cy={countY(latest.rpc_requests_per_minute)} r={3.5} />}
          {latest.push_success_rate != null && <circle className="trend-latest push" cx={x(points.length - 1)} cy={pushY(latest.push_success_rate)} r={3.5} />}
        </svg>
      </div>
      <p className="trend-footnote">{t("dashboard.trend.privacy")}</p>
    </>
  );
}

function seriesPath(
  points: OverviewTrendPoint[],
  x: (index: number) => number,
  value: (point: OverviewTrendPoint) => number | undefined,
  y: (value: number) => number
): string {
  let drawing = false;
  return points.map((point, index) => {
    const current = value(point);
    if (current == null || !Number.isFinite(current)) {
      drawing = false;
      return "";
    }
    const command = drawing ? "L" : "M";
    drawing = true;
    return `${command}${x(index).toFixed(2)},${y(current).toFixed(2)}`;
  }).filter(Boolean).join(" ");
}

function trendTooltip(point: OverviewTrendPoint, lang: string, t: TFunction): string {
  return [
    formatDateTime(point.bucket_at, lang),
    `${t("dashboard.trend.messages")}: ${formatMetric(point.messages_per_minute, lang)}`,
    `${t("dashboard.trend.api")}: ${formatMetric(point.rpc_requests_per_minute, lang)}`,
    `${t("dashboard.trend.push")}: ${point.push_success_rate == null ? "—" : `${formatMetric(point.push_success_rate, lang)}%`}`,
    `${t("dashboard.trend.pushAttempts")}: ${point.push_delivered + point.push_failed}`
  ].join("\n");
}

function niceMaximum(value: number): number {
  if (value <= 0) return 1;
  const magnitude = 10 ** Math.floor(Math.log10(value));
  const normalized = value / magnitude;
  const nice = normalized <= 1 ? 1 : normalized <= 2 ? 2 : normalized <= 5 ? 5 : 10;
  return nice * magnitude;
}

function formatMetric(value: number | undefined, lang: string): string {
  if (value == null || !Number.isFinite(value)) return "—";
  return new Intl.NumberFormat(localeFor(lang), { maximumFractionDigits: value < 10 ? 2 : 1 }).format(value);
}

function formatAxis(value: number, lang: string): string {
  return new Intl.NumberFormat(localeFor(lang), { notation: value >= 1000 ? "compact" : "standard", maximumFractionDigits: value < 10 ? 1 : 0 }).format(value);
}

function formatHour(value: string, lang: string): string {
  return new Intl.DateTimeFormat(localeFor(lang), { hour: "2-digit", minute: "2-digit", hour12: false }).format(new Date(value));
}

function formatDateTime(value: string, lang: string): string {
  return new Intl.DateTimeFormat(localeFor(lang), {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false
  }).format(new Date(value));
}

function localeFor(lang: string): string {
  return lang === "zh" ? "zh-CN" : lang === "ru" ? "ru-RU" : "en-US";
}

function formatLatency(value?: number): string {
  return `${Math.max(1, value ?? 1)} ms`;
}

function formatBytes(value: number): string {
  if (value < 1024) return `${value} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let size = value;
  let unit = -1;
  do {
    size /= 1024;
    unit += 1;
  } while (size >= 1024 && unit < units.length - 1);
  return `${size.toFixed(size >= 10 ? 1 : 2)} ${units[unit]}`;
}
