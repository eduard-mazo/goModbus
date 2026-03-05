import { defineStore } from 'pinia'
import { ref } from 'vue'
import api from '../services/api'

export const useRocStore = defineStore('roc', () => {
  // Pointer config
  const ptrAddr = ref(10000)
  const ptrEndian = ref('cdab')
  const ptrQty = ref(2)

  // DB config (shared with H24)
  const dbAddr = ref(700)
  const dbEndian = ref('cdab')

  // ROC single
  const mode = ref('full') // 'ptr' | 'hist' | 'full'
  const manualPtr = ref(0)
  const loading = ref(false)
  const result = ref(null)
  const error = ref(null)

  // H24
  const h24 = ref({
    dbQty: 2,
    bufSize: 840,
    overrideHour: false,
    customHour: 0,
    loading: false,
    result: null,
    error: null,
  })

  function applyStation(st) {
    ptrAddr.value = st.pointer_address
    ptrEndian.value = st.endian
    ptrQty.value = st.data_registers_count
    dbAddr.value = st.base_data_address
    dbEndian.value = st.endian
  }

  async function sendROC(conn) {
    loading.value = true
    error.value = null
    result.value = null
    try {
      const p = {
        ip: conn.ip, port: +conn.port, slave_id: +conn.slaveId,
        ptr_endian: ptrEndian.value,
        ptr_addr: +ptrAddr.value,
        ptr_qty: +ptrQty.value,
        db_endian: dbEndian.value,
        db_addr: +dbAddr.value,
        mode: mode.value,
      }
      if (mode.value === 'hist') p.manual_ptr = +manualPtr.value
      const { data } = await api.post('/roc', p)
      if (data.error) error.value = data.error
      else result.value = data
    } catch (e) {
      error.value = e.message
    } finally {
      loading.value = false
    }
  }

  async function fetchH24(conn) {
    h24.value.loading = true
    h24.value.error = null
    h24.value.result = null
    try {
      const p = {
        ip: conn.ip, port: +conn.port, slave_id: +conn.slaveId,
        ptr_endian: ptrEndian.value,
        ptr_addr: +ptrAddr.value,
        ptr_qty: +ptrQty.value,
        db_endian: dbEndian.value,
        db_addr: +dbAddr.value,
        db_qty: +h24.value.dbQty,
        buf_size: +h24.value.bufSize,
      }
      if (h24.value.overrideHour) p.current_hour = +h24.value.customHour
      const { data } = await api.post('/roc/history24', p)
      if (data.error) h24.value.error = data.error
      else h24.value.result = data
    } catch (e) {
      h24.value.error = e.message
    } finally {
      h24.value.loading = false
    }
  }

  function clear() { result.value = null; error.value = null }

  return {
    ptrAddr, ptrEndian, ptrQty, dbAddr, dbEndian,
    mode, manualPtr, loading, result, error, h24,
    applyStation, sendROC, fetchH24, clear,
  }
})
