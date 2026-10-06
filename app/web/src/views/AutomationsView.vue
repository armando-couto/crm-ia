<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import { useAuthStore } from '../stores/auth'
import { useToastStore } from '../stores/toast'
import { relativeDate } from '../format'
import ModalDialog from '../components/ModalDialog.vue'
import type { Automation, AutomationAction, ContactList, MessageTemplate, Pipeline, User } from '../types'

const auth = useAuthStore()
const toast = useToastStore()

const automations = ref<Automation[]>([])
const triggers = ref<Record<string, string>>({})
const actionLabels = ref<Record<string, string>>({})
const timeTriggers = ref<string[]>([])
const pending = ref<Record<number, number>>({})

const users = ref<User[]>([])
const lists = ref<ContactList[]>([])
const templates = ref<MessageTemplate[]>([])
const pipelines = ref<Pipeline[]>([])

const loading = ref(true)
const saving = ref(false)
const editing = ref<Automation | null>(null)
const history = ref<{ automation: Automation; runs: any[] } | null>(null)

const canManage = computed(() => auth.can('automations.manage'))

const stages = computed(() =>
  pipelines.value.flatMap((p) => p.stages.map((s) => ({ id: s.id, name: `${p.name} · ${s.name}` })))
)

const lifecycleStages = ['lead', 'mql', 'sql', 'oportunidade', 'cliente', 'perdido']

/** Gatilhos que aceitam configuração extra na tela. */
function triggerNeedsStage(kind: string) {
  return kind === 'negocio_etapa'
}
function triggerNeedsLifecycle(kind: string) {
  return kind === 'contato_estagio'
}
function triggerNeedsDays(kind: string) {
  return timeTriggers.value.includes(kind)
}

// A configuração de cada ação é um JSON livre; a tela edita por campo.
function cfg(action: AutomationAction): Record<string, any> {
  if (!action.config) action.config = {}
  return action.config as Record<string, any>
}

function triggerCfg(): Record<string, any> {
  if (!editing.value) return {}
  if (!editing.value.trigger_config) editing.value.trigger_config = {}
  return editing.value.trigger_config as Record<string, any>
}

