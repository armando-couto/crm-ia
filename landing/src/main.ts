import { createSSRApp } from 'vue'
import App from './App.vue'
import './style.css'

// O HTML já vem pronto do build (pré-renderizado): o Vue só assume o controle
// do que existe, mantendo o conteúdo visível desde o primeiro instante.
createSSRApp(App).mount('#app')
