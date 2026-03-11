<template>
  <div class="space-y-2">

    <!-- ── Signal pills: toggle + rename ─────────────────────────────────── -->
    <div class="flex flex-wrap gap-1.5 items-center pb-2 border-b border-g-100">
      <div
        v-for="(sig, i) in signals" :key="i"
        class="flex items-center gap-1.5 px-2 py-0.5 rounded-full border text-xs cursor-pointer select-none transition-all"
        :class="sig.visible ? 'border-g-300 bg-white shadow-sm' : 'border-g-100 bg-g-50 opacity-40'"
        @click="toggleSignal(i)"
      >
        <span class="w-2.5 h-2.5 rounded-full shrink-0" :style="`background:${sig.color}`" />
        <span
          v-if="!sig.editing"
          class="font-medium text-g-700 whitespace-nowrap"
          :title="'Doble clic para renombrar'"
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

      <!-- Preset zoom buttons -->
      <div class="ml-auto flex items-center gap-1">
        <span class="text-xs text-g-400 mr-1">Vista:</span>
        <button class="btn btn-sm btn-ghost" @click="setWindow(24)">24h</button>
        <button class="btn btn-sm btn-ghost" @click="setWindow(168)">7d</button>
        <button class="btn btn-sm btn-ghost" @click="resetZoom">840h</button>
      </div>
    </div>

    <!-- ── Chart.js canvas ────────────────────────────────────────────────── -->
    <div v-if="hasData" style="height:360px; position:relative;">
      <Line ref="chartRef" :data="chartData" :options="chartOptions" />
    </div>
    <div v-else class="flex items-center justify-center rounded-lg" style="height:200px;background:#f8faf8;">
      <span class="text-g-400 text-sm">Sin datos para mostrar</span>
    </div>

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

// ── Signal defaults ───────────────────────────────────────────────────────────
const DEFAULTS = [
  { name: 'Flow Min',       color: '#7AD400' },
  { name: 'Raw Pulses',     color: '#0ea5e9' },
  { name: 'Pf PSI',         color: '#f59e0b' },
  { name: 'Tf DEG F',       color: '#ec4899' },
  { name: 'Multiplier',     color: '#8b5cf6' },
  { name: 'Uncorr Vol MCF', color: '#14b8a6' },
  { name: 'Vol Accum MCF',  color: '#f97316' },
  { name: 'Energy MMBTU',   color: '#007934' },
]

const signals  = ref(DEFAULTS.map(d => ({ ...d, visible: true, editing: false, _bk: '' })))
const chartRef = ref(null)
const _nameEl  = {}

// ── LocalStorage persistence ──────────────────────────────────────────────────
const lsKey = name => `roc_sig_${name}`

