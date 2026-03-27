<template>
  <div class="flex h-screen overflow-hidden">
    <!-- Sidebar -->
    <AppSidebar />

    <!-- Main area: nav tabs + content + log panel -->
    <div class="flex flex-col flex-1 overflow-hidden">

      <!-- Tab bar -->
      <div class="flex items-center bg-white border-b border-g-200 px-2 shrink-0">
        <router-link to="/query" custom v-slot="{ navigate, isActive }">
          <button class="tab" :class="{ active: isActive }" @click="navigate">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 3H5a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V9"/><polyline points="9 3 9 9 15 9"/><line x1="15" y1="3" x2="21" y2="3"/><line x1="21" y1="3" x2="21" y2="9"/></svg>
            Consulta Modbus
          </button>
        </router-link>
        <router-link to="/roc" custom v-slot="{ navigate, isActive }">
          <button class="tab" :class="{ active: isActive }" @click="navigate">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="3"/><path d="M12 2v3M12 19v3M4.22 4.22l2.12 2.12M17.66 17.66l2.12 2.12M2 12h3M19 12h3M4.22 19.78l2.12-2.12M17.66 6.34l2.12-2.12"/></svg>
            ROC Expert
          </button>
        </router-link>
        <router-link to="/raw" custom v-slot="{ navigate, isActive }">
          <button class="tab" :class="{ active: isActive }" @click="navigate">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="16 18 22 12 16 6"/><polyline points="8 6 2 12 8 18"/></svg>
            Trama RAW
          </button>
        </router-link>
        <router-link to="/sync" custom v-slot="{ navigate, isActive }">
          <button class="tab" :class="{ active: isActive }" @click="navigate">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="1 4 1 10 7 10"/><path d="M3.51 15a9 9 0 1 0 .49-3.51"/></svg>
            Sincronización Total
          </button>
        </router-link>
        <router-link to="/config" custom v-slot="{ navigate, isActive }">
          <button class="tab" :class="{ active: isActive }" @click="navigate">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="3"/><path d="M19.07 4.93a10 10 0 0 1 0 14.14M4.93 4.93a10 10 0 0 0 0 14.14"/><path d="M12 2v2M12 20v2M2 12h2M20 12h2"/></svg>
            Configuración
          </button>
        </router-link>

        <div class="ml-auto flex items-center gap-2 pr-2">
          <span class="text-xs font-mono" :class="logsStore.wsConnected ? 'text-lime' : 'text-red-400'">
            {{ logsStore.wsConnected ? '● WS' : '○ WS' }}
          </span>
        </div>
      </div>

      <!-- Page content -->
      <div class="flex-1 overflow-hidden flex flex-col">
        <router-view />
      </div>

      <!-- Log panel -->
      <LogPanel :logContainerRef="logContainerRef" />
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useLogsStore } from './stores/logs'
import { connectWS } from './services/websocket'
import AppSidebar from './components/layout/AppSidebar.vue'
import LogPanel from './components/layout/LogPanel.vue'

const logsStore = useLogsStore()
const logContainerRef = ref(null)

onMounted(() => {
  connectWS(logContainerRef)
})
</script>
