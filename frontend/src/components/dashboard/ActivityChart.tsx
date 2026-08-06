import type { DashboardTelemetry } from '../../types/system'
import { Icon } from '../common/Icon'

const chartWidth = 598
const chartHeight = 112
const chartBaseline = 106

export function ActivityChart({ telemetry }: { telemetry: DashboardTelemetry }) {
	const live = Date.now() - new Date(telemetry.generatedAt).getTime() < 10_000
  const buckets = telemetry.activity.length ? telemetry.activity : [{
    startedAt: telemetry.generatedAt, launches: 0, uniqueProfiles: 0,
    profilesCreated: 0, proxyTests: 0, failures: 0,
  }]
  const maxValue = Math.max(1, ...buckets.flatMap((bucket) => [bucket.launches, bucket.uniqueProfiles]))
  const step = buckets.length > 1 ? chartWidth / (buckets.length - 1) : chartWidth
  const points = buckets.map((bucket, index) => {
    const x = buckets.length > 1 ? index * step : chartWidth
    const y = chartBaseline - (bucket.launches / maxValue) * 94
    return `${x.toFixed(1)},${y.toFixed(1)}`
  }).join(' ')
  const marker = points.split(' ').at(-1)?.split(',') ?? ['598', '106']
  const timeIndexes = [...new Set([0, Math.floor((buckets.length - 1) * .2), Math.floor((buckets.length - 1) * .4), Math.floor((buckets.length - 1) * .6), Math.floor((buckets.length - 1) * .8), buckets.length - 1])]
  const timeLabel = (iso: string, last: boolean) => last
    ? 'AGORA'
    : new Intl.DateTimeFormat('pt-BR', { hour: '2-digit', minute: '2-digit' }).format(new Date(iso))

  return (
    <section className="operations-strip">
      <article className="activity-panel">
        <header className="panel-heading">
          <div><span className="eyebrow">TELEMETRIA LOCAL</span><h2>Atividade registrada</h2></div>
          <div className="chart-legend">
            <span><i className="chart-legend__line" /> Aberturas</span>
            <span><i className="chart-legend__bar" /> Perfis únicos</span>
            <button type="button">24H <Icon name="chevronDown" size={13} /></button>
          </div>
        </header>
        <div className="chart-wrap">
          <div className="chart-scale"><span>{maxValue}</span><span>{Math.round(maxValue * .66)}</span><span>{Math.round(maxValue * .33)}</span><span>0</span></div>
          <svg aria-label="Aberturas reais registradas nas últimas 24 horas" role="img" viewBox="0 0 598 112">
            <defs><linearGradient id="activityFill" x1="0" x2="0" y1="0" y2="1"><stop offset="0" stopColor="#42ff91" stopOpacity=".2" /><stop offset="1" stopColor="#42ff91" stopOpacity="0" /></linearGradient></defs>
            <g className="chart-grid"><path d="M0 8h598M0 38h598M0 68h598M0 98h598" /></g>
            <g className="chart-bars">{buckets.map((bucket, index) => {
              const height = (bucket.uniqueProfiles / maxValue) * 94
              const x = buckets.length > 1 ? index * step : chartWidth - 15
              return <rect height={height} key={bucket.startedAt} width="15" x={Math.max(0, x - 7)} y={chartBaseline - height} />
            })}</g>
            <path d={`M${points} L598,112 L0,112 Z`} fill="url(#activityFill)" />
            <polyline className="chart-line" fill="none" points={points} />
            <circle className="chart-marker" cx={marker[0]} cy={marker[1]} r="4" />
          </svg>
          <div className="chart-times">{timeIndexes.map((index) => <span key={buckets[index].startedAt}>{timeLabel(buckets[index].startedAt, index === buckets.length - 1)}</span>)}</div>
        </div>
      </article>

      <article className="signal-panel">
        <header className="panel-heading">
          <div><span className="eyebrow">SINAIS REAIS</span><h2>Saúde operacional</h2></div>
          <span className="live-chip"><i /> {live ? 'LIVE' : 'SEM SINAL'}</span>
        </header>
        <div className="signal-score">
          <div className="signal-score__ring"><strong>{telemetry.signals.overall}</strong><span>/100</span></div>
          <div><b>{telemetry.signals.label}</b><span>{telemetry.signals.detail}</span></div>
        </div>
        <div className="signal-list">
          <Signal icon="shield" label="Fingerprints válidos" value={telemetry.signals.fingerprint} />
          <Signal icon="globe" label="Rede & proxy" value={telemetry.signals.network} />
          <Signal icon="zap" label="Aberturas sem falha" value={telemetry.signals.sessions} />
        </div>
      </article>
    </section>
  )
}

function Signal({ icon, label, value }: { icon: 'shield' | 'globe' | 'zap'; label: string; value: number }) {
  return <div><span><Icon name={icon} size={15} /> {label}</span><b>{value}%</b><i><em style={{ width: `${value}%` }} /></i></div>
}
