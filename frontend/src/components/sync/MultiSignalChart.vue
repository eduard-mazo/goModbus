<template>
  <div class="space-y-3">

    <!-- ── Signal legend / toggle / rename ─────────────────────────── -->
    <div class="flex flex-wrap gap-1.5 items-center pb-2 border-b border-g-100">
      <div
        v-for="(sig, i) in signals" :key="i"
        class="flex items-center gap-1.5 px-2 py-1 rounded border text-xs transition-all"
        :class="sig.visible ? 'border-g-300 bg-white shadow-sm' : 'border-g-100 bg-g-50 opacity-40'"
      >
        <!-- Color dot — click toggles visibility -->
        <button
          class="w-3 h-3 rounded-full shrink-0 ring-1 ring-white shadow"
          :style="`background:${sig.color};opacity:${sig.visible ? 1 : 0.4}`"
          :title="sig.visible ? 'Ocultar señal' : 'Mostrar señal'"
          @click="toggleSignal(i)"
        />
        <!-- Name: dblclick to rename inline -->
        <span
          v-if="!sig.editing"
          class="font-medium text-g-700 whitespace-nowrap cursor-pointer"
          style="min-width:56px"
          :title="'Doble clic para renombrar (se guarda por estación)'"
          @dblclick.stop="startEdit(i)"
        >{{ sig.name }}</span>
        <input
          v-else
          :ref="el => { if (el) _nameEl[i] = el }"
          class="border border-lime rounded px-1 outline-none font-medium text-g-700"
          style="width:96px;font-size:11px;"
          v-model="sig.name"
          @blur="finishEdit(i)"
          @keydown.enter="finishEdit(i)"
          @keydown.esc.stop="cancelEdit(i)"
          @click.stop
        />
      </div>

      <!-- Zoom presets -->
      <div class="ml-auto flex items-center gap-1">
        <span class="text-xs text-g-400">Ventana:</span>
        <button class="btn btn-sm btn-ghost" @click="setWindow(24)">24h</button>
        <button class="btn btn-sm btn-ghost" @click="setWindow(168)">7d</button>
        <button class="btn btn-sm btn-ghost" @click="setWindow(840)">840h</button>
      </div>
    </div>

    <!-- ── Chart area ───────────────────────────────────────────────── -->
    <div
      ref="containerRef"
      class="relative select-none rounded"
      style="background:#f8faf8;"
      @mousemove="onMouseMove"
      @mouseleave="tooltip.visible = false"
      @wheel.prevent="onWheel"
      @mousedown.prevent="onMouseDown"
    >
      <svg
        ref="svgRef"
        :viewBox="`0 0 ${W} ${H}`"
        preserveAspectRatio="none"
        class="w-full block"
        :style="`height:${H}px; cursor:${_dragging ? 'grabbing' : 'crosshair'}`"
      >
        <!-- Horizontal grid (5 lines: 0%, 25%, 50%, 75%, 100%) -->
        <g v-for="n in 5" :key="n">
          <line
            :x1="ML" :y1="gy(n - 1)"
            :x2="W - MR" :y2="gy(n - 1)"
            stroke="#d1fae5"
            :stroke-width="n === 1 || n === 5 ? 1 : 0.5"
            :stroke-dasharray="n === 1 || n === 5 ? '' : '4,4'"
          />
          <text :x="ML - 4" :y="gy(n - 1) + 3.5"
            text-anchor="end" fill="#9ab59a" font-size="8">
            {{ 100 - (n - 1) * 25 }}%
          </text>
        </g>

        <!-- X axis tick labels (two lines: time + date) -->
        <g v-for="tick in xTicks" :key="tick.ri">
          <text :x="tick.x" :y="H - 14" text-anchor="middle" fill="#9ab59a" font-size="9">{{ tick.timeLabel }}</text>
          <text :x="tick.x" :y="H - 3"  text-anchor="middle" fill="#b8ccb8" font-size="8">{{ tick.dateLabel }}</text>
        </g>

        <!-- Signal fills + lines -->
        <g v-for="(sig, i) in signals" :key="i">
          <path
            v-if="sig.visible && paths[i].area"
            :d="paths[i].area"
            :fill="sig.color" fill-opacity="0.08"
          />
          <path
            v-if="sig.visible && paths[i].line"
            :d="paths[i].line"
            :stroke="sig.color"
            stroke-width="1.5" fill="none"
            stroke-linejoin="round" stroke-linecap="round"
          />
        </g>

        <!-- Crosshair -->
        <g v-if="tooltip.visible">
          <line
            :x1="tooltip.x" :y1="MT"
            :x2="tooltip.x" :y2="H - MB"
            stroke="#374151" stroke-width="0.8" stroke-dasharray="4,3"
          />
          <circle
            v-for="dot in tooltip.dots" :key="dot.i"
            :cx="tooltip.x" :cy="dot.y" r="4"
            :fill="dot.color" stroke="white" stroke-width="1.5"
          />
        </g>
      </svg>

      <!-- Floating tooltip -->
      <div
        v-if="tooltip.visible"
        class="absolute z-20 pointer-events-none rounded-xl border border-g-200 shadow-xl text-xs"
        style="background:rgba(255,255,255,0.97); min-width:170px; top:10px;"
        :style="tooltipPos"
      >
        <div class="px-3 py-2 border-b border-g-100 flex items-baseline gap-2">
          <span class="font-mono font-bold text-g-800">Ptr {{ tooltip.ri }}</span>
          <span class="text-g-400">{{ tooltip.dateStr }}</span>
        </div>
        <div class="px-3 py-2 space-y-1">
          <div
            v-for="row in tooltip.rows" :key="row.i"
            class="flex items-center gap-1.5"
          >
            <span class="w-2 h-2 rounded-full shrink-0" :style="`background:${row.color}`" />
            <span class="text-g-500 flex-1 truncate" style="max-width:96px">{{ row.name }}</span>
            <span class="font-mono font-semibold text-g-800 ml-auto">{{ row.val }}</span>
          </div>
        </div>
      </div>
    </div>

    <!-- ── Time navigation bar ──────────────────────────────────────── -->
    <div class="flex items-center gap-1.5">
      <button class="btn btn-sm btn-ghost font-mono" @click="pan(-viewSize)" title="Retroceder una ventana">◀◀</button>
      <button class="btn btn-sm btn-ghost font-mono" @click="pan(-Math.ceil(viewSize / 4))" title="Retroceder ¼">◀</button>
      <div class="flex-1">
        <input
          type="range" class="w-full accent-lime"
          :value="viewStart"
          :min="0"
          :max="Math.max(0, 840 - viewSize)"
          step="1"
          @input="viewStart = +$event.target.value"
        />
      </div>
      <button class="btn btn-sm btn-ghost font-mono" @click="pan(Math.ceil(viewSize / 4))" title="Avanzar ¼">▶</button>
      <button class="btn btn-sm btn-ghost font-mono" @click="pan(viewSize)" title="Avanzar una ventana">▶▶</button>
      <span class="text-xs font-mono text-g-500 whitespace-nowrap pl-1">
        {{ viewStart }}–{{ Math.min(839, viewStart + viewSize - 1) }}
        <span class="text-g-300">/839</span>
        <span class="text-g-400 ml-1">({{ viewSize }}h)</span>
      </span>
    </div>

  </div>
