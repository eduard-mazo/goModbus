<template>
  <div class="p-4 space-y-3">

    <!-- ── Station selection ───────────────────────────────────────────────── -->
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

    <!-- ── Progress bars ───────────────────────────────────────────────────── -->
    <div v-if="sync.stationsExpected.length > 0" class="card">
      <div class="card-head"><span class="card-title">Progreso de Descarga</span></div>
      <div class="p-3 space-y-2">
        <div v-for="name in sync.stationsExpected" :key="name">
          <div class="flex items-center justify-between mb-1">
            <span class="text-xs font-semibold text-g-700">{{ name }}</span>
            <div class="flex items-center gap-2">
              <span v-if="sync.stationResults[name]" class="text-xs font-semibold text-forest">✓ Completo</span>
              <span class="text-xs font-mono text-g-500">
                {{ sync.progress[name]?.done || 0 }}/{{ sync.progress[name]?.total || 840 }}
              </span>
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

    <!-- ── Results ─────────────────────────────────────────────────────────── -->
    <template v-if="Object.keys(sync.stationResults).length > 0">

      <!-- Task/station tabs -->
      <div class="flex gap-1 flex-wrap">
        <button
          v-for="name in Object.keys(sync.stationResults)" :key="name"
          class="btn btn-sm"
          :class="sync.selectedIdx === name ? 'btn-forest' : 'btn-ghost'"
          @click="sync.selectedIdx = name"
        >{{ name }}</button>
      </div>

      <!-- View tabs: Gráfica | Tabla -->
      <div v-if="sync.selectedIdx && currentRecords" class="flex border-b border-g-200">
        <button
          class="px-4 py-2 text-sm font-medium border-b-2 transition-colors"
          :class="activeTab === 'chart'
            ? 'border-lime text-lime'
            : 'border-transparent text-g-500 hover:text-g-700'"
          @click="activeTab = 'chart'"
        >Gráfica</button>
        <button
          class="px-4 py-2 text-sm font-medium border-b-2 transition-colors"
          :class="activeTab === 'table'
            ? 'border-lime text-lime'
            : 'border-transparent text-g-500 hover:text-g-700'"
          @click="activeTab = 'table'"
        >Tabla</button>
      </div>

      <!-- ── Chart tab ──────────────────────────────────────────────────────── -->
      <div v-if="activeTab === 'chart' && sync.selectedIdx && currentRecords" class="card">
        <div class="card-head">
          <span class="card-title">
            Análisis — {{ sync.selectedIdx }}
            <span class="text-g-400 font-normal text-xs ml-1">(doble clic en nombre para renombrar señal)</span>
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
            :signalNames="currentSignalNames"
          />
        </div>
      </div>

      <!-- ── Table tab (Supabase style) ────────────────────────────────────── -->
      <div v-if="activeTab === 'table' && sync.selectedIdx && currentRecords"
           class="rounded-xl overflow-hidden border"
           style="border-color:#30363d;">

        <!-- Table toolbar -->
        <div class="flex items-center justify-between px-4 py-2 border-b"
             style="background:#161b22; border-color:#21262d;">
          <div class="flex items-center gap-3">
            <span class="text-xs font-semibold" style="color:#8b949e;">
              {{ currentRecords.length }} registros ·
              {{ currentRecords.filter(r => r.valid).length }} válidos ·
              {{ currentRecords.filter(r => !r.valid).length }} fallidos
            </span>
          </div>
          <div class="flex items-center gap-3">
            <label class="text-xs" style="color:#8b949e;">Endian</label>
            <select
              class="text-xs rounded px-2 py-0.5"
              style="background:#0d1117; color:#e6edf3; border:1px solid #30363d; outline:none;"
              v-model="rocStore.dbEndian"
            >
              <option value="abcd">ABCD</option>
              <option value="dcba">DCBA</option>
              <option value="cdab">CDAB (ROC)</option>
              <option value="badc">BADC</option>
            </select>
            <button
              class="text-xs px-3 py-1 rounded font-medium transition-colors"
              style="background:#21262d; color:#e6edf3; border:1px solid #30363d;"
              :disabled="sync.loading"
              @click="sync.retryStation(sync.selectedIdx, stations, sessionId)"
            >Reintentar fallos</button>
          </div>
        </div>

        <!-- Scrollable table -->
        <div class="overflow-auto" style="max-height:560px; background:#0d1117;">
          <table style="width:100%; border-collapse:collapse; font-family:'JetBrains Mono',monospace; font-size:11px; white-space:nowrap; min-width:1000px;">
            <thead style="position:sticky; top:0; z-index:10;">
              <tr style="background:#161b22; border-bottom:1px solid #21262d;">
                <th style="padding:6px 12px; text-align:right; color:#8b949e; font-weight:500; border-right:1px solid #21262d;">Ptr</th>
                <th style="padding:6px 12px; text-align:left; color:#8b949e; font-weight:500; border-right:1px solid #21262d;">Fecha / Hora</th>
                <th
                  v-for="(name, i) in sigLabels" :key="i"
                  style="padding:6px 12px; text-align:right; color:#8b949e; font-weight:500; border-right:1px solid #21262d; max-width:120px; overflow:hidden; text-overflow:ellipsis;"
                  :title="name"
                >{{ name }}</th>
                <th style="padding:6px 12px; text-align:center; color:#8b949e; font-weight:500;">OK</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="(rec, idx) in currentRecords" :key="rec.ptr"
                :style="`background:${rec.valid ? (idx % 2 === 0 ? '#0d1117' : '#0f1319') : '#160a0a'}; border-bottom:1px solid #21262d;`"
              >
                <td style="padding:3px 12px; text-align:right; color:#484f58; border-right:1px solid #1c2128;">{{ rec.ptr }}</td>
                <td style="padding:3px 12px; text-align:left; color:#8b949e; border-right:1px solid #1c2128;">{{ fmtRecDate(rec) }}</td>
                <td
                  v-for="(_, si) in sigLabels" :key="si"
                  style="padding:3px 12px; text-align:right; border-right:1px solid #1c2128; font-variant-numeric:tabular-nums;"
                  :style="`color:${rec.valid && rec.modes?.[si] ? '#7ad400' : '#484f58'}`"
                >
                  {{ rec.valid && rec.modes?.[si]
                    ? rec.modes[si][rocStore.dbEndian]?.toFixed(4)
                    : '—' }}
                </td>
                <td style="padding:3px 12px; text-align:center;">
                  <span :style="`font-size:9px; color:${rec.valid ? '#3fb950' : '#f85149'}`">
                    {{ rec.valid ? '●' : '○' }}
                  </span>
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
const activeTab = ref('chart')

