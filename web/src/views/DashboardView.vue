<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { dashboardApi } from '@/api/dashboard'

const dashboard = ref({
  member_count: 0,
  today_checkins: 0,
  announcement_count: 0,
  knowledge_count: 0,
  recent_activities: [] as Array<{ text: string; color: string; time: string }>,
  online_members: [] as Array<{ id: number; nickname: string; avatar: string }>
})

onMounted(async () => {
  try {
    const res = await dashboardApi.getDashboard()
    const data = res.data
    // Ensure arrays are not null
    if (!data.recent_activities) data.recent_activities = []
    if (!data.online_members) data.online_members = []
    dashboard.value = data
  } catch {
    console.error('Failed to load dashboard')
  }
})
</script>

<template>
  <div class="dashboard">
    <div class="stats-grid">
      <div class="stat-card">
        <div class="stat-icon icon-blue">
          <el-icon :size="20"><User /></el-icon>
        </div>
        <div class="stat-info">
          <span class="stat-value">{{ dashboard.member_count }}</span>
          <span class="stat-label">成员总数</span>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon icon-green">
          <el-icon :size="20"><Clock /></el-icon>
        </div>
        <div class="stat-info">
          <span class="stat-value">{{ dashboard.today_checkins }}</span>
          <span class="stat-label">今日签到</span>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon icon-amber">
          <el-icon :size="20"><Bell /></el-icon>
        </div>
        <div class="stat-info">
          <span class="stat-value">{{ dashboard.announcement_count }}</span>
          <span class="stat-label">公告数量</span>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon icon-violet">
          <el-icon :size="20"><Folder /></el-icon>
        </div>
        <div class="stat-info">
          <span class="stat-value">{{ dashboard.knowledge_count }}</span>
          <span class="stat-label">知识库</span>
        </div>
      </div>
    </div>

    <div class="content-grid">
      <div class="card">
        <h3 class="card-title">最近动态</h3>
        <div class="activities-list">
          <div v-for="(activity, index) in dashboard.recent_activities" :key="index" class="activity-item">
            <div class="activity-dot" :class="activity.color"></div>
            <div class="activity-content">
              <span class="activity-text">{{ activity.text }}</span>
              <span class="activity-time">{{ activity.time }}</span>
            </div>
          </div>
          <el-empty v-if="dashboard.recent_activities.length === 0" description="暂无动态" />
        </div>
      </div>

      <div class="card">
        <h3 class="card-title">在线成员</h3>
        <div class="online-list">
          <div v-for="member in dashboard.online_members" :key="member.id" class="online-member">
            <el-avatar :size="32" :src="member.avatar ? `/api/avatar/${member.avatar}` : undefined">{{ member.nickname?.[0] || 'U' }}</el-avatar>
            <span class="member-name">{{ member.nickname }}</span>
          </div>
          <el-empty v-if="dashboard.online_members.length === 0" description="暂无在线成员" />
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.dashboard {
  max-width: 1200px;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 1rem;
  margin-bottom: 1.5rem;
}

.stat-card {
  background: var(--bg-card);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  padding: 1.25rem;
  display: flex;
  align-items: center;
  gap: 1rem;
  box-shadow: var(--shadow-sm);
  transition: box-shadow 0.2s;
}

.stat-card:hover {
  box-shadow: var(--shadow-md);
}

.stat-icon {
  width: 40px;
  height: 40px;
  border-radius: var(--radius);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.icon-blue { background: var(--accent-bg); color: var(--accent); }
.icon-green { background: var(--success-bg); color: var(--success); }
.icon-amber { background: var(--warning-bg); color: var(--warning); }
.icon-violet { background: #f3e8ff; color: #7c3aed; }

.stat-info {
  display: flex;
  flex-direction: column;
}

.stat-value {
  font-size: 1.5rem;
  font-weight: 700;
  letter-spacing: -0.02em;
  line-height: 1.2;
  font-variant-numeric: tabular-nums;
}

.stat-label {
  font-size: 0.8125rem;
  color: var(--text-muted);
  margin-top: 0.125rem;
}

.content-grid {
  display: grid;
  grid-template-columns: 2fr 1fr;
  gap: 1rem;
}

.card {
  background: var(--bg-card);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  padding: 1.25rem;
  box-shadow: var(--shadow-sm);
}

.card-title {
  font-size: 0.9375rem;
  font-weight: 600;
  letter-spacing: -0.01em;
  margin-bottom: 1rem;
  color: var(--text);
}

.activities-list {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.activity-item {
  display: flex;
  align-items: flex-start;
  gap: 0.625rem;
  padding: 0.5rem 0;
}

.activity-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  margin-top: 6px;
  flex-shrink: 0;
}

.activity-dot.warning { background: var(--warning); }
.activity-dot.success { background: var(--success); }
.activity-dot.primary { background: var(--accent); }

.activity-content {
  display: flex;
  flex-direction: column;
}

.activity-text {
  font-size: 0.875rem;
  color: var(--text-secondary);
}

.activity-time {
  font-size: 0.75rem;
  color: var(--text-muted);
  margin-top: 0.125rem;
}

.online-list {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.online-member {
  display: flex;
  align-items: center;
  gap: 0.625rem;
  padding: 0.375rem 0;
}

.member-name {
  font-size: 0.875rem;
  color: var(--text-secondary);
}
</style>
