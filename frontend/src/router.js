import { createRouter, createWebHistory } from 'vue-router'
import UploadView from './views/UploadView.vue'
import AdminView from './views/AdminView.vue'
import DownloadGate from './views/DownloadGate.vue'

export default createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', component: UploadView },
    { path: '/dl/:id', component: DownloadGate },
    { path: '/admin', component: AdminView },
  ],
})
