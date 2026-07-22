<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue'
import { checkinApi } from '@/api/checkin'
import { useAuthStore } from '@/stores/auth'
import { ElMessage } from 'element-plus'
import * as echarts from 'echarts'
import RollingNumber from '@/components/RollingNumber.vue'

const authStore = useAuthStore()
const chartRef = ref<HTMLElement | null>(null)
let chart: echarts.ECharts | null = null
let midnightTimer: ReturnType<typeof setTimeout> | null = null
let tickTimer: ReturnType<typeof setInterval> | null = null
const now = ref(new Date())

const status = ref({ status: 'idle', clock_in: '', clock_out: '' })
const records = ref<Array<{ id: number; user_id: number; type: string; date: string; time: string; nickname: string; student_id: string; avatar: string }>>([])
const statsData = ref<Array<{ user_id: number; nickname: string; total_minutes: number; total_hours: number; days: number }>>([])
const onlineMembers = ref<Array<{ user_id: number; nickname: string; online: boolean }>>([])
const chartFilter = ref('week')
const todayBaseMinutes = ref(0)
const recordPage = ref(1)
const recordPageSize = 10

const paginatedRecords = computed(() => {
  const start = (recordPage.value - 1) * recordPageSize
  return records.value.slice(start, start + recordPageSize)
})

const filteredStats = statsData

function formatMinutes(totalMinutes: number): string {
  const m = Math.round(totalMinutes)
  if (m < 1) return '0分钟'
  const hours = Math.floor(m / 60)
  const mins = m % 60
  if (hours === 0) return `${mins}分钟`
  if (mins === 0) return `${hours}小时`
  return `${hours}小时${mins}分钟`
}

function formatTime(time: string): string {
  if (!time) return '--:--'
  return time.substring(0, 5)
}

function formatDate(): string {
  return new Date().toLocaleDateString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', weekday: 'short' })
}

const todayTotalHours = computed(() => {
  let total = todayBaseMinutes.value
  if (status.value.status === 'clocked_in' && status.value.clock_in) {
    const cur = now.value
    const [sh, sm] = status.value.clock_in.split(':').map(Number)
    total += Math.max(0, (cur.getHours() * 60 + cur.getMinutes()) - (sh * 60 + sm))
  }
  return formatMinutes(total)
})

async function loadTodayHours() {
  try {
    const res = await checkinApi.getStats('today')
    const stats: any[] = (res.data || []).filter((s: any) => s.user_id === authStore.user?.id)
    todayBaseMinutes.value = stats.length > 0 ? stats[0].total_minutes : 0
  } catch {
    console.error('Failed to load today hours')
  }
}

function calcDuration(signin: string, signout: string): string {
  if (!signin) return '--'
  const [sh, sm] = signin.split(':').map(Number)
  let endH: number, endM: number
  if (signout) {
    endH = parseInt(signout.split(':')[0])
    endM = parseInt(signout.split(':')[1])
  } else {
    const now = new Date()
    endH = now.getHours()
    endM = now.getMinutes()
  }
  let mins = (endH * 60 + endM) - (sh * 60 + sm)
  if (mins < 0) mins += 1440
  return formatMinutes(mins)
}

function getTodayOnlineCount(): number {
  return onlineMembers.value.filter(m => m.online).length
}

function getAvgHours(): string {
  if (filteredStats.value.length === 0) return '0分钟'
  const total = filteredStats.value.reduce((sum, s) => sum + s.total_minutes, 0)
  return formatMinutes(Math.round(total / filteredStats.value.length))
}

function getMyDays(): number {
  const me = filteredStats.value.find(s => s.user_id === authStore.user?.id)
  return me?.days || 0
}

function getPeriodLabel(): string {
  return chartFilter.value === 'today' ? '今日' : chartFilter.value === 'month' ? '本月' : '本周'
}

// Midnight auto-refresh: schedule next midnight refresh
function scheduleMidnightRefresh() {
  if (midnightTimer) clearTimeout(midnightTimer)
  const now = new Date()
  const midnight = new Date(now)
  midnight.setHours(24, 0, 0, 0)
  const msUntilMidnight = midnight.getTime() - now.getTime()
  midnightTimer = setTimeout(async () => {
    // Auto clock-out if still clocked in
    if (status.value.status === 'clocked_in') {
      try {
        await checkinApi.clockOut()
      } catch { /* ignore */ }
    }
    // Refresh all data
    await loadStatus()
    await loadRecords()
    await loadStats()
    await loadTodayHours()
    await loadOnlineMembers()
    scheduleMidnightRefresh()
  }, msUntilMidnight)
}

