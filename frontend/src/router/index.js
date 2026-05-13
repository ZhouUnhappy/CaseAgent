import { createRouter, createWebHistory } from 'vue-router'

const routes = [
  {
    path: '/',
    redirect: '/projects',
  },
  {
    path: '/projects',
    name: 'projects',
    component: () => import('../views/ProjectList.vue'),
    meta: { title: '项目' },
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

export default router
