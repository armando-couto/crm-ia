<script setup lang="ts">
// Assistente de configuração inicial: em cinco passos o ambiente sai do zero
// para um CRM pronto para usar — modelo do negócio, identidade da empresa,
// e-mail, regras de disparo e equipe. Cada passo salva sozinho; dá para sair
// e voltar depois.
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import { useAuthStore } from '../stores/auth'
import { useToastStore } from '../stores/toast'
import type { CRMTemplate, EmailPreset, EmailProvider, SendingSettings, SetupStatus, WorkspaceSettings } from '../types'

const router = useRouter()
const auth = useAuthStore()
const toast = useToastStore()

const passos = [
  { id: 'template', titulo: 'Modelo do CRM', desc: 'Funil, e-mails e campos prontos para o seu negócio' },
  { id: 'empresa', titulo: 'Sua empresa', desc: 'Nome, cor e logo que a equipe vai ver' },
  { id: 'email', titulo: 'Envio de e-mails', desc: 'Mandrill, Maileroo ou outro SMTP' },
  { id: 'disparo', titulo: 'Regras de disparo', desc: 'Horários e limites das sequências' },
  { id: 'equipe', titulo: 'Equipe', desc: 'Convide quem vai usar o CRM' }
]
const atual = ref(0)
const status = ref<SetupStatus | null>(null)
const carregando = ref(true)
const salvando = ref(false)

// Passo 1 — modelo
const templates = ref<CRMTemplate[]>([])
const escolhido = ref('')
const resultadoTemplate = ref<Record<string, number> | null>(null)
const detalhe = ref<any>(null)

// Passo 2 — empresa
const empresa = ref<WorkspaceSettings>({ name: '', segment: '', color: '#6d5df6', logo_url: '', website: '', timezone: 'America/Sao_Paulo' })

// Passo 3 — e-mail
const presets = ref<Record<string, EmailPreset>>({})
const email = ref({ provider: 'mandrill_api' as EmailProvider, from_email: '', from_name: '', reply_to: '', smtp_host: '', smtp_port: 587, smtp_user: '', secret: '' })
const emailAtual = ref<SetupStatus['email']>(null)
const testando = ref(false)
const resultadoTeste = ref('')

// Passo 4 — disparo
const disparo = ref<SendingSettings>({ daily_limit: 500, window_start: 8, window_end: 19, weekdays_only: true, paused: false, signature: '', track_opens: true, track_clicks: true, timezone: 'America/Sao_Paulo' })

// Passo 5 — equipe
const convite = ref({ name: '', email: '', role: 'seller' })
const convidados = ref<string[]>([])

const feito = computed(() => new Set(status.value?.steps ?? []))
const precisaSegredo = computed(() => email.value.provider !== 'plataforma' && !emailAtual.value?.has_secret)
const dicaProvedor = computed(() => presets.value[email.value.provider]?.dica ?? '')
const smtpFixo = computed(() => !!presets.value[email.value.provider]?.host)

async function carregar() {
  carregando.value = true
  try {
    const [st, tpls, em] = await Promise.all([
      api.get<SetupStatus>('/setup'),
      api.get<CRMTemplate[]>('/setup/templates'),
      api.get<{ settings: SetupStatus['email']; presets: Record<string, EmailPreset> }>('/settings/email')
    ])
    status.value = st
    templates.value = tpls
    presets.value = em.presets
    emailAtual.value = em.settings
    escolhido.value = st.template || ''
    empresa.value = { ...empresa.value, ...st.workspace }
    disparo.value = { ...disparo.value, ...st.sending }
    if (em.settings) {
      email.value = { ...email.value, ...em.settings, smtp_port: em.settings.smtp_port || 587, secret: '' }
    } else {
      email.value.from_name = st.workspace.name
    }
    // Retoma do primeiro passo pendente.
    const idx = passos.findIndex((p) => !st.steps.includes(p.id))
    atual.value = idx === -1 ? passos.length - 1 : idx
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    carregando.value = false
  }
}

async function verDetalhe(codigo: string) {
  escolhido.value = codigo
  try {
    detalhe.value = await api.get(`/setup/templates/${codigo}`)
  } catch {
    detalhe.value = null
  }
}

async function aplicarTemplate() {
  if (!escolhido.value) {
    toast.error('escolha um modelo para continuar')
    return
  }
  salvando.value = true
  try {
    resultadoTemplate.value = await api.post<Record<string, number>>('/setup/template', { template: escolhido.value })
    toast.push('Modelo aplicado ao seu CRM')
    await avancar('template')
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    salvando.value = false
  }
}

