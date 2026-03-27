<template>
  <div class="space-y-2">

    <!-- ── Signal pills: toggle + rename ─────────────────────────────────── -->
    <div class="flex flex-wrap gap-1.5 items-center pb-2 border-b border-g-100">
      <template v-for="(sig, i) in signals" :key="i">
        <div
          v-if="sig && sig.active"
          class="flex items-center gap-1.5 px-2 py-0.5 rounded-full border text-xs cursor-pointer select-none transition-all"
          :class="sig.visible ? 'border-g-300 bg-white shadow-sm' : 'border-g-100 bg-g-50 opacity-40'"
          @click="toggleSignal(i)"
        >
          <span class="w-2.5 h-2.5 rounded-full shrink-0" :style="`background:${sig.color}`" />
          <span
            v-if="!sig.editing"
            class="font-medium text-g-700 whitespace-nowrap"
            title="Doble clic para renombrar"
            @dblclick.stop="startEdit(i)"
          >{{ sig.name }}</span>
          <input
            v-else
            :ref="el => { if (el) _nameEl[i] = el }"
            class="border border-lime rounded outline-none text-g-700 font-medium px-1"
            style="width:88px; font-size:11px;"
            v-model="sig.name"
            @blur="finishEdit(i)"
            @keydown.enter="finishEdit(i)"
            @keydown.esc.stop="cancelEdit(i)"
            @click.stop
          />
        </div>
      </template>

      <!-- Zoom preset buttons -->
      <div class="ml-auto flex items-center gap-1">
        <span class="text-xs text-g-400 mr-1">Vista:</span>
        <button class="btn btn-sm btn-ghost" @click="setWindow(24)">24h</button>
        <button class="btn btn-sm btn-ghost" @click="setWindow(168)">7d</button>
        <button class="btn btn-sm btn-ghost" @click="resetZoom">Todo</button>
      </div>
    </div>

    <!-- ── Chart.js canvas ────────────────────────────────────────────────── -->
    <div v-if="hasData" style="height:360px; position:relative;">
      <Line ref="chartRef" :data="chartData" :options="chartOptions" />
    </div>
    <div v-else class="flex items-center justify-center rounded-lg" style="height:200px;background:#f8faf8;">
      <span class="text-g-400 text-sm">Sin datos para mostrar</span>
    </div>

    <!-- ── Hex inspector ──────────────────────────────────────────────────── -->
    <div v-if="hoveredRecord"
         class="font-mono text-xs rounded border border-g-200 bg-g-50 p-2 leading-relaxed"
         style="word-break:break-all;">
      <span class="text-g-400">ptr </span><span class="text-forest font-bold">{{ hoveredRecord.ptr }}</span>
      <span class="text-g-300 mx-2">·</span>
      <span class="text-g-400">{{ hoveredRecord.fecha }} {{ hoveredRecord.hora }}</span>
      <span class="text-g-300 mx-2">·</span>
      <span class="text-g-500">hex </span><span class="text-g-700">{{ formattedHex }}</span>
    </div>
    <div v-else-if="hasData" class="text-xs text-g-300 italic pl-1">Pase el cursor sobre la gráfica para ver el hex del registro</div>

  </div>
</template>

<script setup>
import { ref, computed, watch, nextTick, onMounted } from 'vue'
import {
  Chart as ChartJS,
  LinearScale,
  PointElement,
  LineElement,
  Tooltip,
  Legend,
  Filler,
} from 'chart.js'
import zoomPlugin from 'chartjs-plugin-zoom'
import { Line } from 'vue-chartjs'

ChartJS.register(LinearScale, PointElement, LineElement, Tooltip, Legend, Filler, zoomPlugin)

// ── Props ─────────────────────────────────────────────────────────────────────
const props = defineProps({
  records:     { type: Array,  default: () => [] },
  endian:      { type: String, default: 'cdab'   },
  stationName: { type: String, default: ''       },
  signalNames: { type: Array,  default: () => [] },
})

// ── Signal defaults (modes[2..9] — first two floats are date/time) ────────────
const DEFAULTS = [
  { name: 'Min. Flujo Acum.',  color: '#7AD400' },
  { name: 'Pulsos Hora',       color: '#0ea5e9' },
  { name: 'Presión Estática',  color: '#f59e0b' },
  { name: 'Temperatura',       color: '#ec4899' },
  { name: 'Multiplicador',     color: '#8b5cf6' },
  { name: 'Vol. No Corr. MCF', color: '#14b8a6' },
  { name: 'Vol. Acum. MCF',    color: '#f97316' },
  { name: 'Energía Acum. MMBTU', color: '#007934' },
]
// SIGNAL_OFFSET: skip modes[0] (date float) and modes[1] (time float)
const SIGNAL_OFFSET = 2

