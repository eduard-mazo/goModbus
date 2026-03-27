import { defineStore } from 'pinia'
import { ref } from 'vue'
import axios from 'axios'

// Shared config store — single source of truth for station config.
// Any component that saves config should call load() to refresh all consumers.
export const useConfigStore = defineStore('config', () => {
  const stations = ref([])

  async function load() {
    try {
      const { data } = await axios.get('/api/config')
      stations.value = data.stations || []
    } catch (_) {}
  }

  // Returns signal_names for a given task key ("STATION" or "STATION / M1").
  // Medidor-level names take precedence over station-level names.
  function signalNamesFor(taskKey) {
    const [stName, medName] = taskKey.split(' / ')
    const st = stations.value.find(s => s.name === stName)
    if (!st) return []
    if (medName && st.medidores?.length) {
      const med = st.medidores.find(m => m.name === medName)
      if (med?.signal_names?.length) return med.signal_names
    }
    return st.signal_names || []
  }

  return { stations, load, signalNamesFor }
})
