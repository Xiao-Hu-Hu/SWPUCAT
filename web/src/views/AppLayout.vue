<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()
const isCollapse = ref(false)
const isMobile = ref(false)
const drawerOpen = ref(false)

function syncIsMobile() {
  isMobile.value = window.matchMedia('(max-width: 768px)').matches
  if (!isMobile.value) drawerOpen.value = false
}

onMounted(() => {
  syncIsMobile()
  window.addEventListener('resize', syncIsMobile)
})

onUnmounted(() => {
  window.removeEventListener('resize', syncIsMobile)
})

const menuItems = [
  { path: '/app', icon: 'Clock', title: '签到打卡', group: '日常工作' },
  { path: '/app/announcements', icon: 'Bell', title: '公告管理', group: '日常工作' },
  { path: '/app/knowledge', icon: 'Folder', title: '知识库', group: '日常工作' },
  { path: '/app/reports', icon: 'Document', title: '月报', group: '日常工作' },
  { path: '/app/agent', icon: 'ChatDotRound', title: '智能助手', group: '日常工作' },
  { path: '/app/members', icon: 'User', title: '成员管理', group: '团队管理' },
  { path: '/app/approvals', icon: 'Check', title: '审批管理', group: '团队管理' },
  { path: '/app/checkin-manage', icon: 'Management', title: '打卡管理', group: '团队管理', show: authStore.isSuperAdmin || authStore.isCaptain },
  { path: '/app/invitations', icon: 'Link', title: '邀请码管理', group: '团队管理', show: authStore.isSuperAdmin || authStore.isCaptain },
  { path: '/app/agent-audits', icon: 'DataAnalysis', title: '助手运行记录', group: '系统', show: authStore.isSuperAdmin },
  { path: '/app/settings', icon: 'Setting', title: '设置', group: '系统' }
]
const menuGroups = ['日常工作', '团队管理', '系统']

function handleLogout() {
  authStore.logout()
  router.push({ name: 'login' })
}
</script>

<template>
  <div class="app-layout" :class="{ mobile: isMobile }">
    <div class="sidebar-mask" v-if="isMobile" :class="{ show: drawerOpen }" @click="drawerOpen = false"></div>
    <aside class="sidebar" :class="{ collapsed: isCollapse, 'drawer-open': drawerOpen }">
      <div class="sidebar-header">
        <img class="brand-logo" src="/association-logo.png" alt="协会徽标" />
        <div v-if="!isCollapse"><strong>SWPUCAT</strong><small>计算机协会 · 成员平台</small></div>
      </div>
      <nav class="sidebar-nav" aria-label="平台导航">
        <section v-for="group in menuGroups" :key="group" class="nav-group">
          <p v-if="!isCollapse" class="group-label">{{ group }}</p>
          <RouterLink v-for="item in menuItems.filter(m => m.group === group && m.show !== false)" :key="item.path" :to="item.path" class="nav-item" :class="{ active: route.path === item.path }" :aria-current="route.path === item.path ? 'page' : undefined" :title="item.title" @click="drawerOpen = false">
            <el-icon :size="18"><component :is="item.icon" /></el-icon>
            <span v-if="!isCollapse" class="nav-label">{{ item.title }}</span>
          </RouterLink>
        </section>
      </nav>
      <div class="sidebar-footer">
        <button class="collapse-btn" :aria-label="isCollapse ? '展开导航' : '收起导航'" :aria-expanded="!isCollapse" @click="isCollapse = !isCollapse">
          <el-icon :size="16"><Fold v-if="!isCollapse" /><Expand v-else /></el-icon>
          <span v-if="!isCollapse" class="nav-label">收起</span>
        </button>
      </div>
    </aside>

    <div class="main-area">
      <header class="topbar">
        <button class="sidebar-toggle" v-if="isMobile" aria-label="菜单" @click="drawerOpen = !drawerOpen">
          <el-icon :size="20"><Menu /></el-icon>
        </button>
        <h2 class="page-title">{{ menuItems.find(m => m.path === route.path)?.title || '签到打卡' }}</h2>
        <el-dropdown>
          <span class="user-info">
            <el-avatar :size="32" :src="authStore.user?.avatar ? `/api/avatar/${authStore.user.avatar}` : undefined">
              {{ authStore.user?.nickname?.[0] || 'U' }}
            </el-avatar>
            <span class="username">{{ authStore.user?.nickname || '用户' }}</span>
          </span>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item @click="router.push('/app/settings')">设置</el-dropdown-item>
              <el-dropdown-item divided @click="handleLogout">退出登录</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </header>
      <main class="content" id="main-content">
        <router-view v-slot="{ Component }"><Transition name="page" mode="out-in"><component :is="Component" /></Transition></router-view>
      </main>
    </div>
  </div>
