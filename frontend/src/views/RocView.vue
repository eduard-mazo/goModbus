<template>
  <div class="p-4 space-y-3 overflow-y-auto flex-1">

    <!-- Config row: Pointer + DB -->
    <div class="grid grid-cols-2 gap-3">

      <!-- Pointer register -->
      <div class="card">
        <div class="card-head">
          <span class="card-title">Registro Puntero</span>
          <span class="text-xs px-2 py-0.5 rounded font-semibold" style="background:#eff6ff;color:#1e40af;">PTR</span>
        </div>
        <div class="p-3 grid grid-cols-3 gap-2">
          <div>
            <label class="lbl">Dirección</label>
            <input class="fi font-mono" v-model.number="roc.ptrAddr" type="number" />
          </div>
          <div>
            <label class="lbl">Cantidad</label>
            <input class="fi font-mono" v-model.number="roc.ptrQty" type="number" min="1" max="4" />
          </div>
          <div>
            <label class="lbl">Endian</label>
            <select class="fs" v-model="roc.ptrEndian">
              <option value="abcd">ABCD</option>
              <option value="dcba">DCBA</option>
              <option value="cdab">CDAB</option>
              <option value="badc">BADC</option>
            </select>
          </div>
        </div>
      </div>

      <!-- Historical DB -->
      <div class="card">
        <div class="card-head">
          <span class="card-title">Base de Datos Histórico</span>
          <span class="text-xs px-2 py-0.5 rounded font-semibold" style="background:#f4fce8;color:#3a5c00;">DB</span>
        </div>
        <div class="p-3 grid grid-cols-2 gap-2">
          <div>
            <label class="lbl">Dirección Base</label>
            <input class="fi font-mono" v-model.number="roc.dbAddr" type="number" />
          </div>
          <div>
            <label class="lbl">Endian</label>
            <select class="fs" v-model="roc.dbEndian">
              <option value="abcd">ABCD (BE)</option>
              <option value="dcba">DCBA (LE)</option>
              <option value="cdab">CDAB (ROC)</option>
              <option value="badc">BADC (BS)</option>
            </select>
          </div>
        </div>
      </div>
    </div>

    <!-- Mode + Execute -->
    <div class="card">
      <div class="p-3 flex flex-wrap items-center gap-4">
        <div class="flex gap-3">
          <label v-for="opt in modeOpts" :key="opt.v" class="flex items-center gap-1.5 cursor-pointer text-sm">
            <input type="radio" :value="opt.v" v-model="roc.mode" class="accent-lime" />
            <span>{{ opt.label }}</span>
          </label>
        </div>

        <div v-if="roc.mode === 'hist'" class="flex items-center gap-2">
          <label class="lbl mb-0">Ptr manual</label>
          <input class="fi font-mono" style="width:80px;" v-model.number="roc.manualPtr" type="number" />
        </div>

        <div class="flex gap-2 ml-auto">
          <button class="btn btn-lime" :disabled="roc.loading" @click="roc.sendROC(conn)">
            <svg v-if="!roc.loading" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><polygon points="5 3 19 12 5 21 5 3"/></svg>
            <svg v-else class="animate-spin" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><circle cx="12" cy="12" r="10"/></svg>
            {{ roc.loading ? 'Leyendo…' : 'Ejecutar' }}
          </button>
          <button v-if="roc.result || roc.error" class="btn btn-ghost" @click="roc.clear()">Limpiar</button>
          <span v-if="roc.result" class="text-xs text-g-400 self-center">{{ roc.result.elapsed_ms }}ms</span>
        </div>
      </div>
    </div>

    <!-- Error -->
    <div v-if="roc.error" class="rounded p-3 text-sm" style="background:#fef2f2;border:1px solid #fecaca;color:#dc2626;">
      <strong>Error:</strong> {{ roc.error }}
    </div>

    <!-- Results -->
    <template v-if="roc.result && !roc.error">

      <!-- Pointer value -->
      <div v-if="roc.result.ptr_hex" class="card">
        <div class="card-head"><span class="card-title">Valor del Puntero</span></div>
        <div class="p-3 flex items-center gap-4">
          <div class="rounded-lg px-6 py-3 text-center" style="background:linear-gradient(135deg,#007934,#7ad400);">
            <div class="text-3xl font-bold font-mono text-white">{{ Math.round(roc.result.ptr_value) }}</div>
            <div class="text-xs text-white opacity-80 mt-1">PTR</div>
          </div>
          <div class="flex flex-wrap gap-1">
            <span v-for="b in hexBytes(roc.result.ptr_hex)" :key="b+Math.random()" class="byte byte-req">{{ b }}</span>
          </div>
          <div class="space-y-1" v-if="roc.result.ptr_modes?.length">
            <div v-for="e in endianModes" :key="e.key" class="flex gap-3 text-xs font-mono">
              <span class="text-g-400 w-16">{{ e.label }}</span>
              <span class="font-semibold">{{ roc.result.ptr_modes[0]?.[e.key]?.toFixed(4) }}</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Historical DB -->
      <div v-if="roc.result.db_modes?.length" class="card">
        <div class="card-head"><span class="card-title">Bloque Histórico</span></div>
        <div class="p-3 flex flex-wrap gap-2">
          <div
            v-for="(modes, i) in roc.result.db_modes" :key="i"
            class="rounded p-2 min-w-[180px]" style="background:#f8faf8;border:1px solid #e2e8e2;"
          >
            <div class="text-xs font-semibold text-g-600 mb-1">{{ histLabel(i) }}</div>
            <div v-for="e in endianModes" :key="e.key" class="flex justify-between text-xs font-mono">
              <span class="text-g-400">{{ e.label }}</span>
              <span :class="e.key === roc.dbEndian ? 'text-forest font-bold' : 'text-g-600'">
                {{ modes[e.key]?.toFixed(4) }}
              </span>
            </div>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup>
import { useRocStore } from '../stores/roc'
import { useConnectionStore } from '../stores/connection'

const roc = useRocStore()
const conn = useConnectionStore()

const modeOpts = [
  { v: 'full', label: 'Completo (PTR + DB)' },
  { v: 'ptr', label: 'Solo Puntero' },
  { v: 'hist', label: 'Solo Histórico' },
]

const endianModes = [
  { key: 'abcd', label: 'ABCD' },
  { key: 'dcba', label: 'DCBA' },
  { key: 'cdab', label: 'CDAB' },
  { key: 'badc', label: 'BADC' },
]

const HIST_LABELS = ['Fecha', 'Hora', 'Flow Min', 'Raw Pulses', 'Pf PSI', 'Tf DEG F', 'Multiplier', 'Uncorr Vol MCF', 'Vol Accum MCF', 'Energy MMBTU']
const histLabel = i => HIST_LABELS[i] || `Registro ${i}`

function hexBytes(h) {
  if (!h) return []
  const s = h.replace(/\s/g, '')
  const out = []
  for (let i = 0; i < s.length; i += 2) out.push(s.substr(i, 2))
  return out
}
</script>