async function load() {
  loading.value = true
  try {
    const [resp, userList, listResp, tplList, pipeResp] = await Promise.all([
      api.get<{
        data: Automation[]
        triggers: Record<string, string>
        actions: Record<string, string>
        time_triggers: string[]
        pending: Record<number, number>
      }>('/automations'),
      api.get<User[]>('/users'),
      api.get<{ data: ContactList[] }>('/lists'),
      api.get<MessageTemplate[]>('/templates'),
      api.get<{ data: Pipeline[] }>('/pipelines')
    ])
    automations.value = resp.data ?? []
    triggers.value = resp.triggers ?? {}
    actionLabels.value = resp.actions ?? {}
    timeTriggers.value = resp.time_triggers ?? []
    pending.value = resp.pending ?? {}
    users.value = userList
    lists.value = listResp.data ?? []
    templates.value = tplList
    pipelines.value = pipeResp.data ?? []
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

function novo() {
  editing.value = {
    id: 0,
    name: '',
    description: '',
    trigger_kind: 'contato_criado',
    trigger_config: {},
    actions: [{ kind: 'enviar_email', config: {} }],
    active: true,
    runs: 0,
    last_run_at: null,
    created_by: null,
    created_at: '',
    updated_at: ''
  }
}

function edit(a: Automation) {
  editing.value = JSON.parse(JSON.stringify(a))
}

function addAction(kind = 'criar_tarefa') {
  editing.value?.actions.push({ kind, config: {} })
}

function removeAction(index: number) {
  editing.value?.actions.splice(index, 1)
}

function moveAction(index: number, delta: number) {
  const list = editing.value?.actions
  if (!list) return
  const target = index + delta
  if (target < 0 || target >= list.length) return
  const [item] = list.splice(index, 1)
  list.splice(target, 0, item)
}

async function save() {
  if (!editing.value) return
  saving.value = true
  try {
    if (editing.value.id) {
      await api.put(`/automations/${editing.value.id}`, editing.value)
    } else {
      await api.post('/automations', editing.value)
    }
    toast.push('Automação salva')
    editing.value = null
    await load()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

async function toggleActive(a: Automation) {
  try {
    await api.put(`/automations/${a.id}`, { ...a, active: !a.active })
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function remove(a: Automation) {
  if (!confirm(`Excluir a automação "${a.name}"? As sequências em andamento são canceladas.`)) return
  try {
    await api.delete(`/automations/${a.id}`)
    toast.push('Automação removida')
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function openHistory(a: Automation) {
  try {
    const resp = await api.get<{ data: any[] }>(`/automations/${a.id}/runs`)
    history.value = { automation: a, runs: resp.data ?? [] }
  } catch (e: any) {
    toast.error(e.message)
  }
}

/** Resumo em uma linha das ações, para a listagem. */
function summarize(a: Automation): string {
  return a.actions.map((x) => actionLabels.value[x.kind] ?? x.kind).join(' → ')
}

onMounted(load)
</script>

<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1>Automações</h1>
        <p class="muted" style="margin: 4px 0 0">
          Um gatilho dispara uma sequência de ações. Com a espera entre passos, viram sequências de e-mail.
        </p>
      </div>
      <button v-if="canManage" class="btn btn-primary" type="button" @click="novo">Nova automação</button>
    </div>

    <p v-if="loading" class="muted">Carregando automações…</p>

    <div v-else-if="!automations.length" class="card empty">
      <h2>Nenhuma automação ainda</h2>
      <p class="muted">
        Exemplos: avisar o dono quando um negócio empaca, mandar boas-vindas para quem preenche o
        formulário, criar tarefa de follow-up três dias depois da proposta.
      </p>
    </div>

    <div v-else class="list">
      <div v-for="a in automations" :key="a.id" class="card auto">
        <div class="auto-head">
          <div>
            <strong>{{ a.name }}</strong>
            <span class="badge" :class="a.active ? 'green' : 'gray'">{{ a.active ? 'ativa' : 'pausada' }}</span>
            <span v-if="pending[a.id]" class="badge blue">{{ pending[a.id] }} em sequência</span>
          </div>
          <div class="auto-actions">
            <button class="btn btn-sm" type="button" @click="openHistory(a)">Histórico</button>
            <template v-if="canManage">
              <button class="btn btn-sm" type="button" @click="toggleActive(a)">
                {{ a.active ? 'Pausar' : 'Ativar' }}
              </button>
              <button class="btn btn-sm" type="button" @click="edit(a)">Editar</button>
              <button class="btn btn-sm danger" type="button" @click="remove(a)">Excluir</button>
            </template>
          </div>
        </div>

        <p v-if="a.description" class="muted desc">{{ a.description }}</p>

        <div class="flow">
          <span class="chip trigger">{{ triggers[a.trigger_kind] ?? a.trigger_kind }}</span>
          <span class="arrow">→</span>
          <span class="chip">{{ summarize(a) }}</span>
        </div>

        <p class="muted small">
          {{ a.runs }} execução(ões)
          <template v-if="a.last_run_at"> · última {{ relativeDate(a.last_run_at) }}</template>
        </p>
      </div>
    </div>

    <!-- ===== Editor ===== -->
    <ModalDialog
      :title="editing?.id ? 'Editar automação' : 'Nova automação'"
      :open="!!editing"
      wide
      @close="editing = null"
    >
      <template v-if="editing">
        <div class="field">
          <label>Nome *</label>
          <input v-model="editing.name" placeholder="Boas-vindas para leads do site" />
        </div>
        <div class="field">
          <label>Descrição</label>
          <input v-model="editing.description" placeholder="O que essa automação faz e por quê" />
        </div>

        <h3 class="section">Quando isto acontecer</h3>
        <div class="field">
          <select v-model="editing.trigger_kind">
            <option v-for="(label, key) in triggers" :key="key" :value="key">{{ label }}</option>
          </select>
        </div>
        <div v-if="triggerNeedsStage(editing.trigger_kind)" class="field">
          <label>Etapa</label>
          <select v-model.number="triggerCfg().stage_id">
            <option :value="0">Qualquer etapa</option>
            <option v-for="s in stages" :key="s.id" :value="s.id">{{ s.name }}</option>
          </select>
        </div>
        <div v-if="triggerNeedsLifecycle(editing.trigger_kind)" class="field">
          <label>Estágio</label>
          <select v-model="triggerCfg().lifecycle_stage">
            <option value="">Qualquer estágio</option>
            <option v-for="s in lifecycleStages" :key="s" :value="s">{{ s }}</option>
          </select>
        </div>
        <div v-if="triggerNeedsDays(editing.trigger_kind)" class="field">
          <label>Depois de quantos dias</label>
          <input v-model.number="triggerCfg().days" type="number" min="1" max="365" placeholder="7" />
        </div>

        <h3 class="section">Faça isto</h3>
        <div v-for="(action, i) in editing.actions" :key="i" class="step">
          <div class="step-head">
            <span class="step-num">{{ i + 1 }}</span>
            <select v-model="action.kind">
              <option v-for="(label, key) in actionLabels" :key="key" :value="key">{{ label }}</option>
            </select>
            <div class="step-actions">
              <button class="icon-btn" type="button" title="Subir" @click="moveAction(i, -1)">↑</button>
              <button class="icon-btn" type="button" title="Descer" @click="moveAction(i, 1)">↓</button>
              <button class="icon-btn danger" type="button" title="Remover" @click="removeAction(i)">✕</button>
            </div>
          </div>

          <!-- Configuração conforme a ação escolhida -->
          <div class="step-config">
            <template v-if="action.kind === 'enviar_email'">
              <div class="field">
                <label>Modelo da biblioteca</label>
                <select v-model.number="cfg(action).template_id">
                  <option :value="0">Escrever aqui mesmo</option>
                  <option v-for="t in templates" :key="t.id" :value="t.id">{{ t.name }}</option>
                </select>
              </div>
              <template v-if="!cfg(action).template_id">
                <div class="field">
                  <label>Assunto</label>
                  <input v-model="cfg(action).subject" placeholder="Olá {{nome}}, tudo bem?" />
                </div>
                <div class="field">
                  <label>Mensagem</label>
                  <textarea v-model="cfg(action).body" rows="4"></textarea>
                  <!-- v-pre: o bloco mostra as variáveis literalmente, sem o Vue interpolar. -->
                  <p class="muted hint" v-pre>
                    Variáveis: <code>{{nome}}</code>, <code>{{sobrenome}}</code>,
                    <code>{{empresa}}</code>, <code>{{cargo}}</code>.
                  </p>
                </div>
              </template>
            </template>

            <template v-else-if="action.kind === 'criar_tarefa'">
              <div class="field">
                <label>Título</label>
                <input v-model="cfg(action).title" placeholder="Ligar para o cliente" />
              </div>
              <div class="form-row">
                <div class="field">
                  <label>Vence em (dias)</label>
                  <input v-model.number="cfg(action).days" type="number" min="0" max="365" />
                </div>
                <div class="field">
                  <label>Para quem</label>
                  <select v-model.number="cfg(action).user_id">
                    <option :value="0">Dono do registro</option>
                    <option v-for="u in users" :key="u.id" :value="u.id">{{ u.name }}</option>
                  </select>
                </div>
              </div>
            </template>

            <template v-else-if="action.kind === 'mudar_dono' || action.kind === 'notificar'">
              <div class="field">
                <label>{{ action.kind === 'notificar' ? 'Avisar' : 'Novo dono' }}</label>
                <select v-model.number="cfg(action).user_id">
                  <option :value="0">Selecione…</option>
                  <option v-for="u in users" :key="u.id" :value="u.id">{{ u.name }}</option>
                </select>
              </div>
              <div v-if="action.kind === 'notificar'" class="field">
                <label>Mensagem do aviso</label>
                <input v-model="cfg(action).content" placeholder="Este negócio está parado há dias" />
              </div>
            </template>

            <template v-else-if="action.kind === 'mudar_etapa'">
              <div class="field">
                <label>Mover para</label>
                <select v-model.number="cfg(action).stage_id">
                  <option :value="0">Selecione…</option>
                  <option v-for="s in stages" :key="s.id" :value="s.id">{{ s.name }}</option>
                </select>
              </div>
            </template>

            <template v-else-if="action.kind === 'mudar_estagio'">
              <div class="field">
                <label>Novo estágio</label>
                <select v-model="cfg(action).lifecycle_stage">
                  <option value="">Selecione…</option>
                  <option v-for="s in lifecycleStages" :key="s" :value="s">{{ s }}</option>
                </select>
              </div>
            </template>

            <template v-else-if="action.kind === 'adicionar_lista'">
              <div class="field">
                <label>Lista</label>
                <select v-model.number="cfg(action).list_id">
                  <option :value="0">Selecione…</option>
                  <option v-for="l in lists" :key="l.id" :value="l.id">{{ l.name }}</option>
                </select>
              </div>
            </template>

            <template v-else-if="action.kind === 'adicionar_nota'">
              <div class="field">
                <label>Observação</label>
                <input v-model="cfg(action).content" placeholder="Registrado automaticamente" />
              </div>
            </template>

            <template v-else-if="action.kind === 'aguardar'">
              <div class="field">
                <label>Aguardar (dias)</label>
                <input v-model.number="cfg(action).days" type="number" min="1" max="365" />
                <p class="muted hint">
                  A sequência para aqui e retoma sozinha depois do prazo. Precisa de um contato.
                </p>
              </div>
            </template>
          </div>
        </div>

        <button class="btn btn-sm" type="button" @click="addAction()">+ Adicionar ação</button>

        <div class="field" style="margin-top: 16px">
          <label>Situação</label>
          <select v-model="editing.active">
            <option :value="true">Ativa</option>
            <option :value="false">Pausada</option>
          </select>
        </div>
      </template>

      <template #footer>
        <button class="btn" type="button" @click="editing = null">Cancelar</button>
        <button class="btn btn-primary" type="button" :disabled="saving" @click="save">
          {{ saving ? 'Salvando…' : 'Salvar' }}
        </button>
      </template>
    </ModalDialog>

    <!-- ===== Histórico ===== -->
    <ModalDialog
      :title="`Histórico de ${history?.automation.name ?? ''}`"
      :open="!!history"
      wide
      @close="history = null"
    >
      <template v-if="history">
        <p v-if="!history.runs.length" class="muted">Esta automação ainda não rodou.</p>
        <ul v-else class="runs">
          <li v-for="r in history.runs" :key="r.id">
            <span
              class="badge"
              :class="r.status === 'erro' ? 'red' : r.status === 'aguardando' ? 'amber' : 'green'"
            >
              {{ r.status }}
            </span>
            <span class="run-detail">{{ r.detail }}</span>
            <span class="muted small">{{ relativeDate(r.created_at) }}</span>
          </li>
        </ul>
      </template>
    </ModalDialog>
  </div>
</template>

<style scoped>
.empty {
  text-align: center;
  padding: 36px;
}

.empty h2 {
  font-size: 16px;
  margin-bottom: 6px;
}

.list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.auto-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.auto-head .badge {
  margin-left: 8px;
}

.auto-actions {
  display: flex;
  gap: 6px;
}

.danger {
  color: var(--ci-red);
}

.desc {
  margin: 6px 0 0;
  font-size: 13px;
}

.flow {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 12px 0 8px;
  flex-wrap: wrap;
}

.chip {
  background: var(--ci-bg);
  border-radius: 8px;
  padding: 5px 10px;
  font-size: 13px;
}

.chip.trigger {
  background: var(--ci-purple-tint);
  color: var(--ci-purple-dark);
  font-weight: 500;
}

.arrow {
  color: var(--ci-text-3);
}

.small {
  font-size: 12px;
}

.section {
  font-size: 14px;
  margin: 20px 0 12px;
  padding-top: 14px;
  border-top: 1px solid var(--ci-border);
}

.step {
  border: 1px solid var(--ci-border);
  border-radius: 10px;
  padding: 12px;
  margin-bottom: 10px;
}

.step-head {
  display: flex;
  align-items: center;
  gap: 10px;
}

.step-head select {
  flex: 1;
}

.step-num {
  width: 24px;
  height: 24px;
  border-radius: 50%;
  background: var(--ci-purple);
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  flex-shrink: 0;
}

.step-actions {
  display: flex;
  gap: 2px;
}

.step-config {
  margin-top: 10px;
  padding-left: 34px;
}

.step-config .field:last-child {
  margin-bottom: 0;
}

.icon-btn {
  background: none;
  border: none;
  cursor: pointer;
  color: var(--ci-text-3);
  padding: 4px 6px;
  border-radius: 6px;
}

.icon-btn:hover {
  background: var(--ci-bg);
}

.icon-btn.danger:hover {
  color: var(--ci-red);
}

.hint {
  font-size: 12px;
  margin: 6px 0 0;
}

.runs {
  list-style: none;
  margin: 0;
  padding: 0;
}

.runs li {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 9px 0;
  border-top: 1px solid var(--ci-border);
  font-size: 13px;
}

.runs li:first-child {
  border-top: none;
}

.run-detail {
  flex: 1;
  min-width: 0;
}
</style>
