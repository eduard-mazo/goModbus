import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useConnectionStore = defineStore('connection', () => {
  const ip = ref('192.168.1.1')
  const port = ref(502)
  const slaveId = ref(1)
  const endian = ref('abcd')

  function applyStation(st) {
    ip.value = st.ip
    port.value = st.port
    slaveId.value = st.id
    endian.value = st.endian
  }

  return { ip, port, slaveId, endian, applyStation }
})
