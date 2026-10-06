<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'

interface DaySlots {
  date: string
  slots: string[]
}

interface BookingDef {
  slug: string
  title: string
  description: string
  location: string
  duration_min: number
  host_name: string
  days: DaySlots[]
}

const route = useRoute()

const page = ref<BookingDef | null>(null)
const loading = ref(true)
const sending = ref(false)
const error = ref('')
const success = ref('')

const selectedDate = ref('')
const selectedTime = ref('')
const form = ref({ name: '', email: '', phone: '', notes: '' })
const honeypot = ref('')

const slotsOfDay = computed(
  () => page.value?.days.find((d) => d.date === selectedDate.value)?.slots ?? []
)

function dayLabel(date: string): string {
  const [y, m, d] = date.split('-').map(Number)
  const dt = new Date(y, m - 1, d)
  return dt.toLocaleDateString('pt-BR', { weekday: 'short', day: '2-digit', month: 'short' })
}

async function load() {
  try {
    const resp = await fetch(`/api/public/booking/${route.params.slug}`)
    if (!resp.ok) throw new Error('não encontrada')
    page.value = await resp.json()
    selectedDate.value = page.value!.days[0]?.date ?? ''
  } catch {
    error.value = 'Página de agendamento não encontrada ou desativada.'
  } finally {
    loading.value = false
  }
}

function pick(time: string) {
  selectedTime.value = time
}

