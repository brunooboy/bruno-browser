import { useState } from 'react'
import type { KeyClaims, KeyHistoryEntry, LicensePlan } from '../../types/system'
import { Icon } from '../common/Icon'

interface AdminPageProps {
  history: KeyHistoryEntry[]
  busy: boolean
  onGenerate: (discordId: string, plan: LicensePlan) => Promise<KeyHistoryEntry>
  onInspect: (key: string) => Promise<KeyClaims>
  onCopy: (value: string) => Promise<void>
}

export function AdminPage({ history, busy, onCopy, onGenerate, onInspect }: AdminPageProps) {
  const [discordId, setDiscordId] = useState('')
  const [plan, setPlan] = useState<LicensePlan>('30')
  const [generated, setGenerated] = useState('')
  const [inspectKey, setInspectKey] = useState('')
  const [claims, setClaims] = useState<KeyClaims | null>(null)

  const generate = async () => {
    try {
      const entry = await onGenerate(discordId, plan)
      setGenerated(entry.key)
    } catch { /* The parent displays the backend error as a toast. */ }
  }

  const inspect = async () => {
    try { setClaims(await onInspect(inspectKey)) }
    catch { /* The parent displays the backend error as a toast. */ }
  }

  return (
    <div className="system-page admin-page">
      <div className="admin-grid">
        <section className="system-panel admin-generator">
          <header><div><span>KEY FACTORY</span><h3>Gerar licença local</h3></div><Icon name="shield" /></header>
          <label className="field-control"><span>DISCORD ID <small>OPCIONAL • VAZIO = GENÉRICA</small></span><input inputMode="numeric" onChange={(event) => setDiscordId(event.target.value.replace(/\D/g, ''))} placeholder="123456789012345678" value={discordId} /></label>
          <label className="field-control"><span>PLANO</span><select onChange={(event) => setPlan(event.target.value as typeof plan)} value={plan}><option value="VITALICIO">VITALÍCIO</option><option value="30">30 DIAS</option><option value="7">7 DIAS</option><option value="1">1 DIA</option></select></label>
          <button className="primary-button" disabled={busy} onClick={generate} type="button"><Icon name="plus" size={16} /> Gerar key</button>
          {generated && <div className="generated-key"><span>KEY GERADA</span><textarea readOnly rows={5} value={generated} /><button className="ghost-button" onClick={() => onCopy(generated)} type="button"><Icon name="check" size={14} /> Copiar</button></div>}
        </section>

        <section className="system-panel key-inspector">
          <header><div><span>DECRYPT INSPECTOR</span><h3>Verificar qualquer key</h3></div><Icon name="search" /></header>
          <textarea onChange={(event) => setInspectKey(event.target.value)} placeholder="Cole uma key para descriptografar..." rows={5} value={inspectKey} />
          <button className="ghost-button" disabled={busy || !inspectKey.trim()} onClick={inspect} type="button"><Icon name="search" size={15} /> Verificar dados</button>
          {claims && <dl className="claims-grid"><div><dt>KEY ID</dt><dd>{claims.key_id}</dd></div><div><dt>DISCORD ID</dt><dd>{claims.discord_id || 'GENÉRICA'}</dd></div><div><dt>PLANO</dt><dd>{claims.plan}</dd></div><div><dt>EXPIRA</dt><dd>{claims.expires_at ? new Date(claims.expires_at * 1000).toLocaleString('pt-BR') : 'NUNCA'}</dd></div></dl>}
        </section>
      </div>

      <section className="system-panel key-history">
        <header><div><span>HISTÓRICO NESTE DISPOSITIVO</span><h3>Keys geradas localmente</h3></div><em>{history.length} REGISTROS</em></header>
        {history.length === 0 ? <div className="system-empty"><Icon name="clock" /><b>Histórico vazio</b></div> : <div className="key-history__table">
          {history.map((entry) => <article key={`${entry.claims.key_id}-${entry.generated_at}`}><span><b>{entry.claims.key_id}</b><small>{entry.claims.discord_id || 'KEY GENÉRICA'}</small></span><em>{entry.claims.plan}</em><time>{new Date(entry.generated_at * 1000).toLocaleString('pt-BR')}</time><button aria-label={`Copiar key ${entry.claims.key_id}`} onClick={() => onCopy(entry.key)} type="button"><Icon name="check" size={14} /></button></article>)}
        </div>}
      </section>
    </div>
  )
}
