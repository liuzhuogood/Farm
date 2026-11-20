import type { RouteRecordRaw } from 'vue-router'
import HomeView from '@/views/HomeView.vue'

const routes: RouteRecordRaw[] = [
  { path: '/approval', name: 'approval', component: () => import('@/views/ApprovalView.vue'), meta: { title: 'Farm - 安全审核' } },
  { path: '/', name: 'home', component: HomeView, meta: { title: 'Farm - 设备列表', shell: true } },
  { path: '/player', name: 'player', component: () => import('@/views/PlayerView.vue'), meta: { title: 'Farm - 投屏操控' } },
  { path: '/dashboard', name: 'dashboard', component: () => import('@/views/DashboardView.vue'), meta: { title: 'Farm - 大屏监控' } },
  { path: '/files', name: 'files', component: () => import('@/views/FilesView.vue'), meta: { title: 'Farm - 文件管理' } },
  { path: '/tasks', name: 'tasks', component: () => import('@/views/TaskRecordsView.vue'), meta: { title: 'Farm - 任务记录', shell: true } },
  { path: '/api', name: 'api', component: () => import('@/views/ApiView.vue'), meta: { title: 'Farm - API', shell: true } },
  { path: '/ai-integration', name: 'ai-integration', component: () => import('@/views/AiIntegrationView.vue'), meta: { title: 'Farm - AI 集成', shell: true } },
  { path: '/ai-integration/skills/:id', name: 'skill-detail', component: () => import('@/views/SkillDetailView.vue'), meta: { title: 'Farm - 技能详情', shell: true } },
  { path: '/settings', redirect: '/settings/general', meta: { shell: true } },
  { path: '/settings/general', name: 'settings-general', component: () => import('@/views/settings/GeneralSettingsView.vue'), meta: { title: 'Farm - 通用设置', shell: true } },
  { path: '/settings/service', name: 'settings-service', component: () => import('@/views/settings/ServiceSettingsView.vue'), meta: { title: 'Farm - 服务设置', shell: true } },
  { path: '/settings/dependencies', name: 'settings-dependencies', component: () => import('@/views/settings/DependencySettingsView.vue'), meta: { title: 'Farm - 依赖设置', shell: true } },
  { path: '/:pathMatch(.*)*', redirect: '/' },
]

export default routes
