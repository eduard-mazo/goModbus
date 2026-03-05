import { useLogsStore } from '../stores/logs'
import { useSyncStore } from '../stores/sync'

// Session ID: UUID persisted in localStorage.
// crypto.randomUUID() requires a secure context (HTTPS/localhost).
// Fallback uses crypto.getRandomValues() which works on plain HTTP.
function generateSessionId() {
  if (typeof crypto !== 'undefined' && crypto.randomUUID) {
    return crypto.randomUUID()
  }
  return ([1e7] + -1e3 + -4e3 + -8e3 + -1e11).replace(/[018]/g, c =>
    (c ^ crypto.getRandomValues(new Uint8Array(1))[0] & 15 >> c / 4).toString(16)
  )
}

let _sessionId = null
export function useSessionId() {
  if (!_sessionId) {
    _sessionId = localStorage.getItem('rocSessionId')
    if (!_sessionId) {
      _sessionId = generateSessionId()
      localStorage.setItem('rocSessionId', _sessionId)
    }
  }
  return _sessionId
}

let ws = null

export function connectWS(logContainerRef) {
  const logsStore = useLogsStore()
  const syncStore = useSyncStore()
  const sid = useSessionId()

  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  try {
    ws = new WebSocket(`${proto}://${location.host}/ws`)
  } catch (_) {
    return
  }

  ws.onopen = () => {
    logsStore.wsConnected = true
    ws.send(JSON.stringify({ type: 'register', sid }))
  }

  ws.onclose = () => {
    logsStore.wsConnected = false
    setTimeout(() => connectWS(logContainerRef), 3000)
  }

  ws.onmessage = e => {
    try {
      const msg = JSON.parse(e.data)
      if (msg.progress) {
        syncStore.handleProgress(msg.progress)
        return
      }
      logsStore.addLog(msg)
      if (logsStore.logOpen && logContainerRef?.value) {
        setTimeout(() => {
          if (logContainerRef.value) {
            logContainerRef.value.scrollTop = logContainerRef.value.scrollHeight
          }
        }, 0)
      }
    } catch (_) {}
  }
}

export function getWS() { return ws }
