<template>
  <div class="w-64 shrink-0 flex flex-col overflow-hidden" style="background:#0f1710;">

    <!-- ── Brand ─────────────────────────────────────────────────────────── -->
    <div class="px-4 py-3 shrink-0" style="border-bottom:1px solid #1e2a1f;">
      <div class="flex items-center gap-2">
        <svg width="20" height="20" viewBox="0 0 32 32">
          <rect width="32" height="32" rx="5" fill="#1a2e1a"/>
          <polyline points="2,16 6,16 8,7 10,25 12,16 16,16 18,9 20,23 22,16 30,16"
            fill="none" stroke="#7AD400" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"/>
        </svg>
        <div>
          <div class="text-white font-bold text-sm tracking-wide leading-none">ROC Modbus Expert</div>
          <div class="text-xs mt-0.5" style="color:#8aaa8a;">EPM · v4.1</div>
        </div>
      </div>
    </div>

    <!-- ── Scrollable content ─────────────────────────────────────────────── -->
    <div class="flex-1 overflow-y-auto space-y-0">

      <!-- TCP Connection -->
      <div class="sbar-section">
        <button
          class="w-full flex items-center justify-between mb-2 group"
          @click="showConn = !showConn"
        >
          <span class="sbar-label mb-0">Conexión TCP</span>
          <svg class="transition-transform" :class="showConn ? 'rotate-180' : ''"
               width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="#8aaa8a" stroke-width="2.5">
            <polyline points="6 9 12 15 18 9"/>
          </svg>
        </button>
        <div v-show="showConn">
          <div class="grid grid-cols-2 gap-2 mb-2">
            <div>
              <label class="lbl" style="color:#8aaa8a;">IP</label>
              <input class="fi fi-sm" v-model="conn.ip" placeholder="192.168.1.1" style="background:#141c14;border-color:#2a3d2a;color:#d4e8d4;" />
            </div>
            <div>
              <label class="lbl" style="color:#8aaa8a;">Puerto</label>
              <input class="fi fi-sm" v-model.number="conn.port" type="number" min="1" max="65535" style="background:#141c14;border-color:#2a3d2a;color:#d4e8d4;" />
            </div>
          </div>
          <div class="grid grid-cols-2 gap-2">
            <div>
              <label class="lbl" style="color:#8aaa8a;">Slave ID</label>
              <input class="fi fi-sm" v-model.number="conn.slaveId" type="number" min="0" max="247" style="background:#141c14;border-color:#2a3d2a;color:#d4e8d4;" />
            </div>
            <div>
              <label class="lbl" style="color:#8aaa8a;">Endian</label>
              <select class="fs fi-sm" v-model="conn.endian" style="background:#141c14;border-color:#2a3d2a;color:#d4e8d4;">
                <option value="abcd">ABCD (BE)</option>
                <option value="dcba">DCBA (LE)</option>
                <option value="cdab">CDAB (ROC)</option>
                <option value="badc">BADC (BS)</option>
              </select>
            </div>
          </div>
        </div>
      </div>

      <!-- Station list -->
      <div class="sbar-section">
        <div class="flex items-center justify-between mb-2">
          <span class="sbar-label mb-0">Estaciones</span>
          <button
            class="flex items-center gap-1 text-xs px-2 py-0.5 rounded transition-colors"
            style="background:#1e3a22;color:#7ad400;border:1px solid #2a5a2e;"
            @click="configStore.load()"
          >
            <svg width="9" height="9" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><polyline points="1 4 1 10 7 10"/><path d="M3.51 15a9 9 0 1 0 .49-3.51"/></svg>
            Actualizar
          </button>
        </div>

        <div v-if="configStore.stations.length === 0" class="text-xs py-2" style="color:#8aaa8a;">
          Sin estaciones configuradas
        </div>

        <div v-for="st in configStore.stations" :key="st.name" class="mb-1">
          <!-- Station header row -->
          <div
            class="flex items-center gap-2 px-2 py-1.5 rounded-lg cursor-pointer transition-all group"
            :style="activeStation === st.name
              ? 'background:#1a2e1a; border-left:2px solid #7ad400; margin-left:-2px;'
              : 'background:transparent;'"
            @click="toggleStation(st)"
          >
            <!-- Expand arrow -->
            <svg
              class="shrink-0 transition-transform"
              :class="expandedStation === st.name ? 'rotate-90' : ''"
              width="8" height="8" viewBox="0 0 24 24" fill="none"
              stroke="#8aaa8a" stroke-width="3"
              @click.stop="expandedStation = expandedStation === st.name ? null : st.name"
            >
              <polyline points="9 18 15 12 9 6"/>
            </svg>
            <!-- Status dot -->
            <span class="w-1.5 h-1.5 rounded-full shrink-0"
              :style="activeStation === st.name ? 'background:#7ad400;box-shadow:0 0 4px #7ad400;' : 'background:#2a3d2a;'" />
            <!-- Name -->
            <div class="min-w-0 flex-1">
              <div class="text-xs font-semibold truncate" style="color:#c5cfc5;">{{ st.name }}</div>
              <div class="font-mono" style="font-size:9px;color:#8aaa8a;">{{ st.ip }}</div>
            </div>
            <!-- Sync data indicator -->
            <div v-if="hasSyncData(st.name)" class="shrink-0 flex gap-1">
              <button
                class="w-5 h-5 flex items-center justify-center rounded transition-colors"
                style="background:#1a2e1a;"
                title="Ver gráfica"
                @click.stop="goToChart(st.name)"
              >
                <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="#7ad400" stroke-width="2.5"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/></svg>
              </button>
              <button
                class="w-5 h-5 flex items-center justify-center rounded transition-colors"
                style="background:#1a2e1a;"
                title="Ver tabla"
                @click.stop="goToTable(st.name)"
              >
                <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="#7ad400" stroke-width="2.5"><rect x="3" y="3" width="18" height="18" rx="2"/><line x1="3" y1="9" x2="21" y2="9"/><line x1="3" y1="15" x2="21" y2="15"/><line x1="9" y1="3" x2="9" y2="21"/></svg>
              </button>
            </div>
          </div>

          <!-- Expanded: station config detail -->
          <div v-if="expandedStation === st.name" class="ml-4 mt-1 mb-1 space-y-1">

            <!-- Basic info -->
            <div class="text-xs rounded-lg p-2 space-y-1" style="background:#0d160d;">
              <div class="flex justify-between">
                <span style="color:#8aaa8a;">IP / Puerto</span>
                <span class="font-mono" style="color:#c5cfc5; font-size:10px;">{{ st.ip }}:{{ st.port }}</span>
              </div>
              <div class="flex justify-between">
                <span style="color:#8aaa8a;">Slave ID</span>
                <span class="font-mono" style="color:#c5cfc5; font-size:10px;">{{ st.id }}</span>
              </div>
              <div class="flex justify-between">
                <span style="color:#8aaa8a;">Ptr endian</span>
                <span class="font-mono uppercase" style="color:#7ad400; font-size:10px;">{{ st.ptr_endian || st.endian || '—' }}</span>
              </div>
              <div class="flex justify-between">
                <span style="color:#8aaa8a;">DB endian</span>
                <span class="font-mono uppercase" style="color:#7ad400; font-size:10px;">{{ st.db_endian || st.endian || '—' }}</span>
              </div>
              <div class="flex justify-between">
                <span style="color:#8aaa8a;">Qty puntero</span>
                <span class="font-mono" style="color:#c5cfc5; font-size:10px;">{{ st.data_registers_count ?? 2 }} reg.</span>
              </div>
            </div>

            <!-- Multi-meter: expandable per-medidor sections -->
            <template v-if="st.medidores?.length">
              <div class="text-xs rounded-lg p-2" style="background:#0d160d;">
                <div class="mb-1.5" style="color:#8aaa8a;">{{ st.medidores.length }} Medidores</div>
                <div v-for="m in st.medidores" :key="m.label" class="mb-1.5 last:mb-0">
                  <!-- Medidor header -->
                  <button
                    class="w-full flex items-center justify-between py-0.5"
                    @click="expandedMed === `${st.name}/${m.name}` ? expandedMed = null : expandedMed = `${st.name}/${m.name}`"
                  >
                    <span class="font-mono font-semibold" style="color:#7ad400; font-size:10px;">{{ m.name }}</span>
                    <div class="flex items-center gap-2" style="color:#484f58; font-size:9px;">
                      <span>Ptr {{ m.pointer_address }} · DB {{ m.base_data_address }}</span>
                      <svg class="transition-transform shrink-0"
                           :class="expandedMed === `${st.name}/${m.name}` ? 'rotate-90' : ''"
                           width="7" height="7" viewBox="0 0 24 24" fill="none" stroke="#484f58" stroke-width="3">
                        <polyline points="9 18 15 12 9 6"/>
                      </svg>
                    </div>
                  </button>

                  <!-- Medidor signals (expanded) -->
                  <div v-if="expandedMed === `${st.name}/${m.name}`" class="mt-1 pl-1 space-y-0.5">
                    <template v-if="activeSignals(m.signal_names || st.signal_names).length">
                      <div v-for="sig in activeSignals(m.signal_names || st.signal_names)" :key="sig.i"
                           class="flex items-center gap-1.5">
                        <span class="w-1.5 h-1.5 rounded-full shrink-0" :style="`background:${SIG_COLORS[sig.i]}`" />
                        <span style="color:#c5cfc5; font-size:9px;">{{ sig.name }}</span>
                      </div>
                    </template>
                    <span v-else style="color:#484f58; font-size:9px;">Sin señales configuradas</span>
                  </div>
                </div>
              </div>
            </template>

            <!-- Single-meter: station-level signal names -->
            <template v-else>
              <div v-if="activeSignals(st.signal_names).length" class="text-xs rounded-lg p-2" style="background:#0d160d;">
                <div class="mb-1" style="color:#8aaa8a;">Señales</div>
                <div v-for="sig in activeSignals(st.signal_names)" :key="sig.i"
                     class="flex items-center gap-1.5 py-0.5">
                  <span class="w-1.5 h-1.5 rounded-full shrink-0" :style="`background:${SIG_COLORS[sig.i]}`" />
                  <span style="color:#c5cfc5; font-size:10px;">{{ sig.name }}</span>
                </div>
              </div>
            </template>

            <!-- Apply button -->
            <button
              class="w-full text-xs py-1 rounded-lg text-center transition-colors font-medium"
              style="background:#1e3a22; color:#7ad400; border:1px solid #2a5a2e;"
              @click="applyStation(st)"
            >Aplicar para Consulta/ROC</button>

          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useConnectionStore } from '../../stores/connection'