onMounted(async () => {
  await loadStatus()
  await loadRecords()
  await loadStats()
  await loadTodayHours()
  await loadOnlineMembers()
  await nextTick()
  initChart()
  scheduleMidnightRefresh()
  tickTimer = setInterval(() => { now.value = new Date() }, 60000)
})

onUnmounted(() => {
  if (midnightTimer) clearTimeout(midnightTimer)
  if (tickTimer) clearInterval(tickTimer)
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
  const periodLabel = chartFilter.value === 'today' ? '今日' : chartFilter.value === 'month' ? '本月' : '本周'
  chart.setOption({
    tooltip: {
      trigger: 'axis',
      backgroundColor: '#ffffff',
      borderColor: '#e4e4e7',
      textStyle: { color: '#18181b' },
      axisPointer: { type: 'shadow' },
      formatter: (params: any) => {
        const d = params[0]
        return `${d.name}<br/>${periodLabel}累计: ${formatMinutes(d.value)}`
      }
    },
    grid: { left: '3%', right: '4%', bottom: '3%', top: '10%', containLabel: true },
    xAxis: {
      type: 'category',
      data: data.map(d => d.nickname),
      axisLabel: { color: '#71717a', rotate: data.length > 6 ? 30 : 0 },
      axisLine: { lineStyle: { color: '#e4e4e7' } }
    },
    yAxis: {
      type: 'value',
      name: '分钟',
      axisLabel: { color: '#71717a' },
      splitLine: { lineStyle: { color: '#f4f4f5' } }
    },
    series: [{
      type: 'bar',
      data: data.map(d => ({
        value: Math.round(d.total_minutes),
        itemStyle: {
          color: d.user_id === authStore.user?.id
            ? new echarts.graphic.LinearGradient(0, 0, 0, 1, [
                { offset: 0, color: '#2563eb' },
                { offset: 1, color: '#60a5fa' }
              ])
            : new echarts.graphic.LinearGradient(0, 0, 0, 1, [
                { offset: 0, color: '#60a5fa' },
                { offset: 1, color: '#93c5fd' }
              ])
        }
      })),
      barMaxWidth: 40,
      label: {
        show: true,
        position: 'top',
        formatter: (params: any) => formatMinutes(params.value),
        color: '#71717a',
        fontSize: 11
      }
    }]
  })
}

async function setChartFilter(filter: string) {
  chartFilter.value = filter
  await loadStats()
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
    const res = await checkinApi.getTodayRecords()
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
    await loadTodayHours()
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
    await loadStats()
    await loadTodayHours()
    await loadOnlineMembers()
  } catch {
    ElMessage.error('签退失败')
  }
}