function loadConfig() {
  const raw = props.stationName && localStorage.getItem(lsKey(props.stationName))
  signals.value = DEFAULTS.map((d, i) => {
    let name    = props.signalNames?.[i] || d.name
    let visible = true
    if (raw) {
      try {
        const s = JSON.parse(raw)[i]
        if (s?.name)                         name    = s.name
        if (typeof s?.visible === 'boolean') visible = s.visible
      } catch (_) {}
    }
    return { ...d, name, visible, editing: false, _bk: '' }
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
  // Update chart dataset label
  const chart = chartRef.value?.chart
  if (chart) { chart.data.datasets[i].label = s.name; chart.update('none') }
}
function cancelEdit(i) {
  signals.value[i].name    = signals.value[i]._bk
  signals.value[i].editing = false
}

// ── ROC date decoding ─────────────────────────────────────────────────────────
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

function fmtTick(ri) {
  const r = props.records[ri]
  if (!r?.valid || !r.date_raw) return `Ptr ${ri}`
  const d = parseRocDate(r.date_raw, r.time_raw)
  if (!d) return `Ptr ${ri}`
  const hh = String(d.getHours()).padStart(2, '0')
  const dd = String(d.getDate()).padStart(2, '0')
  const mm = String(d.getMonth() + 1).padStart(2, '0')
  return `${hh}h ${dd}/${mm}`
}

function fmtTooltipTitle(ri) {
  const r = props.records[ri]
  if (!r?.valid || !r.date_raw) return `Ptr ${ri}`
  const d = parseRocDate(r.date_raw, r.time_raw)
  if (!d) return `Ptr ${ri}`
  const hh = String(d.getHours()).padStart(2, '0')
  const mn = String(d.getMinutes()).padStart(2, '0')
  const dd = String(d.getDate()).padStart(2, '0')
  const mmm = String(d.getMonth() + 1).padStart(2, '0')
  return `Ptr ${ri}  ·  ${dd}/${mmm} ${hh}:${mn}`
}

// ── Data computation ──────────────────────────────────────────────────────────
const hasData = computed(() => props.records?.some(r => r?.valid))

// Real values [si][ri] → float | null
const realVals = computed(() =>
  DEFAULTS.map((_, si) =>
    props.records.map(r => {
      if (!r?.valid || !r.modes?.[si]) return null
      const v = r.modes[si][props.endian]
      return (v != null && isFinite(v)) ? v : null
    })
  )
)

// Per-signal global min/max for normalization
const sigStats = computed(() =>
  realVals.value.map(vs => {
    const valid = vs.filter(v => v !== null)
    if (!valid.length) return { min: 0, rng: 1 }
    const mn = Math.min(...valid), mx = Math.max(...valid)
    return { min: mn, rng: mx - mn || 1 }
  })
)

// Normalized values [si][ri] → 0-1 | null
const normalizedVals = computed(() =>
  realVals.value.map((vs, si) => {
    const { min, rng } = sigStats.value[si]
    return vs.map(v => v === null ? null : (v - min) / rng)
  })
)

// ── Chart.js config ───────────────────────────────────────────────────────────
const chartData = computed(() => ({
  datasets: DEFAULTS.map((d, si) => ({
    label:           signals.value[si].name,
    data:            normalizedVals.value[si]
                       .map((y, x) => y !== null ? { x, y } : null)
                       .filter(Boolean),
    borderColor:     d.color,
    backgroundColor: d.color + '18',
    fill:            true,
    borderWidth:     1.5,
    pointRadius:     0,
    pointHoverRadius: 4,
    tension:         0.1,
    spanGaps:        false,
  })),
}))

const chartOptions = computed(() => ({
  responsive:          true,
  maintainAspectRatio: false,
  animation:           false,
  interaction: { mode: 'index', intersect: false },

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
        title: (items) => fmtTooltipTitle(Math.round(items[0]?.parsed?.x ?? 0)),
        label: (ctx) => {
          const si = ctx.datasetIndex
          const ri = Math.round(ctx.parsed.x)
          const v  = realVals.value[si]?.[ri]
          if (!signals.value[si]?.visible) return null
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
      zoom: { wheel: { enabled: true }, pinch: { enabled: false }, mode: 'x' },
      pan:  { enabled: true, mode: 'x' },
      limits: { x: { min: 0, max: 839, minRange: 2 } },
    },
  },

  scales: {
    x: {
      type: 'linear',
      min:  0,
      max:  839,
      ticks: {
        color:         '#9ab59a',
        font:          { size: 9, family: "'JetBrains Mono', monospace" },
        maxTicksLimit: 8,
        callback:      (val) => fmtTick(Math.round(val)),
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
function setWindow(size) {
  const chart = chartRef.value?.chart
  if (!chart) return
  chart.zoomScale('x', { min: Math.max(0, 840 - size), max: 839 }, 'none')
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

// Sync visibility after chart fully redraws when data changes
watch(() => props.records, () => {
  nextTick(() => {
    const chart = chartRef.value?.chart
    if (!chart) return
    signals.value.forEach((s, i) => chart.setDatasetVisibility(i, s.visible))
    chart.update('none')
    setWindow(24)
  })
})
</script>
