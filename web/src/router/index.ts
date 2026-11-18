import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const router = createRouter({
  history: createWebHistory(),
  scrollBehavior(to, _from, savedPosition) {
    if (savedPosition) return savedPosition
    if (to.hash) return { el: to.hash, top: 110, behavior: 'smooth' }
    return { top: 0 }
  },
  routes: [
    {
      path: '/',
      name: 'home',
      component: () => import('@/views/HomeView.vue')
    },
    {
      path: '/app/login',
      name: 'login',
      component: () => import('@/views/LoginView.vue')
    },
    {
      path: '/app',
      component: () => import('@/views/AppLayout.vue'),
      meta: { requiresAuth: true },
      children: [
        {
          path: '',
          name: 'checkin',
          component: () => import('@/views/CheckinView.vue')
        },
        {
          path: 'agent',
          name: 'agent',
          component: () => import('@/views/AgentView.vue')
        },
        {
          path: 'agent-audits',
          name: 'agent-audits',
          component: () => import('@/views/AgentAuditView.vue')
        },
        {
          path: 'announcements',
          name: 'announcements',
          component: () => import('@/views/AnnouncementsView.vue')
        },
        {
          path: 'knowledge',
          name: 'knowledge',
          component: () => import('@/views/KnowledgeView.vue')
        },
        {
          path: 'reports',
          name: 'reports',
          component: () => import('@/views/ReportView.vue')
        },
        {
          path: 'members',
          name: 'members',
          component: () => import('@/views/MembersView.vue')
        },
        {
          path: 'approvals',
          name: 'approvals',
          component: () => import('@/views/ApprovalsView.vue')
        },
        {
          path: 'invitations',
          name: 'invitations',
          component: () => import('@/views/InvitationsView.vue')
        },
        {
          path: 'settings',
          name: 'settings',
          component: () => import('@/views/SettingsView.vue')
        },
        {
          path: 'checkin-manage',
          name: 'checkin-manage',
          component: () => import('@/views/CheckinManageView.vue')
        }
      ]
    }
  ]
})

router.beforeEach((to, _from, next) => {
  const authStore = useAuthStore()
  if (to.meta.requiresAuth && !authStore.isAuthenticated) {
    next({ name: 'login' })
  } else {
    next()
  }
})

export default router