async function submit() {
  if (!page.value || !selectedDate.value || !selectedTime.value) return
  error.value = ''
  sending.value = true
  try {
    const resp = await fetch(`/api/public/booking/${page.value.slug}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        ...form.value,
        date: selectedDate.value,
        time: selectedTime.value,
        _gotcha: honeypot.value
      })
    })
    const data = await resp.json()
    if (!resp.ok) {
      // 409: alguém pegou o horário enquanto a pessoa preenchia.
      if (resp.status === 409) await load()
      throw new Error(data?.error || 'não foi possível agendar')
    }
    success.value = data.message
  } catch (e: any) {
    error.value = e.message
    selectedTime.value = ''
  } finally {
    sending.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="booking">
    <p v-if="loading" class="muted">Carregando agenda…</p>

    <div v-else-if="!page" class="panel">
      <p class="error-text">{{ error }}</p>
    </div>

    <div v-else class="panel">
      <template v-if="success">
        <div class="done">
          <span class="check">✓</span>
          <p>{{ success }}</p>
          <p class="muted small">
            {{ dayLabel(selectedDate) }} às {{ selectedTime }} · {{ page.duration_min }} min
          </p>
        </div>
      </template>

      <template v-else>
        <header>
          <h1>{{ page.title }}</h1>
          <p class="host">com {{ page.host_name }} · {{ page.duration_min }} minutos</p>
          <p v-if="page.description" class="description">{{ page.description }}</p>
          <p v-if="page.location" class="muted small">Local: {{ page.location }}</p>
        </header>

        <p v-if="!page.days.length" class="muted empty">
          Nenhum horário livre nos próximos dias. Tente de novo mais tarde.
        </p>

        <template v-else>
          <h2>Escolha o dia</h2>
          <div class="days">
            <button
              v-for="d in page.days"
              :key="d.date"
              type="button"
              class="day"
              :class="{ active: d.date === selectedDate }"
              @click="selectedDate = d.date; selectedTime = ''"
            >
              {{ dayLabel(d.date) }}
              <span class="count">{{ d.slots.length }} horários</span>
            </button>
          </div>

          <h2>Escolha o horário</h2>
          <div class="slots">
            <button
              v-for="slot in slotsOfDay"
              :key="slot"
              type="button"
              class="slot"
              :class="{ active: slot === selectedTime }"
              @click="pick(slot)"
            >
              {{ slot }}
            </button>
          </div>

          <!-- O erro fica fora do formulário: quando o horário é tomado no meio
               do caminho a lista recarrega e o formulário some. -->
          <p v-if="error" class="error-text">{{ error }}</p>

          <form v-if="selectedTime" @submit.prevent="submit">
            <h2>Seus dados</h2>
            <div class="field">
              <label for="name">Nome *</label>
              <input id="name" v-model="form.name" required />
            </div>
            <div class="field">
              <label for="email">E-mail *</label>
              <input id="email" v-model="form.email" type="email" required />
            </div>
            <div class="field">
              <label for="phone">Telefone</label>
              <input id="phone" v-model="form.phone" type="tel" />
            </div>
            <div class="field">
              <label for="notes">Sobre o que quer falar?</label>
              <textarea id="notes" v-model="form.notes" rows="3"></textarea>
            </div>

            <input
              v-model="honeypot"
              class="gotcha"
              type="text"
              tabindex="-1"
              autocomplete="off"
              aria-hidden="true"
            />

            <button class="submit" type="submit" :disabled="sending">
              {{ sending ? 'Confirmando…' : `Confirmar ${dayLabel(selectedDate)} às ${selectedTime}` }}
            </button>
          </form>
        </template>
      </template>
    </div>
  </div>
</template>

<style scoped>
.booking {
  min-height: 100vh;
  padding: 24px 16px;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
}

.panel {
  max-width: 520px;
  margin: 0 auto;
  background: #fff;
  border-radius: 14px;
  padding: 26px;
  box-shadow: 0 1px 3px rgba(34, 20, 60, 0.12);
}

h1 {
  font-size: 20px;
  margin: 0 0 4px;
  color: #2e1a47;
}

h2 {
  font-size: 13px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: #8e8e8e;
  margin: 20px 0 10px;
}

.host {
  margin: 0 0 8px;
  font-size: 14px;
  color: #9b52df;
  font-weight: 500;
}

.description {
  margin: 0 0 6px;
  font-size: 14px;
  color: #6b6b6b;
  line-height: 1.5;
}

.days {
  display: flex;
  gap: 8px;
  overflow-x: auto;
  padding-bottom: 4px;
}

.day {
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
  border: 1px solid #e3e0e8;
  background: #fff;
  border-radius: 10px;
  padding: 10px 14px;
  cursor: pointer;
  font-size: 13px;
  color: #464646;
  text-transform: capitalize;
}

.day.active {
  border-color: #9b52df;
  background: #f7f0fd;
  color: #6a2fa8;
  font-weight: 500;
}

.count {
  font-size: 11px;
  color: #8e8e8e;
  text-transform: none;
}

.slots {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(84px, 1fr));
  gap: 8px;
}

.slot {
  border: 1px solid #e3e0e8;
  background: #fff;
  border-radius: 8px;
  padding: 10px;
  cursor: pointer;
  font-size: 14px;
  color: #464646;
}

.slot.active {
  background: #9b52df;
  border-color: #9b52df;
  color: #fff;
  font-weight: 500;
}

.field {
  margin-bottom: 12px;
  display: flex;
  flex-direction: column;
  gap: 5px;
}

label {
  font-size: 13px;
  font-weight: 500;
  color: #464646;
}

input,
textarea {
  border: 1px solid #e3e0e8;
  border-radius: 8px;
  padding: 10px 12px;
  font-size: 14px;
  font-family: inherit;
  outline: none;
  width: 100%;
  box-sizing: border-box;
}

input:focus,
textarea:focus {
  border-color: #9b52df;
  box-shadow: 0 0 0 3px rgba(155, 82, 223, 0.15);
}

.gotcha {
  position: absolute;
  left: -9999px;
  width: 1px;
  height: 1px;
  opacity: 0;
}

.submit {
  width: 100%;
  background: #9b52df;
  color: #fff;
  border: none;
  border-radius: 8px;
  padding: 12px;
  font-size: 15px;
  font-weight: 500;
  cursor: pointer;
  margin-top: 6px;
}

.submit:disabled {
  opacity: 0.6;
  cursor: default;
}

.error-text {
  color: #d64545;
  font-size: 13px;
  margin: 8px 0;
}

.muted {
  color: #8e8e8e;
  font-size: 14px;
}

.small {
  font-size: 12px;
}

.empty {
  text-align: center;
  padding: 24px 0;
}

.done {
  text-align: center;
  padding: 20px 0;
}

.check {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 48px;
  height: 48px;
  border-radius: 50%;
  background: #e6f5ec;
  color: #2e9e5b;
  font-size: 24px;
  margin-bottom: 12px;
}

.done p {
  margin: 0 0 6px;
  font-size: 15px;
  color: #464646;
  line-height: 1.5;
}
</style>
