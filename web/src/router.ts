import { createRouter, createWebHistory } from 'vue-router'
import { getToken } from './api'

const routes = [
  { path: '/login', name: 'login', component: () => import('./views/LoginView.vue'), meta: { public: true } },
  { path: '/esqueci-senha', name: 'forgot', component: () => import('./views/ForgotPasswordView.vue'), meta: { public: true } },
  { path: '/redefinir-senha', name: 'reset', component: () => import('./views/ResetPasswordView.vue'), meta: { public: true } },
  { path: '/', name: 'dashboard', component: () => import('./views/DashboardView.vue') },
  { path: '/contatos', name: 'contacts', component: () => import('./views/ContactsView.vue') },
  { path: '/contatos/:id', name: 'contact-detail', component: () => import('./views/ContactDetailView.vue') },
  { path: '/empresas', name: 'companies', component: () => import('./views/CompaniesView.vue') },
  { path: '/empresas/:id', name: 'company-detail', component: () => import('./views/CompanyDetailView.vue') },
  { path: '/negocios', name: 'deals', component: () => import('./views/DealsBoardView.vue') },
  { path: '/negocios/:id', name: 'deal-detail', component: () => import('./views/DealDetailView.vue') },
  { path: '/tarefas', name: 'tasks', component: () => import('./views/TasksView.vue') },
  { path: '/configuracoes/usuarios', name: 'settings-users', component: () => import('./views/SettingsUsersView.vue') },
  { path: '/configuracoes/pipelines', name: 'settings-pipelines', component: () => import('./views/SettingsPipelinesView.vue') },
  { path: '/minha-conta', name: 'profile', component: () => import('./views/ProfileView.vue') },
  { path: '/:pathMatch(.*)*', redirect: '/' }
]

export const router = createRouter({
  history: createWebHistory(),
  routes
})

router.beforeEach((to) => {
  if (!to.meta.public && !getToken()) {
    return { name: 'login', query: to.fullPath !== '/' ? { r: to.fullPath } : {} }
  }
  if (to.name === 'login' && getToken()) {
    return { name: 'dashboard' }
  }
})
