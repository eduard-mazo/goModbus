<template>
  <div class="flex flex-col h-full">

    <!-- ── Top: station selection + sync control ─────────────────────────── -->
    <div class="shrink-0 px-4 pt-3 pb-2 border-b border-g-200 bg-white">

      <!-- Station chips with inline progress -->
      <div class="flex flex-wrap gap-2 mb-2">
        <label
          v-for="st in stations" :key="st.name"
          class="relative flex flex-col gap-0.5 rounded-lg border px-3 py-1.5 cursor-pointer transition-all select-none"
          :class="sync.selectedNames.includes(st.name)
            ? 'border-lime bg-lime-x-lt'
            : 'border-g-200 hover:border-g-300 bg-white'"
          style="min-width:140px; max-width:200px;"
        >
          <div class="flex items-center gap-2">
            <input type="checkbox" :value="st.name" v-model="sync.selectedNames" class="accent-lime shrink-0" />
            <span class="text-xs font-semibold text-g-700 truncate">{{ st.name }}</span>
            <span v-if="isStationDone(st.name)" class="ml-auto text-forest text-xs font-bold shrink-0">✓</span>
            <span v-else-if="stationPct(st.name) > 0" class="ml-auto text-xs font-mono text-lime shrink-0">
              {{ stationPct(st.name) }}%
            </span>
          </div>
          <div class="flex items-center gap-1.5">
            <span class="font-mono text-g-400 truncate" style="font-size:9px;">{{ st.ip }}</span>
            <span v-if="st.medidores?.length" class="text-g-400 shrink-0" style="font-size:9px;">
              · {{ st.medidores.length }} med.
            </span>
          </div>
          <div v-if="stationPct(st.name) > 0 && !isStationDone(st.name)"
               class="h-0.5 rounded-full bg-g-200 overflow-hidden mt-0.5">
            <div class="h-full bg-lime rounded-full transition-all duration-300"
                 :style="`width:${stationPct(st.name)}%`" />
          </div>
          <div v-if="stationError(st.name)" class="text-red-500" style="font-size:9px;">
            {{ stationError(st.name) }}
          </div>
        </label>
      </div>

      <!-- Controls row -->
      <div class="flex items-center gap-3 flex-wrap">
        <button class="btn btn-forest btn-sm"
                :disabled="sync.loading || sync.selectedNames.length === 0"
                @click="startSync">
          <svg v-if="!sync.loading" width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><polyline points="1 4 1 10 7 10"/><path d="M3.51 15a9 9 0 1 0 .49-3.51"/></svg>
          <svg v-else class="animate-spin" width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><circle cx="12" cy="12" r="10"/></svg>
          {{ sync.loading ? 'Sincronizando…' : `Sincronizar (${sync.selectedNames.length})` }}
        </button>
        <button class="btn btn-ghost btn-sm" :disabled="sync.loading"
                title="Cargar datos almacenados sin conectar al equipo"
                @click="loadFromDB">
          <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
          Ver BD
        </button>
        <button class="btn btn-ghost btn-sm" @click="sync.selectedNames = stations.map(s => s.name)">Todas</button>
        <button class="btn btn-ghost btn-sm" @click="sync.selectedNames = []">Ninguna</button>
        <span v-if="sync.loading" class="text-xs text-g-400 ml-1">
          {{ completedCount }}/{{ sync.stationsExpected.length }} tareas completadas
        </span>
        <span v-if="dbLoadMsg" class="text-xs ml-1"
              :class="dbLoadMsg.ok ? 'text-forest' : 'text-g-400'">
          {{ dbLoadMsg.text }}
        </span>
      </div>
    </div>

    <!-- ── Main: task tabs + content ─────────────────────────────────────── -->
    <div class="flex-1 flex flex-col overflow-hidden">

      <!-- Task tabs -->
      <div v-if="Object.keys(sync.stationResults).length > 0"
           class="shrink-0 flex items-center gap-1 px-4 py-1.5 border-b border-g-100 overflow-x-auto"
           style="scrollbar-width:thin;">
        <button v-for="key in Object.keys(sync.stationResults)" :key="key"
                class="btn btn-sm shrink-0"
                :class="sync.selectedIdx === key ? 'btn-forest' : 'btn-ghost'"
                @click="sync.selectedIdx = key">
          {{ key }}
          <span class="ml-1 text-g-400 font-mono" style="font-size:9px;">
            ({{ sync.stationResults[key]?.length || 0 }})
          </span>
        </button>
      </div>

      <!-- View tabs + content -->
      <div v-if="sync.selectedIdx && currentRecords" class="flex-1 flex flex-col overflow-hidden">

        <!-- View tabs -->
        <div class="shrink-0 flex items-center border-b border-g-200 px-4" style="background:#fafcfa;">
          <button class="px-4 py-2 text-sm font-medium border-b-2 transition-colors"
                  :class="sync.viewTab === 'chart' ? 'border-lime text-lime' : 'border-transparent text-g-500 hover:text-g-700'"
                  @click="sync.viewTab = 'chart'">
            <svg class="inline mr-1.5" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/></svg>
            Gráfica
          </button>
          <button class="px-4 py-2 text-sm font-medium border-b-2 transition-colors"
                  :class="sync.viewTab === 'table' ? 'border-lime text-lime' : 'border-transparent text-g-500 hover:text-g-700'"
                  @click="sync.viewTab = 'table'">
            <svg class="inline mr-1.5" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="3" width="18" height="18" rx="2"/><line x1="3" y1="9" x2="21" y2="9"/><line x1="3" y1="15" x2="21" y2="15"/><line x1="9" y1="3" x2="9" y2="21"/></svg>
            Tabla
          </button>
          <div class="ml-auto flex items-center gap-2">
            <label class="text-xs text-g-500">Endian</label>
            <select class="fs" style="width:auto;" v-model="rocStore.dbEndian">
              <option value="abcd">ABCD</option>
              <option value="dcba">DCBA</option>
              <option value="cdab">CDAB (ROC)</option>
              <option value="badc">BADC</option>
            </select>
            <span class="text-g-400 font-mono" style="font-size:9px;">
              {{ validCount }} / {{ currentRecords.length }} válidos
            </span>
            <button class="btn btn-sm btn-ghost" :disabled="sync.loading"
                    @click="sync.retryStation(sync.selectedIdx, stations, sessionId)"
                    title="Reintentar fallidos">
              <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><polyline points="1 4 1 10 7 10"/><path d="M3.51 15a9 9 0 1 0 .49-3.51"/></svg>
              Reintentar
            </button>
          </div>
        </div>

        <!-- ── Gráfica ───────────────────────────────────────────────────── -->
        <div v-if="sync.viewTab === 'chart'" class="flex-1 overflow-y-auto p-4">
          <div class="card">
            <div class="card-head">
              <span class="card-title">{{ sync.selectedIdx }}</span>
              <span class="text-g-400 text-xs font-normal" v-if="currentRecords.length">
                {{ currentRecords[0]?.fecha }} → {{ currentRecords[currentRecords.length-1]?.fecha }}
              </span>
            </div>
            <div class="p-4">
              <MultiSignalChart
                :records="currentRecords"
                :endian="rocStore.dbEndian"
                :stationName="sync.selectedIdx"
                :signalNames="currentSignalNames"
              />
            </div>
          </div>
        </div>

        <!-- ── Tabla ─────────────────────────────────────────────────────── -->
        <div v-if="sync.viewTab === 'table'" class="flex-1 overflow-hidden flex flex-col">
          <div class="shrink-0 flex items-center justify-between px-4 py-2 border-b"
               style="background:#161b22; border-color:#21262d;">
            <span style="color:#8b949e; font-size:11px; font-family:monospace;">
              {{ currentRecords.length }} registros históricos
            </span>
          </div>
          <div class="flex-1 overflow-auto" style="background:#0d1117;">
            <table style="width:100%; border-collapse:collapse; font-family:'JetBrains Mono',monospace; font-size:11px; white-space:nowrap; min-width:900px;">
              <thead style="position:sticky; top:0; z-index:10; background:#161b22; border-bottom:1px solid #21262d;">
                <tr>
                  <th style="padding:6px 12px; text-align:right; color:#8b949e; font-weight:500; border-right:1px solid #21262d;">Ptr</th>
                  <th style="padding:6px 12px; text-align:left; color:#8b949e; font-weight:500; border-right:1px solid #21262d; min-width:140px;">Fecha / Hora</th>
                  <th v-for="(name, i) in sigLabels" :key="i"
                      style="padding:6px 10px; text-align:right; color:#8b949e; font-weight:500; border-right:1px solid #21262d; max-width:110px; overflow:hidden; text-overflow:ellipsis;"
                      :title="name">{{ name }}</th>
                  <th style="padding:6px 12px; text-align:center; color:#8b949e; font-weight:500;">OK</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="(rec, idx) in currentRecords" :key="`${rec.ptr}-${rec.fecha}-${rec.hora}`"
                    :style="`background:${idx % 2 === 0 ? '#0d1117' : '#0f1319'}; border-bottom:1px solid #1c2128;`">
                  <td style="padding:2px 12px; text-align:right; color:#484f58; border-right:1px solid #1c2128;">{{ rec.ptr }}</td>
                  <td style="padding:2px 12px; text-align:left; color:#8b949e; border-right:1px solid #1c2128;">
                    {{ rec.fecha }} {{ rec.hora }}
                  </td>
                  <td v-for="(_, si) in sigLabels" :key="si"
                      style="padding:2px 10px; text-align:right; border-right:1px solid #1c2128;"
                      :style="`color:${rec.valid && rec.modes?.[si+2] ? '#7ad400' : '#333'}`">
                    {{ rec.valid && rec.modes?.[si+2]
                      ? rec.modes[si+2][rocStore.dbEndian]?.toFixed(4)
                      : '—' }}
                  </td>
                  <td style="padding:2px 12px; text-align:center;">
                    <span :style="`font-size:8px; color:${rec.valid ? '#3fb950' : '#f85149'}`">{{ rec.valid ? '●' : '○' }}</span>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

      </div>

      <!-- Empty state -->
      <div v-else-if="!sync.loading" class="flex-1 flex items-center justify-center">
        <div class="text-center space-y-2">
          <svg class="mx-auto text-g-300" width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><polyline points="1 4 1 10 7 10"/><path d="M3.51 15a9 9 0 1 0 .49-3.51"/></svg>
          <p class="text-g-400 text-sm">Sincroniza para obtener datos frescos, o usa <strong>Ver BD</strong> para cargar el historial almacenado</p>
        </div>
      </div>

    </div>
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
const dbLoadMsg = ref(null)

