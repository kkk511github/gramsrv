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
import { useMemo, type ReactNode } from "react";
import { AppLink } from "../components/AppLink";
import { useI18n } from "../i18n";
import type { Navigate } from "../routing";

type Tone = "good" | "pending" | "high" | "medium";

export function Dashboard({ navigate }: { navigate: Navigate }) {
  const { t, lang } = useI18n();
  const services = [
    {
      icon: <Radio />,
      label: "MTProto",
      value: t("dashboard.service.telemetryPending"),
      detail: t("dashboard.service.metricsNotConnected"),
      tone: "pending" as Tone
    },
    {
      icon: <Server />,
      label: "Admin API",
      value: t("dashboard.service.connected"),
      detail: t("dashboard.service.currentSession"),
      tone: "good" as Tone
    },
    {
      icon: <Database />,
      label: "PostgreSQL",
      value: t("dashboard.service.readAvailable"),
      detail: t("dashboard.service.readPathReady"),
      tone: "good" as Tone
    },
    {
      icon: <BellRing />,
      label: t("dashboard.service.push"),
      value: t("dashboard.service.telemetryPending"),
      detail: t("dashboard.service.metricsNotConnected"),
      tone: "pending" as Tone
    },
    {
      icon: <HardDrive />,
      label: t("dashboard.service.media"),
      value: t("dashboard.service.telemetryPending"),
      detail: t("dashboard.service.metricsNotConnected"),
      tone: "pending" as Tone
    }
  ];
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
      <section className="health-band">
        <div className="health-summary">
          <span className="health-icon"><CircleCheckBig size={31} /></span>
          <div>
            <h2>{t("dashboard.healthTitle")}</h2>
            <p>{t("dashboard.healthBody")}</p>
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
