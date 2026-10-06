<script setup lang="ts">
import { ref } from 'vue'
import { segmentos } from '../conteudo'
defineProps<{ planos: any[] }>()
const f = ref({ nome: '', email: '', telefone: '', empresa: '', segmento: '', cidade: '', usuarios: 3, plano: '', mensagem: '' })
const enviando = ref(false)
const ok = ref(false)
const erro = ref('')
async function enviar() {
  enviando.value = true
  erro.value = ''
  try {
    const r = await fetch('/painel/api/leads', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ ...f.value, usuarios: Number(f.value.usuarios) }) })
    if (!r.ok) throw new Error((await r.json()).erro || 'erro')
    ok.value = true
  } catch {
    erro.value = 'Não foi possível enviar agora. Tente de novo ou escreva para o nosso e-mail.'
  } finally {
    enviando.value = false
  }
}
</script>
<template>
  <div class="cartao-contato">
    <div v-if="ok" class="ok" role="status">Recebemos seu contato! Em breve falamos com você para preparar o seu ambiente.</div>
    <form v-else class="form" @submit.prevent="enviar">
      <label>Seu nome *<input v-model="f.nome" name="nome" autocomplete="name" required /></label>
      <label>E-mail *<input v-model="f.email" name="email" type="email" autocomplete="email" required /></label>
      <label>WhatsApp<input v-model="f.telefone" name="telefone" type="tel" autocomplete="tel" /></label>
      <label>Empresa<input v-model="f.empresa" name="empresa" autocomplete="organization" /></label>
      <label>Segmento<select v-model="f.segmento" name="segmento"><option value="">Selecione</option><option v-for="s in segmentos" :key="s.nome">{{ s.nome }}</option></select></label>
      <label>Cidade/UF<input v-model="f.cidade" name="cidade" /></label>
      <label>Quantas pessoas vão usar?<input v-model="f.usuarios" name="usuarios" type="number" min="1" /></label>
      <label>Plano de interesse<select v-model="f.plano" name="plano"><option value="">Ainda não sei</option><option v-for="p in (planos || [])" :key="p.codigo" :value="p.codigo">{{ p.nome }} — {{ p.faixa }}</option></select></label>
      <label class="full">Mensagem<textarea v-model="f.mensagem" name="mensagem" placeholder="Como vocês vendem hoje e o que mais atrapalha"></textarea></label>
      <div v-if="erro" class="erro full" role="alert">{{ erro }}</div>
      <button class="btn btn--p btn--g full" :disabled="enviando">{{ enviando ? 'Enviando…' : 'Quero testar grátis' }}</button>
      <p class="form__nota full">Ao enviar, você concorda em receber nosso contato sobre este pedido.</p>
    </form>
  </div>
</template>
