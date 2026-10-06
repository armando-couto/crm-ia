<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import { useAuthStore } from '../stores/auth'
import { useToastStore } from '../stores/toast'
import { formatDate } from '../format'
import ModalDialog from '../components/ModalDialog.vue'

interface DuplicateEntry {
  id: number
  label: string
  email?: string
  phone?: string
  extra?: string
  owner_name?: string
  deals: number
  created_at: string
}

interface DuplicateGroup {
  reason: string
  value: string
  records: DuplicateEntry[]
}

const auth = useAuthStore()
const toast = useToastStore()

const entity = ref<'contacts' | 'companies'>('contacts')
const groups = ref<DuplicateGroup[]>([])
const loading = ref(true)
const merging = ref(false)

// Grupo aberto no modal e qual registro fica como principal.
const current = ref<DuplicateGroup | null>(null)
const primaryId = ref(0)

const canMerge = computed(() => auth.can('records.merge'))
const entityLabel = computed(() => (entity.value === 'contacts' ? 'contato' : 'empresa'))

const duplicatesOf = computed(() =>
  current.value ? current.value.records.filter((r) => r.id !== primaryId.value) : []
)

async function load() {
  loading.value = true
  try {
    const resp = await api.get<{ groups: DuplicateGroup[] }>(`/duplicates?entity=${entity.value}`)
    groups.value = resp.groups ?? []
  } catch (e: any) {
    toast.error(e.message)
    groups.value = []
  } finally {
    loading.value = false
  }
}

function switchEntity(value: 'contacts' | 'companies') {
  entity.value = value
  load()
}

function openGroup(group: DuplicateGroup) {
  current.value = group
  // Sugere como principal o registro mais antigo com mais negócios ligados.
  const best = [...group.records].sort((a, b) => b.deals - a.deals || a.id - b.id)[0]
  primaryId.value = best.id
}

async function merge() {
  if (!current.value || !primaryId.value) return
  const others = duplicatesOf.value
  if (!others.length) return
  if (
    !confirm(
      `Mesclar ${others.length} registro(s) no principal? O histórico, negócios e anexos migram e os duplicados deixam de existir. Não há como desfazer.`
    )
  ) {
    return
  }

  merging.value = true
  try {
    // Um duplicado por vez: cada mesclagem é uma transação no backend.
    for (const other of others) {
      await api.post('/duplicates/merge', {
        entity: entity.value,
        primary_id: primaryId.value,
        duplicate_id: other.id
      })
    }
    toast.push(others.length > 1 ? 'Registros mesclados' : 'Registro mesclado')
    current.value = null
    await load()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    merging.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1>Duplicados</h1>
        <p class="muted" style="margin: 4px 0 0">
          Registros que parecem ser a mesma pessoa ou empresa. Escolha o principal e mescle:
          histórico, negócios, tarefas e anexos migram para ele.
        </p>
      </div>
      <div class="tabs">
        <button class="btn" :class="{ 'btn-primary': entity === 'contacts' }" type="button" @click="switchEntity('contacts')">
          Contatos
        </button>
        <button class="btn" :class="{ 'btn-primary': entity === 'companies' }" type="button" @click="switchEntity('companies')">
          Empresas
        </button>
      </div>
    </div>

    <p v-if="loading" class="muted">Procurando duplicados…</p>

    <div v-else-if="!groups.length" class="card clean">
      <h2>Nenhum duplicado encontrado</h2>
      <p class="muted">
        Comparamos e-mail, telefone e nome dos contatos; CNPJ, domínio e nome das empresas.
      </p>
    </div>

    <div v-else class="groups">
      <div v-for="(group, i) in groups" :key="`${group.reason}-${i}`" class="card group">
        <div class="group-head">
          <div>
            <span class="badge">{{ group.reason }}</span>
            <strong class="value">{{ group.value }}</strong>
          </div>
          <button v-if="canMerge" class="btn btn-primary" type="button" @click="openGroup(group)">
            Revisar e mesclar
          </button>
        </div>
        <ul class="records">
          <li v-for="r in group.records" :key="r.id">
            <router-link :to="`/${entity === 'contacts' ? 'contatos' : 'empresas'}/${r.id}`">
              {{ r.label || '(sem nome)' }}
            </router-link>
            <span class="muted small">
              #{{ r.id }}
              <template v-if="r.email"> · {{ r.email }}</template>
              <template v-if="r.phone"> · {{ r.phone }}</template>
              <template v-if="r.extra"> · {{ r.extra }}</template>
              <template v-if="r.owner_name"> · {{ r.owner_name }}</template>
              · {{ r.deals }} negócio(s) · criado em {{ formatDate(r.created_at) }}
            </span>
          </li>
        </ul>
      </div>
    </div>

    <ModalDialog :title="`Mesclar ${entityLabel}s`" :open="!!current" wide @close="current = null">
      <template v-if="current">
        <p class="muted">
          Escolha o registro que <strong>permanece</strong>. Os demais são apagados e o que eles têm
          de diferente preenche os campos vazios do principal.
        </p>
        <ul class="choose">
          <li v-for="r in current.records" :key="r.id">
            <label>
              <input type="radio" :value="r.id" :checked="primaryId === r.id" @change="primaryId = r.id" />
              <span class="choose-main">
                <strong>{{ r.label || '(sem nome)' }}</strong>
                <span class="muted small">
                  #{{ r.id }}
                  <template v-if="r.email"> · {{ r.email }}</template>
                  <template v-if="r.phone"> · {{ r.phone }}</template>
                  <template v-if="r.extra"> · {{ r.extra }}</template>
                  · {{ r.deals }} negócio(s) · criado em {{ formatDate(r.created_at) }}
                </span>
              </span>
            </label>
          </li>
        </ul>
        <p class="warn">
          {{ duplicatesOf.length }} registro(s) serão apagados definitivamente. Não há como desfazer.
        </p>
      </template>
      <template #footer>
        <button class="btn" type="button" @click="current = null">Cancelar</button>
        <button class="btn btn-primary" type="button" :disabled="merging || !duplicatesOf.length" @click="merge">
          {{ merging ? 'Mesclando…' : 'Mesclar' }}
        </button>
      </template>
    </ModalDialog>
  </div>
</template>

<style scoped>
.tabs {
  display: flex;
  gap: 8px;
}

.clean {
  text-align: center;
  padding: 36px;
}

.clean h2 {
  font-size: 16px;
  margin-bottom: 6px;
}

.groups {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.group-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  margin-bottom: 10px;
}

.value {
  margin-left: 10px;
  font-size: 14px;
}

.records,
.choose {
  list-style: none;
  margin: 0;
  padding: 0;
}

.records li {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 8px 0;
  border-top: 1px solid var(--ci-border);
}

.small {
  font-size: 12px;
}

.choose li {
  border: 1px solid var(--ci-border);
  border-radius: 10px;
  margin-top: 8px;
  padding: 10px 12px;
}

.choose label {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  cursor: pointer;
}

.choose-main {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.warn {
  margin-top: 14px;
  font-size: 13px;
  color: var(--ci-red);
}
</style>
