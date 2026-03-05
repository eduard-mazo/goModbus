<template>
  <svg :viewBox="`0 0 ${W} ${H}`" preserveAspectRatio="none" class="w-full" :style="`height:${H}px;display:block;`">
    <!-- Grid lines -->
    <line x1="0" :y1="H" :x2="W" :y2="H" stroke="#e2e8e2" stroke-width="1"/>
    <line x1="0" :y1="H/2" :x2="W" :y2="H/2" stroke="#e2e8e2" stroke-width="0.5" stroke-dasharray="4,4"/>
    <line x1="0" y1="4" :x2="W" y2="4" stroke="#e2e8e2" stroke-width="0.5" stroke-dasharray="4,4"/>

    <!-- Area fill -->
    <path v-if="areaD" :d="areaD" :fill="color" fill-opacity="0.15" />
    <!-- Line -->
    <path v-if="lineD" :d="lineD" :stroke="color" stroke-width="1.5" fill="none" />

    <!-- Empty state -->
    <text v-if="!areaD" x="420" :y="H/2" text-anchor="middle" fill="#c5cfc5" font-size="12">Sin datos</text>
  </svg>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  records: Array,   // HourRecord[840]
  sigIdx: Number,   // 0-7
  endian: String,   // 'abcd' | 'dcba' | 'cdab' | 'badc'
  color: { type: String, default: '#7AD400' },
})

const W = 840
const H = 180

const vals = computed(() => {
  if (!props.records?.length) return []
  return props.records.map(r => {
    if (!r?.valid || !r.modes?.[props.sigIdx]) return null
    const v = r.modes[props.sigIdx][props.endian]
    return (v == null || !isFinite(v)) ? null : v
  })
})

function buildPaths() {
  const vs = vals.value
  const defined = vs.filter(v => v !== null)
  if (!defined.length) return { line: '', area: '' }

  const mn = Math.min(...defined)
  const mx = Math.max(...defined)
  const range = mx - mn || 1
  const scaleY = v => H - Math.round(((v - mn) / range) * (H - 8)) - 4

  let line = ''
  let inPath = false
  for (let i = 0; i < vs.length; i++) {
    const v = vs[i]
    if (v === null) { inPath = false; continue }
    const x = Math.round((i / (W - 1)) * W)
    const y = scaleY(v)
    line += inPath ? `L${x},${y} ` : `M${x},${y} `
    inPath = true
  }

  const firstI = vs.findIndex(v => v !== null)
  const lastI = vs.reduceRight((a, v, i) => a !== null ? a : (v !== null ? i : null), null)
  let area = line
  if (firstI !== null && lastI !== null) {
    area += `L${Math.round((lastI / (W - 1)) * W)},${H} L${Math.round((firstI / (W - 1)) * W)},${H} Z`
  }

  return { line, area }
}

const lineD = computed(() => buildPaths().line)
const areaD = computed(() => buildPaths().area)
</script>
