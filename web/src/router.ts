import { createRouter, createWebHistory } from 'vue-router'
import { getToken } from './api'
import { useAuthStore } from './stores/auth'

const routes = [
  { path: '/login', name: 'login', component: () => import('./views/LoginView.vue'), meta: { public: true } },
  { path: '/esqueci-senha', name: 'forgot', component: () => import('./views/ForgotPasswordView.vue'), meta: { public: true } },
  { path: '/redefinir-senha', name: 'reset', component: () => import('./views/ResetPasswordView.vue'), meta: { public: true } },
  {
    path: '/trocar-senha',
    name: 'change-password',
    component: () => import('./views/ChangePasswordView.vue'),
    meta: { forcePassword: true }
  },
  {
    path: '/agendar/:slug',
    name: 'public-booking',
    component: () => import('./views/PublicBookingView.vue'),
    meta: { public: true }
  },
  {
    path: '/f/:slug',
    name: 'public-form',
    component: () => import('./views/PublicFormView.vue'),
    meta: { public: true }
  },
  {
    path: '/',
    name: 'dashboard',
    component: () => import('./views/DashboardView.vue'),
    meta: { permission: 'dashboard.view' }
  },
  {
    path: '/contatos',
    name: 'contacts',
    component: () => import('./views/ContactsView.vue'),
    meta: { permission: 'contacts.view' }
  },
  {
    path: '/contatos/:id',
    name: 'contact-detail',
    component: () => import('./views/ContactDetailView.vue'),
    meta: { permission: 'contacts.view' }
  },
  {
    path: '/duplicados',
    name: 'duplicates',
    component: () => import('./views/DuplicatesView.vue'),
    meta: { permission: 'contacts.view' }
  },
  {
    path: '/vendas',
    name: 'sales-workspace',
    component: () => import('./views/SalesWorkspaceView.vue')
  },
  {
    path: '/atividades',
    name: 'activities',
    component: () => import('./views/ActivitiesView.vue')
  },
  {
    path: '/contas-alvo',
    name: 'target-accounts',
    component: () => import('./views/TargetAccountsView.vue'),
    meta: { permission: 'companies.view' }
  },
  {
    path: '/empresas',
    name: 'companies',
    component: () => import('./views/CompaniesView.vue'),
    meta: { permission: 'companies.view' }
  },
  {
    path: '/empresas/:id',
    name: 'company-detail',
    component: () => import('./views/CompanyDetailView.vue'),
    meta: { permission: 'companies.view' }
  },
  {
    path: '/negocios',
    name: 'deals',
    component: () => import('./views/DealsBoardView.vue'),
    meta: { permission: 'deals.view' }
  },
  {
    path: '/negocios/:id',
    name: 'deal-detail',
    component: () => import('./views/DealDetailView.vue'),
    meta: { permission: 'deals.view' }
  },
  {
    path: '/tarefas',
    name: 'tasks',
    component: () => import('./views/TasksView.vue'),
    meta: { permission: 'tasks.view' }
  },
  {
    path: '/tickets',
    name: 'tickets',
    component: () => import('./views/TicketsView.vue'),
    meta: { permission: 'tickets.view' }
  },
  {
    path: '/tickets/:id',
    name: 'ticket-detail',
    component: () => import('./views/TicketDetailView.vue'),
    meta: { permission: 'tickets.view' }
  },
  {
    path: '/listas',
    name: 'lists',
    component: () => import('./views/ListsView.vue'),
    meta: { permission: 'lists.view' }
  },
  {
    path: '/listas/:id',
    name: 'list-detail',
    component: () => import('./views/ListDetailView.vue'),
    meta: { permission: 'lists.view' }
  },
  { path: '/visualizacoes', name: 'saved-views', component: () => import('./views/SavedViewsView.vue') },
  {
    path: '/projetos',
    name: 'projects',
    component: () => import('./views/ProjectsView.vue'),
    meta: { permission: 'projects.view' }
  },
  {
    path: '/projetos/:id',
    name: 'project-detail',
    component: () => import('./views/ProjectDetailView.vue'),
    meta: { permission: 'projects.view' }
  },
  {
    path: '/caixa-de-entrada',
    name: 'inbox',
    component: () => import('./views/InboxView.vue'),
    meta: { permission: 'inbox.view' }
  },
  {
    path: '/emails',
    name: 'emails',
    component: () => import('./views/EmailsView.vue'),
    meta: { permission: 'email.send' }
  },
  {
    path: '/chamadas',
    name: 'calls',
    component: () => import('./views/CallsView.vue'),
    meta: { permission: 'calls.view' }
  },
  {
    path: '/reunioes',
    name: 'meetings',
    component: () => import('./views/MeetingsView.vue'),
    meta: { permission: 'meetings.view' }
  },
  {
    path: '/previsao',
    name: 'forecast',
    component: () => import('./views/ForecastView.vue'),
    meta: { permission: 'forecast.view' }
  },
  {
    path: '/analise-de-vendas',
    name: 'sales-analytics',
    component: () => import('./views/SalesAnalyticsView.vue'),
    meta: { permission: 'forecast.view' }
  },
  {
    path: '/metas',
    name: 'goals',
    component: () => import('./views/GoalsView.vue'),
    meta: { permission: 'forecast.view' }
  },
  {
    path: '/relatorios',
    name: 'reports',
    component: () => import('./views/ReportsView.vue'),
    meta: { permission: 'reports.view' }
  },
  {
    path: '/sequencias',
    name: 'sequences',
    component: () => import('./views/SequencesPage.vue'),
    meta: { permission: 'automations.view' }
  },
  {
    path: '/automacoes',
    name: 'automations',
    component: () => import('./views/AutomationsView.vue'),
    meta: { permission: 'automations.view' }
  },
  {
    path: '/manuais',
    name: 'playbooks',
    component: () => import('./views/PlaybooksView.vue'),
    meta: { permission: 'library.view' }
  },
  {
    path: '/modelos',
    name: 'templates',
    component: () => import('./views/TemplatesView.vue'),
    meta: { permission: 'library.view' }
  },
  {
    path: '/snippets',
    name: 'snippets',
    component: () => import('./views/SnippetsView.vue'),
    meta: { permission: 'library.view' }
  },
  {
    path: '/configuracoes',
    component: () => import('./views/SettingsLayout.vue'),
    children: [
      { path: '', redirect: '/configuracoes/perfil' },
      { path: 'perfil', name: 'profile', component: () => import('./views/ProfileView.vue') },
      { path: 'notificacoes', name: 'notifications', component: () => import('./views/NotificationsView.vue') },
      {
        path: 'usuarios',
        name: 'settings-users',
        component: () => import('./views/SettingsUsersView.vue'),
        meta: { permission: 'settings.users' }
      },
      {
        path: 'permissoes',
        name: 'settings-permissions',
        component: () => import('./views/SettingsPermissionsView.vue'),
        meta: { permission: 'settings.permissions' }
      },
      {
        path: 'propriedades',
        name: 'settings-properties',
        component: () => import('./views/SettingsPropertiesView.vue'),
        meta: { permission: 'settings.properties' }
      },
      {
        path: 'pipelines',
        name: 'settings-pipelines',
        component: () => import('./views/SettingsPipelinesView.vue'),
        meta: { permission: 'settings.pipelines' }
      },
      {
        path: 'agendamento',
        name: 'settings-booking',
        component: () => import('./views/SettingsBookingView.vue'),
        meta: { permission: 'meetings.manage' }
      },
      {
        path: 'formularios',
        name: 'settings-forms',
        component: () => import('./views/SettingsFormsView.vue'),
        meta: { permission: 'settings.forms' }
      },
      {
        path: 'auditoria',
        name: 'settings-audit',
        component: () => import('./views/SettingsAuditView.vue'),
        meta: { permission: 'settings.audit' }
      }
    ]
  },
  { path: '/minha-conta', redirect: '/configuracoes/perfil' },
  { path: '/:pathMatch(.*)*', redirect: '/' }
]

export const router = createRouter({
  history: createWebHistory(),
  routes
})

router.beforeEach(async (to) => {
  if (!to.meta.public && !getToken()) {
    return { name: 'login', query: to.fullPath !== '/' ? { r: to.fullPath } : {} }
  }
  if (to.name === 'login' && getToken()) {
    return { name: 'dashboard' }
  }
  if (to.meta.public) return

  const auth = useAuthStore()
  if (getToken() && !auth.user) await auth.fetchMe()

  // Senha temporária pendente: só a tela de troca fica acessível.
  if (auth.mustChangePassword) {
    return to.meta.forcePassword ? undefined : { name: 'change-password' }
  }
  if (to.meta.forcePassword) {
    return { name: 'dashboard' }
  }

  // Bloqueia rotas sem permissão no perfil do usuário.
  const permission = to.meta.permission as string | undefined
  if (permission && !auth.can(permission)) {
    return { name: 'profile' }
  }
})
