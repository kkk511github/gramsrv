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
  Snowflake,
  Users
} from "lucide-react";
import { useEffect, useMemo, useState, type ReactNode } from "react";
import { api } from "../api";
import { AppLink } from "../components/AppLink";
import { useI18n, type TFunction } from "../i18n";
import type { Navigate } from "../routing";
import type { RuntimeServiceStatus, RuntimeStatusResponse } from "../types";

type Tone = "good" | "pending" | "high" | "medium";

export function Dashboard({ navigate }: { navigate: Navigate }) {
  const { t, lang } = useI18n();
  const [runtime, setRuntime] = useState<RuntimeStatusResponse | null>(null);
  const [runtimeFailed, setRuntimeFailed] = useState(false);

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
    ? new Intl.DateTimeFormat(lang === "zh" ? "zh-CN" : lang === "ru" ? "ru-RU" : "en-US", {
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
        hour12: false
      }).format(new Date(runtime.checked_at))
    : "";
  const activity = useMemo(() => {
    const formatter = new Intl.DateTimeFormat(lang === "zh" ? "zh-CN" : "en-US", {
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
      hour12: false
    });
    const at = (minutesAgo: number) => formatter.format(new Date(Date.now() - minutesAgo * 60_000));
    return [
      { time: at(0), icon: <Users />, title: t("dashboard.activity.session"), detail: t("dashboard.activity.sessionText") },
      { time: at(2), icon: <Database />, title: t("dashboard.activity.readPath"), detail: t("dashboard.activity.readPathText") },
      { time: at(4), icon: <ShieldCheck />, title: t("dashboard.activity.preview"), detail: t("dashboard.activity.previewText") },
      { time: at(6), icon: <Activity />, title: t("dashboard.activity.runtime"), detail: t("dashboard.activity.runtimeText") }
    ];
  }, [lang, t]);

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
            <h2>{t("dashboard.attentionTitle")}<b className="attention-count">3</b></h2>
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
          <AttentionRow
            icon={<Snowflake />}
            title={t("dashboard.queue.freezeTitle")}
            text={t("dashboard.queue.freezeText")}
            severity={t("dashboard.severity.high")}
            status={t("dashboard.queue.dataPending")}
            owner={t("dashboard.queue.riskOwner")}
            href="/accounts"
            navigate={navigate}
            tone="high"
          />
          <AttentionRow
            icon={<MessageSquareText />}
            title={t("dashboard.queue.pushTitle")}
            text={t("dashboard.queue.pushText")}
            severity={t("dashboard.severity.medium")}
            status={t("dashboard.queue.dataPending")}
            owner={t("dashboard.queue.messageOwner")}
            href="/messages"
            navigate={navigate}
            tone="medium"
          />
          <AttentionRow
            icon={<HardDrive />}
            title={t("dashboard.queue.mediaTitle")}
            text={t("dashboard.queue.mediaText")}
            severity={t("dashboard.severity.medium")}
            status={t("dashboard.queue.dataPending")}
            owner={t("dashboard.queue.mediaOwner")}
            href="/gifts"
            navigate={navigate}
            tone="medium"
          />
        </div>
      </section>

      <div className="dashboard-lower-grid">
        <section className="dashboard-panel activity-panel">
          <div className="dashboard-panel-head compact">
            <div>
              <h2>{t("dashboard.activityTitle")}</h2>
              <p>{t("dashboard.activityBody")}</p>
            </div>
            <span className="panel-meta"><Clock3 size={14} />{t("dashboard.realtime")}</span>
          </div>
          <div className="activity-list">
            {activity.map((item) => (
              <div className="activity-row" key={item.title}>
                <time>{item.time}</time>
                <span className="activity-node" />
                <span className="activity-icon">{item.icon}</span>
                <div><strong>{item.title}</strong><small>{item.detail}</small></div>
                <span className="activity-success"><Check size={12} />{t("dashboard.success")}</span>
              </div>
            ))}
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
          <div className="trend-legend" aria-hidden="true">
            <span><i className="cyan" />{t("dashboard.trend.messages")}</span>
            <span><i className="blue" />{t("dashboard.trend.api")}</span>
            <span><i className="gray" />{t("dashboard.trend.push")}</span>
          </div>
          <div className="trend-empty">
            <LineChart size={28} />
            <strong>{t("dashboard.trend.pendingTitle")}</strong>
            <span>{t("dashboard.trend.pendingText")}</span>
          </div>
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

function AttentionRow({
  icon,
  title,
  text,
  severity,
  status,
  owner,
  href,
  navigate,
  tone
}: {
  icon: ReactNode;
  title: string;
  text: string;
  severity: string;
  status: string;
  owner: string;
  href: string;
  navigate: Navigate;
  tone: "high" | "medium";
}) {
  const { t } = useI18n();
  return (
    <div className="attention-row" role="row">
      <div className="attention-item">
        <span className={`attention-icon ${tone}`}>{icon}</span>
        <div><strong>{title}</strong><small>{text}</small></div>
      </div>
      <span><b className={`severity-badge ${tone}`}>{severity}</b></span>
      <span className="attention-number">—</span>
      <div className="attention-state"><strong>{status}</strong><small>{owner}</small></div>
      <span className="attention-time">—</span>
      <AppLink className="queue-action" href={href} navigate={navigate}>{t("dashboard.view")}</AppLink>
    </div>
  );
}
