<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import { useAuthStore } from '../stores/auth'
import { useToastStore } from '../stores/toast'
import { relativeDate } from '../format'
import type { Sequence } from '../types'

const auth = useAuthStore()
const toast = useToastStore()
const emit = defineEmits<{ open: [id: number] }>()

const sequences = ref<Sequence[]>([])
const loading = ref(true)

const canManage = computed(() => auth.can('automations.manage'))

/** Resumo da cadência em uma linha: quantas automáticas e quantas manuais. */
function resumo(seq: Sequence): string {
  const auto = seq.steps.filter((s) => s.kind === 'email_auto').length
  const manual = seq.steps.length - auto
  const partes = []
  if (auto) partes.push(`${auto} e-mail${auto > 1 ? 's' : ''} automático${auto > 1 ? 's' : ''}`)
  if (manual) partes.push(`${manual} tarefa${manual > 1 ? 's' : ''} manual${manual > 1 ? 'is' : ''}`)
  return partes.join(' · ') || 'sem etapas'
}

async function load() {
  loading.value = true
  try {
    const resp = await api.get<{ data: Sequence[] }>('/sequences')
    sequences.value = resp.data ?? []
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

async function toggleActive(seq: Sequence) {
  try {
    await api.put(`/sequences/${seq.id}`, { ...seq, active: !seq.active })
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function remove(seq: Sequence) {
  if (!confirm(`Excluir "${seq.name}"? Quem estiver no meio da cadência sai dela.`)) return
  try {
    await api.delete(`/sequences/${seq.id}`)
    toast.push('Sequência removida')
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}

defineExpose({ load })
onMounted(load)
</script>

<template>
  <div>
    <div class="page-head">
      <div>
        <h1>Sequências</h1>
        <p class="muted" style="margin: 4px 0 0">
          Cadências de prospecção: e-mails automáticos e tarefas que entram na sua fila.
        </p>
      </div>
      <button v-if="canManage" class="btn btn-primary" type="button" @click="emit('open', 0)">
        Criar sequência
      </button>
    </div>

    <p v-if="loading" class="muted">Carregando sequências…</p>

    <div v-else-if="!sequences.length" class="card empty">
      <h2>Nenhuma sequência ainda</h2>
      <p class="muted">
        Uma sequência manda os primeiros e-mails sozinha e, quando o contato responde ou engaja,
        coloca a ligação na sua fila em vez de continuar insistindo por e-mail.
      </p>
    </div>

    <div v-else class="table-wrap">
      <table class="data">
        <thead>
          <tr>
            <th>Nome</th>
            <th style="width: 110px">Inscritos</th>
            <th style="width: 110px">Abertura</th>
            <th style="width: 110px">Resposta</th>
            <th style="width: 130px">Dono</th>
            <th style="width: 120px">Modificada</th>
            <th style="width: 180px"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="s in sequences" :key="s.id">
            <td>
              <button class="link" type="button" @click="emit('open', s.id)">
                <strong>{{ s.name }}</strong>
              </button>
              <span v-if="s.dynamic" class="badge">dinâmica</span>
              <span v-if="!s.active" class="badge gray">pausada</span>
              <div class="muted small">{{ resumo(s) }}</div>
            </td>
            <td>
              {{ s.enrolled }}
              <div v-if="s.active_members" class="muted small">{{ s.active_members }} em andamento</div>
            </td>
            <td>{{ s.open_rate.toFixed(0) }}%</td>
            <td>{{ s.reply_rate.toFixed(0) }}%</td>
            <td class="muted">{{ s.owner_name || '—' }}</td>
            <td class="muted">{{ relativeDate(s.updated_at) }}</td>
            <td class="actions">
              <button class="btn btn-sm" type="button" @click="emit('open', s.id)">Abrir</button>
              <template v-if="canManage">
                <button class="btn btn-sm" type="button" @click="toggleActive(s)">
                  {{ s.active ? 'Pausar' : 'Ativar' }}
                </button>
                <button class="btn btn-sm danger" type="button" @click="remove(s)">Excluir</button>
              </template>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
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

.link {
  background: none;
  border: none;
  padding: 0;
  cursor: pointer;
  color: var(--fix-purple-dark);
  font-size: 14px;
}

.link:hover strong {
  text-decoration: underline;
}

.badge {
  margin-left: 6px;
}

.small {
  font-size: 12px;
}

.actions {
  display: flex;
  gap: 6px;
  justify-content: flex-end;
}

.danger {
  color: var(--fix-red);
}
</style>
