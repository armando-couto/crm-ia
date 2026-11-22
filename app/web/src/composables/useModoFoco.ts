import { onBeforeUnmount, onMounted } from 'vue'

/** Classe do `body` que esconde menu lateral e barra do topo (regra em styles.css). */
export const CLASSE_MODO_FOCO = 'modo-foco'

/**
 * Liga o modo foco enquanto o componente está montado: adiciona a classe no
 * `onMounted` e remove no `onBeforeUnmount` (fila de ligações).
 */
export function useModoFoco(classe: string = CLASSE_MODO_FOCO): void {
  onMounted(() => document.body.classList.add(classe))
  onBeforeUnmount(() => document.body.classList.remove(classe))
}
