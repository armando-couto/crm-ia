<script setup lang="ts">
// Ícones de traço embutidos: a landing não carrega imagem nem fonte de terceiros.
defineProps<{ nome: string }>()
const tracos: Record<string, string> = {
  pessoas: 'M9 11a3.5 3.5 0 100-7 3.5 3.5 0 000 7z M2.5 20c0-3.3 2.9-6 6.5-6s6.5 2.7 6.5 6 M16 4.5a3.5 3.5 0 010 6.5 M18 14.5c2 .8 3.5 2.9 3.5 5.5',
  kanban: 'M3 4h5v16H3z M9.5 4h5v10h-5z M16 4h5v7h-5z',
  check: 'M5 12l5 5 9-10',
  relogio: 'M12 21a9 9 0 100-18 9 9 0 000 18z M12 7v5l3 2',
  seta: 'M4 12h14 M13 6l6 6-6 6',
  raio: 'M13 2L4 14h7l-1 8 9-12h-7z',
  calendario: 'M4 6h16v14H4z M4 10h16 M8 3v4 M16 3v4',
  grafico: 'M4 20V4 M4 20h16 M8 16v-4 M12 16V8 M16 16v-6',
  email: 'M3 6h18v12H3z M3 7l9 6 9-6',
  caixa: 'M3 8l9-5 9 5v8l-9 5-9-5V8z M3 8l9 5 9-5 M12 13v8',
  modelo: 'M6 3h8l4 4v14H6z M14 3v4h4 M9 12h6 M9 16h6',
  formulario: 'M4 5h16v14H4z M8 9h8 M8 13h5',
  alvo: 'M12 21a9 9 0 100-18 9 9 0 000 18z M12 16a4 4 0 100-8 4 4 0 000 8z M12 12h.01',
  escudo: 'M12 3l8 3v6c0 4.5-3.4 8-8 9-4.6-1-8-4.5-8-9V6z M9 12l2 2 4-4',
  nuvem: 'M7 18a4.5 4.5 0 01-.5-9A6 6 0 0118 10.5a3.8 3.8 0 01-.5 7.5z',
  faisca: 'M12 3l1.8 5.2L19 10l-5.2 1.8L12 17l-1.8-5.2L5 10l5.2-1.8z M19 17l.7 2 2 .7-2 .7-.7 2-.7-2-2-.7 2-.7z',
  pasta: 'M3 7h6l2 2h10v10H3z M3 7V5h7',
  loja: 'M4 9l1.5-5h13L20 9 M4 9v11h16V9 M4 9h16 M9 20v-6h6v6',
  casa: 'M4 11l8-7 8 7 M6 10v10h12V10 M10 20v-6h4v6',
  carrinho: 'M3 4h2l2.5 11h11L21 7H6 M9 20a1 1 0 100-2 1 1 0 000 2z M17 20a1 1 0 100-2 1 1 0 000 2z',
  lampada: 'M9 18h6 M10 21h4 M12 3a6 6 0 00-3.5 10.9c.5.4.5 1 .5 1.6V17h6v-1.5c0-.6 0-1.2.5-1.6A6 6 0 0012 3z',
  cartao: 'M3 6h18v12H3z M3 10h18 M7 15h4',
  celular: 'M7 3h10v18H7z M11 18h2'
}
</script>
<template>
  <svg class="ic" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path :d="tracos[nome] || tracos.check" /></svg>
</template>
