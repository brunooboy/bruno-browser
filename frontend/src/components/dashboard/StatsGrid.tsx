import type { DashboardTelemetry } from '../../types/system'
import { Icon, type IconName } from '../common/Icon'

interface Stat { label: string; value: string; delta: string; tone: 'green' | 'blue' | 'amber' | 'red'; icon: IconName; sparkline: number[] }

function Sparkline({ values }: { values: number[] }) {
  const safeValues = values.length > 1 ? values : [0, ...(values.length ? values : [0])]
  const width = 92, height = 30, min = Math.min(...safeValues), max = Math.max(...safeValues), range = Math.max(1, max - min)
  const points = safeValues.map((value, index) => `${(index / (safeValues.length - 1)) * width},${height - ((value - min) / range) * (height - 4) - 2}`).join(' ')
  return <svg aria-hidden="true" className="sparkline" viewBox={`0 0 ${width} ${height}`}><polyline fill="none" points={points} vectorEffect="non-scaling-stroke" /></svg>
}

export function StatsGrid({ telemetry }: { telemetry: DashboardTelemetry }) {
  const { summary, activity } = telemetry
  const generatedDate = new Date(telemetry.generatedAt)
  const hasMeasurement = Number.isFinite(generatedDate.getTime()) && generatedDate.getTime() > 0
  const generatedTime = hasMeasurement
    ? new Intl.DateTimeFormat('pt-BR', { hour: '2-digit', minute: '2-digit' }).format(generatedDate)
    : ''
  const proxyValue = summary.configuredProxies ? `${summary.proxyHealthPercent}%` : '--'
  const proxyDetail = summary.configuredProxies
    ? summary.medianProxyLatencyMs > 0 ? `mediana real ${summary.medianProxyLatencyMs} ms` : `${summary.healthyProxies}/${summary.configuredProxies} testados saudáveis`
    : 'nenhum proxy configurado'
  const stats: Stat[] = [
    { label: 'Perfis totais', value: summary.totalProfiles.toString().padStart(2, '0'), delta: `${summary.newProfilesThisMonth} criados neste mês`, tone: 'green', icon: 'layers', sparkline: activity.map((item) => item.profilesCreated) },
    { label: 'Em execução', value: summary.runningProfiles.toString().padStart(2, '0'), delta: `${summary.successfulLaunches24h} aberturas nas últimas 24h`, tone: 'blue', icon: 'activity', sparkline: activity.map((item) => item.launches) },
    { label: 'Proxies saudáveis', value: proxyValue, delta: proxyDetail, tone: 'amber', icon: 'globe', sparkline: activity.map((item) => item.proxyTests) },
    { label: 'Atenção exigida', value: summary.attentionProfiles.toString().padStart(2, '0'), delta: hasMeasurement ? `calculado às ${generatedTime}` : 'aguardando núcleo local', tone: 'red', icon: 'alert', sparkline: activity.map((item) => item.failures) },
  ]
  return <section aria-label="Resumo operacional real" className="stats-grid">{stats.map((stat) => <article className={`stat-card stat-card--${stat.tone}`} key={stat.label}>
    <div className="stat-card__top"><span className="stat-card__icon"><Icon name={stat.icon} /></span><span className="stat-card__label">{stat.label}</span><Icon className="stat-card__trend" name="trend" size={15} /></div>
    <div className="stat-card__body"><div><strong>{stat.value}</strong><span>{stat.delta}</span></div><Sparkline values={stat.sparkline} /></div>
  </article>)}</section>
}
