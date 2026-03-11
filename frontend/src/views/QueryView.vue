<template>
  <div class="p-4 space-y-3 overflow-y-auto flex-1">

    <!-- Parameters card -->
    <div class="card">
      <div class="card-head">
        <span class="card-title">Parámetros</span>
        <span class="font-mono text-xs text-g-400">{{ aduPreviewStr }}</span>
      </div>
      <div class="p-3 grid grid-cols-6 gap-3">

        <!-- FC selector -->
        <div class="col-span-2">
          <label class="lbl">Función (FC)</label>
          <select class="fs" v-model.number="mb.fc">
            <option :value="1">FC01 · Read Coils</option>
            <option :value="2">FC02 · Read Discrete Inputs</option>
            <option :value="3">FC03 · Read Holding Registers</option>
            <option :value="4">FC04 · Read Input Registers</option>
            <option :value="5">FC05 · Write Single Coil</option>
            <option :value="6">FC06 · Write Single Register</option>
            <option :value="15">FC15 · Write Multiple Coils</option>
            <option :value="16">FC16 · Write Multiple Registers</option>
          </select>
        </div>

        <!-- Address -->
        <div>
          <label class="lbl">
            Dirección
            <button class="ml-1 text-g-400 hover:text-lime transition-colors" @click="mb.addrHex = !mb.addrHex; mb.parseAddr()">
              {{ mb.addrHex ? 'HEX' : 'DEC' }}
            </button>
          </label>
          <input class="fi" v-model="mb.startRaw" :placeholder="mb.addrHex ? '0x0000' : '0'" @input="mb.parseAddr()" />
        </div>

        <!-- Qty -->
        <div>
          <label class="lbl">Cantidad</label>
          <input class="fi" v-model.number="mb.qty" type="number" min="1" max="125" />
        </div>

        <!-- Write data (only for write FCs) -->
        <div class="col-span-2" v-if="mb.isWrite">
          <label class="lbl">Datos (HEX)</label>
          <input class="fi font-mono" v-model="mb.writeHex" placeholder="01 02 03 04 ..." />
        </div>

        <!-- Actions -->
        <div class="col-span-6 flex items-center gap-2">
          <button class="btn btn-lime" :disabled="mb.loading" @click="mb.sendQuery(conn)">
            <svg v-if="!mb.loading" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><line x1="22" y1="2" x2="11" y2="13"/><polygon points="22 2 15 22 11 13 2 9 22 2"/></svg>
            <svg v-else class="animate-spin" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2"/></svg>
            {{ mb.loading ? 'Enviando…' : 'Enviar' }}
          </button>
          <button v-if="mb.result || mb.error" class="btn btn-ghost" @click="mb.clear()">Limpiar</button>
          <span v-if="mb.result" class="text-xs text-g-500 ml-auto">
            {{ mb.result.elapsed_ms }}ms · {{ mb.result.byte_count }} bytes
          </span>
        </div>
      </div>
    </div>

    <!-- Error -->
    <div v-if="mb.error" class="rounded p-3 text-sm" style="background:#fef2f2;border:1px solid #fecaca;color:#dc2626;">
      <strong>Error:</strong> {{ mb.error }}
    </div>

    <!-- Results -->
    <template v-if="mb.result && !mb.error">

      <!-- ADU hex display -->
      <div class="grid grid-cols-2 gap-3">
        <div class="card" v-for="src in ['req_hex','res_hex']" :key="src">
          <div class="card-head">
            <span class="card-title">{{ src === 'req_hex' ? 'Trama TX' : 'Trama RX' }}</span>
            <button class="text-xs text-g-400 hover:text-lime" @click="copyText(mb.result[src])">copiar</button>
          </div>
          <div class="p-3 flex flex-wrap gap-1">
            <span
              v-for="(b, i) in hexBytes(mb.result[src])" :key="i"
              class="byte" :class="src === 'req_hex' ? 'byte-req' : 'byte-res'"
            >{{ b }}</span>
          </div>
        </div>
      </div>

      <!-- Register table -->
      <div v-if="mb.result.registers?.length" class="card overflow-hidden">
        <div class="card-head"><span class="card-title">Registros</span></div>
        <div class="overflow-x-auto">
          <table class="w-full text-xs font-mono">
            <thead>
              <tr class="text-g-500 border-b border-g-200">
                <th class="px-3 py-1 text-left">#</th>
                <th class="px-3 py-1 text-left">Dirección</th>
                <th class="px-3 py-1 text-left">HEX</th>
                <th class="px-3 py-1 text-right">Dec U</th>
                <th class="px-3 py-1 text-right">Dec S</th>
                <th class="px-3 py-1 text-left">Binario</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="row in mb.result.registers" :key="row.i" class="border-b border-g-100 hover:bg-g-50">
                <td class="px-3 py-1 text-g-400">{{ row.i }}</td>
                <td class="px-3 py-1">{{ row.addr }}</td>
                <td class="px-3 py-1 text-blue-600">{{ row.hex }}</td>
                <td class="px-3 py-1 text-right">{{ row.dec }}</td>
                <td class="px-3 py-1 text-right text-g-500">{{ row.sdec }}</td>
                <td class="px-3 py-1 text-g-400">{{ row.bin }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- Float32 modes -->
      <div v-if="mb.result.float_modes?.length" class="card">
        <div class="card-head"><span class="card-title">Decodificación Float32</span></div>
        <div class="p-3 flex flex-wrap gap-2">
          <div
            v-for="(modes, i) in mb.result.float_modes" :key="i"
            class="rounded p-2 min-w-[160px]" style="background:#f8faf8;border:1px solid #e2e8e2;"
          >
            <div class="text-xs text-g-500 mb-1">Registro {{ i * 2 }}–{{ i * 2 + 1 }}</div>
            <div v-for="e in endianModes" :key="e.key" class="flex justify-between text-xs font-mono">
              <span class="text-g-500">{{ e.label }}</span>
              <span class="font-semibold text-g-700">{{ modes[e.key]?.toFixed(6) }}</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Coils -->
      <div v-if="mb.result.coils?.length" class="card">
        <div class="card-head"><span class="card-title">Bobinas</span></div>
        <div class="p-3 flex flex-wrap gap-1">
          <div
            v-for="(bit, i) in mb.result.coils" :key="i"
            class="flex flex-col items-center text-xs rounded px-1.5 py-1"
            :class="bit ? 'coil-on' : 'coil-off'"
            style="min-width:36px;"
          >
            <span class="font-mono font-bold">{{ bit ? '1' : '0' }}</span>
            <span style="font-size:9px;">{{ i }}</span>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { useModbusStore } from '../stores/modbus'
import { useConnectionStore } from '../stores/connection'

const mb = useModbusStore()
const conn = useConnectionStore()

const endianModes = [
  { key: 'abcd', label: 'ABCD (BE)' },
  { key: 'dcba', label: 'DCBA (LE)' },
  { key: 'cdab', label: 'CDAB (ROC)' },
  { key: 'badc', label: 'BADC (BS)' },
]

const aduPreviewStr = computed(() => {
  const h2 = n => (+n).toString(16).padStart(2, '0').toUpperCase()
  const h4 = n => (+n).toString(16).padStart(4, '0').toUpperCase()
  return `0001 0000 0006 ${h2(conn.slaveId)} ${h2(mb.fc)} ${h4(mb.startAddr)} ${h4(mb.qty)}`
})

function hexBytes(h) {
  if (!h) return []
  const s = h.replace(/\s/g, '')
  const out = []
  for (let i = 0; i < s.length; i += 2) out.push(s.substr(i, 2))
  return out
}

async function copyText(h) {
  try { await navigator.clipboard.writeText(hexBytes(h).join(' ')) } catch (_) {}
}
</script>
