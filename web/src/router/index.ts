import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/',
      name: 'home',
      component: () => import('@/views/HomeView.vue')
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
        }
      ]
    }
  ]
})

router.beforeEach((to, _from, next) => {
  const authStore = useAuthStore()
  if (to.meta.requiresAuth && !authStore.isAuthenticated) {
    next({ name: 'home' })
  } else {
    next()
  }
})

export default router
