<template>
  <div class="p-4 space-y-3 overflow-y-auto flex-1">

    <!-- Input card -->
    <div class="card">
      <div class="card-head"><span class="card-title">Trama RAW (Modbus TCP)</span></div>
      <div class="p-3 space-y-3">
        <div>
          <label class="lbl">Trama HEX (separada por espacios o ':')</label>
          <textarea
            class="fi font-mono"
            style="height:64px;resize:vertical;padding:6px 8px;"
            v-model="raw"
            placeholder="00 01 00 00 00 06 01 03 00 00 00 0A"
          ></textarea>
        </div>

        <!-- Byte visualization -->
        <div v-if="parsed">
          <div class="flex flex-wrap gap-1 mb-2">
            <div
              v-for="(grp, i) in parsed.groups" :key="i"
              class="flex flex-col items-center gap-0.5"
            >
              <div class="flex gap-0.5">
                <span v-for="b in grp.bytes" :key="b" class="byte byte-req" style="width:30px;height:30px;font-size:10px;">{{ b }}</span>
              </div>
              <span style="font-size:9px;color:#6b7d6b;">{{ grp.label }}</span>
            </div>
          </div>
          <div class="flex flex-wrap gap-x-6 gap-y-1">
            <div v-for="f in parsed.fields" :key="f.name" class="flex gap-2 text-xs font-mono">
              <span class="text-g-400 w-20">{{ f.name }}</span>
              <span class="font-semibold text-g-700">{{ f.value }}</span>
              <span v-if="f.hint" class="text-g-400">{{ f.hint }}</span>
            </div>
          </div>
        </div>

        <div class="flex gap-2">
          <button class="btn btn-lime" :disabled="loading || !raw.trim()" @click="sendRaw">
            <svg v-if="!loading" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><line x1="22" y1="2" x2="11" y2="13"/><polygon points="22 2 15 22 11 13 2 9 22 2"/></svg>
            <svg v-else class="animate-spin" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><circle cx="12" cy="12" r="10"/></svg>
            {{ loading ? 'Enviando…' : 'Enviar' }}
          </button>
          <span class="text-xs text-g-400 self-center">→ {{ conn.ip }}:{{ conn.port }}</span>
        </div>
      </div>
    </div>

    <!-- Error -->
    <div v-if="error" class="rounded p-3 text-sm" style="background:#fef2f2;border:1px solid #fecaca;color:#dc2626;">
      <strong>Error:</strong> {{ error }}
    </div>

    <!-- Result -->
    <div v-if="result" class="grid grid-cols-2 gap-3">
      <div class="card" v-for="src in ['sent_hex','recv_hex']" :key="src">
        <div class="card-head">
          <span class="card-title">{{ src === 'sent_hex' ? 'TX Enviado' : 'RX Recibido' }}</span>
          <span class="text-xs text-g-400">{{ result.elapsed_ms }}ms</span>
        </div>
        <div class="p-3 flex flex-wrap gap-1">
          <span
            v-for="(b, i) in hexBytes(result[src])" :key="i"
            class="byte" :class="src === 'sent_hex' ? 'byte-req' : 'byte-res'"
          >{{ b }}</span>
        </div>
        <div v-if="parsedResponse && src === 'recv_hex'" class="px-3 pb-3">
          <div class="flex flex-wrap gap-x-4 gap-y-1 mt-2">
            <div v-for="f in parsedResponse.fields" :key="f.name" class="flex gap-2 text-xs font-mono">
              <span class="text-g-400 w-20">{{ f.name }}</span>
              <span class="font-semibold text-g-700">{{ f.value }}</span>
              <span v-if="f.hint" class="text-g-400">{{ f.hint }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import api from '../services/api'
import { useConnectionStore } from '../stores/connection'

const conn = useConnectionStore()
const raw = ref('')
const loading = ref(false)
const result = ref(null)
const error = ref(null)

function hexBytes(h) {
  if (!h) return []
  const s = h.replace(/\s/g, '')
  const out = []
  for (let i = 0; i < s.length; i += 2) out.push(s.substr(i, 2))
  return out
}

function parseFrame(hexInput) {
  const s = (hexInput || '').replace(/[\s:]/g, '').toUpperCase()
  if (s.length < 16 || s.length % 2 !== 0) return null
  const b = []
  for (let i = 0; i < s.length; i += 2) b.push(parseInt(s.substr(i, 2), 16))
  if (b.length < 8) return null
  const u16 = i => (b[i] << 8) | b[i + 1]
  const h2 = v => v.toString(16).padStart(2, '0').toUpperCase()
  const h4 = v => v.toString(16).padStart(4, '0').toUpperCase()
  const fcNames = { 1:'Read Coils',2:'Read Discrete',3:'Read Holding',4:'Read Input',5:'Write Coil',6:'Write Reg',15:'Write Coils',16:'Write Regs' }
  const txID = u16(0), protoID = u16(2), length = u16(4), unitID = b[6], fc = b[7]
  const groups = [
    { label:'TxID', bytes:[h4(txID).substr(0,2),h4(txID).substr(2,2)] },
    { label:'Proto', bytes:[h4(protoID).substr(0,2),h4(protoID).substr(2,2)] },
    { label:'Len', bytes:[h4(length).substr(0,2),h4(length).substr(2,2)] },
    { label:'UID', bytes:[h2(unitID)] },
    { label:'FC', bytes:[h2(fc)] },
  ]
  const fields = [
    { name:'TxID', value:`0x${h4(txID)}`, hint:`${txID}` },
    { name:'Protocol', value:`0x${h4(protoID)}`, hint:protoID===0?'Modbus TCP':'' },
    { name:'Length', value:`${length}`, hint:`${length} bytes restantes` },
    { name:'Unit ID', value:`${unitID}`, hint:`0x${h2(unitID)}` },
    { name:'FC', value:`0x${h2(fc)}`, hint:fcNames[fc]||'' },
  ]
  if (b.length >= 12 && [1,2,3,4].includes(fc)) {
    const addr = u16(8), qty = u16(10)
    groups.push({label:'Dir',bytes:[h4(addr).substr(0,2),h4(addr).substr(2,2)]})
    groups.push({label:'Qty',bytes:[h4(qty).substr(0,2),h4(qty).substr(2,2)]})
    fields.push({name:'Dirección',value:`${addr}`,hint:`0x${h4(addr)}`})
    fields.push({name:'Cantidad',value:`${qty}`,hint:`${qty} reg`})
  } else if (b.length >= 12 && (fc===5||fc===6)) {
    const addr=u16(8),val=u16(10)
    groups.push({label:'Dir',bytes:[h4(addr).substr(0,2),h4(addr).substr(2,2)]})
    groups.push({label:'Val',bytes:[h4(val).substr(0,2),h4(val).substr(2,2)]})
    fields.push({name:'Dirección',value:`${addr}`,hint:`0x${h4(addr)}`})
    fields.push({name:'Valor',value:`0x${h4(val)}`,hint:fc===5?(val===0xff00?'ON':'OFF'):`${val}`})
  }
  return { groups, fields, valid: true }
}

const parsed = computed(() => parseFrame(raw.value))
const parsedResponse = computed(() => result.value?.recv_hex ? parseFrame(result.value.recv_hex) : null)

async function sendRaw() {
  const hex = raw.value.replace(/[\s:]/g, '')
  if (!hex) return
  loading.value = true
  error.value = null
  result.value = null
  try {
    const { data } = await api.post('/raw', { ip: conn.ip, port: +conn.port, hex_frame: hex })
    if (data.error) error.value = data.error
    else result.value = data
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}
</script>
