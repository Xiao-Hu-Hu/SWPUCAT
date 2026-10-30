<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import {
  agentAuditApi,
  type AgentAuditDetail,
  type AgentAuditRun,
  type AgentAuditSummary
} from '@/api/agentAudit'

const now = new Date()
const sevenDaysAgo = new Date(now.getTime() - 7 * 24 * 60 * 60 * 1000)
const range = ref<[Date, Date]>([sevenDaysAgo, now])
const status = ref('')
const page = ref(1)
const pageSize = 20
const loading = ref(false)
const detailLoading = ref(false)
const total = ref(0)
const runs = ref<AgentAuditRun[]>([])
const summary = ref<AgentAuditSummary | null>(null)
const detail = ref<AgentAuditDetail | null>(null)
const detailVisible = ref(false)

const statusRows = computed(() => Object.entries(summary.value?.status_counts || {})
  .map(([name, count]) => ({ name, count }))
  .sort((left, right) => right.count - left.count))

const toolRows = computed(() => Object.entries(summary.value?.tool_counts || {})
  .map(([name, count]) => ({ name, count }))
  .sort((left, right) => right.count - left.count))

function query() {
  return {
    from: range.value[0].toISOString(),
    to: range.value[1].toISOString(),
    status: status.value || undefined,
    page: page.value,
    page_size: pageSize
  }
}

async function load() {
  loading.value = true
  try {
    const [listResponse, summaryResponse] = await Promise.all([
      agentAuditApi.list(query()),
      agentAuditApi.summary(query())
    ])
    runs.value = listResponse.data.items || []
    total.value = listResponse.data.total || 0
    summary.value = summaryResponse.data
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.message || 'Agent 审计数据加载失败')
  } finally {
    loading.value = false
  }
}

function applyFilters() {
  page.value = 1
  void load()
}

async function openDetail(run: AgentAuditRun) {
  detailVisible.value = true
  detailLoading.value = true
  detail.value = null
  try {
    const response = await agentAuditApi.detail(run.id)
    detail.value = response.data
  } catch (error: any) {
    detailVisible.value = false
    ElMessage.error(error?.response?.data?.message || '审计详情加载失败')
  } finally {
    detailLoading.value = false
  }
}

function formatPercent(value?: number) {
  return `${((value || 0) * 100).toFixed(1)}%`
}

function formatDuration(value?: number) {
  if (value === undefined || value === null) return '--'
  return value >= 1000 ? `${(value / 1000).toFixed(2)} 秒` : `${Math.round(value)} 毫秒`
}

function formatTokens(value?: number) {
  return value === undefined || value === null ? '--' : value.toLocaleString('zh-CN')
}

function formatCost(cost?: { known: boolean; micro_usd?: number }) {
  if (!cost?.known) return '未知'
  return `$${((cost.micro_usd || 0) / 1_000_000).toFixed(6)}`
}

function formatTime(value: string) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '--' : date.toLocaleString('zh-CN')
}

function statusType(value: string) {
  if (value === 'success') return 'success'
  if (value === 'busy' || value === 'timeout') return 'warning'
  if (value === 'rejected') return 'info'
  return 'danger'
}

function prettyJSON(value: string) {
  try {
    return JSON.stringify(JSON.parse(value), null, 2)
  } catch {
    return '{}'
  }
}

onMounted(() => void load())
</script>