</template>

<script setup>
import { ref, computed, watch, nextTick, onMounted, onUnmounted } from 'vue'

// ── Props ────────────────────────────────────────────────────────────────────
const props = defineProps({
  records:     { type: Array,  default: () => [] },   // HourRecord[840]
  endian:      { type: String, default: 'cdab'   },   // float32 byte order
  stationName: { type: String, default: ''       },   // key for localStorage
  signalNames: { type: Array,  default: () => [] },   // override default signal names from config
})

// ── SVG layout constants ─────────────────────────────────────────────────────
const W  = 900   // viewBox width
const H  = 230   // viewBox height
const ML = 36    // left margin  (Y labels)
const MR = 8     // right margin
const MT = 12    // top margin
const MB = 36    // bottom margin (X labels — two lines)
const CW = W - ML - MR   // chart width
const CH = H - MT - MB   // chart height

// Map grid row index (0–4) to SVG Y coordinate (0=top/100%, 4=bottom/0%)
function gy(n) { return MT + n * CH / 4 }

// ── Signal configuration ─────────────────────────────────────────────────────
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

const signals = ref(DEFAULTS.map(d => ({ ...d, visible: true, editing: false, _bk: '' })))
const _nameEl = {}   // DOM element cache for rename inputs

const lsKey = name => `roc_sig_${name}`

