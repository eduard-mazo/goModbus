import { defineStore } from 'pinia'
import { ref } from 'vue'
import api from '../services/api'

export const useSyncStore = defineStore('sync', () => {
  const loading = ref(false)
  const selectedNames = ref([])
  const selectedIdx = ref(null)
  const viewTab = ref('chart')   // 'chart' | 'table'
  const progress = ref({})        // { taskKey: SyncProgress }
  const stationResults = ref({})  // { taskKey: HourRecord[] } — chronological, variable length
  const stationsExpected = ref([])

  function handleProgress(prog) {
    if (prog.station === '__done__') {
      loading.value = false
      const first = Object.keys(stationResults.value)[0]
      if (first) selectedIdx.value = first
      return
    }
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
    stationsExpected.value = []
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

  // Load all history from DB without connecting to any device.
  async function loadFromDB(stationNames) {
    try {
      const { data } = await api.post('/stations/load-db', {
        stations: stationNames && stationNames.length ? stationNames : [],
      })
      let found = false
      for (const [key, result] of Object.entries(data)) {
        if (result.records && result.records.length > 0) {
          stationResults.value = { ...stationResults.value, [key]: result.records }
          if (!stationsExpected.value.includes(key)) {
            stationsExpected.value = [...stationsExpected.value, key]
          }
          if (!selectedIdx.value) selectedIdx.value = key
          found = true
        }
      }
      return found
    } catch (e) {
      console.error(e)
      return false
    }
  }

  async function retryStation(taskKey, stations, sessionId) {
    const records = stationResults.value[taskKey]
    if (!records) return

    const failedPtrs = records.filter(r => !r.valid).map(r => r.ptr)
    if (!failedPtrs.length) return

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
        // Partial-sync returns records with ts; merge by ptr
        const updated = [...records]
        data.records.forEach(newRec => {
          const idx = updated.findIndex(r => r.ptr === newRec.ptr)
          if (idx !== -1) updated[idx] = newRec
          else updated.push(newRec)
        })
        updated.sort((a, b) => (a.ts || 0) - (b.ts || 0))
        stationResults.value = { ...stationResults.value, [taskKey]: updated }
      }
    } catch (e) {
      console.error(e)
    } finally {
      loading.value = false
    }
  }

  return {
    loading, selectedNames, selectedIdx, viewTab,
    progress, stationResults, stationsExpected,
    handleProgress, startFullSync, loadFromDB, retryStation,
  }
})