// Default signal labels (used when station has no signal_names in config)
const DEFAULT_SIG = [
  'Flow Min', 'Raw Pulses', 'Pf PSI', 'Tf DEG F',
  'Multiplier', 'Uncorr Vol MCF', 'Vol Accum MCF', 'Energy MMBTU',
]

const currentRecords = computed(() =>
  sync.selectedIdx ? sync.stationResults[sync.selectedIdx] : null
)

// Resolve station config for the currently selected task key ("STATION / M1" → "STATION")
const currentStation = computed(() => {
  if (!sync.selectedIdx) return null
  const stationName = sync.selectedIdx.split(' / ')[0]
  return stations.value.find(s => s.name === stationName) || null
})

// Signal names from config (falls back to DEFAULT_SIG)
const currentSignalNames = computed(() => currentStation.value?.signal_names || [])

// Labels used in the table header (config → DEFAULT_SIG)
const sigLabels = computed(() =>
  DEFAULT_SIG.map((d, i) => currentSignalNames.value[i] || d)
)

// ── Date decoder (matches MultiSignalChart) ──────────────────────────────────
function parseRocDate(dateRaw, timeRaw) {
  if (!dateRaw && !timeRaw) return null
  const year   = ((dateRaw >> 9) & 0x7F) + 2000
  const month  = (dateRaw >> 5) & 0x0F
  const day    = dateRaw & 0x1F
  const hour   = (timeRaw >> 11) & 0x1F
  const minute = (timeRaw >> 5) & 0x3F
  if (month < 1 || month > 12 || day < 1 || day > 31 || year < 2000 || year > 2099) return null
  return new Date(year, month - 1, day, hour, minute)
}

function fmtRecDate(rec) {
  const d = parseRocDate(rec?.date_raw, rec?.time_raw)
  if (!d) return '—'
  const dd = String(d.getDate()).padStart(2, '0')
  const mm = String(d.getMonth() + 1).padStart(2, '0')
  const hh = String(d.getHours()).padStart(2, '0')
  const mn = String(d.getMinutes()).padStart(2, '0')
  return `${dd}/${mm}/${d.getFullYear()} ${hh}:${mn}`
}

// ── Actions ──────────────────────────────────────────────────────────────────
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