async function salvarEmpresa() {
  salvando.value = true
  try {
    await api.put('/settings/workspace', empresa.value)
    await auth.fetchWorkspace()
    toast.push('Identidade da empresa salva')
    await avancar('empresa')
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    salvando.value = false
  }
}

async function salvarEmail() {
  salvando.value = true
  try {
    const resp = await api.put<{ settings: SetupStatus['email']; description: string }>('/settings/email', email.value)
    emailAtual.value = resp.settings
    email.value.secret = ''
    toast.push(`E-mail configurado: ${resp.description}`)
    await avancar('email')
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    salvando.value = false
  }
}

async function testarEmail() {
  testando.value = true
  resultadoTeste.value = ''
  try {
    const r = await api.post<{ to: string }>('/settings/email/test', {})
    resultadoTeste.value = `Enviado para ${r.to}. Confira a caixa de entrada.`
  } catch (e: any) {
    resultadoTeste.value = e.message
  } finally {
    testando.value = false
  }
}

async function pularEmail() {
  await avancar('email')
}

async function salvarDisparo() {
  salvando.value = true
  try {
    await api.put('/settings/sending', disparo.value)
    toast.push('Regras de disparo salvas')
    await avancar('disparo')
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    salvando.value = false
  }
}

async function convidar() {
  if (!convite.value.name || !convite.value.email) return
  salvando.value = true
  try {
    await api.post('/users', convite.value)
    convidados.value.push(convite.value.email)
    convite.value = { name: '', email: '', role: 'seller' }
    toast.push('Convite enviado')
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    salvando.value = false
  }
}

async function avancar(passo: string) {
  status.value = await api.post<SetupStatus>('/setup/step', { step: passo })
  if (atual.value < passos.length - 1) atual.value++
}

async function concluir() {
  salvando.value = true
  try {
    await api.post('/setup/step', { step: 'equipe' })
    await api.post('/setup/complete', {})
    await auth.fetchWorkspace()
    toast.push('Tudo pronto! Seu CRM está configurado.')
    router.push({ name: 'dashboard' })
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    salvando.value = false
  }
}

onMounted(carregar)
</script>

