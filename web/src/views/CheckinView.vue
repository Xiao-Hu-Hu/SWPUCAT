<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { checkinApi } from '@/api/checkin'
import { ElMessage } from 'element-plus'

const status = ref({ status: 'idle', clock_in: '', clock_out: '' })
const records = ref<Array<{ id: number; type: string; date: string; time: string }>>([])
const currentTime = ref(new Date().toLocaleTimeString('zh-CN'))

setInterval(() => {
  currentTime.value = new Date().toLocaleTimeString('zh-CN')
}, 1000)

onMounted(async () => {
  await loadStatus()
  await loadRecords()
})

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
  } catch {
    ElMessage.error('签退失败')
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
</style>
