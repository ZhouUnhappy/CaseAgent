import { createRouter, createWebHistory } from 'vue-router'

const routes = [
  {
    path: '/',
    redirect: '/generate',
  },
  {
    path: '/generate',
    name: 'generate',
    component: () => import('../views/CaseGenerationWorkspace.vue'),
    meta: { title: '生成用例' },
  },
  {
    path: '/projects',
    name: 'projects',
    component: () => import('../views/ProjectList.vue'),
    meta: { title: '项目' },
  },
  {
    path: '/projects/:id',
    name: 'project-detail',
    component: () => import('../views/ProjectDetail.vue'),
    meta: { title: '项目详情' },
    props: true,
  },
  {
    path: '/tasks/:id',
    name: 'task-detail',
    component: () => import('../views/TaskDetail.vue'),
    meta: { title: '任务详情' },
    props: true,
  },
  {
    path: '/knowledge',
    name: 'knowledge',
    component: () => import('../views/KnowledgeBase.vue'),
    meta: { title: '知识库' },
  },
  {
    path: '/knowledge-suggestions',
    name: 'knowledge-suggestions',
    component: () => import('../views/KnowledgeSuggestions.vue'),
    meta: { title: '知识建议' },
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

export default router
