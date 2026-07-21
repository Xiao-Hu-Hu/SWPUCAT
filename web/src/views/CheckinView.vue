<script setup lang="ts">
import { ref, computed, onMounted, nextTick } from 'vue'
import { checkinApi } from '@/api/checkin'
import { useAuthStore } from '@/stores/auth'
import { ElMessage } from 'element-plus'
import * as echarts from 'echarts'

const authStore = useAuthStore()
const chartRef = ref<HTMLElement | null>(null)
let chart: echarts.ECharts | null = null

const status = ref({ status: 'idle', clock_in: '', clock_out: '' })
const records = ref<Array<{ id: number; type: string; date: string; time: string }>>([])
const weeklyStats = ref<Array<{ user_id: number; nickname: string; total_minutes: number; total_hours: number; days: number }>>([])
const onlineMembers = ref<Array<{ user_id: number; nickname: string; online: boolean }>>([])
const currentTime = ref(new Date().toLocaleTimeString('zh-CN'))

const filteredStats = computed(() => {
  return weeklyStats.value.filter(s => s.nickname !== '超级管理员')
})

setInterval(() => {
  currentTime.value = new Date().toLocaleTimeString('zh-CN')
}, 1000)

onMounted(async () => {
  await loadStatus()
  await loadRecords()
  await loadWeeklyStats()
  await loadOnlineMembers()
  await nextTick()
  initChart()
})

function initChart() {
  if (!chartRef.value) return
  chart = echarts.init(chartRef.value)
  updateChart()
  window.addEventListener('resize', () => chart?.resize())
}

function updateChart() {
  if (!chart) return
  const data = filteredStats.value
  const option = {
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'shadow' },
      formatter: (params: any) => {
        const d = params[0]
        return `${d.name}<br/>本周累计: ${d.value} 小时`
      }
    },
    grid: {
      left: '3%',
      right: '4%',
      bottom: '3%',
      top: '15%',
      containLabel: true
    },
    xAxis: {
      type: 'category',
      data: data.map(d => d.nickname),
      axisLabel: {
        color: '#909399',
        rotate: data.length > 5 ? 30 : 0
      }
    },
    yAxis: {
      type: 'value',
      name: '小时',
      axisLabel: { color: '#909399' }
    },
    series: [{
      type: 'bar',
      data: data.map(d => ({
        value: d.total_hours,
        itemStyle: {
          color: d.user_id === authStore.user?.id
            ? new echarts.graphic.LinearGradient(0, 0, 0, 1, [
                { offset: 0, color: '#67c23a' },
                { offset: 1, color: '#409eff' }
              ])
            : new echarts.graphic.LinearGradient(0, 0, 0, 1, [
                { offset: 0, color: '#409eff' },
                { offset: 1, color: '#909399' }
              ])
        }
      })),
      barMaxWidth: 40,
      label: {
        show: true,
        position: 'top',
        formatter: '{c}h',
        color: '#909399'
      }
    }]
  }
  chart.setOption(option)
}

async function loadStatus() {
  try {
    const res = await checkinApi.getStatus()
    status.value = res.data
  } catch {
    console.error('Failed to load status')
  }
}

async function loadRecords() {
  try {
    const res = await checkinApi.getRecords(10)
    records.value = res.data
  } catch {
    console.error('Failed to load records')
  }
}

async function handleClockIn() {
  try {
    await checkinApi.clockIn()
    ElMessage.success('签到成功')
    await loadStatus()
    await loadRecords()
    await loadOnlineMembers()
  } catch {
    ElMessage.error('签到失败')
  }
}

async function handleClockOut() {
  try {
    await checkinApi.clockOut()
    ElMessage.success('签退成功')
    await loadStatus()
    await loadRecords()
    await loadOnlineMembers()
  } catch {
    ElMessage.error('签退失败')
  }
}

async function loadWeeklyStats() {
  try {
    const res = await checkinApi.getWeeklyStats()
    weeklyStats.value = res.data || []
    updateChart()
  } catch {
    console.error('Failed to load weekly stats')
  }
}

async function loadOnlineMembers() {
  try {
    const res = await checkinApi.getOnlineMembers()
    onlineMembers.value = res.data || []
  } catch {
    console.error('Failed to load online members')
  }
}
</script>