</template>

<style scoped>
.brand-logo { width:36px; height:36px; flex-shrink:0; }
.sidebar-header { gap:10px; }
.sidebar-header small { display:block; font-size:10px; color:var(--text-muted); }
.group-label { padding:16px 12px 6px; font-size:11px; color:var(--text-muted); letter-spacing:.08em; }
.nav-group + .nav-group { margin-top:6px; }
.collapse-btn { border:0; background:transparent; width:100%; text-align:left; }
.page-enter-active,.page-leave-active { transition:opacity .12s ease; }
.page-enter-from,.page-leave-to { opacity:0; }
@media (prefers-reduced-motion:reduce) { .page-enter-active,.page-leave-active { transition:none; } }

.app-layout {
  display: flex;
  min-height: 100dvh;
}

/* Sidebar */
.sidebar {
  width: 232px;
  background: var(--bg-card);
  border-right: 1px solid var(--border);
  display: flex;
  flex-direction: column;
  transition: width 0.2s ease;
  flex-shrink: 0;
  position: sticky;
  top: 0;
  height: 100dvh;
  overflow-y: auto;
}

.sidebar.collapsed {
  width: 64px;
}

.sidebar-header {
  height: 56px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
}

.logo-text {
  font-size: 1.125rem;
  font-weight: 700;
  letter-spacing: -0.02em;
  color: var(--accent);
  cursor: pointer;
}

.sidebar-nav {
  flex: 1;
  padding: 0.375rem 0.375rem;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.nav-item {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.5rem 0.75rem;
  border-radius: var(--radius);
  color: var(--text-secondary);
  font-size: 0.875rem;
  font-weight: 500;
  cursor: pointer;
  transition: color 0.2s, background 0.2s;
  text-decoration: none;
  white-space: nowrap;
}

.nav-item:hover {
  background: var(--bg-hover);
  color: var(--text);
}

.nav-item.active {
  background: var(--accent-bg);
  color: var(--accent);
  font-weight: 600;
}

.sidebar-footer {
  padding: 0.5rem;
  border-top: 1px solid var(--border);
  flex-shrink: 0;
}

.collapse-btn {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.5rem 0.75rem;
  border-radius: var(--radius);
  color: var(--text-muted);
  font-size: 0.875rem;
  cursor: pointer;
  transition: color 0.2s, background 0.2s;
}

.collapse-btn:hover {
  background: var(--bg-hover);
  color: var(--text);
}

.nav-label {
  flex: 1;
}

/* Main area */
.main-area {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.topbar {
  height: 56px;
  background: var(--bg-card);
  border-bottom: 1px solid var(--border);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 1.25rem;
  position: sticky;
  top: 0;
  z-index: 10;
}

.page-title {
  font-size: 1rem;
  font-weight: 600;
  letter-spacing: -0.01em;
  color: var(--text);
}

.user-info {
  display: flex;
  align-items: center;
  gap: 0.625rem;
  cursor: pointer;
  padding: 0.25rem 0;
}

.username {
  font-size: 0.875rem;
  font-weight: 500;
  color: var(--text-secondary);
}

.content {
  flex: 1;
  padding: 1rem 1.25rem;
  background: var(--bg-deep);
}

/* Mobile (<=768px) */
@media (max-width: 768px) {
  .sidebar {
    position: fixed;
    top: 0;
    left: 0;
    bottom: 0;
    width: 240px;
    max-width: 82vw;
    z-index: 40;
    transform: translateX(-100%);
    transition: transform 0.25s cubic-bezier(.2, .8, .2, 1);
    height: 100dvh;
  }
  .sidebar.drawer-open {
    transform: translateX(0);
  }
  .sidebar-mask {
    position: fixed;
    inset: 0;
    background: rgba(24, 24, 27, 0.45);
    z-index: 30;
    opacity: 0;
    pointer-events: none;
    transition: opacity 0.2s;
  }
  .sidebar-mask.show {
    opacity: 1;
    pointer-events: auto;
  }
  .sidebar-toggle {
    border: none;
    background: transparent;
    color: var(--text-secondary);
    width: 36px;
    height: 36px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: 0.5rem;
    cursor: pointer;
    margin-right: 0.25rem;
  }
  .username {
    display: none;
  }
  .topbar {
    padding: 0 0.875rem;
  }
  .page-title {
    font-size: 0.9375rem;
  }
  .content {
    padding: 0.875rem;
  }
}
</style>