<template>
  <div class="audit-page">
    <div class="page-heading">
      <div>
        <h2>Agent 运行审计</h2>
        <p>仅展示隐私安全投影；不保存提示词、回答正文、授权头或原始敏感参数。</p>
      </div>
      <div class="filters">
        <el-date-picker v-model="range" type="datetimerange" :clearable="false" start-placeholder="开始时间" end-placeholder="结束时间" />
        <el-select v-model="status" clearable placeholder="全部状态" style="width: 150px">
          <el-option v-for="item in ['success', 'rejected', 'busy', 'unavailable', 'timeout', 'canceled', 'error']" :key="item" :label="item" :value="item" />
        </el-select>
        <el-button type="primary" :loading="loading" @click="applyFilters">查询</el-button>
      </div>
    </div>

    <div class="metric-grid" v-loading="loading">
      <div class="metric-card"><span>调用量</span><strong>{{ summary?.total_runs || 0 }}</strong></div>
      <div class="metric-card"><span>成功率</span><strong>{{ formatPercent(summary?.success_rate) }}</strong></div>
      <div class="metric-card"><span>平均延迟</span><strong>{{ formatDuration(summary?.average_duration_ms) }}</strong></div>
      <div class="metric-card"><span>总 Token</span><strong>{{ formatTokens(summary?.total_tokens) }}</strong></div>
      <div class="metric-card"><span>估算费用</span><strong>{{ formatCost(summary?.estimated_cost) }}</strong></div>
    </div>

    <div class="distribution-grid">
      <section class="panel">
        <h3>状态分布</h3>
        <el-table :data="statusRows" size="small" empty-text="暂无运行记录">
          <el-table-column prop="name" label="状态" />
          <el-table-column prop="count" label="次数" width="100" />
        </el-table>
      </section>
      <section class="panel">
        <h3>工具使用</h3>
        <el-table :data="toolRows" size="small" empty-text="暂无工具调用">
          <el-table-column prop="name" label="工具" />
          <el-table-column prop="count" label="次数" width="100" />
        </el-table>
      </section>
    </div>

    <section class="panel">
      <h3>运行明细</h3>
      <el-table v-loading="loading" :data="runs" empty-text="当前筛选范围内没有记录" @row-click="openDetail">
        <el-table-column label="时间" min-width="170"><template #default="{ row }">{{ formatTime(row.created_at) }}</template></el-table-column>
        <el-table-column prop="actor_role" label="角色" width="120" />
        <el-table-column label="状态" width="120"><template #default="{ row }"><el-tag :type="statusType(row.status)" size="small">{{ row.status }}</el-tag></template></el-table-column>
        <el-table-column label="耗时" width="120"><template #default="{ row }">{{ formatDuration(row.duration_ms) }}</template></el-table-column>
        <el-table-column label="Token" width="120"><template #default="{ row }">{{ formatTokens(row.total_tokens) }}</template></el-table-column>
        <el-table-column label="费用" width="120"><template #default="{ row }">{{ formatCost(row.estimated_cost) }}</template></el-table-column>
        <el-table-column prop="tool_call_count" label="工具" width="80" />
        <el-table-column prop="failure_code" label="错误码" min-width="180" />
      </el-table>
      <el-pagination v-model:current-page="page" :page-size="pageSize" :total="total" layout="prev, pager, next, total" @current-change="load" />
    </section>

    <el-dialog v-model="detailVisible" title="运行审计详情" width="760px">
      <div v-loading="detailLoading" class="detail-body">
        <template v-if="detail">
          <el-descriptions :column="2" border>
            <el-descriptions-item label="请求 ID">{{ detail.request_id }}</el-descriptions-item>
            <el-descriptions-item label="状态">{{ detail.status }}</el-descriptions-item>
            <el-descriptions-item label="耗时">{{ formatDuration(detail.duration_ms) }}</el-descriptions-item>
            <el-descriptions-item label="费用">{{ formatCost(detail.estimated_cost) }}</el-descriptions-item>
          </el-descriptions>
          <div v-for="call in detail.tool_calls" :key="call.sequence" class="tool-call">
            <div class="tool-title"><strong>#{{ call.sequence }} {{ call.name }}</strong><el-tag size="small">{{ call.status }}</el-tag></div>
            <span>参数投影</span><pre>{{ prettyJSON(call.arguments_json) }}</pre>
            <span>结果投影</span><pre>{{ prettyJSON(call.result_json) }}</pre>
          </div>
        </template>
      </div>
    </el-dialog>
  </div>
</template>

<style scoped>
.audit-page { display: flex; flex-direction: column; gap: 1rem; }
.page-heading { display: flex; align-items: flex-end; justify-content: space-between; gap: 1rem; }
.page-heading h2 { margin: 0 0 0.25rem; font-size: 1.25rem; }
.page-heading p { margin: 0; color: var(--text-muted); font-size: 0.8125rem; }
.filters { display: flex; align-items: center; gap: 0.5rem; }
.metric-grid { display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); gap: 0.75rem; }
.metric-card, .panel { background: var(--bg-card); border: 1px solid var(--border); border-radius: var(--radius-lg); box-shadow: var(--shadow-sm); }
.metric-card { padding: 1rem; display: flex; flex-direction: column; gap: 0.4rem; }
.metric-card span { color: var(--text-muted); font-size: 0.75rem; }
.metric-card strong { font-size: 1.25rem; font-variant-numeric: tabular-nums; }
.distribution-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 1rem; }
.panel { padding: 1rem; }
.panel h3 { margin: 0 0 0.75rem; font-size: 0.9375rem; }
.panel :deep(.el-pagination) { margin-top: 1rem; justify-content: flex-end; }
.detail-body { min-height: 120px; }
.tool-call { margin-top: 1rem; padding: 0.875rem; border: 1px solid var(--border); border-radius: var(--radius); }
.tool-title { display: flex; justify-content: space-between; margin-bottom: 0.75rem; }
.tool-call > span { display: block; margin: 0.5rem 0 0.25rem; color: var(--text-muted); font-size: 0.75rem; }
pre { margin: 0; padding: 0.75rem; max-height: 220px; overflow: auto; background: var(--bg-deep); border-radius: var(--radius); font-size: 0.75rem; }
@media (max-width: 1000px) { .page-heading { align-items: stretch; flex-direction: column; } .filters { flex-wrap: wrap; } .metric-grid { grid-template-columns: repeat(2, 1fr); } .distribution-grid { grid-template-columns: 1fr; } }
</style>