function loadConfig() {
  // Reset to defaults first, then overlay saved names/visibility
  const raw = props.stationName && localStorage.getItem(lsKey(props.stationName))
  signals.value = DEFAULTS.map((d, i) => {
    // config.yaml signal_names takes precedence over DEFAULTS; localStorage overrides both
    let name = props.signalNames?.[i] || d.name, visible = true
    if (raw) {
      try {
        const s = JSON.parse(raw)[i]
        if (s?.name)                        name    = s.name
        if (typeof s?.visible === 'boolean') visible = s.visible
      } catch (_) { /* ignore corrupt data */ }
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
}

function startEdit(i) {
  signals.value[i]._bk      = signals.value[i].name
  signals.value[i].editing   = true
  nextTick(() => _nameEl[i]?.focus())
}
function finishEdit(i) {
  const s = signals.value[i]
  if (!s.name.trim()) s.name = s._bk
  s.editing = false
  saveConfig()
}
function cancelEdit(i) {
  signals.value[i].name    = signals.value[i]._bk
  signals.value[i].editing = false
}

// ── View window (pan + zoom) ─────────────────────────────────────────────────
const viewSize  = ref(24)
const viewStart = ref(816)   // default: last 24 of 840

function setWindow(size) {
  viewSize.value  = Math.min(size, 840)
  viewStart.value = Math.max(0, Math.min(840 - viewSize.value, viewStart.value))
}

function pan(delta) {
  viewStart.value = Math.max(0, Math.min(840 - viewSize.value, viewStart.value + delta))
}

// ── ROC date/time decoding ────────────────────────────────────────────────────
// ROC date register: bit[15:9]=year(+2000), bit[8:5]=month, bit[4:0]=day
// ROC time register: bit[15:11]=hour, bit[10:5]=minute, bit[4:0]=second/2
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

function fmtDate(d) {
  if (!d) return null
  const dd = String(d.getDate()).padStart(2, '0')
  const mm = String(d.getMonth() + 1).padStart(2, '0')
  const hh = String(d.getHours()).padStart(2, '0')
  const mn = String(d.getMinutes()).padStart(2, '0')
  return { date: `${dd}/${mm}`, time: `${hh}:${mn}`, full: `${dd}/${mm} ${hh}:${mn}` }
}

// Parsed Date per record index (null if record invalid or date not available)
const recDates = computed(() => {
  if (!props.records?.length) return []
  return props.records.map(r =>
    (r?.valid) ? parseRocDate(r.date_raw, r.time_raw) : null
  )
})

// ── Raw values from records ──────────────────────────────────────────────────
// Cached computed: [sigIdx][recordIdx] → float | null
// Recomputed only when records or endian change (not on pan/zoom).
const allVals = computed(() => {
  return DEFAULTS.map((_, si) => {
    if (!props.records?.length) return []
    return props.records.map(r => {
      if (!r?.valid || !r.modes?.[si]) return null
      const v = r.modes[si][props.endian]
      return v != null && isFinite(v) ? v : null
    })
  })
})

// Per-signal min/max over ALL 840 records (stable normalization regardless of zoom)
const sigStats = computed(() =>
  allVals.value.map(vs => {
    const valid = vs.filter(v => v !== null)
    if (!valid.length) return { min: 0, max: 1, rng: 1 }
    let mn = valid[0], mx = valid[0]
    for (const v of valid) { if (v < mn) mn = v; if (v > mx) mx = v }
    return { min: mn, max: mx, rng: mx - mn || 1 }
  })
)

// Build SVG path strings for one signal within the current viewport
function buildPath(si) {
  const vs  = allVals.value[si]
  if (!vs.length) return { line: '', area: '' }
  const { min, rng } = sigStats.value[si]
  const sz  = viewSize.value
  const st  = viewStart.value
  const div = Math.max(1, sz - 1)

  let line = '', inPath = false
  let firstX = null, lastX = null

  for (let i = 0; i < sz; i++) {
    const ri = st + i
    if (ri >= vs.length) break
    const v = vs[ri]
    if (v === null) { inPath = false; continue }
    const x = (ML + i / div * CW).toFixed(1)
    const y = (MT + (1 - (v - min) / rng) * CH).toFixed(1)
    line += inPath ? `L${x},${y}` : `M${x},${y}`
    inPath = true
    if (firstX === null) firstX = x
    lastX = x
  }

  let area = ''
  if (firstX !== null) {
    const bot = (MT + CH).toFixed(1)
    area = line + `L${lastX},${bot}L${firstX},${bot}Z`
  }
  return { line, area }
}

const paths = computed(() => signals.value.map((_, i) => buildPath(i)))

// ── X-axis ticks ─────────────────────────────────────────────────────────────
const xTicks = computed(() => {
  const sz    = viewSize.value
  const st    = viewStart.value
  const div   = Math.max(1, sz - 1)
  const iv    = sz > 336 ? 48 : sz > 168 ? 24 : sz > 48 ? 12 : sz > 24 ? 6 : 4
  const dates = recDates.value
  const ticks = []
  for (let i = 0; i < sz; i++) {
    const ri = st + i
    if (ri % iv !== 0) continue
    const x  = (ML + i / div * CW).toFixed(1)
    const d  = dates[ri] ? fmtDate(dates[ri]) : null
    // timeLabel: hour line; dateLabel: date line below
    const timeLabel = d ? d.time : `${String(ri % 24).padStart(2, '0')}h`
    const dateLabel = d ? d.date : `D${Math.floor(ri / 24) + 1}`
    ticks.push({ ri, x, timeLabel, dateLabel })
  }
  return ticks
})

// ── Crosshair + Tooltip ───────────────────────────────────────────────────────
const svgRef       = ref(null)
const containerRef = ref(null)
const _dragging    = ref(false)

const tooltip = ref({ visible: false, x: 0, ri: 0, rows: [], dots: [], dateStr: '' })

const tooltipPos = computed(() => {
  const el = containerRef.value
  if (!el) return {}
  const w  = el.getBoundingClientRect().width
  const px = tooltip.value.x / W * w
  return px > w * 0.55
    ? { right: `${w - px + 12}px` }
    : { left:  `${px + 12}px` }
})

function _svgFraction(clientX) {
  const rect = svgRef.value?.getBoundingClientRect()
  if (!rect) return 0
  // (clientX→viewBox px − left margin) / chart width → fraction 0–1
  return Math.max(0, Math.min(1, ((clientX - rect.left) / rect.width * W - ML) / CW))
}

function _fracToRi(frac) {
  return Math.max(0, Math.min(839, viewStart.value + Math.round(frac * (viewSize.value - 1))))
}

function onMouseMove(e) {
  if (_dragging.value) return
  const frac = _svgFraction(e.clientX)
  const ri   = _fracToRi(frac)
  const i    = ri - viewStart.value
  const div  = Math.max(1, viewSize.value - 1)
  const cx   = (ML + i / div * CW).toFixed(1)

  const rows = [], dots = []
  allVals.value.forEach((vs, si) => {
    if (!signals.value[si].visible) return
    const v = vs[ri] ?? null
    if (v !== null) {
      const { min, rng } = sigStats.value[si]
      dots.push({ i: si, color: signals.value[si].color,
        y: (MT + (1 - (v - min) / rng) * CH).toFixed(1) })
    }
    rows.push({ i: si, name: signals.value[si].name,
      color: signals.value[si].color,
      val: v !== null ? v.toFixed(4) : '—' })
  })

  const d = recDates.value[ri] ? fmtDate(recDates.value[ri]) : null
  const dateStr = d ? d.full : `D${Math.floor(ri / 24) + 1} · ${String(ri % 24).padStart(2, '0')}:00`
  tooltip.value = { visible: true, x: cx, ri, rows, dots, dateStr }
}

// ── Scroll-wheel zoom (cursor-anchored) ──────────────────────────────────────
function onWheel(e) {
  const frac    = _svgFraction(e.clientX)
  const factor  = e.deltaY > 0 ? 1.3 : 0.77
  const newSz   = Math.max(2, Math.min(840, Math.round(viewSize.value * factor)))
  const anchor  = viewStart.value + frac * viewSize.value
  let   newSt   = Math.round(anchor - frac * newSz)
  newSt = Math.max(0, Math.min(840 - newSz, newSt))
  viewSize.value  = newSz
  viewStart.value = newSt
}

// ── Drag-to-pan ───────────────────────────────────────────────────────────────
let _drag = null

function onMouseDown(e) {
  _dragging.value = true
  _drag = { x0: e.clientX, vs0: viewStart.value }
  window.addEventListener('mousemove', _doDrag)
  window.addEventListener('mouseup',   _endDrag)
}

function _doDrag(e) {
  if (!_drag || !svgRef.value) return
  const rect = svgRef.value.getBoundingClientRect()
  const pxPerRec = rect.width / Math.max(1, viewSize.value)
  const dRec = -Math.round((e.clientX - _drag.x0) / pxPerRec)
  viewStart.value = Math.max(0, Math.min(840 - viewSize.value, _drag.vs0 + dRec))
}

function _endDrag() {
  _dragging.value = false
  _drag = null
  window.removeEventListener('mousemove', _doDrag)
  window.removeEventListener('mouseup',   _endDrag)
}

// ── Lifecycle ────────────────────────────────────────────────────────────────
onMounted(loadConfig)
onUnmounted(() => {
  window.removeEventListener('mousemove', _doDrag)
  window.removeEventListener('mouseup',   _endDrag)
})
watch([() => props.stationName, () => props.signalNames], loadConfig)
</script>
