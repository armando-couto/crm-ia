<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { api, foiAbortado } from '../../api'
import { useAuthStore } from '../../stores/auth'
import type { User } from '../../types'
import type { DiaDisponibilidade, Disponibilidade } from '../../types-prospeccao'
import { horaDe, rotuloDia } from './util'

/**
 * Seletor de horário da reunião: escolhe o vendedor, o dia e um horário livre
 * da agenda dele, com os compromissos do dia ao lado. O valor escolhido é
 * sempre o `inicio` RFC 3339 devolvido por `/agenda/disponibilidade`.
 */
const props = withDefaults(
  defineProps<{
    /** `reuniao.inicio` (RFC 3339) ou vazio. */
    inicio: string
    /** `reuniao.user_id`. */
    userId: number | null
    dias?: number
  }>(),
  { dias: 7 }
)

const emit = defineEmits<{
  'update:inicio': [valor: string]
  'update:userId': [valor: number]
  /** Duração padrão da agenda do vendedor escolhido. */
  duracao: [minutos: number]
}>()

const auth = useAuthStore()

const usuarios = ref<User[]>([])
const disponibilidade = ref<Disponibilidade | null>(null)
const carregando = ref(false)
const erro = ref('')
const diaAtual = ref('')
let controle: AbortController | null = null

const dia = computed<DiaDisponibilidade | null>(
  () => disponibilidade.value?.dias.find((d) => d.data === diaAtual.value) ?? null
)

async function carregarUsuarios() {
  try {
    const lista = await api.get<User[] | null>('/users')
    usuarios.value = (Array.isArray(lista) ? lista : []).filter((u) => u.active)
  } catch {
    usuarios.value = []
  }
  // Sem acesso à lista de usuários, o vendedor marca na própria agenda.
  if (!usuarios.value.length && auth.user) usuarios.value = [auth.user]
}

async function carregar() {
  const id = props.userId ?? auth.user?.id
  if (!id) return
  controle?.abort()
  controle = new AbortController()
  carregando.value = true
  erro.value = ''
  try {
    const r = await api.get<Disponibilidade>(`/agenda/disponibilidade?user_id=${id}&dias=${props.dias}`, {
      signal: controle.signal
    })
    disponibilidade.value = { ...r, dias: (r?.dias ?? []).map((d) => ({ ...d, horarios: d.horarios ?? [], ocupados: d.ocupados ?? [] })) }
    emit('duracao', r?.duracao_min ?? 30)
    const dias = disponibilidade.value.dias
    // Mantém o dia aberto se ainda existe; senão vai ao primeiro com horário livre.
    if (!dias.some((d) => d.data === diaAtual.value)) {
      diaAtual.value = (dias.find((d) => d.horarios.length) ?? dias[0])?.data ?? ''
    }
    // O horário escolhido deixou de estar livre: limpa a escolha.
    if (props.inicio && !dias.some((d) => d.horarios.some((h) => h.inicio === props.inicio))) {
      emit('update:inicio', '')
    }
  } catch (e: any) {
    if (foiAbortado(e)) return
    erro.value = e?.message || 'não foi possível ler a agenda'
    disponibilidade.value = null
  } finally {
    carregando.value = false
  }
}

function trocarVendedor(event: Event) {
  const id = Number((event.target as HTMLSelectElement).value)
  emit('update:inicio', '')
  emit('update:userId', id)
}

watch(() => props.userId, carregar)

onMounted(async () => {
  await carregarUsuarios()
  if (props.userId == null && auth.user) emit('update:userId', auth.user.id)
  else await carregar()
})

/** Recarrega a grade depois de um 409 `agenda_ocupada`, mantendo o formulário. */
defineExpose({ recarregar: carregar })
</script>

<template>
  <div class="slot-picker">
    <div class="field">
      <label for="reuniao-vendedor">Quem faz a reunião</label>
      <select id="reuniao-vendedor" :value="userId ?? ''" @change="trocarVendedor">
        <option v-for="u in usuarios" :key="u.id" :value="u.id">
          {{ u.name }}{{ u.id === auth.user?.id ? ' (eu)' : '' }}
        </option>
      </select>
    </div>

    <p v-if="carregando && !disponibilidade" class="muted">Lendo a agenda…</p>
    <p v-else-if="erro" class="error-text">{{ erro }}</p>

    <template v-else-if="disponibilidade">
      <p v-if="!disponibilidade.dias.length" class="muted">Sem dias disponíveis nesta agenda.</p>

      <div class="dias" role="tablist" aria-label="Dia da reunião">
        <button
          v-for="d in disponibilidade.dias"
          :key="d.data"
          type="button"
          class="chip dia"
          role="tab"
          :aria-selected="d.data === diaAtual"
          :class="{ active: d.data === diaAtual, vazio: !d.horarios.length }"
          @click="diaAtual = d.data"
        >
          {{ rotuloDia(d.data) }}
          <span class="muted">{{ d.horarios.length }}</span>
        </button>
      </div>

      <div v-if="dia" class="grade">
        <div class="livres">
          <span class="rotulo">Horários livres ({{ disponibilidade.duracao_min }} min)</span>
          <p v-if="!dia.horarios.length" class="muted">Nenhum horário livre neste dia.</p>
          <div class="horarios">
            <button
              v-for="h in dia.horarios"
              :key="h.inicio"
              type="button"
              class="horario"
              :class="{ active: h.inicio === inicio }"
              :aria-pressed="h.inicio === inicio"
              @click="emit('update:inicio', h.inicio)"
            >
              {{ h.rotulo }}
            </button>
          </div>
        </div>

        <div class="agenda">
          <span class="rotulo">Agenda do dia</span>
          <p v-if="!dia.ocupados.length" class="muted">Nada marcado.</p>
          <ul>
            <li v-for="(o, i) in dia.ocupados" :key="i">
              <strong>{{ horaDe(o.inicio) }}–{{ horaDe(o.fim) }}</strong>
              <span>{{ o.titulo || 'Ocupado' }}</span>
            </li>
          </ul>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.dias {
  display: flex;
  gap: 6px;
  overflow-x: auto;
  padding-bottom: 6px;
  margin-bottom: 10px;
}

.dia {
  text-transform: capitalize;
  flex-shrink: 0;
}

.dia.vazio {
  opacity: 0.55;
}

.grade {
  display: grid;
  grid-template-columns: minmax(0, 3fr) minmax(0, 2fr);
  gap: 14px;
}

.rotulo {
  display: block;
  font-size: 12px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--ci-text-3);
  margin-bottom: 6px;
}

.horarios {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(68px, 1fr));
  gap: 6px;
}

.horario {
  min-height: 40px;
  border: 1px solid var(--ci-border);
  border-radius: 8px;
  background: var(--ci-surface);
  font-size: 14px;
  font-variant-numeric: tabular-nums;
  cursor: pointer;
}

.horario:hover {
  border-color: var(--ci-purple);
}

.horario.active {
  background: var(--ci-purple);
  border-color: var(--ci-purple);
  color: #fff;
  font-weight: 600;
}

.agenda ul {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
  font-size: 13px;
}

.agenda li {
  display: flex;
  flex-direction: column;
  padding: 6px 8px;
  border-left: 3px solid var(--ci-amber);
  background: var(--ci-amber-tint);
  border-radius: 0 6px 6px 0;
}

.agenda p,
.livres p {
  margin: 0;
  font-size: 13px;
}

@media (max-width: 720px) {
  .grade {
    grid-template-columns: 1fr;
  }

  .horario {
    min-height: 46px;
    font-size: 15px;
  }
}
</style>
