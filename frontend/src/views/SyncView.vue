<template>
  <div class="p-4 space-y-3">

    <!-- Station selection -->
    <div class="card">
      <div class="card-head">
        <span class="card-title">Selección de Estaciones</span>
        <div class="flex gap-2">
          <button class="btn btn-ghost btn-sm" @click="sync.selectedNames = stations.map(s => s.name)">Todas</button>
          <button class="btn btn-ghost btn-sm" @click="sync.selectedNames = []">Ninguna</button>
        </div>
      </div>
      <div class="p-3">
        <div v-if="stations.length === 0" class="text-xs text-g-400 py-2">
          Sin estaciones en config.yaml — cargue desde la barra lateral.
        </div>
        <div class="grid grid-cols-3 gap-2 mb-3" v-else>
          <label
            v-for="st in stations" :key="st.name"
            class="flex items-center gap-2 rounded p-2 cursor-pointer border transition-all"
            :class="sync.selectedNames.includes(st.name)
              ? 'border-lime bg-lime-x-lt'
              : 'border-g-200 hover:border-g-300'"
          >
            <input type="checkbox" :value="st.name" v-model="sync.selectedNames" class="accent-lime" />
            <div class="min-w-0">
              <div class="text-xs font-semibold text-g-700 truncate">{{ st.name }}</div>
              <div class="font-mono text-g-400" style="font-size:10px;">{{ st.ip }}</div>
            </div>
          </label>
        </div>
        <button
          class="btn btn-forest"
          :disabled="sync.loading || sync.selectedNames.length === 0"
          @click="startSync"
        >
          <svg v-if="!sync.loading" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><polyline points="1 4 1 10 7 10"/><path d="M3.51 15a9 9 0 1 0 .49-3.51"/></svg>
          <svg v-else class="animate-spin" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><circle cx="12" cy="12" r="10"/></svg>
          {{ sync.loading ? 'Sincronizando…' : `Sincronizar ${sync.selectedNames.length} estación(es)` }}
        </button>
      </div>
    </div>

    <!-- Per-station progress bars (during download) -->
    <div v-if="sync.stationsExpected.length > 0" class="card">
      <div class="card-head"><span class="card-title">Progreso de Descarga</span></div>
      <div class="p-3 space-y-2">
        <div v-for="name in sync.stationsExpected" :key="name">
          <div class="flex items-center justify-between mb-1">
            <span class="text-xs font-semibold text-g-700">{{ name }}</span>
            <div class="flex items-center gap-2">
              <span v-if="sync.stationResults[name]" class="text-xs font-semibold text-forest">✓ Completo</span>
              <span class="text-xs font-mono text-g-500">{{ sync.progress[name]?.done || 0 }}/840</span>
              <span class="text-xs font-bold font-mono" :class="sync.stationResults[name] ? 'text-forest' : 'text-lime'">
                {{ sync.progress[name]?.pct || 0 }}%
              </span>
              <span v-if="sync.progress[name]?.error" class="text-xs text-red-500">{{ sync.progress[name].error }}</span>
            </div>
          </div>
          <div class="h-1.5 rounded-full bg-g-200 overflow-hidden">
            <div
              class="h-full rounded-full transition-all duration-300"
              :class="sync.stationResults[name] ? 'bg-forest' : 'bg-lime'"
              :style="`width:${sync.progress[name]?.pct || 0}%`"
            ></div>
          </div>
        </div>
      </div>
    </div>

    <!-- Results -->
    <template v-if="Object.keys(sync.stationResults).length > 0">

      <!-- Station tabs -->
      <div class="flex gap-1 flex-wrap">
        <button
          v-for="name in Object.keys(sync.stationResults)" :key="name"
          class="btn btn-sm"
          :class="sync.selectedIdx === name ? 'btn-forest' : 'btn-ghost'"
          @click="sync.selectedIdx = name"
        >{{ name }}</button>
      </div>

      <!-- Multi-signal chart -->
      <div v-if="sync.selectedIdx && currentRecords" class="card">
        <div class="card-head">
          <span class="card-title">
            Análisis — {{ sync.selectedIdx }}
            <span class="text-g-400 font-normal text-xs ml-1">(840 registros · doble clic en nombre para renombrar señal)</span>
          </span>
          <div class="flex items-center gap-2">
            <label class="lbl mb-0 mr-1">Endian</label>
            <select class="fs" style="width:auto;" v-model="rocStore.dbEndian">
              <option value="abcd">ABCD</option>
              <option value="dcba">DCBA</option>
              <option value="cdab">CDAB (ROC)</option>
              <option value="badc">BADC</option>
            </select>
          </div>
        </div>
        <div class="p-3">
          <MultiSignalChart
            :records="currentRecords"
            :endian="rocStore.dbEndian"
            :stationName="sync.selectedIdx"
          />
        </div>
      </div>

      <!-- Record table (signal selector kept for detail view) -->
      <div v-if="sync.selectedIdx && currentRecords" class="card overflow-hidden">
        <div class="card-head">
          <span class="card-title">Tabla de Registros — {{ sync.selectedIdx }}</span>
          <div class="flex items-center gap-2">
            <select class="fs" style="width:auto;" v-model.number="sync.chartSig">
              <option v-for="(n, i) in SIG_NAMES" :key="i" :value="i">{{ n }}</option>
            </select>
            <button
              class="btn btn-sm btn-ghost"
              @click="sync.retryStation(sync.selectedIdx, stations, sessionId)"
              :disabled="sync.loading"
            >Reintentar fallos</button>
          </div>
        </div>
        <div class="overflow-y-auto" style="max-height:280px;">
          <table class="w-full text-xs font-mono">
            <thead class="sticky top-0 bg-white border-b border-g-200">
              <tr class="text-g-500">
                <th class="px-3 py-1 text-left">Ptr</th>
                <th class="px-3 py-1 text-right">{{ SIG_NAMES[sync.chartSig] }}</th>
                <th class="px-3 py-1 text-center">Estado</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="rec in currentRecords" :key="rec.ptr"
                class="border-b border-g-100"
                :class="rec.valid ? 'hover:bg-g-50' : 'bg-red-50'"
              >
                <td class="px-3 py-0.5 text-g-600">{{ rec.ptr }}</td>
                <td class="px-3 py-0.5 text-right text-g-700">
                  {{ rec.valid && rec.modes?.[sync.chartSig]
                    ? rec.modes[sync.chartSig][rocStore.dbEndian]?.toFixed(4)
                    : '—' }}
                </td>
                <td class="px-3 py-0.5 text-center">
                  <span v-if="rec.valid" class="text-forest">●</span>
                  <span v-else class="text-red-500 font-bold">ERR</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import axios from 'axios'
import { useSyncStore } from '../stores/sync'
import { useRocStore } from '../stores/roc'
import { useSessionId } from '../services/websocket'
import MultiSignalChart from '../components/sync/MultiSignalChart.vue'

const sync      = useSyncStore()
const rocStore  = useRocStore()
const sessionId = useSessionId()
const stations  = ref([])

const SIG_NAMES = [
  'Flow Min', 'Raw Pulses', 'Pf PSI', 'Tf DEG F',
  'Multiplier', 'Uncorr Vol MCF', 'Vol Accum MCF', 'Energy MMBTU',
]

const currentRecords = computed(() =>
  sync.selectedIdx ? sync.stationResults[sync.selectedIdx] : null
)

async function loadStations() {
  try {
    const { data } = await axios.get('/api/config')
    stations.value = data.stations || []
    if (!sync.selectedNames.length)
      sync.selectedNames = stations.value.map(s => s.name)
  } catch (_) {}
}

async function startSync() {
  await sync.startFullSync(sessionId)
}

onMounted(loadStations)
</script>
