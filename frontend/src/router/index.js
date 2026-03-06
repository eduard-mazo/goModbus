import { createRouter, createWebHistory } from 'vue-router'
import QueryView from '../views/QueryView.vue'
import RocView from '../views/RocView.vue'
import SyncView from '../views/SyncView.vue'
import RawView from '../views/RawView.vue'

const routes = [
  { path: '/', redirect: '/roc' },
  { path: '/query', component: QueryView },
  { path: '/roc', component: RocView },
  { path: '/sync', component: SyncView },
  { path: '/raw', component: RawView },
]

export default createRouter({
  history: createWebHistory(),
  routes,
})
