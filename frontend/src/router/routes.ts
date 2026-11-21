import type { RouteRecordRaw } from 'vue-router'
import HomeView from '@/views/HomeView.vue'
import TaskRecordsView from '@/views/TaskRecordsView.vue'
import ApiView from '@/views/ApiView.vue'

const routes: RouteRecordRaw[] = [
  { path: '/approval', name: 'approval', component: () => import('@/views/ApprovalView.vue'), meta: { title: 'Farm - 安全审核', shell: false } },
  { path: '/', name: 'home', component: HomeView, meta: { title: 'Farm - 设备列表', shell: true, pageHeader: true } },
  { path: '/player', name: 'player', component: () => import('@/views/PlayerView.vue'), meta: { title: 'Farm - 投屏操控', pageHeader: true } },
  { path: '/dashboard', name: 'dashboard', component: () => import('@/views/DashboardView.vue'), meta: { title: 'Farm - 大屏监控', pageHeader: true } },
  { path: '/files', name: 'files', component: () => import('@/views/FilesView.vue'), meta: { title: 'Farm - 文件管理', pageHeader: true } },
  { path: '/tasks', name: 'tasks', component: TaskRecordsView, meta: { title: 'Farm - 任务记录', shell: true } },
  { path: '/api', name: 'api', component: ApiView, meta: { title: 'Farm - API', shell: true } },
  { path: '/ai-integration', name: 'ai-integration', component: () => import('@/views/AiIntegrationView.vue'), meta: { title: 'Farm - AI 集成', shell: true } },
  { path: '/ai-integration/skills/:id', name: 'skill-detail', component: () => import('@/views/SkillDetailView.vue'), meta: { title: 'Farm - 技能详情', shell: true } },
  { path: '/settings', redirect: '/settings/general', meta: { shell: true } },
  { path: '/settings/general', name: 'settings-general', component: () => import('@/views/settings/GeneralSettingsView.vue'), meta: { title: 'Farm - 通用设置', shell: true } },
  { path: '/settings/service', name: 'settings-service', component: () => import('@/views/settings/ServiceSettingsView.vue'), meta: { title: 'Farm - 服务设置', shell: true } },
  { path: '/settings/dependencies', name: 'settings-dependencies', component: () => import('@/views/settings/DependencySettingsView.vue'), meta: { title: 'Farm - 依赖设置', shell: true } },
  { path: '/settings/feedback', name: 'settings-feedback', component: () => import('@/views/settings/FeedbackSettingsView.vue'), meta: { title: 'Farm - 反馈', shell: true } },
  { path: '/settings/about', name: 'settings-about', component: () => import('@/views/settings/AboutSettingsView.vue'), meta: { title: 'Farm - 关于', shell: true } },
  { path: '/:pathMatch(.*)*', redirect: '/' },
]

export default routes