<template>
  <div class="setup">
    <aside class="setup-steps">
      <h1>Vamos configurar o seu CRM</h1>
      <p class="muted">Leva uns 5 minutos. Tudo pode ser mudado depois em Configurações.</p>
      <ol>
        <li v-for="(p, i) in passos" :key="p.id" :class="{ active: i === atual, done: feito.has(p.id) }" @click="atual = i">
          <span class="num">{{ feito.has(p.id) ? '✓' : i + 1 }}</span>
          <span><strong>{{ p.titulo }}</strong><small>{{ p.desc }}</small></span>
        </li>
      </ol>
      <button v-if="status?.completed" class="btn btn-outline btn-sm" type="button" @click="router.push({ name: 'dashboard' })">Voltar ao CRM</button>
    </aside>

    <section class="setup-content">
      <div v-if="carregando" class="muted">Carregando…</div>

      <!-- Passo 1: modelo -->
      <template v-else-if="atual === 0">
        <h2>Que tipo de negócio é o seu?</h2>
        <p class="muted">Escolha o modelo mais parecido. Ele cria funil, etapas, modelos de e-mail, uma cadência de prospecção e campos próprios. Nada é definitivo.</p>
        <div class="templates">
          <button
            v-for="t in templates"
            :key="t.codigo"
            type="button"
            class="template"
            :class="{ selected: escolhido === t.codigo }"
            :data-template="t.codigo"
            @click="verDetalhe(t.codigo)"
          >
            <strong>{{ t.nome }}</strong>
            <p>{{ t.descricao }}</p>
            <ul>
              <li v-for="d in t.destaques" :key="d">{{ d }}</li>
            </ul>
            <small>{{ t.pipelines }} funil(is) · {{ t.etapas }} etapas · {{ t.emails }} e-mails · {{ t.cadencias }} cadência(s) · {{ t.campos }} campos</small>
          </button>
        </div>
        <div v-if="detalhe" class="card detail">
          <h3>O que o modelo cria</h3>
          <div class="detail-grid">
            <div>
              <h4>Funis</h4>
              <p v-for="p in detalhe.pipelines" :key="p.nome"><strong>{{ p.nome }}</strong>: {{ p.etapas.join(' → ') }}</p>
            </div>
            <div>
              <h4>Modelos de e-mail</h4>
              <p>{{ detalhe.emails.join(', ') }}</p>
              <h4 v-if="detalhe.cadencias.length">Cadências</h4>
              <p v-for="c in detalhe.cadencias" :key="c.nome"><strong>{{ c.nome }}</strong> ({{ c.passos }} passos) — {{ c.descricao }}</p>
            </div>
          </div>
        </div>
        <div v-if="resultadoTemplate" class="card ok">
          Criados: {{ resultadoTemplate.pipelines }} funil(is), {{ resultadoTemplate.etapas }} etapas, {{ resultadoTemplate.emails }} modelos,
          {{ resultadoTemplate.cadencias }} cadência(s), {{ resultadoTemplate.propriedades }} campos e {{ resultadoTemplate.snippets }} snippets.
          <span v-if="resultadoTemplate.ignorados">{{ resultadoTemplate.ignorados }} item(ns) já existiam e foram mantidos.</span>
        </div>
        <div class="actions">
          <button class="btn btn-primary" type="button" :disabled="salvando || !escolhido" @click="aplicarTemplate">
            {{ salvando ? 'Aplicando…' : feito.has('template') ? 'Aplicar de novo e continuar' : 'Aplicar modelo e continuar' }}
          </button>
          <button v-if="feito.has('template')" class="btn btn-outline" type="button" @click="atual = 1">Pular</button>
        </div>
      </template>

      <!-- Passo 2: empresa -->
      <template v-else-if="atual === 1">
        <h2>Como a sua equipe vai ver o CRM</h2>
        <form class="card form" @submit.prevent="salvarEmpresa">
          <div class="field"><label>Nome da empresa *</label><input v-model="empresa.name" required maxlength="80" /></div>
          <div class="two">
            <div class="field"><label>Segmento</label><input v-model="empresa.segment" placeholder="ex.: Agência de marketing" /></div>
            <div class="field"><label>Site</label><input v-model="empresa.website" placeholder="https://" /></div>
          </div>
          <div class="two">
            <div class="field"><label>Cor principal</label><div class="color"><input v-model="empresa.color" type="color" /><input v-model="empresa.color" maxlength="7" /></div></div>
            <div class="field"><label>URL da logo (opcional)</label><input v-model="empresa.logo_url" placeholder="https://…/logo.png" /></div>
          </div>
          <div class="preview" :style="{ '--prev': empresa.color }">
            <span class="prev-mark"><img v-if="empresa.logo_url" :src="empresa.logo_url" alt="" /><template v-else>{{ (empresa.name || 'C').charAt(0) }}</template></span>
            <span>{{ empresa.name || 'Sua empresa' }}</span>
          </div>
          <div class="actions">
            <button class="btn btn-primary" type="submit" :disabled="salvando">{{ salvando ? 'Salvando…' : 'Salvar e continuar' }}</button>
            <button class="btn btn-outline" type="button" @click="atual = 0">Voltar</button>
          </div>
        </form>
      </template>

      <!-- Passo 3: e-mail -->
      <template v-else-if="atual === 2">
        <h2>Por onde os e-mails vão sair</h2>
        <p class="muted">Sequências, automações e e-mails manuais usam este provedor. Use um domínio seu, com SPF e DKIM configurados, para cair na caixa de entrada.</p>
        <form class="card form" @submit.prevent="salvarEmail">
          <div class="providers">
            <label v-for="(p, codigo) in presets" :key="codigo" class="provider" :class="{ selected: email.provider === codigo }">
              <input v-model="email.provider" type="radio" :value="codigo" />
              <strong>{{ p.nome }}<small v-if="codigo === 'mandrill_api'"> · API</small><small v-else-if="p.host"> · SMTP</small></strong>
              <small>{{ p.host || (codigo === 'mandrill_api' ? 'envio pela API HTTPS' : 'servidor próprio') }}</small>
            </label>
          </div>
          <p v-if="dicaProvedor" class="hint">{{ dicaProvedor }}</p>
          <div class="two">
            <div class="field"><label>E-mail do remetente *</label><input v-model="email.from_email" type="email" required placeholder="vendas@suaempresa.com.br" /></div>
            <div class="field"><label>Nome do remetente</label><input v-model="email.from_name" :placeholder="empresa.name" /></div>
          </div>
          <div class="two">
            <div class="field"><label>Responder para (opcional)</label><input v-model="email.reply_to" type="email" /></div>
            <div class="field">
              <label>{{ email.provider === 'mandrill_api' ? 'Chave de API' : 'Senha SMTP' }} {{ precisaSegredo ? '*' : '' }}</label>
              <input v-model="email.secret" type="password" autocomplete="off" :required="precisaSegredo" :placeholder="emailAtual?.has_secret ? '•••••• (mantida se vazio)' : ''" />
            </div>
          </div>
          <div v-if="email.provider !== 'mandrill_api'" class="two">
            <div class="field"><label>Servidor SMTP</label><input v-model="email.smtp_host" :disabled="smtpFixo" :placeholder="presets[email.provider]?.host" /></div>
            <div class="two">
              <div class="field"><label>Porta</label><input v-model.number="email.smtp_port" type="number" :disabled="smtpFixo" /></div>
              <div class="field"><label>Usuário SMTP</label><input v-model="email.smtp_user" :placeholder="email.provider === 'mandrill_smtp' ? 'qualquer texto' : ''" /></div>
            </div>
          </div>
          <div class="actions">
            <button class="btn btn-primary" type="submit" :disabled="salvando">{{ salvando ? 'Salvando…' : 'Salvar e continuar' }}</button>
            <button class="btn btn-outline" type="button" :disabled="testando || !emailAtual" @click="testarEmail">{{ testando ? 'Enviando…' : 'Enviar e-mail de teste' }}</button>
            <button class="btn btn-outline" type="button" @click="pularEmail">Configurar depois</button>
          </div>
          <p v-if="resultadoTeste" class="hint">{{ resultadoTeste }}</p>
        </form>
      </template>

      <!-- Passo 4: disparo -->
      <template v-else-if="atual === 3">
        <h2>Quando as sequências podem disparar</h2>
        <p class="muted">Vale para e-mails automáticos (sequências e automações). O envio manual pela tela respeita só o teto diário.</p>
        <form class="card form" @submit.prevent="salvarDisparo">
          <div class="two">
            <div class="field"><label>Teto diário de e-mails (0 = sem limite)</label><input v-model.number="disparo.daily_limit" type="number" min="0" /></div>
            <div class="field"><label>Fuso horário</label><input v-model="disparo.timezone" /></div>
          </div>
          <div class="two">
            <div class="field"><label>Início da janela (hora)</label><input v-model.number="disparo.window_start" type="number" min="0" max="23" /></div>
            <div class="field"><label>Fim da janela (hora)</label><input v-model.number="disparo.window_end" type="number" min="0" max="23" /></div>
          </div>
          <label class="check"><input v-model="disparo.weekdays_only" type="checkbox" /> Só em dias úteis</label>
          <label class="check"><input v-model="disparo.track_opens" type="checkbox" /> Rastrear aberturas</label>
          <label class="check"><input v-model="disparo.track_clicks" type="checkbox" /> Rastrear cliques</label>
          <div class="field"><label>Assinatura padrão (opcional)</label><textarea v-model="disparo.signature" rows="3" placeholder="Nome · Cargo · Telefone"></textarea></div>
          <div class="actions">
            <button class="btn btn-primary" type="submit" :disabled="salvando">{{ salvando ? 'Salvando…' : 'Salvar e continuar' }}</button>
            <button class="btn btn-outline" type="button" @click="atual = 2">Voltar</button>
          </div>
        </form>
      </template>

      <!-- Passo 5: equipe -->
      <template v-else>
        <h2>Quem mais vai usar o CRM?</h2>
        <p class="muted">
          Cada pessoa recebe um e-mail com a senha temporária.
          <span v-if="status?.users_max">Seu plano permite até {{ status.users_max }} usuários ativos ({{ status.users_active }} em uso).</span>
        </p>
        <form class="card form" @submit.prevent="convidar">
          <div class="two">
            <div class="field"><label>Nome</label><input v-model="convite.name" /></div>
            <div class="field"><label>E-mail</label><input v-model="convite.email" type="email" /></div>
          </div>
          <div class="field">
            <label>Perfil</label>
            <select v-model="convite.role">
              <option value="seller">Vendedor</option>
              <option value="manager">Gestor</option>
              <option value="admin">Administrador</option>
            </select>
          </div>
          <button class="btn btn-outline" type="submit" :disabled="salvando || !convite.email">Enviar convite</button>
          <ul v-if="convidados.length" class="invited"><li v-for="c in convidados" :key="c">✓ {{ c }}</li></ul>
        </form>
        <div class="actions">
          <button class="btn btn-primary" type="button" :disabled="salvando" @click="concluir">{{ salvando ? 'Concluindo…' : 'Concluir e abrir o CRM' }}</button>
          <button class="btn btn-outline" type="button" @click="atual = 3">Voltar</button>
        </div>
      </template>
    </section>
  </div>
