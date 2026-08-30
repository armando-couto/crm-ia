<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import { useAuthStore } from '../stores/auth'
import { useToastStore } from '../stores/toast'

const toast = useToastStore()
const auth = useAuthStore()

interface PermissionDef {
  key: string
  label: string
  group: string
}

const catalog = ref<PermissionDef[]>([])
const roles = ref<string[]>([])
const labels = ref<Record<string, string>>({})
const matrix = ref<Record<string, Record<string, boolean>>>({})
const loading = ref(true)
const saving = ref(false)
const dirty = ref(false)

// Agrupa o catálogo para exibir em blocos (Contatos, Negócios, Configurações…).
const groups = computed(() => {
  const out: { name: string; items: PermissionDef[] }[] = []
  for (const item of catalog.value) {
    const last = out[out.length - 1]
    if (last && last.name === item.group) {
      last.items.push(item)
    } else {
      out.push({ name: item.group, items: [item] })
    }
  }
  return out
})

async function load() {
  loading.value = true
  try {
    const resp = await api.get<{
      catalog: PermissionDef[]
      roles: string[]
      labels: Record<string, string>
      matrix: Record<string, Record<string, boolean>>
    }>('/permissions')
    catalog.value = resp.catalog ?? []
    roles.value = resp.roles ?? []
    labels.value = resp.labels ?? {}
    matrix.value = resp.matrix ?? {}
    dirty.value = false
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

function isLocked(role: string): boolean {
  // O Admin sempre tem acesso total: não é editável.
  return role === 'admin'
}

function toggle(role: string, key: string) {
  if (isLocked(role)) return
  matrix.value[role][key] = !matrix.value[role][key]
  dirty.value = true
}

function toggleGroup(role: string, items: PermissionDef[], value: boolean) {
  if (isLocked(role)) return
  for (const item of items) {
    matrix.value[role][item.key] = value
  }
  dirty.value = true
}

function groupAllChecked(role: string, items: PermissionDef[]): boolean {
  return items.every((i) => matrix.value[role]?.[i.key])
}

async function save() {
  saving.value = true
  try {
    for (const role of roles.value) {
      if (isLocked(role)) continue
      await api.put('/permissions', { role, permissions: matrix.value[role] })
    }
    toast.push('Permissões atualizadas para toda a equipe')
    dirty.value = false
    await auth.fetchPermissions()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1>Permissões</h1>
        <p class="muted" style="margin: 4px 0 0">
          Defina o que cada perfil pode fazer. As mudanças valem imediatamente para todos os usuários do perfil.
        </p>
      </div>
      <button class="btn btn-primary" type="button" :disabled="saving || !dirty" @click="save">
        {{ saving ? 'Salvando…' : 'Salvar alterações' }}
      </button>
    </div>

    <p v-if="loading" class="muted">Carregando permissões…</p>

    <div v-else class="table-wrap">
      <table class="data perms">
        <thead>
          <tr>
            <th>Permissão</th>
            <th v-for="role in roles" :key="role" class="role-col">
              {{ labels[role] || role }}
              <span v-if="isLocked(role)" class="badge gray">acesso total</span>
            </th>
          </tr>
        </thead>
        <tbody>
          <template v-for="group in groups" :key="group.name">
            <tr class="group-row">
              <td><strong>{{ group.name }}</strong></td>
              <td v-for="role in roles" :key="role" class="role-col">
                <button
                  v-if="!isLocked(role)"
                  type="button"
                  class="group-toggle"
                  @click="toggleGroup(role, group.items, !groupAllChecked(role, group.items))"
                >
                  {{ groupAllChecked(role, group.items) ? 'desmarcar todos' : 'marcar todos' }}
                </button>
              </td>
            </tr>
            <tr v-for="item in group.items" :key="item.key" class="perm-row">
              <td>
                {{ item.label }}
                <div class="muted key">{{ item.key }}</div>
              </td>
              <td v-for="role in roles" :key="role" class="role-col">
                <input
                  type="checkbox"
                  :checked="isLocked(role) ? true : !!matrix[role]?.[item.key]"
                  :disabled="isLocked(role)"
                  @change="toggle(role, item.key)"
                />
              </td>
            </tr>
          </template>
        </tbody>
      </table>
    </div>

    <p class="muted footer-note" v-if="!loading">
      O perfil <strong>Admin</strong> sempre mantém acesso total — assim ninguém fica trancado fora do sistema.
    </p>
  </div>
</template>

<style scoped>
.perms td,
.perms th {
  vertical-align: middle;
}

.role-col {
  text-align: center;
  width: 130px;
}

.role-col .badge {
  display: block;
  margin-top: 2px;
  font-size: 10px;
}

.group-row td {
  background: var(--fix-bg);
  font-size: 13px;
}

.perm-row td:first-child {
  padding-left: 24px;
}

.key {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 11px;
}

.group-toggle {
  border: none;
  background: none;
  color: var(--fix-purple);
  font-size: 11px;
  cursor: pointer;
}

.group-toggle:hover {
  text-decoration: underline;
}

.perms input[type='checkbox'] {
  width: 16px;
  height: 16px;
  cursor: pointer;
}

.perms input[type='checkbox']:disabled {
  cursor: default;
  opacity: 0.5;
}

.footer-note {
  margin-top: 14px;
  font-size: 13px;
}
</style>
