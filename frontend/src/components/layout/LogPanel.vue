<template>
  <div class="terminal shrink-0 border-t border-g-800" :style="logsStore.logOpen ? 'height:180px;' : 'height:30px;'">

    <!-- Header -->
    <div class="flex items-center gap-2 px-3 h-[30px] border-b" style="border-color:#1a2a1a;">
      <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="#7ad400" stroke-width="2"><polyline points="4 17 10 11 4 5"/><line x1="12" y1="19" x2="20" y2="19"/></svg>
      <span class="text-xs font-semibold" style="color:#7ad400;">Terminal</span>

      <div class="flex gap-1 ml-2">
        <button
          v-for="lvl in ['INFO','DEBUG','ERROR']" :key="lvl"
          class="text-xs px-1.5 rounded font-mono transition-opacity"
          :class="`log-${lvl}`"
          :style="logsStore.logFilters.includes(lvl) ? 'opacity:1;' : 'opacity:0.25;'"
          @click="logsStore.toggleFilter(lvl)"
        >{{ lvl }}</button>
      </div>

      <span class="text-xs ml-auto" style="color:#4d6b4d;">{{ logsStore.filteredLogs.length }} líneas</span>
      <button class="text-xs px-1.5" style="color:#4d6b4d;" @click="logsStore.clearLogs()">limpiar</button>
      <button class="text-xs" style="color:#4d6b4d;" @click="logsStore.logOpen = !logsStore.logOpen">
        {{ logsStore.logOpen ? '▼' : '▲' }}
      </button>
    </div>

    <!-- Log entries -->
    <div
      v-if="logsStore.logOpen"
      ref="logContainer"
      class="overflow-y-auto h-[150px] p-2 space-y-0.5"
    >
      <div
        v-for="(m, i) in logsStore.filteredLogs" :key="i"
        class="flex gap-2 leading-5"
      >
        <span class="shrink-0 w-[90px]" style="color:#4d6b4d;">{{ m.ts }}</span>
        <span class="shrink-0 w-[42px] font-semibold" :class="`log-${m.level}`">{{ m.level }}</span>
        <span style="color:#c5cfc5;">{{ m.msg }}</span>
        <span v-if="m.dur" class="ml-1" style="color:#4d6b4d;">[{{ m.dur }}]</span>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, watch, nextTick } from 'vue'
import { useLogsStore } from '../../stores/logs'

const logsStore = useLogsStore()
const logContainer = ref(null)

watch(() => logsStore.filteredLogs.length, async () => {
  if (logsStore.logOpen) {
    await nextTick()
    if (logContainer.value) logContainer.value.scrollTop = logContainer.value.scrollHeight
  }
})
</script>
