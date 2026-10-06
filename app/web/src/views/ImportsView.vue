<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api'
import { useToastStore } from '../stores/toast'
import { formatDateTime } from '../format'
import type { ImportRecord } from '../types'

const toast = useToastStore()

const history = ref<ImportRecord[]>([])
const loading = ref(true)
const uploading = ref(false)

const entity = ref<'contatos' | 'empresas'>('contatos')
const file = ref<File | null>(null)
const fileInput = ref<HTMLInputElement | null>(null)
const lastResult = ref<ImportRecord | null>(null)
const expanded = ref<number | null>(null)

const numberFmt = new Intl.NumberFormat('pt-BR')

async function load() {
  loading.value = true
  try {
    const resp = await api.get<{ data: ImportRecord[] }>('/imports')
    history.value = resp.data ?? []
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

function pickFile(event: Event) {
  const input = event.target as HTMLInputElement
  file.value = input.files?.[0] ?? null
}

async function runImport() {
  if (!file.value) return
  uploading.value = true
  try {
    const data = new FormData()
    data.append('file', file.value)
    data.append('entity', entity.value)
    const record = await api.post<ImportRecord>('/imports', data)
    lastResult.value = record
    toast.push(
      `Importação concluída: ${record.new_records} novos, ${record.updated_records} atualizados`
    )
    file.value = null
    if (fileInput.value) fileInput.value.value = ''
    await load()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    uploading.value = false
  }
}

function statusLabel(s: string): string {
  return s === 'concluida' ? 'Concluída' : 'Falhou'
}

function toggleErrors(id: number) {
  expanded.value = expanded.value === id ? null : id
}

onMounted(load)
</script>

<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1>Importações</h1>
        <p class="muted" style="margin: 4px 0 0">
          Suba um CSV ou XLSX: quem já existe é atualizado (contato pelo e-mail, empresa pelo
          CNPJ ou nome) e ninguém é duplicado.
        </p>
      </div>
    </div>

    <!-- ===== Cartão de importar arquivo ===== -->
    <div class="card upload-card">
      <div class="upload-head">
        <span class="upload-icon">⇪</span>
        <div>
          <strong>Importar arquivo</strong>
          <p class="muted small">
            A primeira linha precisa ser o cabeçalho. Colunas reconhecidas —
            contatos: Nome, Sobrenome, E-mail (obrigatória), Telefone, Cargo, Empresa, Origem;
            empresas: Razão Social, CNPJ, Cidade, UF, Site, Setor.
          </p>
        </div>
      </div>
      <div class="upload-row">
        <div class="field">
          <label>O que está importando?</label>
          <select v-model="entity">
            <option value="contatos">Contatos</option>
            <option value="empresas">Empresas</option>
          </select>
        </div>
        <div class="field">
          <label>Arquivo (.csv ou .xlsx, até 10 MB)</label>
          <input ref="fileInput" type="file" accept=".csv,.xlsx" @change="pickFile" />
        </div>
        <button
          class="btn btn-primary"
          type="button"
          :disabled="!file || uploading"
          @click="runImport"
        >
          {{ uploading ? 'Importando…' : 'Importar' }}
        </button>
      </div>

      <div v-if="lastResult" class="result" :class="lastResult.status">
        <strong>{{ lastResult.file_name }}</strong> —
        {{ numberFmt.format(lastResult.new_records) }} novos,
        {{ numberFmt.format(lastResult.updated_records) }} atualizados,
        {{ numberFmt.format(lastResult.new_associations) }} associações,
        {{ numberFmt.format(lastResult.error_count) }} erros.
        <ul v-if="lastResult.errors?.length" class="error-list">
          <li v-for="(err, i) in lastResult.errors" :key="i">{{ err }}</li>
        </ul>
      </div>
    </div>

    <!-- ===== Histórico ===== -->
    <div class="card">
      <h2 class="section-title">Importações de arquivos</h2>
      <p v-if="loading" class="muted">Carregando…</p>
      <p v-else-if="!history.length" class="muted">Nenhuma importação ainda.</p>
      <div v-else class="table-wrap">
        <table class="data">
          <thead>
            <tr>
              <th>Nome da importação</th>
              <th style="width: 110px">Status</th>
              <th style="width: 110px" class="num">Novos registros</th>
              <th style="width: 130px" class="num">Registros atualizados</th>
              <th style="width: 120px" class="num">Novas associações</th>
              <th style="width: 90px" class="num">Erros</th>
              <th style="width: 160px">Criado por</th>
              <th style="width: 150px">Data</th>
            </tr>
          </thead>
          <tbody>
            <template v-for="imp in history" :key="imp.id">
              <tr>
                <td>
                  <strong>{{ imp.file_name }}</strong>
                  <span class="muted small entity-tag">{{ imp.entity }}</span>
                </td>
                <td>
                  <span class="badge" :class="imp.status === 'concluida' ? 'ok' : 'bad'">
                    {{ statusLabel(imp.status) }}
                  </span>
                </td>
                <td class="num">{{ numberFmt.format(imp.new_records) }}</td>
                <td class="num">{{ numberFmt.format(imp.updated_records) }}</td>
                <td class="num">{{ numberFmt.format(imp.new_associations) }}</td>
                <td class="num">
                  <button
                    v-if="imp.error_count"
                    class="link-btn"
                    type="button"
                    @click="toggleErrors(imp.id)"
                  >
                    {{ numberFmt.format(imp.error_count) }}
                  </button>
                  <span v-else class="muted">—</span>
                </td>
                <td>{{ imp.created_by_name || '—' }}</td>
                <td class="muted">{{ formatDateTime(imp.created_at) }}</td>
              </tr>
              <tr v-if="expanded === imp.id">
                <td colspan="8" class="errors-cell">
                  <ul class="error-list">
                    <li v-for="(err, i) in imp.errors" :key="i">{{ err }}</li>
                  </ul>
                </td>
              </tr>
            </template>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<style scoped>
.upload-card {
  margin-bottom: 18px;
}

.upload-head {
  display: flex;
  gap: 12px;
  align-items: flex-start;
  margin-bottom: 14px;
}

.upload-icon {
  font-size: 22px;
  color: var(--ci-purple);
  line-height: 1;
}

.small {
  font-size: 12px;
  margin: 4px 0 0;
}

.upload-row {
  display: flex;
  gap: 14px;
  align-items: flex-end;
  flex-wrap: wrap;
}

.upload-row .field {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.upload-row label {
  font-size: 12px;
  color: var(--ci-text-3);
}

.result {
  margin-top: 14px;
  padding: 10px 12px;
  border-radius: 8px;
  font-size: 13px;
  background: var(--ci-green-tint, #e7f6ec);
  border: 1px solid var(--ci-green, #2e9e5b);
}

.result.falhou {
  background: var(--ci-red-tint, #fdecec);
  border-color: var(--ci-red, #c74343);
}

.section-title {
  font-size: 15px;
  margin-bottom: 12px;
}

.num {
  text-align: right;
  font-variant-numeric: tabular-nums;
}

.entity-tag {
  margin-left: 8px;
  text-transform: capitalize;
}

.badge.ok {
  background: var(--ci-green-tint, #e7f6ec);
  color: var(--ci-green, #2e9e5b);
}

.badge.bad {
  background: var(--ci-red-tint, #fdecec);
  color: var(--ci-red, #c74343);
}

.link-btn {
  background: none;
  border: none;
  color: var(--ci-purple-dark);
  text-decoration: underline;
  cursor: pointer;
  font-size: 13px;
  padding: 0;
}

.errors-cell {
  background: var(--ci-bg);
}

.error-list {
  margin: 6px 0 0;
  padding-left: 18px;
  font-size: 12px;
  color: var(--ci-text-2);
}
</style>
