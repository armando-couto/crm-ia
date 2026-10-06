// Entrada de servidor: usada só no build, para gerar o HTML pronto e os dados
// estruturados a partir do mesmo conteúdo que a página mostra.
import { createSSRApp } from 'vue'
import { renderToString } from '@vue/server-renderer'
import App from './App.vue'

export { criarFaq, grupos } from './conteudo'

export async function renderizar(): Promise<string> {
  return renderToString(createSSRApp(App))
}
