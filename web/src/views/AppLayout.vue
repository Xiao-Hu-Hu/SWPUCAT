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
  { path: '/app/settings', icon: 'Setting', title: '设置' }
]

function handleLogout() {
  authStore.logout()
  router.push({ name: 'home' })
}
</script>

<template>
  <el-container class="app-layout">
    <el-aside :width="isCollapse ? '64px' : '240px'" class="sidebar">
      <div class="logo" @click="router.push('/app')">
        <span v-if="!isCollapse">SWPUCAT</span>
        <span v-else>S</span>
      </div>
      <el-menu
        :default-active="route.path"
        :collapse="isCollapse"
        router
        background-color="var(--bg-card)"
        text-color="var(--text)"
        active-text-color="var(--accent)"
      >
        <el-menu-item v-for="item in menuItems" :key="item.path" :index="item.path">
          <el-icon><component :is="item.icon" /></el-icon>
          <template #title>{{ item.title }}</template>
        </el-menu-item>
      </el-menu>
      <div class="collapse-btn" @click="isCollapse = !isCollapse">
        <el-icon><Fold v-if="!isCollapse" /><Expand v-else /></el-icon>
      </div>
    </el-aside>

    <el-container>
      <el-header class="header">
        <div class="header-left">
          <h2>{{ menuItems.find(m => m.path === route.path)?.title || '仪表盘' }}</h2>
        </div>
        <div class="header-right">
          <el-dropdown>
            <span class="user-info">
              <el-avatar :size="32">{{ authStore.user?.nickname?.[0] || 'U' }}</el-avatar>
              <span class="username">{{ authStore.user?.nickname || '用户' }}</span>
            </span>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item @click="router.push('/app/settings')">设置</el-dropdown-item>
                <el-dropdown-item divided @click="handleLogout">退出登录</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </el-header>

      <el-main class="main">
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<style scoped>
.app-layout {
  min-height: 100vh;
}

.sidebar {
  background: var(--bg-card);
  border-right: 1px solid var(--border);
  display: flex;
  flex-direction: column;
  transition: width 0.3s;
}

.logo {
  height: 60px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 1.5rem;
  font-weight: 800;
  background: linear-gradient(135deg, var(--accent), var(--violet));
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  cursor: pointer;
}

.el-menu {
  border-right: none;
  flex: 1;
}

.collapse-btn {
  padding: 1rem;
  text-align: center;
  cursor: pointer;
  color: var(--text-muted);
  border-top: 1px solid var(--border);
}

.collapse-btn:hover {
  color: var(--text);
}

.header {
  background: var(--bg-card);
  border-bottom: 1px solid var(--border);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 2rem;
}

.header-left h2 {
  font-size: 1.25rem;
  font-weight: 600;
}

.user-info {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  cursor: pointer;
}

.username {
  font-size: 0.875rem;
}

.main {
  background: var(--bg-deep);
  padding: 2rem;
}
</style>
