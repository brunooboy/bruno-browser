import { useEffect, useMemo, useState } from 'react'
import { activePlan, onboardingProgress } from '../../lib/onboarding'
import type { DiscordAccount, PlanStatus, SystemDiagnostics } from '../../types/system'
import { Icon, type IconName } from '../common/Icon'
import { Modal } from '../common/Modal'

interface OnboardingModalProps {
  account?: DiscordAccount
  busy: boolean
  diagnostics: SystemDiagnostics
  desktopMode: boolean
  oauthConfigured: boolean
  onActivate: (key: string) => Promise<void>
  onClose: () => void
  onCreateProfile: () => void
  onLogin: () => Promise<void>
  onRunDiagnostics: () => Promise<void>
  open: boolean
  plan: PlanStatus
  profileCount: number
}

const steps: Array<{ id: 'system' | 'account' | 'license' | 'profile'; label: string; icon: IconName }> = [
  { id: 'system', label: 'Sistema', icon: 'activity' },
  { id: 'account', label: 'Discord', icon: 'globe' },
  { id: 'license', label: 'Plano', icon: 'shield' },
  { id: 'profile', label: 'Perfil', icon: 'layers' },
]

export function OnboardingModal(props: OnboardingModalProps) {
  const [activeStep, setActiveStep] = useState(0)
  const [key, setKey] = useState('')
  const progress = useMemo(() => onboardingProgress({ account: props.account, diagnostics: props.diagnostics, plan: props.plan, profileCount: props.profileCount }), [props.account, props.diagnostics, props.plan, props.profileCount])

  useEffect(() => {
    if (!props.open) return
    setActiveStep(progress.recommendedIndex)
  }, [progress.recommendedIndex, props.open])

  const renderStep = () => {
    if (progress.complete || activeStep >= steps.length) return <div className="onboarding-complete">
      <span><Icon name="check" size={28} /></span>
      <div><small>CONFIGURAÇÃO CONCLUÍDA</small><h3>Bruno Browser pronto para operar</h3><p>Conta, plano, armazenamento, Bruno Engine e primeiro perfil foram validados neste dispositivo.</p></div>
      <button className="primary-button" onClick={props.onClose} type="button"><Icon name="play" size={15} /> Abrir central de operações</button>
    </div>

    if (activeStep === 0) return <div className="onboarding-stage">
      <div className="onboarding-stage__copy"><span>PASSO 01</span><h3>Validar instalação local</h3><p>Confirme que o armazenamento persistente e o Bruno Engine instalado estão acessíveis.</p></div>
      <div className="onboarding-system-checks">
        {props.diagnostics.checks.filter((check) => ['storage', 'engine', 'updates'].includes(check.id)).map((check) => <article className={`diagnostic-check diagnostic-check--${check.status}`} key={check.id}><span>{check.status === 'pass' ? <Icon name="check" size={14} /> : <Icon name="alert" size={14} />}</span><div><strong>{check.label}</strong><small>{check.detail}</small></div></article>)}
      </div>
      <button className="primary-button" disabled={props.busy || !props.desktopMode} onClick={props.onRunDiagnostics} type="button"><Icon name="activity" size={15} /> Executar diagnóstico real</button>
    </div>

    if (activeStep === 1) return <div className="onboarding-stage">
      <div className="onboarding-stage__copy"><span>PASSO 02</span><h3>Conectar sua conta Discord</h3><p>O primeiro login abre o Discord no navegador. Depois disso, os dados básicos da conta permanecem disponíveis offline.</p></div>
      {props.account ? <div className="onboarding-account"><span>{props.account.username.slice(0, 1).toUpperCase()}</span><div><strong>{props.account.globalName || props.account.username}</strong><small>@{props.account.username}</small></div><i><Icon name="check" size={14} /> CONECTADO</i></div> : <div className="onboarding-callout"><Icon name="globe" /><div><strong>Autorização segura com PKCE</strong><small>O Bruno Browser não armazena seu token do Discord.</small></div></div>}
      {!props.oauthConfigured && <p className="onboarding-error"><Icon name="alert" size={14} /> A configuração pública do Discord não está disponível nesta instalação.</p>}
      <button className="primary-button" disabled={props.busy || !props.desktopMode || !props.oauthConfigured || Boolean(props.account)} onClick={props.onLogin} type="button"><Icon name="globe" size={15} /> {props.account ? 'Discord conectado' : 'Entrar com Discord'}</button>
    </div>

    if (activeStep === 2) return <div className="onboarding-stage">
      <div className="onboarding-stage__copy"><span>PASSO 03</span><h3>Ativar seu plano local</h3><p>Cole a key recebida. A validade será conferida novamente pelo núcleo antes de cada operação premium.</p></div>
      {activePlan(props.plan) ? <div className="onboarding-callout onboarding-callout--success"><Icon name="shield" /><div><strong>Plano {props.plan.plan} ativo</strong><small>{props.plan.expires_at ? `Válido até ${new Date(props.plan.expires_at * 1000).toLocaleString('pt-BR')}` : 'Sem data de expiração'}</small></div></div> : <label className="onboarding-key"><span>KEY DE ATIVAÇÃO</span><textarea onChange={(event) => setKey(event.target.value)} placeholder="Cole a key Base64URL..." rows={4} value={key} /></label>}
      <button className="primary-button" disabled={props.busy || !props.desktopMode || !props.account || activePlan(props.plan) || !key.trim()} onClick={() => props.onActivate(key)} type="button"><Icon name="shield" size={15} /> {activePlan(props.plan) ? 'Plano validado' : 'Ativar e validar key'}</button>
    </div>

    return <div className="onboarding-stage">
      <div className="onboarding-stage__copy"><span>PASSO 04</span><h3>Criar o primeiro perfil</h3><p>Defina nome, plataforma e página inicial. Cookies, sessões e armazenamento serão mantidos na pasta física exclusiva do perfil.</p></div>
      <div className="onboarding-callout onboarding-callout--disk"><Icon name="layers" /><div><strong>{props.profileCount ? `${props.profileCount} perfil(is) no disco` : 'Diretório físico exclusivo'}</strong><small>Sem modo anônimo e sem tmpfs. Os dados persistem entre reinicializações.</small></div></div>
      <button className="primary-button" disabled={props.busy || !props.desktopMode || props.profileCount > 0} onClick={props.onCreateProfile} type="button"><Icon name="plus" size={15} /> {props.profileCount ? 'Primeiro perfil criado' : 'Configurar primeiro perfil'}</button>
    </div>
  }

  return <Modal description="Prepare o aplicativo instalado para o primeiro uso real." onClose={props.onClose} open={props.open} title="Configuração inicial" width="lg">
    <div className="onboarding-layout">
      <aside className="onboarding-steps">
        <div><small>PROGRESSO LOCAL</small><strong>{progress.completedSteps}/4</strong><span><i style={{ width: `${progress.completedSteps * 25}%` }} /></span></div>
        <nav aria-label="Etapas da configuração inicial">
          {steps.map((step, index) => <button aria-current={activeStep === index ? 'step' : undefined} className={`${activeStep === index ? 'active' : ''} ${progress.steps[index].complete ? 'complete' : ''}`} key={step.id} onClick={() => setActiveStep(index)} type="button"><span><Icon name={step.icon} size={15} /></span><div><small>0{index + 1}</small><strong>{step.label}</strong></div>{progress.steps[index].complete && <Icon name="check" size={13} />}</button>)}
        </nav>
        <p><Icon name="shield" size={15} /> Nenhum dado do perfil é enviado para servidores do Bruno Browser.</p>
      </aside>
      <section className="onboarding-content">{!props.desktopMode && <div className="onboarding-preview"><Icon name="alert" size={13} /> PRÉVIA VISUAL — ações reais disponíveis no aplicativo instalado</div>}{renderStep()}<footer><button className="ghost-button" onClick={props.onClose} type="button">Fazer depois</button><span>Você pode reabrir este guia em Configurações.</span></footer></section>
    </div>
  </Modal>
}
