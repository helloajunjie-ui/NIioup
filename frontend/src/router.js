import { createRouter, createWebHistory } from 'vue-router'
import UploadView from './views/UploadView.vue'
import AdminView from './views/AdminView.vue'

export default createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', component: UploadView },
    { path: '/admin', component: AdminView },
  ],
})
