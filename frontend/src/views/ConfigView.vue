<template>
  <div class="flex h-full overflow-hidden bg-g-50">

    <!-- ── Station list (left panel) ──────────────────────────────────────── -->
    <div class="w-56 shrink-0 flex flex-col border-r border-g-200 bg-white overflow-hidden">
      <div class="px-3 py-2 border-b border-g-200 flex items-center justify-between">
        <span class="text-xs font-semibold text-g-700 uppercase tracking-wider">Estaciones</span>
        <button
          class="text-xs px-2 py-0.5 rounded font-medium transition-colors"
          style="background:#f0fce8;color:#007934;border:1px solid #c3f09a;"
          @click="addStation"
        >+ Nueva</button>
      </div>
      <div class="flex-1 overflow-y-auto">
        <div
          v-for="(st, idx) in cfg.stations"
          :key="idx"
          class="px-3 py-2 cursor-pointer border-b border-g-100 transition-colors"
          :class="selectedIdx === idx
            ? 'bg-lime/10 border-l-2 border-l-lime'
            : 'hover:bg-g-50'"
          @click="selectedIdx = idx"
        >
          <div class="text-xs font-semibold truncate" :class="selectedIdx === idx ? 'text-forest' : 'text-g-800'">
            {{ st.name || '(sin nombre)' }}
          </div>
          <div class="font-mono text-g-400 truncate" style="font-size:10px;">{{ st.ip }}:{{ st.port }}</div>
        </div>
      </div>
    </div>

    <!-- ── Edit form (right panel) ────────────────────────────────────────── -->
    <div class="flex-1 overflow-y-auto p-4" v-if="station">
      <div class="max-w-3xl mx-auto space-y-4">

        <!-- Header -->
        <div class="flex items-center justify-between">
          <h2 class="text-base font-bold text-g-900">{{ station.name || 'Nueva estación' }}</h2>
          <div class="flex gap-2">
            <button
              class="text-xs px-3 py-1.5 rounded font-medium text-red-600 border border-red-200 hover:bg-red-50 transition-colors"
              @click="removeStation(selectedIdx)"
            >Eliminar</button>
            <button
              class="text-xs px-4 py-1.5 rounded font-semibold text-white transition-colors"
              style="background:#007934;"
              :disabled="saving"
              @click="save"
            >{{ saving ? 'Guardando…' : 'Guardar config.yaml' }}</button>
          </div>
        </div>

        <div v-if="saveError" class="text-xs text-red-600 bg-red-50 border border-red-200 rounded px-3 py-2">{{ saveError }}</div>
        <div v-if="saveOk" class="text-xs text-forest bg-lime/10 border border-lime/40 rounded px-3 py-2">✓ config.yaml guardado correctamente</div>

        <!-- Connection -->
        <div class="card p-4 space-y-3">
          <h3 class="text-xs font-semibold text-g-500 uppercase tracking-wider mb-2">Conexión</h3>
          <div class="grid grid-cols-3 gap-3">
            <div class="col-span-3 sm:col-span-1">
              <label class="lbl">Nombre</label>
              <input class="fi" v-model="station.name" placeholder="EET TASAJERA" />
            </div>
            <div>
              <label class="lbl">IP</label>
              <input class="fi font-mono" v-model="station.ip" placeholder="10.0.0.1" />
            </div>
            <div>
              <label class="lbl">Puerto</label>
              <input class="fi" type="number" v-model.number="station.port" min="1" max="65535" />
            </div>
          </div>
          <div class="grid grid-cols-3 gap-3">
            <div>
              <label class="lbl">Slave ID</label>
              <input class="fi" type="number" v-model.number="station.id" min="0" max="247" />
            </div>
            <div>
              <label class="lbl">Data Registers Count</label>
              <input class="fi" type="number" v-model.number="station.data_registers_count" min="1" max="4" />
            </div>
            <div>
              <label class="lbl">Data Type</label>
              <select class="fs">
                <option value="float32">float32</option>
              </select>
            </div>
          </div>
        </div>

        <!-- Endianness -->
        <div class="card p-4">
          <h3 class="text-xs font-semibold text-g-500 uppercase tracking-wider mb-3">Endianness</h3>
          <div class="grid grid-cols-2 gap-3">
            <div>
              <label class="lbl">Ptr Endian <span class="text-g-400">(lectura de puntero)</span></label>
              <select class="fs" v-model="station.ptr_endian">
                <option value="cdab">CDAB (ROC estándar)</option>
                <option value="abcd">ABCD (Big-Endian)</option>
                <option value="dcba">DCBA (Little-Endian)</option>
                <option value="badc">BADC (Byte-Swap)</option>
              </select>
            </div>
            <div>
              <label class="lbl">DB Endian <span class="text-g-400">(registros históricos)</span></label>
              <select class="fs" v-model="station.db_endian">
                <option value="cdab">CDAB (ROC estándar)</option>
                <option value="abcd">ABCD (Big-Endian)</option>
                <option value="dcba">DCBA (Little-Endian)</option>
                <option value="badc">BADC (Byte-Swap)</option>
              </select>
            </div>
          </div>
        </div>

        <!-- Single-meter addresses + signal names -->
        <template v-if="!station.medidores?.length">
          <div class="card p-4 space-y-3">
            <h3 class="text-xs font-semibold text-g-500 uppercase tracking-wider mb-2">Registros Modbus</h3>
            <div class="grid grid-cols-2 gap-3">
              <div>
                <label class="lbl">Dirección Puntero</label>
                <input class="fi font-mono" type="number" v-model.number="station.pointer_address" />
              </div>
              <div>
                <label class="lbl">Dirección Base Historial</label>
                <input class="fi font-mono" type="number" v-model.number="station.base_data_address" />
              </div>
            </div>
          </div>

          <SignalNamesEditor
            :names="station.signal_names"
            @update="station.signal_names = $event"
          />
        </template>

        <!-- Multi-meter section -->
        <template v-else>
          <div class="card p-4">
            <div class="flex items-center justify-between mb-3">
              <h3 class="text-xs font-semibold text-g-500 uppercase tracking-wider">Medidores ({{ station.medidores.length }})</h3>
              <button
                class="text-xs px-2 py-0.5 rounded font-medium transition-colors"
                style="background:#f0fce8;color:#007934;border:1px solid #c3f09a;"
                @click="addMedidor"
              >+ Medidor</button>
            </div>

            <!-- Medidor tabs -->
            <div class="flex gap-1 mb-3 flex-wrap">
              <button
                v-for="(m, mi) in station.medidores"
                :key="mi"
                class="text-xs px-3 py-1 rounded-full font-medium transition-colors border"
                :class="selectedMed === mi
                  ? 'text-white border-forest'
                  : 'text-g-600 border-g-200 hover:border-g-300'"
                :style="selectedMed === mi ? 'background:#007934;' : ''"
                @click="selectedMed = mi"
              >{{ m.name || `M${mi+1}` }}</button>
            </div>

            <!-- Medidor fields -->
            <div v-if="medidor" class="space-y-3">
              <div class="grid grid-cols-3 gap-3">
                <div>
                  <label class="lbl">Label (número)</label>
                  <input class="fi" type="number" v-model.number="medidor.label" min="1" />
                </div>
                <div>
                  <label class="lbl">Nombre</label>
                  <input class="fi" v-model="medidor.name" placeholder="M1" />
                </div>
              </div>
              <div class="grid grid-cols-2 gap-3">
                <div>
                  <label class="lbl">Dirección Puntero</label>
                  <input class="fi font-mono" type="number" v-model.number="medidor.pointer_address" />
                </div>
                <div>
                  <label class="lbl">Dirección Base Historial</label>
                  <input class="fi font-mono" type="number" v-model.number="medidor.base_data_address" />
                </div>
              </div>
              <!-- Endian override per medidor -->
              <div class="grid grid-cols-2 gap-3">
                <div>
                  <label class="lbl">Ptr Endian (override) <span class="text-g-400">opcional</span></label>
                  <select class="fs" v-model="medidor.ptr_endian">
                    <option value="">— hereda de estación —</option>
                    <option value="cdab">CDAB</option>
                    <option value="abcd">ABCD</option>
                    <option value="dcba">DCBA</option>
                    <option value="badc">BADC</option>
                  </select>
                </div>
                <div>
                  <label class="lbl">DB Endian (override) <span class="text-g-400">opcional</span></label>
                  <select class="fs" v-model="medidor.db_endian">
                    <option value="">— hereda de estación —</option>
                    <option value="cdab">CDAB</option>
                    <option value="abcd">ABCD</option>
                    <option value="dcba">DCBA</option>
                    <option value="badc">BADC</option>
                  </select>
                </div>
              </div>

              <SignalNamesEditor
                :names="medidor.signal_names"
                @update="medidor.signal_names = $event"
              />

              <button
                v-if="station.medidores.length > 1"
                class="text-xs text-red-500 hover:text-red-700"
                @click="removeMedidor(selectedMed)"
              >Eliminar este medidor</button>
            </div>
          </div>
        </template>

        <!-- Toggle between single / multi meter -->
        <div class="text-xs text-g-500">
          <button
            class="underline hover:text-g-700"
            @click="toggleMedidores"
          >
            {{ station.medidores?.length ? 'Convertir a estación simple (sin medidores)' : 'Agregar múltiples medidores' }}
          </button>
        </div>

      </div>
    </div>

    <!-- Empty state -->
    <div v-else class="flex-1 flex items-center justify-center text-g-400 text-sm">
      Selecciona una estación para editar
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import axios from 'axios'
import { useConfigStore } from '../stores/config'

