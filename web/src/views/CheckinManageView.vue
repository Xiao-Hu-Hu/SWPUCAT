<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { checkinApi } from '@/api/checkin'
import { memberApi } from '@/api/member'

const activeTab = ref('makeup')

// Members
interface Member {
  id: number
  nickname: string
  username: string
  student_id: string
  role: string
}
const members = ref<Member[]>([])

// Makeup
const makeupForm = ref({
  userId: null as number | null,
  date: new Date().toISOString().slice(0, 10),
  minutes: 60
})
const makeupLoading = ref(false)

// Week range for date picker (Monday to Sunday)
const now = new Date()
const dayOfWeek = now.getDay() || 7 // 0=Sunday -> 7
const weekStart = new Date(now)
weekStart.setDate(now.getDate() - dayOfWeek + 1)
weekStart.setHours(0, 0, 0, 0)
const weekEnd = new Date(weekStart)
weekEnd.setDate(weekStart.getDate() + 6)
weekEnd.setHours(23, 59, 59, 999)

function disabledDate(date: Date) {
  return date < weekStart || date > now
}

// Requirements (frontend uses hours, backend uses minutes)
interface Requirement {
  grade: number
  hours: number
}
const gradeNames: Record<number, string> = { 1: '大一', 2: '大二', 3: '大三', 4: '大四' }
const requirements = ref<Requirement[]>([
  { grade: 1, hours: 10 },
  { grade: 2, hours: 8 },
  { grade: 3, hours: 6 },
  { grade: 4, hours: 4 }
])
const requirementsLoading = ref(false)

// Report
const reportLoading = ref(false)

async function loadMembers() {
  try {
    const res = await memberApi.list()
    members.value = res.data || res || []
  } catch {
    members.value = []
  }
}

async function loadRequirements() {
  try {
    const res = await checkinApi.getRequirements()
    const data = res.data || res
    if (data?.requirements?.length) {
      requirements.value = data.requirements.map((r: any) => ({
        grade: r.grade,
        hours: Math.round(r.minutes / 60)
      }))
    }
  } catch {
    // keep defaults
  }
}

async function handleMakeup() {
  if (!makeupForm.value.userId) {
    ElMessage.warning('请选择成员')
    return
  }
  if (!makeupForm.value.date) {
    ElMessage.warning('请选择日期')
    return
  }
  if (makeupForm.value.minutes < 1 || makeupForm.value.minutes > 480) {
    ElMessage.warning('时长需在 1-480 分钟之间')
    return
  }
  makeupLoading.value = true
  try {
    await checkinApi.makeup(
      makeupForm.value.userId,
      makeupForm.value.date,
      makeupForm.value.minutes
    )
    ElMessage.success('补签成功')
    makeupForm.value.userId = null
    makeupForm.value.minutes = 60
  } catch (e: any) {
    ElMessage.error(e?.message || '补签失败')
  } finally {
    makeupLoading.value = false
  }
}

async function handleSaveRequirements() {
  requirementsLoading.value = true
  try {
    const payload = requirements.value.map(r => ({
      grade: r.grade,
      minutes: r.hours * 60
    }))
    await checkinApi.setRequirements(payload)
    ElMessage.success('打卡要求已保存')
  } catch {
    ElMessage.error('保存失败')
  } finally {
    requirementsLoading.value = false
  }
}

async function handlePublishReport() {
  reportLoading.value = true
  try {
    await checkinApi.publishReport()
    ElMessage.success('打卡报告已发布到公告')
  } catch {
    ElMessage.error('发布失败')
  } finally {
    reportLoading.value = false
  }
}

onMounted(() => {
  loadMembers()
  loadRequirements()
})
</script>

<template>
  <div class="checkin-manage">
    <el-tabs v-model="activeTab" class="manage-tabs">
      <!-- Tab 1: Makeup Check-in -->
      <el-tab-pane label="补签" name="makeup">
        <div class="tab-content">
          <el-form label-width="100px" class="makeup-form">
            <el-form-item label="选择成员">
              <el-select
                v-model="makeupForm.userId"
                placeholder="请选择成员"
                filterable
                style="width: 100%"
              >
                <el-option
                  v-for="m in members"
                  :key="m.id"
                  :label="`${m.nickname}（${m.student_id}）`"
                  :value="m.id"
                />
              </el-select>
            </el-form-item>
            <el-form-item label="补签日期">
              <el-date-picker
                v-model="makeupForm.date"
                type="date"
                placeholder="选择日期"
                value-format="YYYY-MM-DD"
                :disabled-date="disabledDate"
                style="width: 100%"
              />
            </el-form-item>
            <el-form-item label="打卡时长">
              <el-input-number
                v-model="makeupForm.minutes"
                :min="1"
                :max="480"
                :step="30"
              />
              <span class="unit-label">分钟</span>
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="makeupLoading" @click="handleMakeup">
                提交补签
              </el-button>
            </el-form-item>
          </el-form>
        </div>
      </el-tab-pane>

      <!-- Tab 2: Requirements -->
      <el-tab-pane label="打卡要求" name="requirements">
        <div class="tab-content">
          <p class="tab-desc">设置每个年级每周的最低打卡时长要求（年级根据学号前4位自动识别）</p>
          <el-table :data="requirements" border class="req-table">
            <el-table-column label="年级" width="120">
              <template #default="{ row }">
                {{ gradeNames[row.grade] || `大${row.grade}` }}
              </template>
            </el-table-column>
            <el-table-column label="每周要求时长（小时）">
              <template #default="{ row }">
                <el-input-number
                  v-model="row.hours"
                  :min="0"
                  :max="168"
                  :step="1"
                />
              </template>
            </el-table-column>
          </el-table>
          <div class="req-actions">
            <el-button type="primary" :loading="requirementsLoading" @click="handleSaveRequirements">
              保存
            </el-button>
          </div>
        </div>
      </el-tab-pane>

      <!-- Tab 3: Report -->
      <el-tab-pane label="发布报告" name="report">
        <div class="tab-content report-tab">
          <p class="tab-desc">
            点击按钮将自动生成本周打卡统计报告并发布为公告。报告包含所有成员的本周打卡时长、要求时长和差额情况。
          </p>
          <el-button
            type="primary"
            size="large"
            :loading="reportLoading"
            @click="handlePublishReport"
          >
            发布本周打卡报告
          </el-button>
        </div>
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<style scoped>
.checkin-manage {
  max-width: 720px;
}

.manage-tabs {
  background: var(--bg-card);
  border-radius: var(--radius);
  padding: 1rem 1.25rem;
}

.tab-content {
  padding: 1rem 0;
}

.tab-desc {
  color: var(--text-secondary);
  font-size: 0.875rem;
  margin-bottom: 1rem;
}

.makeup-form {
  max-width: 480px;
}

.unit-label {
  margin-left: 0.5rem;
  color: var(--text-secondary);
  font-size: 0.875rem;
}

.req-table {
  max-width: 480px;
}

.req-actions {
  margin-top: 1rem;
}

.report-tab {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 1rem;
}
</style>
