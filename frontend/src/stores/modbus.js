import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import api from '../services/api'

export const useModbusStore = defineStore('modbus', () => {
  const fc = ref(3)
  const startRaw = ref('0')
  const startAddr = ref(0)
  const addrHex = ref(false)
  const qty = ref(10)
  const writeHex = ref('')
  const loading = ref(false)
  const result = ref(null)
  const error = ref(null)

  const isWrite = computed(() => [5, 6, 15, 16].includes(+fc.value))

  const aduPreview = computed(() => {
    const h2 = n => n.toString(16).padStart(2, '0').toUpperCase()
    const h4 = n => n.toString(16).padStart(4, '0').toUpperCase()
    // slaveId injected at call time; preview uses placeholder 01
    return `0001 0000 0006 01 ${h2(+fc.value)} ${h4(+startAddr.value)} ${h4(+qty.value)}`
  })

  function parseAddr() {
    const raw = startRaw.value.trim().replace(/^0x/i, '')
    startAddr.value = addrHex.value ? parseInt(raw, 16) || 0 : parseInt(raw, 10) || 0
  }

  async function sendQuery(conn) {
    parseAddr()
    loading.value = true
    error.value = null
    result.value = null
    try {
      const { data } = await api.post('/query', {
        ip: conn.ip,
        port: +conn.port,
        slave_id: +conn.slaveId,
        fc: +fc.value,
        start_address: +startAddr.value,
        quantity: +qty.value,
        write_data_hex: writeHex.value,
        endianness: conn.endian,
      })
      if (data.error) error.value = data.error
      else result.value = data
    } catch (e) {
      error.value = e.message
    } finally {
      loading.value = false
    }
  }

  function clear() {
    result.value = null
    error.value = null
  }

  return {
    fc, startRaw, startAddr, addrHex, qty, writeHex,
    loading, result, error,
    isWrite, aduPreview,
    parseAddr, sendQuery, clear,
  }
})