async function loadStats() {
  try {
    const res = await checkinApi.getStats(chartFilter.value)
    statsData.value = (res.data || []).filter((s: any) => s.nickname !== '超级管理员' && s.user_id > 0)
    updateChart()
  } catch {
    console.error('Failed to load stats')
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
    <!-- Punch Zone: two side-by-side cards -->
    <div class="punch-zone">
      <div class="punch-card punch-signin">
        <div class="punch-label">签到</div>
        <div class="punch-time">{{ formatTime(status.clock_in) }}</div>
        <div class="punch-date">{{ status.clock_in ? formatDate() : '尚未签到' }}</div>
        <el-button
          type="primary"
          size="large"
          :disabled="status.status === 'clocked_in'"
          @click="handleClockIn"
        >
          签到打卡
        </el-button>
        <div class="punch-status">
          <span class="status-indicator" :class="{ active: status.status === 'clocked_in' }"></span>
          <span v-if="status.status === 'clocked_in'">已签到</span>
          <span v-else-if="status.status === 'clocked_out'">已完成签到</span>
          <span v-else>等待签到</span>
        </div>
      </div>

      <div class="punch-card punch-signout">
        <div class="punch-label">签退</div>
        <div class="punch-time">{{ formatTime(status.clock_out) }}</div>
        <div class="punch-date">{{ status.clock_out ? formatDate() : '尚未签退' }}</div>
        <el-button
          type="danger"
          size="large"
          :disabled="status.status !== 'clocked_in'"
          @click="handleClockOut"
        >
          签退打卡
        </el-button>
        <div class="punch-status">
          <span class="status-indicator" :class="{ active: status.status === 'clocked_out' }"></span>
          <span v-if="status.status === 'clocked_out'">已签退</span>
          <span v-else-if="status.status === 'clocked_in'">等待签退</span>
          <span v-else>等待签退</span>
        </div>
      </div>
    </div>

    <!-- Stats Row -->
    <div class="stats-row">
      <div class="stat-card">
        <div class="stat-label">今日在线</div>
        <div class="stat-value accent">{{ getTodayOnlineCount() }}</div>
        <div class="stat-sub">当前在线成员</div>
      </div>
      <div class="stat-card">
        <div class="stat-label">平均工时</div>
        <div class="stat-value blue">{{ getAvgHours() }}</div>
        <div class="stat-sub">{{ getPeriodLabel() }}平均</div>
      </div>
      <div class="stat-card">
        <div class="stat-label">我的签到天数</div>
        <div class="stat-value amber">{{ getMyDays() }}</div>
        <div class="stat-sub">{{ getPeriodLabel() }}累计</div>
      </div>
      <div class="stat-card">
        <div class="stat-label">我的今日工时</div>
        <div class="stat-value violet">
          <RollingNumber :text="todayTotalHours" />
        </div>
        <div class="stat-sub">{{ status.status === 'clocked_in' ? '计时中' : status.status === 'clocked_out' ? '已完成' : '尚未签到' }}</div>
      </div>
    </div>

    <!-- Chart Section -->
    <div class="chart-section">
      <div class="card">
        <div class="section-header">
          <h3>成员工时统计</h3>
          <div class="chart-filters">
            <button class="filter-btn" :class="{ active: chartFilter === 'today' }" @click="setChartFilter('today')">今日</button>
            <button class="filter-btn" :class="{ active: chartFilter === 'week' }" @click="setChartFilter('week')">本周</button>
            <button class="filter-btn" :class="{ active: chartFilter === 'month' }" @click="setChartFilter('month')">本月</button>
          </div>
        </div>
        <div ref="chartRef" class="chart-container"></div>
      </div>
    </div>

    <!-- Log Table -->
    <div class="log-section">
      <div class="card">
        <div class="section-header">
          <h3>今日打卡记录</h3>
        </div>
        <el-table :data="paginatedRecords" style="width: 100%">
          <el-table-column label="成员" min-width="200">
            <template #default="{ row }">
              <div class="member-cell">
                <el-avatar :size="28" :src="row.avatar ? `/api/avatar/${row.avatar}` : undefined">
                  {{ row.nickname?.[0] || 'U' }}
                </el-avatar>
                <div class="member-info">
                  <span class="member-name">{{ row.nickname || '--' }}</span>
                  <span class="member-id" v-if="row.student_id">{{ row.student_id }}</span>
                </div>
              </div>
            </template>
          </el-table-column>
          <el-table-column label="类型" width="100">
            <template #default="{ row }">
              <span class="tag" :class="row.type === 'in' ? 'tag-in' : 'tag-out'">
                {{ row.type === 'in' ? '签到' : '签退' }}
              </span>
            </template>
          </el-table-column>
          <el-table-column label="签到时间" width="120">
            <template #default="{ row }">
              <span class="mono-text">{{ row.type === 'in' ? row.time : '--' }}</span>
            </template>
          </el-table-column>
          <el-table-column label="签退时间" width="120">
            <template #default="{ row }">
              <span class="mono-text">{{ row.type === 'out' ? row.time : '--' }}</span>
            </template>
          </el-table-column>
          <el-table-column label="工时" width="120">
            <template #default="{ row }">
              <span class="mono-text bold">{{ row.type === 'out' ? calcDuration(row.clock_in_time, row.time) : '--' }}</span>
            </template>
          </el-table-column>
        </el-table>
        <div class="pagination-wrap" v-if="records.length > recordPageSize">
          <el-pagination
            v-model:current-page="recordPage"
            :page-size="recordPageSize"
            :total="records.length"
            layout="prev, pager, next"
            small
          />
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.checkin {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

/* Punch Zone */
.punch-zone {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 1rem;
}

.punch-card {
  background: var(--bg-card);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-sm);
  padding: 1.5rem;
  position: relative;
  overflow: hidden;
  transition: border-color 0.2s, box-shadow 0.2s;
}

.punch-card:hover {
  box-shadow: var(--shadow-md);
}

.punch-card::before {
  content: '';
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  height: 3px;
}

.punch-signin::before {
  background: linear-gradient(90deg, var(--accent), transparent);
}

.punch-signout::before {
  background: linear-gradient(90deg, var(--danger), transparent);
}

.punch-label {
  font-size: 0.6875rem;
  text-transform: uppercase;
  letter-spacing: 0.1em;
  margin-bottom: 0.5rem;
  font-weight: 600;
  color: var(--text-muted);
}

.punch-signin .punch-label {
  color: var(--accent);
}

.punch-signout .punch-label {
  color: var(--danger);
}

.punch-time {
  font-size: 2.5rem;
  font-weight: 700;
  letter-spacing: -0.03em;
  line-height: 1;
  margin-bottom: 0.25rem;
  font-variant-numeric: tabular-nums;
  font-family: 'Courier New', monospace;
  color: var(--text);
}

.punch-signout .punch-time {
  color: var(--text-secondary);
}

.punch-date {
  font-size: 0.8125rem;
  color: var(--text-muted);
  margin-bottom: 1.25rem;
}

.punch-status {
  margin-top: 0.75rem;
  font-size: 0.75rem;
  color: var(--text-muted);
  display: flex;
  align-items: center;
  gap: 0.375rem;
}

.status-indicator {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--text-muted);
}

.status-indicator.active {
  background: var(--success);
  animation: pulse-dot 2s infinite;
}

@keyframes pulse-dot {
  0%, 100% { box-shadow: 0 0 0 0 rgba(22, 163, 74, 0.4); }
  50% { box-shadow: 0 0 0 6px transparent; }
}

/* Stats Row */
.stats-row {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 1rem;
}

.stat-card {
  background: var(--bg-card);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-sm);
  padding: 1rem 1.25rem;
  transition: border-color 0.2s;
}

