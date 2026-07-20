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
    dashboard.value = res.data
  } catch {
    console.error('Failed to load dashboard')
  }
})
</script>

<template>
  <div class="dashboard">
    <div class="stats-grid">
      <div class="stat-card">
        <div class="stat-icon" style="background: linear-gradient(135deg, #06b6d4, #0891b2)">
          <el-icon :size="24"><User /></el-icon>
        </div>
        <div class="stat-info">
          <span class="stat-value">{{ dashboard.member_count }}</span>
          <span class="stat-label">成员总数</span>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon" style="background: linear-gradient(135deg, #22c55e, #16a34a)">
          <el-icon :size="24"><Clock /></el-icon>
        </div>
        <div class="stat-info">
          <span class="stat-value">{{ dashboard.today_checkins }}</span>
          <span class="stat-label">今日签到</span>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon" style="background: linear-gradient(135deg, #f59e0b, #d97706)">
          <el-icon :size="24"><Bell /></el-icon>
        </div>
        <div class="stat-info">
          <span class="stat-value">{{ dashboard.announcement_count }}</span>
          <span class="stat-label">公告数量</span>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon" style="background: linear-gradient(135deg, #8b5cf6, #7c3aed)">
          <el-icon :size="24"><Folder /></el-icon>
        </div>
        <div class="stat-info">
          <span class="stat-value">{{ dashboard.knowledge_count }}</span>
          <span class="stat-label">知识库</span>
        </div>
      </div>
    </div>

    <div class="content-grid">
      <div class="card activities-card">
        <h3>最近动态</h3>
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

      <div class="card online-card">
        <h3>在线成员</h3>
        <div class="online-list">
          <div v-for="member in dashboard.online_members" :key="member.id" class="online-member">
            <el-avatar :size="36">{{ member.avatar }}</el-avatar>
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
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: 1.5rem;
  margin-bottom: 2rem;
}

.stat-card {
  background: var(--bg-card);
  border: 1px solid var(--border);
  border-radius: 12px;
  padding: 1.5rem;
  display: flex;
  align-items: center;
  gap: 1rem;
}

.stat-icon {
  width: 48px;
  height: 48px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: white;
}

.stat-info {
  display: flex;
  flex-direction: column;
}

.stat-value {
  font-size: 1.75rem;
  font-weight: 700;
}

.stat-label {
  font-size: 0.875rem;
  color: var(--text-muted);
}

.content-grid {
  display: grid;
  grid-template-columns: 2fr 1fr;
  gap: 1.5rem;
}

.card {
  background: var(--bg-card);
  border: 1px solid var(--border);
  border-radius: 12px;
  padding: 1.5rem;
}

.card h3 {
  margin-bottom: 1rem;
  font-size: 1rem;
  font-weight: 600;
}

.activities-list {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.activity-item {
  display: flex;
  align-items: flex-start;
  gap: 0.75rem;
}

.activity-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-top: 6px;
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
}

.activity-time {
  font-size: 0.75rem;
  color: var(--text-muted);
}

.online-list {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.online-member {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.member-name {
  font-size: 0.875rem;
}
</style>
