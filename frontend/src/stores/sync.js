import { defineStore } from 'pinia'
import { ref } from 'vue'
import api from '../services/api'

export const useSyncStore = defineStore('sync', () => {
  const loading = ref(false)
  const selectedNames = ref([])
  const selectedIdx = ref(null)
  const progress = ref({})        // { taskKey: SyncProgress }
  const stationResults = ref({})  // { taskKey: HourRecord[840] }
  const stationsExpected = ref([])
  const chartSig = ref(2)         // signal index 0-7

  function handleProgress(prog) {
    if (prog.station === '__done__') {
      loading.value = false
      const first = Object.keys(stationResults.value)[0]
      if (first) selectedIdx.value = first
      return
    }
    // Register task key on first sight (backend expands medidores into task keys)
    if (!stationsExpected.value.includes(prog.station)) {
      stationsExpected.value = [...stationsExpected.value, prog.station]
    }
    progress.value = { ...progress.value, [prog.station]: prog }
    if (prog.pct === 100 && prog.records && prog.records.length > 0) {
      stationResults.value = { ...stationResults.value, [prog.station]: prog.records }
      if (!selectedIdx.value) selectedIdx.value = prog.station
    }
  }

  async function startFullSync(sessionId) {
    loading.value = true
    progress.value = {}
    stationResults.value = {}
    stationsExpected.value = []   // populated dynamically as progress events arrive
    selectedIdx.value = null

    try {
      await api.post('/stations/full-sync',
        { stations: selectedNames.value },
        { headers: { 'X-Session-ID': sessionId } }
      )
    } catch (e) {
      console.error(e)
      loading.value = false
    }
  }

  // taskKey may be "STATION" or "STATION / MEDIDOR"
  async function retryStation(taskKey, stations, sessionId) {
    const records = stationResults.value[taskKey]
    if (!records) return

    const failedPtrs = records.filter(r => !r.valid).map(r => r.ptr)
    if (!failedPtrs.length) return

    // Parse task key to find station config and medidor-specific db_address
    const parts = taskKey.split(' / ')
    const stationName = parts[0]
    const medidorName = parts[1] || null

    const cfg = stations.find(s => s.name === stationName)
    if (!cfg) return

    let dbAddress = cfg.base_data_address
    if (medidorName && cfg.medidores?.length) {
      const med = cfg.medidores.find(m => m.name === medidorName)
      if (med) dbAddress = med.base_data_address
    }

    loading.value = true
    try {
      const { data } = await api.post('/stations/partial-sync',
        {
          task_key: taskKey,
          ip: cfg.ip, port: cfg.port,
          slave_id: cfg.id, endian: cfg.endian,
          db_address: dbAddress,
          pointers: failedPtrs,
        },
        { headers: { 'X-Session-ID': sessionId } }
      )
      if (data.records) {
        const updated = [...records]
        data.records.forEach(newRec => {
          const idx = updated.findIndex(r => r.ptr === newRec.ptr)
          if (idx !== -1) updated[idx] = newRec
        })
        stationResults.value = { ...stationResults.value, [taskKey]: updated }
      }
    } catch (e) {
      console.error(e)
    } finally {
      loading.value = false
    }
  }

  return {
    loading, selectedNames, selectedIdx,
    progress, stationResults, stationsExpected, chartSig,
    handleProgress, startFullSync, retryStation,
  }
})