</template>

<style scoped>
.setup { display: grid; grid-template-columns: 300px minmax(0, 1fr); gap: 28px; padding: 28px; max-width: 1200px; margin: 0 auto; }
@media (max-width: 900px) { .setup { grid-template-columns: 1fr; } }
.setup-steps h1 { font-size: 22px; margin-bottom: 6px; }
.setup-steps ol { list-style: none; padding: 0; margin: 20px 0; }
.setup-steps li { display: flex; gap: 12px; padding: 12px; border-radius: 10px; cursor: pointer; color: var(--ci-text-2); }
.setup-steps li.active { background: var(--ci-purple-tint); color: var(--ci-text); }
.setup-steps li.done .num { background: var(--ci-green); }
.setup-steps li small { display: block; font-size: 12px; color: var(--ci-text-3); }
.num { width: 28px; height: 28px; border-radius: 50%; background: var(--ci-purple); color: #fff; display: flex; align-items: center; justify-content: center; font-size: 13px; font-weight: 600; flex-shrink: 0; }
.setup-content h2 { font-size: 20px; margin-bottom: 6px; }
.setup-content > .muted { margin-bottom: 18px; }
.templates { display: grid; grid-template-columns: repeat(auto-fill, minmax(260px, 1fr)); gap: 14px; margin-bottom: 16px; }
.template { text-align: left; background: var(--ci-surface); border: 2px solid var(--ci-border); border-radius: 12px; padding: 16px; cursor: pointer; font: inherit; color: inherit; }
.template.selected { border-color: var(--ci-purple); box-shadow: 0 0 0 3px rgba(var(--ci-purple-rgb), 0.15); }
.template p { margin: 6px 0 10px; color: var(--ci-text-2); font-size: 13px; }
.template ul { margin: 0 0 10px; padding-left: 18px; font-size: 13px; }
.template small { color: var(--ci-text-3); }
.detail h3 { margin-bottom: 10px; }
.detail h4 { font-size: 12px; text-transform: uppercase; letter-spacing: 0.05em; color: var(--ci-text-3); margin: 10px 0 4px; }
.detail-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 20px; }
@media (max-width: 700px) { .detail-grid { grid-template-columns: 1fr; } }
.card.ok { background: var(--ci-green-tint); border-color: var(--ci-green); margin-top: 12px; }
.form { display: flex; flex-direction: column; gap: 4px; }
.two { display: grid; grid-template-columns: 1fr 1fr; gap: 14px; }
@media (max-width: 700px) { .two { grid-template-columns: 1fr; } }
.color { display: flex; gap: 8px; }
.color input[type='color'] { width: 52px; padding: 2px; }
.preview { display: flex; align-items: center; gap: 10px; padding: 12px 14px; border-radius: 10px; background: var(--ci-purple-deep); color: #fff; font-weight: 600; margin: 8px 0 14px; }
.prev-mark { width: 34px; height: 34px; border-radius: 9px; background: var(--prev); display: flex; align-items: center; justify-content: center; overflow: hidden; }
.prev-mark img { width: 100%; height: 100%; object-fit: contain; background: #fff; }
.providers { display: grid; grid-template-columns: repeat(auto-fill, minmax(150px, 1fr)); gap: 10px; margin-bottom: 12px; }
.provider { border: 2px solid var(--ci-border); border-radius: 10px; padding: 12px; cursor: pointer; display: flex; flex-direction: column; gap: 2px; }
.provider.selected { border-color: var(--ci-purple); background: var(--ci-purple-tint); }
.provider input { display: none; }
.provider small { color: var(--ci-text-3); font-size: 12px; }
.hint { font-size: 13px; color: var(--ci-text-2); background: var(--ci-blue-tint); padding: 10px 12px; border-radius: 8px; margin: 0 0 12px; }
.check { display: flex; gap: 8px; align-items: center; margin: 4px 0; font-size: 14px; }
.actions { display: flex; gap: 10px; margin-top: 14px; flex-wrap: wrap; }
.invited { list-style: none; padding: 0; margin: 12px 0 0; color: var(--ci-green); font-size: 14px; }
</style>