// active = signal is used by this station/medidor.
// A signal is inactive when signalNames is provided and its slot is "".
const signals = ref(DEFAULTS.map(d => ({ ...d, active: true, visible: true, editing: false, _bk: '' })))
const chartRef     = ref(null)
const _nameEl      = {}
const hoveredRecord = ref(null)

const formattedHex = computed(() => {
  const h = hoveredRecord.value?.hex || ''
  // Group into 8-char (4-byte) blocks for readability
  return h.match(/.{1,8}/g)?.join(' ') || h
})

// ── Timestamp axis (from rec.ts — unix seconds embedded in each record) ───────
const validRecords = computed(() => props.records.filter(r => r?.valid && r.ts))

const xMin = computed(() => {
  if (!validRecords.value.length) return 0
  return validRecords.value[0].ts * 1000
})
const xMax = computed(() => {
  if (!validRecords.value.length) return 1
  return validRecords.value[validRecords.value.length - 1].ts * 1000
})

function fmtTick(val) {
  const d = new Date(val)
  const hh = String(d.getHours()).padStart(2, '0')
  const dd = String(d.getDate()).padStart(2, '0')
  const mm = String(d.getMonth() + 1).padStart(2, '0')
  return `${hh}h ${dd}/${mm}`
}

function fmtTooltipTitle(val) {
  const d = new Date(val)
  const hh = String(d.getHours()).padStart(2, '0')
  const mn = String(d.getMinutes()).padStart(2, '0')
  const dd = String(d.getDate()).padStart(2, '0')
  const mmm = String(d.getMonth() + 1).padStart(2, '0')
  const yyyy = d.getFullYear()
  return `${dd}/${mmm}/${yyyy} ${hh}:${mn}`
}

// ── LocalStorage persistence ──────────────────────────────────────────────────
const lsKey = name => `roc_sig_${name}`

function loadConfig() {
  const raw = props.stationName && localStorage.getItem(lsKey(props.stationName))
  const namesGiven = props.signalNames?.length > 0
  signals.value = DEFAULTS.map((d, i) => {
    const cfgName = props.signalNames?.[i]
    // active = false when signalNames explicitly provides "" (unused slot for this medidor)
    const active  = !namesGiven || cfgName !== ''
    let name      = (cfgName && cfgName !== '') ? cfgName : d.name
    let visible   = active  // inactive signals start hidden
    if (raw && active) {
      try {
        const s = JSON.parse(raw)[i]
        if (s?.name)                         name    = s.name
        if (typeof s?.visible === 'boolean') visible = s.visible
      } catch (_) {}
    }
    return { ...d, name, active, visible, editing: false, _bk: '' }
  })
}

function saveConfig() {
  if (!props.stationName) return
  localStorage.setItem(lsKey(props.stationName),
    JSON.stringify(signals.value.map(s => ({ name: s.name, visible: s.visible }))))
}

function toggleSignal(i) {
  signals.value[i].visible = !signals.value[i].visible
  saveConfig()
  const chart = chartRef.value?.chart
  if (chart) {
    chart.setDatasetVisibility(i, signals.value[i].visible)
    chart.update('none')
  }
}

function startEdit(i) {
  signals.value[i]._bk    = signals.value[i].name
  signals.value[i].editing = true
  nextTick(() => _nameEl[i]?.focus())
}
function finishEdit(i) {
  const s = signals.value[i]
  if (!s.name.trim()) s.name = s._bk
  s.editing = false
  saveConfig()
  const chart = chartRef.value?.chart
  if (chart) { chart.data.datasets[i].label = s.name; chart.update('none') }
}
function cancelEdit(i) {
  signals.value[i].name    = signals.value[i]._bk
  signals.value[i].editing = false
}

// ── Data computation ──────────────────────────────────────────────────────────
const hasData = computed(() => validRecords.value.length > 0)

// Real values [si][ri] → float | null  (signals start at modeIdx = SIGNAL_OFFSET + si)
// Inactive signals (empty name slot) return all-null so they produce no chart line.
const realVals = computed(() =>
  DEFAULTS.map((_, si) => {
    if (!signals.value[si]?.active) return props.records.map(() => null)
    const modeIdx = SIGNAL_OFFSET + si
    return props.records.map(r => {
      if (!r?.valid || !r.ts || !r.modes?.[modeIdx]) return null
      const v = r.modes[modeIdx][props.endian]
      return (v != null && isFinite(v)) ? v : null
    })
  })
)

// Per-signal min/max for normalization to 0-1
const sigStats = computed(() =>
  realVals.value.map(vs => {
    const valid = vs.filter(v => v !== null)
    if (!valid.length) return { min: 0, rng: 1 }
    const mn = Math.min(...valid), mx = Math.max(...valid)
    return { min: mn, rng: mx - mn || 1 }
  })
)

