import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

export const useLogsStore = defineStore('logs', () => {
  const logs = ref([])
  const logOpen = ref(true)
  const logFilters = ref(['INFO', 'DEBUG', 'ERROR'])
  const wsConnected = ref(false)

  const filteredLogs = computed(() =>
    logs.value.filter(m => logFilters.value.includes(m.level)).slice(-200)
  )

  function addLog(msg) {
    logs.value.push(msg)
    if (logs.value.length > 500) logs.value.shift()
  }

  function clearLogs() {
    logs.value = []
  }

  function toggleFilter(lvl) {
    if (logFilters.value.includes(lvl)) {
      logFilters.value = logFilters.value.filter(f => f !== lvl)
    } else {
      logFilters.value = [...logFilters.value, lvl]
    }
  }

  return {
    logs, logOpen, logFilters, wsConnected,
    filteredLogs, addLog, clearLogs, toggleFilter,
  }
})
