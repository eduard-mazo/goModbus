import axios from 'axios'
import { useSessionId } from './websocket'

const api = axios.create({
  baseURL: '/api',
  timeout: 15000,
})

api.interceptors.request.use(config => {
  config.headers['X-Session-ID'] = useSessionId()
  return config
})

export default api