const configStore = useConfigStore()

// ── Inline sub-component: signal names editor ────────────────────────────────
const SignalNamesEditor = {
  props: { names: Array },
  emits: ['update'],
  setup(props, { emit }) {
    const local = ref(padTo8(props.names))
    watch(() => props.names, v => { local.value = padTo8(v) })
    function padTo8(arr) {
      const a = [...(arr || [])]
      while (a.length < 8) a.push('')
      return a.slice(0, 8)
    }
    function onChange() { emit('update', [...local.value]) }
    const SIG_COLORS = ['#7AD400','#0ea5e9','#f59e0b','#ec4899','#8b5cf6','#14b8a6','#f97316','#007934']
    return { local, onChange, SIG_COLORS }
  },
  template: `
    <div class="card p-4">
      <h3 class="text-xs font-semibold text-g-500 uppercase tracking-wider mb-3">
        Nombres de Señales <span class="font-normal normal-case text-g-400">(dato3 … dato10)</span>
      </h3>
      <div class="grid grid-cols-2 gap-2">
        <div v-for="(_, i) in local" :key="i" class="flex items-center gap-2">
          <span class="w-2 h-2 rounded-full shrink-0" :style="'background:'+SIG_COLORS[i]"></span>
          <span class="text-xs text-g-400 w-4 shrink-0">{{ i+1 }}</span>
          <input
            class="fi fi-sm flex-1"
            v-model="local[i]"
            :placeholder="'Señal ' + (i+1)"
            @input="onChange"
          />
        </div>
      </div>
    </div>
  `
}

