import type { RouteRecordRaw } from 'vue-router'
import HomeView from '@/views/HomeView.vue'

const routes: RouteRecordRaw[] = [
  { path: '/', name: 'home', component: HomeView, meta: { title: 'Farm - 设备列表' } },
  { path: '/player', name: 'player', component: () => import('@/views/PlayerView.vue'), meta: { title: 'Farm - 投屏操控' } },
  { path: '/dashboard', name: 'dashboard', component: () => import('@/views/DashboardView.vue'), meta: { title: 'Farm - 大屏监控' } },
  { path: '/files', name: 'files', component: () => import('@/views/FilesView.vue'), meta: { title: 'Farm - 文件管理' } },
  { path: '/dependencies', name: 'dependencies', component: () => import('@/views/DependenciesView.vue'), meta: { title: 'Farm - 依赖管理' } },
  { path: '/ai-integration', name: 'ai-integration', component: () => import('@/views/AiIntegrationView.vue'), meta: { title: 'Farm - AI 集成' } },
  { path: '/ai-integration/skills/:id', name: 'skill-detail', component: () => import('@/views/SkillDetailView.vue'), meta: { title: 'Farm - 技能详情' } },
  { path: '/:pathMatch(.*)*', redirect: '/' },
]

export default routes
