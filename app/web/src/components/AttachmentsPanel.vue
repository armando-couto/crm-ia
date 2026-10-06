<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api, getToken } from '../api'
import { useAuthStore } from '../stores/auth'
import { useToastStore } from '../stores/toast'
import { relativeDate } from '../format'
import type { Attachment } from '../types'

const props = defineProps<{
  entity: 'contato' | 'empresa' | 'negocio' | 'ticket'
  entityId: number
}>()

const auth = useAuthStore()
const toast = useToastStore()

const files = ref<Attachment[]>([])
const loading = ref(false)
const uploading = ref(false)
const dragging = ref(false)
const input = ref<HTMLInputElement | null>(null)

const canManage = computed(() => auth.can('files.manage'))

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

// Ícone por tipo, só para dar uma pista visual rápida na lista.
function icon(file: Attachment): string {
  const type = file.content_type
  if (type.startsWith('image/')) return '🖼️'
  if (type.includes('pdf')) return '📕'
  if (type.includes('csv') || type.includes('excel') || type.includes('sheet')) return '📊'
  if (type.includes('word') || type.includes('document')) return '📝'
  return '📎'
}

async function load() {
  loading.value = true
  try {
    files.value = await api.get<Attachment[]>(
      `/attachments?entity=${props.entity}&entity_id=${props.entityId}`
    )
  } catch {
    files.value = []
  } finally {
    loading.value = false
  }
}

async function upload(list: FileList | null) {
  if (!list || list.length === 0) return
  uploading.value = true
  try {
    for (const file of Array.from(list)) {
      const form = new FormData()
      form.append('entity', props.entity)
      form.append('entity_id', String(props.entityId))
      form.append('file', file)
      await api.post('/attachments', form)
    }
    toast.push(list.length > 1 ? 'Arquivos anexados' : 'Arquivo anexado')
    await load()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    uploading.value = false
    if (input.value) input.value.value = ''
  }
}

function onDrop(event: DragEvent) {
  dragging.value = false
  if (!canManage.value) return
  upload(event.dataTransfer?.files ?? null)
}

async function remove(file: Attachment) {
  if (!confirm(`Remover ${file.filename}?`)) return
  try {
    await api.delete(`/attachments/${file.id}`)
    files.value = files.value.filter((f) => f.id !== file.id)
    toast.push('Arquivo removido')
  } catch (e: any) {
    toast.error(e.message)
  }
}

/**
 * O download passa pela API autenticada, então buscamos o arquivo com o token
 * e entregamos como blob — <a href> puro iria sem o Authorization.
 */
async function download(file: Attachment) {
  try {
    const resp = await fetch(`/api/v1/attachments/${file.id}/download`, {
      headers: { Authorization: `Bearer ${getToken()}` }
    })
    if (!resp.ok) throw new Error('não foi possível baixar o arquivo')
    const blob = await resp.blob()
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = file.filename
    link.click()
    URL.revokeObjectURL(url)
  } catch (e: any) {
    toast.error(e.message)
  }
}

onMounted(load)
</script>

<template>
  <div class="card attachments">
    <div class="head">
      <h2>Arquivos <span class="muted count" v-if="files.length">({{ files.length }})</span></h2>
      <button v-if="canManage" class="btn" type="button" :disabled="uploading" @click="input?.click()">
        {{ uploading ? 'Enviando…' : 'Anexar arquivo' }}
      </button>
      <input ref="input" type="file" multiple hidden @change="upload(($event.target as HTMLInputElement).files)" />
    </div>

    <div
      v-if="canManage"
      class="dropzone"
      :class="{ over: dragging }"
      @dragover.prevent="dragging = true"
      @dragleave.prevent="dragging = false"
      @drop.prevent="onDrop"
    >
      Arraste arquivos aqui ou clique em <strong>Anexar arquivo</strong> · até 10 MB cada
    </div>

    <p v-if="loading" class="muted">Carregando arquivos…</p>
    <p v-else-if="!files.length" class="muted empty">Nenhum arquivo anexado.</p>

    <ul v-else class="file-list">
      <li v-for="file in files" :key="file.id">
        <span class="icon">{{ icon(file) }}</span>
        <button class="name" type="button" @click="download(file)">{{ file.filename }}</button>
        <span class="muted meta">
          {{ formatSize(file.size_bytes) }}
          <template v-if="file.uploader_name"> · {{ file.uploader_name }}</template>
          · {{ relativeDate(file.created_at) }}
        </span>
        <button v-if="canManage" class="remove" type="button" title="Remover" @click="remove(file)">✕</button>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}

.head h2 {
  font-size: 15px;
}

.count {
  font-weight: 400;
  font-size: 13px;
}

.dropzone {
  border: 1px dashed var(--ci-border);
  border-radius: 10px;
  padding: 14px;
  text-align: center;
  font-size: 13px;
  color: var(--ci-text-3);
  margin-bottom: 12px;
  transition: background 0.15s, border-color 0.15s;
}

.dropzone.over {
  border-color: var(--ci-purple);
  background: var(--ci-purple-tint);
  color: var(--ci-purple-dark);
}

.empty {
  margin: 4px 0 0;
}

.file-list {
  list-style: none;
  margin: 0;
  padding: 0;
}

.file-list li {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 9px 0;
  border-top: 1px solid var(--ci-border);
}

.file-list li:first-child {
  border-top: none;
}

.icon {
  font-size: 16px;
}

.name {
  background: none;
  border: none;
  padding: 0;
  font-size: 14px;
  color: var(--ci-purple-dark);
  cursor: pointer;
  text-align: left;
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.name:hover {
  text-decoration: underline;
}

.meta {
  font-size: 12px;
  white-space: nowrap;
}

.remove {
  background: none;
  border: none;
  color: var(--ci-text-3);
  cursor: pointer;
  font-size: 13px;
  padding: 2px 4px;
}

.remove:hover {
  color: var(--ci-red);
}
</style>