// ── State ─────────────────────────────────────────────────────────────────────
const cfg         = ref({ stations: [] })
const selectedIdx = ref(null)
const selectedMed = ref(0)
const saving      = ref(false)
const saveOk      = ref(false)
const saveError   = ref('')

const station = computed(() => selectedIdx.value !== null ? cfg.value.stations[selectedIdx.value] : null)
const medidor = computed(() => station.value?.medidores?.[selectedMed.value] ?? null)

// Reset medidor tab when station changes
watch(selectedIdx, () => { selectedMed.value = 0; saveOk.value = false; saveError.value = '' })

// ── Load ──────────────────────────────────────────────────────────────────────
async function load() {
  try {
    const { data } = await axios.get('/api/config')
    cfg.value = data
    if (data.stations?.length) selectedIdx.value = 0
  } catch (_) {}
}

// Also update cfg when the shared store refreshes (e.g. if opened while already cached)
watch(() => configStore.stations, () => {
  if (!cfg.value.stations?.length && configStore.stations.length) {
    cfg.value = { stations: JSON.parse(JSON.stringify(configStore.stations)) }
    selectedIdx.value = 0
  }
})

// ── Save ──────────────────────────────────────────────────────────────────────
async function save() {
  saving.value = true
  saveOk.value = false
  saveError.value = ''
  try {
    await axios.post('/api/config/save', cfg.value)
    saveOk.value = true
    configStore.load()  // refresh sidebar + all consumers
    setTimeout(() => { saveOk.value = false }, 4000)
  } catch (e) {
    saveError.value = e.response?.data?.error || e.message
  } finally {
    saving.value = false
  }
}

// ── Station management ────────────────────────────────────────────────────────
function addStation() {
  cfg.value.stations.push({
    name: '', ip: '', port: 502, id: 1,
    ptr_endian: 'cdab', db_endian: 'cdab',
    data_type: 'float32', data_registers_count: 2,
    pointer_address: 10000, base_data_address: 700,
    signal_names: Array(8).fill(''),
  })
  selectedIdx.value = cfg.value.stations.length - 1
}

function removeStation(idx) {
  cfg.value.stations.splice(idx, 1)
  selectedIdx.value = cfg.value.stations.length ? 0 : null
}

function toggleMedidores() {
  const st = station.value
  if (st.medidores?.length) {
    delete st.medidores
  } else {
    st.medidores = [{
      label: 1, name: 'M1',
      pointer_address: st.pointer_address ?? 10000,
      base_data_address: st.base_data_address ?? 700,
      ptr_endian: '', db_endian: '',
      signal_names: Array(8).fill(''),
    }]
    delete st.pointer_address
    delete st.base_data_address
  }
  selectedMed.value = 0
}

function addMedidor() {
  const meds = station.value.medidores
  meds.push({
    label: meds.length + 1, name: `M${meds.length + 1}`,
    pointer_address: 10000, base_data_address: 700,
    ptr_endian: '', db_endian: '',
    signal_names: Array(8).fill(''),
  })
  selectedMed.value = meds.length - 1
}

function removeMedidor(mi) {
  station.value.medidores.splice(mi, 1)
  selectedMed.value = Math.max(0, mi - 1)
}

load()
</script>
