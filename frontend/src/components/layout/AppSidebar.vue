<template>
  <div class="w-60 shrink-0 flex flex-col overflow-hidden" style="background:#0f1710;">

    <!-- Logo -->
    <div class="px-4 py-3 border-b border-g-900" style="border-color:#1e2a1f;">
      <div class="text-white font-bold text-sm tracking-wide">ROC Modbus Expert</div>
      <div class="text-xs mt-0.5" style="color:#8aaa8a;">EPM · v4.0</div>
    </div>

    <!-- Scrollable content -->
    <div class="flex-1 overflow-y-auto">

      <!-- TCP Connection config -->
      <div class="sbar-section">
        <div class="sbar-label">Conexión TCP</div>
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

      <!-- Station presets -->
      <div class="sbar-section">
        <div class="flex items-center justify-between mb-2">
          <div class="sbar-label mb-0">Estaciones</div>
          <button class="btn btn-sm" style="height:22px;font-size:10px;background:#1e3a22;color:#7ad400;border:1px solid #2a5a2e;" @click="loadStations">
            <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><polyline points="1 4 1 10 7 10"/><path d="M3.51 15a9 9 0 1 0 .49-3.51"/></svg>
            Cargar
          </button>
        </div>

        <div v-if="stations.length === 0" class="text-xs py-2" style="color:#8aaa8a;">
          Sin estaciones en config.yaml
        </div>

        <div
          v-for="st in stations" :key="st.name"
          class="flex items-start gap-2 px-2 py-1.5 rounded cursor-pointer mb-1 transition-all"
          :class="activeStation === st.name ? 'border-l-2' : 'border-l-2 border-transparent'"
          :style="activeStation === st.name
            ? 'background:#1a2e1a;border-color:#7ad400;'
            : 'background:transparent;border-color:transparent;'"
          @click="applyStation(st)"
        >
          <span class="mt-0.5 shrink-0 w-2 h-2 rounded-full"
            :style="activeStation === st.name ? 'background:#7ad400;box-shadow:0 0 4px #7ad400;' : 'background:#2a3d2a;'"
          ></span>
          <div class="min-w-0">
            <div class="text-xs font-semibold truncate" style="color:#c5cfc5;">{{ st.name }}</div>
            <div class="font-mono" style="font-size:10px;color:#8aaa8a;">{{ st.ip }}:{{ st.port }} · ID {{ st.id }}</div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import axios from 'axios'
import { useConnectionStore } from '../../stores/connection'
import { useRocStore } from '../../stores/roc'

const conn = useConnectionStore()
const rocStore = useRocStore()

const stations = ref([])
const activeStation = ref(null)

async function loadStations() {
  try {
    const { data } = await axios.get('/api/config')
    stations.value = data.stations || []
  } catch (_) {}
}

function applyStation(st) {
  conn.applyStation(st)
  rocStore.applyStation(st)
  activeStation.value = st.name
}

loadStations()
</script>