.stat-card:hover {
  border-color: var(--accent);
}

.stat-label {
  font-size: 0.6875rem;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.05em;
  margin-bottom: 0.5rem;
}

.stat-value {
  font-size: 1.5rem;
  font-weight: 700;
  letter-spacing: -0.02em;
  font-variant-numeric: tabular-nums;
  color: var(--text);
}

.stat-value.accent { color: var(--accent); }
.stat-value.blue { color: #2563eb; }
.stat-value.amber { color: var(--warning); }
.stat-value.violet { color: #7c3aed; }

.stat-sub {
  font-size: 0.75rem;
  color: var(--text-muted);
  margin-top: 0.25rem;
}

/* Chart Section */
.chart-section .card {
  background: var(--bg-card);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-sm);
  padding: 1.25rem;
}

.section-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1rem;
}

.section-header h3 {
  font-size: 0.9375rem;
  font-weight: 600;
  letter-spacing: -0.01em;
  color: var(--text);
  margin-bottom: 0;
}

.chart-filters {
  display: flex;
  gap: 2px;
  background: var(--bg-deep);
  border-radius: 0.5rem;
  padding: 3px;
}

.filter-btn {
  padding: 0.375rem 0.875rem;
  border: none;
  background: transparent;
  color: var(--text-muted);
  font-size: 0.75rem;
  border-radius: 0.375rem;
  cursor: pointer;
  transition: all 0.2s;
}

.filter-btn.active {
  background: var(--bg-card);
  color: var(--text);
  box-shadow: var(--shadow-sm);
}

.filter-btn:hover:not(.active) {
  color: var(--text-secondary);
}

.chart-container {
  width: 100%;
  height: 300px;
}

/* Log Section */
.log-section .card {
  background: var(--bg-card);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-sm);
  padding: 1.25rem;
}

.member-cell {
  display: flex;
  align-items: center;
  gap: 0.625rem;
}

.member-info {
  display: flex;
  flex-direction: column;
  line-height: 1.3;
}

.member-name {
  font-size: 0.8125rem;
  font-weight: 500;
  color: var(--text);
}

.member-id {
  font-size: 0.6875rem;
  color: var(--text-muted);
  font-variant-numeric: tabular-nums;
}

.tag {
  display: inline-block;
  padding: 0.125rem 0.5rem;
  border-radius: 0.25rem;
  font-size: 0.6875rem;
  font-weight: 500;
}

.tag-in {
  background: var(--success-bg);
  color: var(--success);
}

.tag-out {
  background: var(--danger-bg);
  color: var(--danger);
}

.mono-text {
  font-family: 'Courier New', monospace;
  font-size: 0.8125rem;
  color: var(--text-secondary);
}

.mono-text.bold {
  font-weight: 600;
  color: var(--text);
}

.pagination-wrap {
  display: flex;
  justify-content: center;
  padding-top: 0.75rem;
}

/* Responsive */
@media (max-width: 900px) {
  .punch-zone {
    grid-template-columns: 1fr;
  }
  .stats-row {
    grid-template-columns: repeat(2, 1fr);
  }
}
</style>
