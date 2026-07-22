<script setup lang="ts">
import { ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()
const isCollapse = ref(false)

const menuItems = [
  { path: '/app', icon: 'DataBoard', title: '仪表盘' },
  { path: '/app/checkin', icon: 'Clock', title: '签到打卡' },
  { path: '/app/announcements', icon: 'Bell', title: '公告管理' },
  { path: '/app/knowledge', icon: 'Folder', title: '知识库' },
  { path: '/app/members', icon: 'User', title: '成员管理' },
  { path: '/app/approvals', icon: 'Check', title: '审批管理' },
  { path: '/app/invitations', icon: 'Link', title: '邀请码管理', show: authStore.isSuperAdmin || authStore.isCaptain },
  { path: '/app/settings', icon: 'Setting', title: '设置' }
]

function handleLogout() {
  authStore.logout()
  router.push({ name: 'home' })
}
</script>

<template>
  <div class="app-layout">
    <aside class="sidebar" :class="{ collapsed: isCollapse }">
      <div class="sidebar-header">
        <span class="logo-text" v-if="!isCollapse">SWPUCAT</span>
        <span class="logo-text" v-else>S</span>
      </div>
      <nav class="sidebar-nav">
        <a
          v-for="item in menuItems"
          :key="item.path"
          v-show="item.show !== false"
          class="nav-item"
          :class="{ active: route.path === item.path }"
          @click="router.push(item.path)"
        >
          <el-icon :size="18"><component :is="item.icon" /></el-icon>
          <span v-if="!isCollapse" class="nav-label">{{ item.title }}</span>
        </a>
      </nav>
      <div class="sidebar-footer">
        <div class="collapse-btn" @click="isCollapse = !isCollapse">
          <el-icon :size="16"><Fold v-if="!isCollapse" /><Expand v-else /></el-icon>
          <span v-if="!isCollapse" class="nav-label">收起</span>
        </div>
      </div>
    </aside>

    <div class="main-area">
      <header class="topbar">
        <h2 class="page-title">{{ menuItems.find(m => m.path === route.path)?.title || '仪表盘' }}</h2>
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
      <main class="content">
        <router-view />
      </main>
    </div>
  </div>
</template>

<style scoped>
.app-layout {
  display: flex;
  min-height: 100vh;
}

/* Sidebar */
.sidebar {
  width: 240px;
  background: var(--bg-card);
  border-right: 1px solid var(--border);
  display: flex;
  flex-direction: column;
  transition: width 0.2s ease;
  flex-shrink: 0;
  position: sticky;
  top: 0;
  height: 100vh;
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
  padding: 0.5rem 0.5rem;
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
  padding: 0 1.5rem;
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
  padding: 1.5rem;
  background: var(--bg-deep);
}
</style>
