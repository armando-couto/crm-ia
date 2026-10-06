<script setup lang="ts">
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
</script>

<template>
  <div class="settings">
    <aside class="settings-menu">
      <h2>Configurações</h2>

      <div class="menu-section">Suas preferências</div>
      <router-link to="/configuracoes/perfil" active-class="active">Perfil e segurança</router-link>
      <router-link to="/configuracoes/notificacoes" active-class="active">Notificações</router-link>
      <router-link v-if="auth.can('meetings.manage')" to="/configuracoes/agendamento" active-class="active">
        Agendamento
      </router-link>

      <template v-if="auth.canManage">
        <div class="menu-section">Empresa</div>
        <router-link v-if="auth.can('settings.users')" to="/configuracoes/empresa" active-class="active">Identidade</router-link>
        <router-link v-if="auth.can('settings.users')" to="/configuracoes/email" active-class="active">Envio de e-mails</router-link>
        <router-link v-if="auth.can('settings.users')" to="/configuracoes/disparo" active-class="active">Disparo</router-link>
        <router-link v-if="auth.can('settings.users') && auth.workspace?.plan_enabled" to="/configuracoes/plano" active-class="active">Meu plano</router-link>
        <router-link v-if="auth.can('settings.users')" to="/configurar" active-class="active">Assistente inicial</router-link>

        <div class="menu-section">Administração</div>
        <router-link v-if="auth.can('settings.users')" to="/configuracoes/usuarios" active-class="active">
          Usuários e equipes
        </router-link>
        <router-link v-if="auth.can('settings.permissions')" to="/configuracoes/permissoes" active-class="active">
          Permissões
        </router-link>
        <router-link v-if="auth.can('settings.properties')" to="/configuracoes/propriedades" active-class="active">
          Propriedades
        </router-link>
        <router-link v-if="auth.can('settings.pipelines')" to="/configuracoes/pipelines" active-class="active">
          Pipelines
        </router-link>
        <router-link v-if="auth.can('settings.forms')" to="/configuracoes/formularios" active-class="active">
          Formulários
        </router-link>
        <router-link v-if="auth.can('settings.audit')" to="/configuracoes/auditoria" active-class="active">
          Auditoria
        </router-link>
        <router-link v-if="auth.can('records.merge')" to="/duplicados" active-class="active">
          Duplicados
        </router-link>
        <router-link to="/visualizacoes" active-class="active">Visualizações</router-link>
        <router-link v-if="auth.can('lists.view')" to="/listas" active-class="active">Listas</router-link>
      </template>
    </aside>

    <section class="settings-content">
      <router-view />
    </section>
  </div>
</template>

<style scoped>
.settings {
  display: grid;
  grid-template-columns: 240px minmax(0, 1fr);
  min-height: 100%;
}

@media (max-width: 800px) {
  .settings {
    grid-template-columns: 1fr;
  }
  .settings-menu {
    border-right: none !important;
    border-bottom: 1px solid var(--ci-border);
  }
}

.settings-menu {
  background: var(--ci-surface);
  border-right: 1px solid var(--ci-border);
  padding: 22px 14px;
}

.settings-menu h2 {
  font-size: 17px;
  padding: 0 10px 14px;
}

.menu-section {
  padding: 14px 10px 6px;
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--ci-text-3);
  font-weight: 600;
}

.settings-menu a {
  display: block;
  padding: 8px 10px;
  border-radius: 8px;
  color: var(--ci-text-2);
  font-size: 14px;
  margin-bottom: 2px;
}

.settings-menu a:hover {
  background: var(--ci-purple-tint);
  color: var(--ci-purple-dark);
}

.settings-menu a.active {
  background: var(--ci-purple);
  color: #fff;
  font-weight: 500;
}

.settings-content {
  min-width: 0;
}
</style>
