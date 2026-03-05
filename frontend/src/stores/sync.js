import { defineStore } from 'pinia'
import { ref } from 'vue'
import api from '../services/api'

export const useSyncStore = defineStore('sync', () => {
  const loading = ref(false)
  const selectedNames = ref([])
  const selectedIdx = ref(null)
  const progress = ref({})        // { stationName: SyncProgress }
  const stationResults = ref({})  // { stationName: HourRecord[840] }
  const stationsExpected = ref([])
  const chartSig = ref(2)         // signal index 0-7 (default: Flow Min = Modes[0])

  function handleProgress(prog) {
    if (prog.station === '__done__') {
      loading.value = false
      const first = Object.keys(stationResults.value)[0]
      if (first) selectedIdx.value = first
      return
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
    stationsExpected.value = [...selectedNames.value]
    selectedIdx.value = null

    selectedNames.value.forEach(n => {
      progress.value[n] = { station: n, done: 0, total: 840, pct: 0 }
    })

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

  async function retryStation(stationName, stations, sessionId) {
    const records = stationResults.value[stationName]
    if (!records) return

    const failedPtrs = records.filter(r => !r.valid).map(r => r.ptr)
    if (!failedPtrs.length) return

    const cfg = stations.find(s => s.name === stationName)
    if (!cfg) return

    loading.value = true
    try {
      const { data } = await api.post('/stations/partial-sync',
        {
          ip: cfg.ip, port: cfg.port,
          slave_id: cfg.id, endian: cfg.endian,
          db_address: cfg.base_data_address,
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
        stationResults.value = { ...stationResults.value, [stationName]: updated }
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
