<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import { initials, roleLabels } from '../format'
import GlobalSearch from './GlobalSearch.vue'

const router = useRouter()
const auth = useAuthStore()

function logout() {
  auth.logout()
  router.push({ name: 'login' })
}
</script>

<template>
  <div class="shell">
    <aside class="sidebar">
      <div class="brand">
        <span class="brand-mark">F</span>
        <span class="brand-name">Fix CRM</span>
      </div>

      <nav>
        <router-link v-if="auth.can('dashboard.view')" to="/" exact-active-class="active">
          <span class="icon">◧</span><span class="label">Dashboard</span>
        </router-link>
        <router-link v-if="auth.can('contacts.view')" to="/contatos" active-class="active">
          <span class="icon">☺</span><span class="label">Contatos</span>
        </router-link>
        <router-link v-if="auth.can('companies.view')" to="/empresas" active-class="active">
          <span class="icon">▣</span><span class="label">Empresas</span>
        </router-link>
        <router-link v-if="auth.can('deals.view')" to="/negocios" active-class="active">
          <span class="icon">◈</span><span class="label">Negócios</span>
        </router-link>
        <router-link v-if="auth.can('tickets.view')" to="/tickets" active-class="active">
          <span class="icon">◎</span><span class="label">Tickets</span>
        </router-link>
        <router-link v-if="auth.can('tasks.view')" to="/tarefas" active-class="active">
          <span class="icon">✓</span><span class="label">Tarefas</span>
        </router-link>
        <router-link v-if="auth.can('projects.view')" to="/projetos" active-class="active">
          <span class="icon">▤</span><span class="label">Projetos</span>
        </router-link>
        <router-link v-if="auth.can('lists.view')" to="/listas" active-class="active">
          <span class="icon">☰</span><span class="label">Listas</span>
        </router-link>
        <router-link to="/visualizacoes" active-class="active">
          <span class="icon">⊞</span><span class="label">Visualizações</span>
        </router-link>

        <div class="nav-section">Comunicação</div>
        <router-link v-if="auth.can('inbox.view')" to="/caixa-de-entrada" active-class="active">
          <span class="icon">✉</span><span class="label">Caixa de entrada</span>
        </router-link>
        <router-link v-if="auth.can('calls.view')" to="/chamadas" active-class="active">
          <span class="icon">☎</span><span class="label">Chamadas</span>
        </router-link>
        <router-link v-if="auth.can('meetings.view')" to="/reunioes" active-class="active">
          <span class="icon">⚑</span><span class="label">Reuniões</span>
        </router-link>

        <div class="nav-section">Biblioteca</div>
        <router-link v-if="auth.can('library.view')" to="/manuais" active-class="active">
          <span class="icon">✎</span><span class="label">Manuais</span>
        </router-link>
        <router-link v-if="auth.can('library.view')" to="/modelos" active-class="active">
          <span class="icon">▣</span><span class="label">Modelos</span>
        </router-link>
        <router-link v-if="auth.can('library.view')" to="/snippets" active-class="active">
          <span class="icon">❝</span><span class="label">Snippets</span>
        </router-link>

      </nav>

      <div class="sidebar-footer" v-if="auth.user">
        <router-link to="/configuracoes/perfil" class="user-chip">
          <span class="avatar">{{ initials(auth.user.name) }}</span>
          <span class="user-info">
            <strong>{{ auth.user.name }}</strong>
            <small>{{ roleLabels[auth.user.role] || auth.user.role }}</small>
          </span>
        </router-link>
        <button class="btn-logout" title="Sair" @click="logout">⎋</button>
      </div>
    </aside>

    <div class="main">
      <header class="topbar">
        <GlobalSearch />
        <router-link to="/configuracoes/perfil" class="topbar-gear" title="Configurações" active-class="on">⚙</router-link>
      </header>
      <main class="content">
        <slot />
      </main>
    </div>
  </div>
</template>

<style scoped>
.shell {
  display: flex;
  height: 100vh;
}

.sidebar {
  width: 230px;
  flex-shrink: 0;
  background: var(--fix-purple-deep);
  color: #e9e0f5;
  display: flex;
  flex-direction: column;
}

.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 20px 18px;
}

.brand-mark {
  width: 34px;
  height: 34px;
  border-radius: 9px;
  background: var(--fix-purple);
  color: #fff;
  font-weight: 700;
  font-size: 18px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.brand-name {
  font-weight: 600;
  font-size: 16px;
  color: #fff;
}

nav {
  flex: 1;
  padding: 8px 10px;
  overflow-y: auto;
}

nav a {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  border-radius: 8px;
  color: #cbb8e6;
  font-size: 14px;
  margin-bottom: 2px;
  transition: background 0.15s, color 0.15s;
}

nav a:hover {
  background: rgba(155, 82, 223, 0.18);
  color: #fff;
}

nav a.active {
  background: var(--fix-purple);
  color: #fff;
}

.icon {
  width: 18px;
  text-align: center;
  font-size: 15px;
}

.nav-section {
  padding: 16px 12px 6px;
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: #8d76ad;
}

.sidebar-footer {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 14px;
  border-top: 1px solid rgba(255, 255, 255, 0.08);
}

.user-chip {
  display: flex;
  align-items: center;
  gap: 10px;
  flex: 1;
  color: inherit;
  min-width: 0;
}

.avatar {
  width: 34px;
  height: 34px;
  border-radius: 50%;
  background: var(--fix-purple);
  color: #fff;
  font-size: 13px;
  font-weight: 600;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.user-info {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.user-info strong {
  font-size: 13px;
  color: #fff;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.user-info small {
  font-size: 11px;
  color: #8d76ad;
}

.btn-logout {
  background: none;
  border: none;
  color: #8d76ad;
  font-size: 16px;
  cursor: pointer;
  padding: 6px;
  border-radius: 6px;
}

.btn-logout:hover {
  color: #fff;
  background: rgba(255, 255, 255, 0.08);
}

.main {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.topbar {
  height: 56px;
  background: var(--fix-surface);
  border-bottom: 1px solid var(--fix-border);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 0 24px;
  flex-shrink: 0;
}

.topbar-gear {
  font-size: 18px;
  color: var(--fix-text-3);
  padding: 6px 10px;
  border-radius: 8px;
  line-height: 1;
  transition: background 0.15s, color 0.15s;
}

.topbar-gear:hover,
.topbar-gear.on {
  color: var(--fix-purple);
  background: var(--fix-purple-tint);
}

.content {
  flex: 1;
  overflow-y: auto;
}

@media (max-width: 820px) {
  .sidebar {
    width: 64px;
  }
  .brand-name,
  .user-info,
  .nav-section,
  nav a .label {
    display: none;
  }
  nav a {
    justify-content: center;
  }
}
</style>