// Signal names for display (start at modes index 2 — indices 0,1 are fecha/hora floats)
const DEFAULT_SIG = [
  'Flow Min', 'Raw Pulses', 'Pf PSI', 'Tf DEG F',
  'Multiplier', 'Uncorr Vol MCF', 'Vol Accum MCF', 'Energy MMBTU',
]

const currentRecords = computed(() =>
  sync.selectedIdx ? sync.stationResults[sync.selectedIdx] : null
)

const currentStation = computed(() => {
  if (!sync.selectedIdx) return null
  const stName = sync.selectedIdx.split(' / ')[0]
  return stations.value.find(s => s.name === stName) || null
})

const currentSignalNames = computed(() => currentStation.value?.signal_names || [])

const sigLabels = computed(() =>
  DEFAULT_SIG.map((d, i) => currentSignalNames.value[i] || d)
)

const validCount = computed(() =>
  currentRecords.value?.filter(r => r.valid).length ?? 0
)

const completedCount = computed(() =>
  sync.stationsExpected.filter(k => sync.stationResults[k]).length
)

function stationPct(stName) {
  let done = 0, total = 0
  for (const [key, prog] of Object.entries(sync.progress)) {
    if (key.split(' / ')[0] !== stName) continue
    done  += prog.done  ?? 0
    total += prog.total ?? 840
  }
  return total > 0 ? Math.round(done * 100 / total) : 0
}

function isStationDone(stName) {
  const tasks = sync.stationsExpected.filter(k => k.split(' / ')[0] === stName)
  return tasks.length > 0 && tasks.every(k => !!sync.stationResults[k])
}

function stationError(stName) {
  for (const [key, prog] of Object.entries(sync.progress)) {
    if (key.split(' / ')[0] === stName && prog.error) return prog.error
  }
  return null
}

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

async function loadFromDB() {
  dbLoadMsg.value = null
  const found = await sync.loadFromDB(sync.selectedNames)
  dbLoadMsg.value = found
    ? { ok: true, text: 'Historial cargado desde BD' }
    : { ok: false, text: 'Sin datos en BD para la selección' }
  setTimeout(() => { dbLoadMsg.value = null }, 3000)
}

onMounted(loadStations)
</script>
