import { defineStore } from 'pinia'

export interface Toast {
  id: number
  message: string
  kind: 'success' | 'error'
}

let seq = 0

export const useToastStore = defineStore('toast', {
  state: () => ({
    toasts: [] as Toast[]
  }),

  actions: {
    push(message: string, kind: 'success' | 'error' = 'success'): void {
      const id = ++seq
      this.toasts.push({ id, message, kind })
      setTimeout(() => this.dismiss(id), 4000)
    },

    error(message: string): void {
      this.push(message, 'error')
    },

    dismiss(id: number): void {
      this.toasts = this.toasts.filter((t) => t.id !== id)
    }
  }
})