import { useRocStore } from '../../stores/roc'
import { useSyncStore } from '../../stores/sync'
import { useConfigStore } from '../../stores/config'

const conn        = useConnectionStore()
const rocStore    = useRocStore()
const sync        = useSyncStore()
const configStore = useConfigStore()
const router      = useRouter()

const activeStation  = ref(null)
const expandedStation = ref(null)
const expandedMed    = ref(null)
const showConn       = ref(true)

const SIG_COLORS = [
  '#7AD400', '#0ea5e9', '#f59e0b', '#ec4899',
  '#8b5cf6', '#14b8a6', '#f97316', '#007934',
]

// Returns array of { i, name } for signals with non-empty names
function activeSignals(names) {
  if (!names?.length) return []
  return names
    .map((name, i) => ({ i, name }))
    .filter(s => s.name !== '')
}

function toggleStation(st) {
  expandedStation.value = expandedStation.value === st.name ? null : st.name
  expandedMed.value = null
}

function applyStation(st) {
  conn.applyStation(st)
  rocStore.applyStation(st)
  activeStation.value = st.name
}

function hasSyncData(stName) {
  return Object.keys(sync.stationResults).some(k => k.split(' / ')[0] === stName)
}

function goToChart(stName) {
  const taskKey = Object.keys(sync.stationResults).find(k => k.split(' / ')[0] === stName)
  if (taskKey) {
    sync.selectedIdx = taskKey
    sync.viewTab = 'chart'
    router.push('/sync')
  }
}

function goToTable(stName) {
  const taskKey = Object.keys(sync.stationResults).find(k => k.split(' / ')[0] === stName)
  if (taskKey) {
    sync.selectedIdx = taskKey
    sync.viewTab = 'table'
    router.push('/sync')
  }
}
</script>