const normalizedVals = computed(() =>
  realVals.value.map((vs, si) => {
    const { min, rng } = sigStats.value[si]
    return vs.map(v => v === null ? null : (v - min) / rng)
  })
)

// ── Chart.js config ───────────────────────────────────────────────────────────
const chartData = computed(() => ({
  datasets: DEFAULTS.map((d, si) => ({
    label:            signals.value[si]?.name  ?? d.name,
    hidden:           !signals.value[si]?.active,
    data:             normalizedVals.value[si]
                        .map((y, i) => {
                          if (y === null) return null
                          const ts = props.records[i]?.ts
                          if (!ts) return null
                          return { x: ts * 1000, y }
                        })
                        .filter(Boolean),
    borderColor:      d.color,
    backgroundColor:  d.color + '18',
    fill:             true,
    borderWidth:      1.5,
    pointRadius:      0,
    pointHoverRadius: 4,
    tension:          0.1,
    spanGaps:         false,
  })),
}))

const chartOptions = computed(() => ({
  responsive:          true,
  maintainAspectRatio: false,
  animation:           false,
  interaction: { mode: 'index', intersect: false },

  onHover: (_event, elements, chart) => {
    if (!elements?.length) { hoveredRecord.value = null; return }
    const xMs = chart.data.datasets[elements[0].datasetIndex]?.data[elements[0].index]?.x
    if (!xMs) { hoveredRecord.value = null; return }
    hoveredRecord.value = props.records.find(r => r.ts && Math.abs(r.ts * 1000 - xMs) < 1800000) || null
  },

  plugins: {
    legend: { display: false },

    tooltip: {
      backgroundColor: 'rgba(255,255,255,0.97)',
      borderColor:     '#e5e7eb',
      borderWidth:     1,
      titleColor:      '#111827',
      bodyColor:       '#374151',
      titleFont: { family: "'JetBrains Mono', monospace", size: 11, weight: 'bold' },
      bodyFont:  { family: "'JetBrains Mono', monospace", size: 11 },
      padding:   10,
      callbacks: {
        title: (items) => fmtTooltipTitle(items[0]?.parsed?.x ?? 0),
        label: (ctx) => {
          const si = ctx.datasetIndex
          if (!signals.value[si]?.active || !signals.value[si]?.visible) return null
          const xMs = ctx.parsed.x
          const ri = props.records.findIndex(r => r.ts && Math.abs(r.ts * 1000 - xMs) < 1800000)
          const v = realVals.value[si]?.[ri >= 0 ? ri : 0]
          return `  ${signals.value[si].name}: ${v !== null && v !== undefined ? v.toFixed(4) : '—'}`
        },
        labelColor: (ctx) => ({
          borderColor:     DEFAULTS[ctx.datasetIndex]?.color,
          backgroundColor: DEFAULTS[ctx.datasetIndex]?.color,
          borderWidth: 2, borderRadius: 2,
        }),
      },
    },

    zoom: {
      zoom:   { wheel: { enabled: true }, pinch: { enabled: false }, mode: 'x' },
      pan:    { enabled: true, mode: 'x' },
      limits: { x: { min: xMin.value, max: xMax.value, minRange: 3600 * 1000 } },
    },
  },

  scales: {
    x: {
      type: 'linear',
      min:  xMin.value,
      max:  xMax.value,
      ticks: {
        color:         '#9ab59a',
        font:          { size: 9, family: "'JetBrains Mono', monospace" },
        maxTicksLimit: 8,
        callback:      (val) => fmtTick(val),
      },
      grid: { color: '#d1fae5', lineWidth: 0.5 },
    },
    y: {
      min:     0,
      max:     1,
      display: false,
      grid:    { color: '#d1fae5' },
    },
  },
}))

// ── Zoom controls ─────────────────────────────────────────────────────────────
function setWindow(hours) {
  const chart = chartRef.value?.chart
  if (!chart) return
  const max = xMax.value
  const min = max - hours * 3600 * 1000
  chart.zoomScale('x', { min: Math.max(xMin.value, min), max }, 'none')
}

function resetZoom() {
  chartRef.value?.chart?.resetZoom()
}

// ── Lifecycle ─────────────────────────────────────────────────────────────────
onMounted(() => {
  loadConfig()
  nextTick(() => setWindow(24))
})

watch([() => props.stationName, () => props.signalNames], loadConfig)

watch(() => props.records, () => {
  hoveredRecord.value = null
  nextTick(() => {
    const chart = chartRef.value?.chart
    if (!chart) return
    signals.value.forEach((s, i) => chart.setDatasetVisibility(i, s.visible))
    chart.update('none')
    setWindow(24)
  })
})
</script>