<template>
  <div class="checkin">
    <div class="clock-section">
      <div class="clock-card">
        <div class="clock-display">
          <span class="time">{{ currentTime }}</span>
          <span class="date">{{ new Date().toLocaleDateString('zh-CN') }}</span>
        </div>
        <div class="status-badge" :class="status.status">
          {{ status.status === 'clocked_in' ? '已签到' : status.status === 'clocked_out' ? '已签退' : '未签到' }}
        </div>
        <div class="clock-actions">
          <el-button
            type="primary"
            size="large"
            :disabled="status.status === 'clocked_in'"
            @click="handleClockIn"
          >
            签到
          </el-button>
          <el-button
            type="danger"
            size="large"
            :disabled="status.status !== 'clocked_in'"
            @click="handleClockOut"
          >
            签退
          </el-button>
        </div>
        <div class="clock-info" v-if="status.clock_in || status.clock_out">
          <span v-if="status.clock_in">签到时间: {{ status.clock_in }}</span>
          <span v-if="status.clock_out">签退时间: {{ status.clock_out }}</span>
        </div>
      </div>
    </div>

    <div class="chart-section">
      <div class="card">
        <h3>本周签到时长统计</h3>
        <div ref="chartRef" class="chart-container"></div>
      </div>
    </div>

    <div class="stats-section">
      <div class="card">
        <h3>本周签到统计</h3>
        <el-table :data="filteredStats" style="width: 100%">
          <el-table-column prop="nickname" label="成员" width="120" />
          <el-table-column label="本周累计时长" width="150">
            <template #default="{ row }">
              {{ row.total_hours.toFixed(1) }} 小时
            </template>
          </el-table-column>
          <el-table-column prop="days" label="签到天数" width="100" />
          <el-table-column label="在线状态" width="100">
            <template #default="{ row }">
              <span class="online-dot" :class="{ online: onlineMembers.find(m => m.user_id === row.user_id)?.online }"></span>
              {{ onlineMembers.find(m => m.user_id === row.user_id)?.online ? '在线' : '离线' }}
            </template>
          </el-table-column>
        </el-table>
      </div>
    </div>

    <div class="online-section">
      <div class="card">
        <h3>在线成员</h3>
        <div class="online-list">
          <div v-for="member in onlineMembers" :key="member.user_id" class="online-item">
            <span class="online-dot" :class="{ online: member.online }"></span>
            <span>{{ member.nickname }}</span>
          </div>
        </div>
      </div>
    </div>

    <div class="records-section">
      <div class="card">
        <h3>签到记录</h3>
        <el-table :data="records" style="width: 100%">
          <el-table-column prop="date" label="日期" width="120" />
          <el-table-column prop="time" label="时间" width="120" />
          <el-table-column prop="type" label="类型" width="100">
            <template #default="{ row }">
              <el-tag :type="row.type === 'in' ? 'success' : 'danger'">
                {{ row.type === 'in' ? '签到' : '签退' }}
              </el-tag>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </div>
  </div>
</template>

<style scoped>
.checkin {
  max-width: 800px;
}

.clock-section {
  margin-bottom: 2rem;
}

.clock-card {
  background: var(--bg-card);
  border: 1px solid var(--border);
  border-radius: 12px;
  padding: 2rem;
  text-align: center;
}

.clock-display {
  margin-bottom: 1.5rem;
}

.time {
  display: block;
  font-size: 3rem;
  font-weight: 700;
  font-family: 'Courier New', monospace;
}

.date {
  font-size: 1rem;
  color: var(--text-muted);
}

.status-badge {
  display: inline-block;
  padding: 0.5rem 1rem;
  border-radius: 20px;
  font-size: 0.875rem;
  margin-bottom: 1.5rem;
}

.status-badge.idle {
  background: rgba(148, 163, 184, 0.2);
  color: var(--text-muted);
}

.status-badge.clocked_in {
  background: rgba(34, 197, 94, 0.2);
  color: var(--success);
}

.status-badge.clocked_out {
  background: rgba(239, 68, 68, 0.2);
  color: var(--danger);
}

.clock-actions {
  display: flex;
  gap: 1rem;
  justify-content: center;
  margin-bottom: 1rem;
}

.clock-info {
  display: flex;
  gap: 2rem;
  justify-content: center;
  color: var(--text-muted);
  font-size: 0.875rem;
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

.stats-section {
  margin-bottom: 2rem;
}

.online-section {
  margin-bottom: 2rem;
}

.online-list {
  display: flex;
  flex-wrap: wrap;
  gap: 1rem;
}

.online-item {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.5rem 1rem;
  background: var(--bg-deep);
  border-radius: 8px;
}

.online-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--text-muted);
}

.online-dot.online {
  background: var(--success);
  box-shadow: 0 0 6px var(--success);
}

.chart-section {
  margin-bottom: 2rem;
}

.chart-container {
  width: 100%;
  height: 300px;
}
</style>
