<script setup lang="ts">
import { ref, reactive, computed, onMounted, watch } from 'vue'
import { reportApi, type ReportPeriod, type Report, type ReportVersion, type ReportSection, type QualityFinding, type RosterEntry } from '@/api/report'
import { memberApi } from '@/api/member'
import { useAuthStore } from '@/stores/auth'
import { ElMessage } from 'element-plus'
import { renderMarkdown } from '@/utils/markdown'

function formatDeadline(value: string) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '未设置' : date.toLocaleString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false })
}

const authStore = useAuthStore()
const isManager = computed(() => authStore.isCaptain || authStore.isSuperAdmin)

interface Member {
  id: number
  nickname: string
  username: string
  student_id: string
  role: string
}

interface GradeGroup {
  year: string
  label: string
  members: Member[]
}

// The editor stays open while drafting or submitted so the author can edit and
// resubmit. Only a finalized (reviewed/archived) report is locked.
const isLocked = computed(() => ['reviewed', 'archived'].includes(myReport.value?.status || ''))

// When the period's submission deadline has passed, the editor is locked even
// for a draft, and create/save/submit are disabled.
const periodLocked = computed(() => isPeriodLocked(selectedPeriod.value))
const periodLateWindow = computed(() => !!selectedPeriod.value && !periodLocked.value && isPeriodLateWindow(selectedPeriod.value))

const periods = ref<ReportPeriod[]>([])
const selectedPeriodId = ref<number | undefined>(undefined)
const reports = ref<Report[]>([])
const myReport = ref<Report | null>(null)
const currentVersion = ref<ReportVersion | null>(null)
const isCurrentPeriodMember = ref(false)

const editorTitle = ref('')
const editorBody = ref('')
const showPreview = ref(false)
const saving = ref(false)
const submitting = ref(false)
const checking = ref(false)
const withdrawing = ref(false)

const members = ref<Member[]>([])
const gradeGroups = ref<GradeGroup[]>([])
const selectedUserIds = ref<number[]>([])
const roster = ref<RosterEntry[]>([])
const addingMembers = ref(false)

const showCreatePeriod = ref(false)
const createPeriodForm = reactive({ period_key: '', starts_on: '', ends_on: '', due_at: '' })

const showAddMember = ref(false)

const showRosterPreview = ref(false)
const rosterPreview = ref<{ report: Report; version: ReportVersion } | null>(null)

const showReviewDialog = ref(false)
const reviewTarget = ref<{ report: Report; version: ReportVersion } | null>(null)
const reviewing = ref(false)

const parsedSections = computed<ReportSection[]>(() => currentVersion.value?.parsed_json?.sections || [])
const qualityFindings = computed<QualityFinding[]>(() => currentVersion.value?.quality_json?.findings || [])
const reviewSections = computed<ReportSection[]>(() => reviewTarget.value?.version.parsed_json?.sections || [])
const reviewFindings = computed<QualityFinding[]>(() => reviewTarget.value?.version.quality_json?.findings || [])
const submittedReports = computed(() => reports.value.filter(r => r.status === 'submitted'))
const submittedCount = computed(() => roster.value.filter(r => !!r.submitted_at).length)

const firstDraftHint = computed(() => !selectedPeriod.value ? '请选择月报周期' : !isCurrentPeriodMember.value ? '你不在本期提交名单中' : periodLocked.value ? '本期已截止，尚未创建月报' : '尚未创建本期月报，可点击创建月报开始填写')

onMounted(async () => {
  await loadPeriods()
  await loadMembership()
  if (isManager.value) {
    await loadMembers()
  }
  if (selectedPeriodId.value) {
    await loadReports()
    if (isManager.value) await loadRoster()
  }
})

function periodStatusLabel(status: string): string {
  if (status === 'open') return '进行中'
  if (status === 'closed') return '已关闭'
  if (status === 'archived') return '已归档'
  if (status === 'expired') return '已截止'
  return status
}

// The stored status only ever transitions via explicit manager actions, so an
// open period whose deadline has passed must be surfaced as "已截止" here. The
// backend keeps the raw status; this is a display-only derivation.
function periodEffectiveStatus(p: ReportPeriod): string {
  if (p.status === 'closed' || p.status === 'archived') return p.status
  if (p.status === 'open' && (periodDeadlineMs(p) < Date.now() || periodEndMs(p) < Date.now())) return 'expired'
  return p.status
}

function periodDeadlineMs(p?: ReportPeriod): number {
  if (!p) return 0
  const d = p.due_at ?? p.ends_on
  return d ? new Date(d).getTime() : 0
}

function periodEndMs(p?: ReportPeriod): number {
  if (!p) return 0
  return p.ends_on ? new Date(p.ends_on).getTime() : 0
}

// The period is locked once the (now mandatory) submission deadline passes: no
// create/save/submit is allowed and the editor is locked.
function isPeriodLocked(p?: ReportPeriod): boolean {
  return !!p && Date.now() > periodDeadlineMs(p)
}

// The late window opens when the reporting window has closed but the submission
// deadline has not yet passed: submissions still go through, flagged as overdue.
function isPeriodLateWindow(p?: ReportPeriod): boolean {
  if (!p || p.status !== 'open') return false
  const now = Date.now()
  return periodEndMs(p) < now && now <= periodDeadlineMs(p)
}

function periodStatusTag(status: string): string {
  if (status === 'expired') return 'danger'
  if (status === 'closed' || status === 'archived') return 'info'
  return 'success'
}

function reportStatusLabel(status: string): string {
  if (status === 'draft') return '草稿'
  if (status === 'submitted') return '待审核'
  if (status === 'reviewed') return '已审核'
  if (status === 'archived') return '已归档'
  return status
}

function versionStatusLabel(status: string): string {
  if (status === 'draft') return '草稿'
  if (status === 'submitted') return '待审核'
  if (status === 'reviewed') return '已通过'
  if (status === 'rejected') return '未通过'
  if (status === 'invalidated') return '已失效'
  return status
}

function versionStatusTag(status: string): string {
  if (status === 'reviewed') return 'success'
  if (status === 'submitted') return 'warning'
  if (status === 'rejected' || status === 'invalidated') return 'danger'
  return 'info'
}

function parseStatusLabel(status?: string): string {
  if (status === 'ready') return '解析成功'
  if (status === 'failed') return '解析失败'
  if (status === 'processing') return '解析中'
  return '未解析'
}

function qualityStatusLabel(status?: string): string {
  if (status === 'ready') return '质检通过'
  if (status === 'failed') return '质检失败'
  return '未质检'
}

function kindLabel(kind: string): string {
  if (kind === 'outcomes') return '成果'
  if (kind === 'issues') return '问题'
  if (kind === 'next_plan') return '下月计划'
  return '其它'
}

function findingKindLabel(kind: string): string {
  if (kind === 'missing_content') return '内容缺失'
  if (kind === 'vague_expression') return '表述笼统'
  if (kind === 'possible_plan_as_outcome') return '计划误作成果'
  return kind
}

async function loadPeriods() {
  try {
    const res = await reportApi.listPeriods()
    periods.value = res.data || []
    periods.value.sort((a, b) => (b.period_key || '').localeCompare(a.period_key || ''))
    if (!selectedPeriodId.value && periods.value.length > 0) {
      selectedPeriodId.value = periods.value[0].id
    }
  } catch {
    console.error('Failed to load periods')
  }
}

async function loadMembership() {
  if (!selectedPeriodId.value) return
  try {
    const res = await reportApi.isPeriodMember(selectedPeriodId.value)
    isCurrentPeriodMember.value = !!res.data?.is_member
  } catch {
    // Fail open only for an unconfigured roster; the backend still enforces the
    // real membership check on create, so this only affects the button visibility.
    isCurrentPeriodMember.value = false
  }
}

const canCreateReport = computed(() => {
  if (myReport.value) return false
  if (!selectedPeriod.value) return false
  if (isPeriodLocked(selectedPeriod.value)) return false
  // A period with no configured members isn't roster-gated, so anyone may draft.
  if ((selectedPeriod.value.member_count ?? 0) === 0) return true
  return isCurrentPeriodMember.value
})

async function loadMembers() {
  try {
    const res = await memberApi.list()
    const list: Member[] = (res.data || []).filter((m: Member) => m.role !== 'super_admin')
    members.value = list

    const groups = new Map<string, Member[]>()
    for (const m of list) {
      const year = m.student_id?.slice(0, 4) || '未知'
      if (!groups.has(year)) groups.set(year, [])
      groups.get(year)!.push(m)
    }

    gradeGroups.value = Array.from(groups.entries())
      .sort((a, b) => {
        if (a[0] === '未知') return 1
        if (b[0] === '未知') return -1
        return b[0].localeCompare(a[0])
      })
      .map(([year, group]) => ({
        year,
        label: year === '未知' ? '未知' : `${year}级`,
        members: group
      }))
  } catch {
    members.value = []
    gradeGroups.value = []
  }
}

async function loadRoster() {
  if (!selectedPeriodId.value) return
  try {
    const res = await reportApi.listRoster(selectedPeriodId.value)
    roster.value = res.data || []
  } catch {
    roster.value = []
  }
}

async function loadCurrentVersion() {
  if (!myReport.value || myReport.value.current_version < 1) {
    currentVersion.value = null
    editorTitle.value = ''
    editorBody.value = ''
    return
  }
  try {
    const res = await reportApi.getVersion(myReport.value.id, myReport.value.current_version)
    currentVersion.value = res.data
    editorTitle.value = res.data.title || ''
    editorBody.value = res.data.body_markdown || ''
  } catch {
    console.error('Failed to load report version')
  }
}

async function loadReports() {
  if (!selectedPeriodId.value) return
  try {
    const res = await reportApi.listReports({ period_id: selectedPeriodId.value })
    reports.value = res.data || []
    myReport.value = reports.value.find(r => r.author_id === authStore.user?.id) || null
    await loadCurrentVersion()
  } catch {
    console.error('Failed to load reports')
  }
}

async function handlePeriodChange() {
  await loadMembership()
  await loadReports()
  if (isManager.value) await loadRoster()
}

async function handleCreateReport() {
  if (!selectedPeriodId.value) {
    ElMessage.warning('请先选择月份')
    return
  }
  try {
    await reportApi.createReport({ period_id: selectedPeriodId.value, report_type: 'monthly' })
    ElMessage.success('月报已创建，请撰写内容')
    await loadReports()
  } catch (err: any) {
    const msg = err?.response?.data?.message
    ElMessage.error(msg || '创建失败')
  }
}

async function handleSaveDraft() {
  if (!myReport.value) return
  if (!editorTitle.value.trim()) {
    ElMessage.warning('请填写标题')
    return
  }
  if (!editorBody.value.trim()) {
    ElMessage.warning('请填写正文')
    return
  }
  saving.value = true
  try {
    const res = await reportApi.createVersion(myReport.value.id, {
      expected_version: myReport.value.current_version,
      title: editorTitle.value.trim(),
      body_markdown: editorBody.value
    })
    currentVersion.value = res.data
    myReport.value.current_version = res.data.version
    editorBody.value = res.data.body_markdown
    ElMessage.success(`已生成版本 v${res.data.version}`)
    await loadReports()
  } catch {
    ElMessage.error('保存失败')
  } finally {
    saving.value = false
  }
}

async function handleCheckQuality() {
  if (!myReport.value || myReport.value.current_version < 1) {
    ElMessage.warning('请先保存一个版本')
    return
  }
  checking.value = true
  try {
    const res = await reportApi.checkQuality(myReport.value.id, myReport.value.current_version)
    currentVersion.value = res.data
    ElMessage.success('质检完成')
  } catch {
    ElMessage.error('质检失败')
  } finally {
    checking.value = false
  }
}

async function handleSubmit() {
  if (!myReport.value || myReport.value.current_version < 1) {
    ElMessage.warning('请先保存一个版本')
    return
  }
  submitting.value = true
  try {
    await reportApi.submitVersion(myReport.value.id, myReport.value.current_version)
    ElMessage.success('已提交，等待队长审核')
    await loadReports()
  } catch {
    ElMessage.error('提交失败')
  } finally {
    submitting.value = false
  }
}

async function handleWithdraw() {
  if (!myReport.value || myReport.value.current_version < 1) {
    ElMessage.warning('已无可撤回的提交')
    return
  }
  withdrawing.value = true
  try {
    const res = await reportApi.withdrawVersion(myReport.value.id, myReport.value.current_version)
    myReport.value.status = 'draft'
    currentVersion.value = res.data
    ElMessage.success('已撤回，可修改后重新提交')
  } catch {
    ElMessage.error('撤回失败')
  } finally {
    withdrawing.value = false
  }
}

async function openReview(report: Report) {
  try {
    const res = await reportApi.getSubmittedVersion(report.id)
    reviewTarget.value = { report, version: res.data }
    showReviewDialog.value = true
  } catch {
    ElMessage.error('加载报告失败')
  }
}

async function handleReview(approved: boolean) {
  if (!reviewTarget.value) return
  reviewing.value = true
  try {
    await reportApi.reviewVersion(reviewTarget.value.report.id, reviewTarget.value.version.version, approved)
    ElMessage.success(approved ? '已通过' : '已退回修改')
    showReviewDialog.value = false
    await loadReports()
    await loadRoster()
  } catch {
    ElMessage.error('操作失败')
  } finally {
    reviewing.value = false
  }
}

function formatTime(value?: string): string {
  if (!value) return ''
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

async function openRosterPreview(entry: RosterEntry) {
  if (!entry.report_id) {
    ElMessage.warning('该成员还未提交')
    return
  }
  const report = reports.value.find(r => r.id === entry.report_id)
  if (!report) {
    ElMessage.warning('未找到对应报告')
    return
  }
  try {
    const res = await reportApi.getVersion(report.id, report.current_version)
    rosterPreview.value = { report, version: res.data }
    showRosterPreview.value = true
  } catch {
    ElMessage.error('加载报告失败')
  }
}

const periodKeyPattern = /^\d{4}-(0[1-9]|1[0-2])$/

watch(() => createPeriodForm.starts_on, (v) => {
  if (v && !createPeriodForm.period_key) {
    createPeriodForm.period_key = (v as string).slice(0, 7)
  }
})

async function handleCreatePeriod() {
  const key = createPeriodForm.period_key.trim()
  if (!periodKeyPattern.test(key)) {
    ElMessage.warning('周期格式应为 YYYY-MM（如 2026-09，月份需两位数）')
    return
  }
  if (!createPeriodForm.starts_on || !createPeriodForm.ends_on) {
    ElMessage.warning('请选择开始和结束日期')
    return
  }
  if (createPeriodForm.starts_on > createPeriodForm.ends_on) {
    ElMessage.warning('开始日期不能晚于结束日期')
    return
  }
  if (!createPeriodForm.due_at) {
    ElMessage.warning('请选择提交截止时间')
    return
  }
  if (new Date(createPeriodForm.due_at).getTime() < new Date(createPeriodForm.ends_on).getTime()) {
    ElMessage.warning('提交截止时间不能早于结束日期')
    return
  }
  try {
    await reportApi.createPeriod({
      period_key: key,
      starts_on: createPeriodForm.starts_on,
      ends_on: createPeriodForm.ends_on,
      timezone: 'Asia/Shanghai',
      due_at: createPeriodForm.due_at
    })
    ElMessage.success('周期已创建')
    showCreatePeriod.value = false
    createPeriodForm.period_key = ''
    createPeriodForm.starts_on = ''
    createPeriodForm.ends_on = ''
    createPeriodForm.due_at = ''
    await loadPeriods()
  } catch (err: any) {
    const msg = err?.response?.data?.message
    ElMessage.error(msg || '创建失败')
  }
}

function inRoster(userId: number): boolean {
  return roster.value.some(r => r.user_id === userId)
}

function toggleMember(userId: number, checked: boolean) {
  const idx = selectedUserIds.value.indexOf(userId)
  if (checked && idx === -1) selectedUserIds.value.push(userId)
  else if (!checked && idx !== -1) selectedUserIds.value.splice(idx, 1)
}

function groupSelectableMembers(g: GradeGroup): Member[] {
  return g.members.filter(m => !inRoster(m.id))
}

function groupAllSelected(g: GradeGroup): boolean {
  const sel = groupSelectableMembers(g)
  return sel.length > 0 && sel.every(m => selectedUserIds.value.includes(m.id))
}

function groupIndeterminate(g: GradeGroup): boolean {
  const sel = groupSelectableMembers(g)
  const count = sel.filter(m => selectedUserIds.value.includes(m.id)).length
  return count > 0 && count < sel.length
}

function toggleGroup(g: GradeGroup, checked: boolean) {
  for (const m of groupSelectableMembers(g)) toggleMember(m.id, checked)
}

function onGroupToggle(g: GradeGroup, val: unknown) {
  toggleGroup(g, val === true)
}

function onMemberToggle(m: Member, val: unknown) {
  toggleMember(m.id, val === true)
}

function openAddMember() {
  selectedUserIds.value = []
  showAddMember.value = true
}

async function handleAddMember() {
  if (!selectedPeriodId.value) {
    ElMessage.warning('请先选择月份')
    return
  }
  if (selectedUserIds.value.length === 0) {
    ElMessage.warning('请选择成员')
    return
  }
  const existing = new Set(roster.value.map(r => r.user_id))
  const toAdd = selectedUserIds.value.filter(id => !existing.has(id))
  if (toAdd.length === 0) {
    ElMessage.info('所选成员已在提交名单中')
    return
  }
  addingMembers.value = true
  let added = 0
  let failed = 0
  for (const id of toAdd) {
    const member = members.value.find(m => m.id === id)
    try {
      await reportApi.addPeriodMember(selectedPeriodId.value, {
        user_id: id,
        display_name: member?.nickname || '',
        required: true,
        exempt: false
      })
      added++
    } catch {
      failed++
    }
  }
  addingMembers.value = false
  showAddMember.value = false
  selectedUserIds.value = []
  await loadRoster()
  await loadPeriods()
  if (failed > 0) ElMessage.warning(`成功添加 ${added} 人，另有 ${failed} 人失败`)
  else ElMessage.success(`已加入提交名单 ${added} 人`)
}

const selectedPeriod = computed(() => periods.value.find(p => p.id === selectedPeriodId.value))
</script>

<template>
  <div class="report">
    <div class="card">
      <div class="card-header">
        <h3>月报周期</h3>
        <div class="actions">
          <el-select v-model="selectedPeriodId" placeholder="选择月份" @change="handlePeriodChange" style="width: 240px">
            <el-option v-for="p in periods" :key="p.id" :value="p.id" :label="`${p.period_key} · ${periodStatusLabel(periodEffectiveStatus(p))}`" />
          </el-select>
          <el-button v-if="isManager" type="primary" @click="showCreatePeriod = true">新建周期</el-button>
        </div>
      </div>
      <div v-if="selectedPeriod" class="period-meta">
        <span>{{ selectedPeriod.starts_on }} ~ {{ selectedPeriod.ends_on }}</span>
        <el-tag size="small" :type="periodStatusTag(periodEffectiveStatus(selectedPeriod))">{{ periodStatusLabel(periodEffectiveStatus(selectedPeriod)) }}</el-tag>
        <span v-if="selectedPeriod.due_at">截止 {{ formatDeadline(selectedPeriod.due_at) }}</span>
        <span v-if="selectedPeriod.member_count">应提交 {{ selectedPeriod.member_count }} 人</span>
      </div>
    </div>

    <div v-if="isManager && selectedPeriod" class="report-progress" aria-label="月报提交进度">
      <div><span>应提交</span><strong>{{ roster.length }} <small>人</small></strong></div>
      <div><span>已提交</span><strong>{{ submittedCount }} <small>人</small></strong></div>
      <div><span>待审核</span><strong>{{ submittedReports.length }} <small>份</small></strong></div>
      <div><span>{{ periodLocked ? '截止未提交' : '待提交' }}</span><strong>{{ Math.max(0, roster.length - submittedCount) }} <small>人</small></strong></div>
    </div>
    <div class="card report-mine">
      <div class="card-header">
        <h3>我的月报</h3>
        <div class="actions">
          <el-button v-if="canCreateReport" type="primary" @click="handleCreateReport">创建月报</el-button>
          <template v-else-if="myReport">
            <el-tag size="small" :type="myReport.status === 'reviewed' ? 'success' : myReport.status === 'submitted' ? 'warning' : 'info'">
              {{ reportStatusLabel(myReport.status) }}
            </el-tag>
            <el-tag v-if="myReport.late_submission" size="small" type="warning">逾期提交</el-tag>
            <template v-if="myReport.status === 'submitted'">
              <el-button size="small" type="warning" :loading="withdrawing" @click="handleWithdraw">撤回提交</el-button>
            </template>
            <template v-else-if="myReport.status === 'draft'">
              <el-button size="small" :loading="checking" :disabled="periodLocked" @click="handleCheckQuality">检查质量</el-button>
              <el-button size="small" type="success" :loading="saving" :disabled="periodLocked" @click="handleSaveDraft">保存版本</el-button>
              <el-button size="small" type="primary" :loading="submitting" :disabled="periodLocked" @click="handleSubmit">提审</el-button>
            </template>
          </template>
        </div>
      </div>

      <template v-if="myReport">
        <div v-if="currentVersion" class="version-meta">
          <span>当前版本 v{{ currentVersion.version }}</span>
          <el-tag size="small" :type="versionStatusTag(currentVersion.status)">{{ versionStatusLabel(currentVersion.status) }}</el-tag>
          <el-tag size="small" type="info">{{ parseStatusLabel(currentVersion.parse_status) }}</el-tag>
          <el-tag size="small" :type="currentVersion.quality_status === 'ready' ? 'success' : 'info'">{{ qualityStatusLabel(currentVersion.quality_status) }}</el-tag>
        </div>

        <div v-if="periodLocked" class="editor-lock lock-danger">
          提交时间已过，无法提交，编辑区已锁定
        </div>
        <div v-else-if="periodLateWindow && myReport" class="editor-lock lock-warning">
          本周期已截止，将记为逾期提交
        </div>
        <div v-else-if="myReport && myReport.status === 'submitted'" class="editor-lock">
          已提交，等待队长审核；如需修改请点击「撤回提交」，修改后可重新提审。
        </div>
        <div v-else-if="isLocked" class="editor-lock">
          该月报已审核，内容已锁定
        </div>

        <div class="editor-header">
          <el-input v-model="editorTitle" placeholder="请输入本期标题" style="max-width: 480px" :disabled="isLocked || periodLocked" />
          <el-button size="small" @click="showPreview = !showPreview">{{ showPreview ? '编辑' : '预览' }}</el-button>
        </div>

        <el-input
          v-if="!showPreview"
          v-model="editorBody"
          type="textarea"
          :rows="10"
          :disabled="isLocked || periodLocked"
          placeholder="请按 成果 / 问题 / 下月计划 三部分撰写月报正文（支持 Markdown），例：&#10;&#10;## 本月成果&#10;- 完成了...&#10;&#10;## 问题&#10;- 遇到了...&#10;&#10;## 下月计划&#10;- 计划..." />
        <div v-else class="markdown-preview" v-html="renderMarkdown(editorBody)"></div>

        <template v-if="currentVersion">
          <div v-if="parsedSections.length" class="parsed-panel">
            <div class="panel-title">解析结构</div>
            <div v-for="s in parsedSections" :key="s.kind + s.start_line" class="parsed-section">
              <div class="parsed-heading">{{ kindLabel(s.kind) }} · {{ s.heading }}</div>
              <div class="parsed-body">{{ s.content || '（空）' }}</div>
            </div>
          </div>

          <div v-if="qualityFindings.length" class="finding-panel">
            <div class="panel-title">质检意见</div>
            <div v-for="f in qualityFindings" :key="f.index" class="finding">
              <el-tag size="small" type="warning">{{ findingKindLabel(f.kind) }}</el-tag>
              <span class="finding-msg">{{ f.message }}</span>
              <span class="finding-suggestion">{{ f.suggestion }}</span>
            </div>
          </div>

          <div v-if="currentVersion.status === 'rejected'" class="reject-note">
            该版本未通过审核，请根据质检意见修改后保存新版本并重新提审。
          </div>
        </template>
      </template>
      <p v-else class="compact-empty" role="status">{{ firstDraftHint }}</p>
    </div>

    <template v-if="isManager">
      <div class="card">
        <div class="card-header">
          <h3>审批管理</h3>
          <span class="pending-count" v-if="submittedReports.length">{{ submittedReports.length }} 条待审核</span>
        </div>
        <div v-if="submittedReports.length" class="review-list">
          <div v-for="r in submittedReports" :key="r.id" class="review-row">
            <div class="review-info">
              <span class="review-name">{{ r.author_name }}</span>
              <span class="review-meta">{{ reportStatusLabel(r.status) }}</span>
              <el-tag v-if="r.late_submission" size="small" type="warning">逾期</el-tag>
            </div>
            <div class="review-actions">
              <el-button size="small" @click="openReview(r)">查看</el-button>
            </div>
          </div>
        </div>
        <p v-else class="compact-empty" role="status">当前没有待审核月报</p>
      </div>

      <div class="card">
        <div class="card-header">
          <h3>提交名单</h3>
          <el-button size="small" type="primary" @click="openAddMember">添加成员</el-button>
        </div>
        <div v-if="roster.length" class="roster-list">
          <div class="roster-count">需提交 {{ roster.length }} 人 · 已提交 {{ submittedCount }} 人</div>
          <div v-for="r in roster" :key="r.user_id" class="roster-row">
            <span class="roster-name">{{ r.display_name }}</span>
            <template v-if="r.submitted_at">
              <span class="roster-time">已提交 {{ formatTime(r.submitted_at) }}</span>
              <el-button size="small" link type="primary" @click="openRosterPreview(r)">预览</el-button>
            </template>
            <span v-else class="roster-time">未提交</span>
          </div>
        </div>
        <el-empty v-else description="尚未配置提交成员，请点击添加" :image-size="48" />
      </div>
    </template>

    <el-dialog v-model="showCreatePeriod" title="新建月报周期" width="520px">
      <el-form label-width="100px">
        <el-form-item label="周期">
          <el-input v-model="createPeriodForm.period_key" placeholder="2026-09" />
        </el-form-item>
        <el-form-item label="开始日期">
          <el-date-picker v-model="createPeriodForm.starts_on" type="date" value-format="YYYY-MM-DD" placeholder="开始日期" style="width: 100%" />
        </el-form-item>
        <el-form-item label="结束日期">
          <el-date-picker v-model="createPeriodForm.ends_on" type="date" value-format="YYYY-MM-DD" placeholder="结束日期" style="width: 100%" />
        </el-form-item>
        <el-form-item label="提交截止">
          <el-date-picker v-model="createPeriodForm.due_at" type="datetime" value-format="YYYY-MM-DDTHH:mm:ssZ" placeholder="提交截止时间（必填）" style="width: 100%" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCreatePeriod = false">取消</el-button>
        <el-button type="primary" @click="handleCreatePeriod">创建</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="showAddMember" title="添加提交成员" width="520px">
      <p class="notify-desc">按年级勾选要加入提交名单的成员，已加入的成员不可重复选择：</p>
      <div v-if="gradeGroups.length" class="notify-members">
        <div v-for="g in gradeGroups" :key="g.year" class="grade-group">
          <el-checkbox
            :model-value="groupAllSelected(g)"
            :indeterminate="groupIndeterminate(g)"
            :disabled="groupSelectableMembers(g).length === 0"
            @change="onGroupToggle(g, $event)"
          >{{ g.label }}</el-checkbox>
          <div class="grade-members">
            <el-checkbox
              v-for="m in g.members"
              :key="m.id"
              :model-value="selectedUserIds.includes(m.id)"
              :disabled="inRoster(m.id)"
              @change="onMemberToggle(m, $event)"
            >{{ m.nickname }}</el-checkbox>
          </div>
        </div>
      </div>
      <el-empty v-else description="暂无成员可选" :image-size="48" />
      <template #footer>
        <el-button @click="showAddMember = false">取消</el-button>
        <el-button type="primary" :loading="addingMembers" @click="handleAddMember">添加</el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="showReviewDialog"
      :title="reviewTarget ? `${reviewTarget.report.author_name} · 月报` : '月报详情'"
      width="720px"
      @closed="reviewTarget = null"
    >
      <template v-if="reviewTarget">
        <div class="version-meta">
          <span>v{{ reviewTarget.version.version }}</span>
          <el-tag size="small" :type="versionStatusTag(reviewTarget.version.status)">{{ versionStatusLabel(reviewTarget.version.status) }}</el-tag>
          <el-tag size="small" type="info">{{ parseStatusLabel(reviewTarget.version.parse_status) }}</el-tag>
          <el-tag size="small" :type="reviewTarget.version.quality_status === 'ready' ? 'success' : 'info'">{{ qualityStatusLabel(reviewTarget.version.quality_status) }}</el-tag>
          <el-tag v-if="reviewTarget.report.late_submission" size="small" type="warning">逾期提交</el-tag>
        </div>

        <div class="review-body" v-html="renderMarkdown(reviewTarget.version.body_markdown)"></div>

        <div v-if="reviewSections.length" class="parsed-panel">
          <div class="panel-title">解析结构</div>
          <div v-for="s in reviewSections" :key="s.kind + s.start_line" class="parsed-section">
            <div class="parsed-heading">{{ kindLabel(s.kind) }} · {{ s.heading }}</div>
            <div class="parsed-body">{{ s.content || '（空）' }}</div>
          </div>
        </div>

        <div v-if="reviewFindings.length" class="finding-panel">
          <div class="panel-title">质检意见</div>
          <div v-for="f in reviewFindings" :key="f.index" class="finding">
            <el-tag size="small" type="warning">{{ findingKindLabel(f.kind) }}</el-tag>
            <span class="finding-msg">{{ f.message }}</span>
            <span class="finding-suggestion">{{ f.suggestion }}</span>
          </div>
        </div>
      </template>
      <template #footer>
        <el-button @click="showReviewDialog = false">关闭</el-button>
        <el-button type="success" :loading="reviewing" @click="handleReview(true)">通过</el-button>
        <el-button type="danger" :loading="reviewing" @click="handleReview(false)">退回</el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="showRosterPreview"
      :title="rosterPreview ? `${rosterPreview.report.author_name} · 月报` : '月报预览'"
      width="720px"
      @closed="rosterPreview = null"
    >
      <template v-if="rosterPreview">
        <div class="version-meta">
          <span>v{{ rosterPreview.version.version }}</span>
          <el-tag size="small" :type="versionStatusTag(rosterPreview.version.status)">{{ versionStatusLabel(rosterPreview.version.status) }}</el-tag>
          <el-tag size="small" type="info">{{ parseStatusLabel(rosterPreview.version.parse_status) }}</el-tag>
          <el-tag size="small" :type="rosterPreview.version.quality_status === 'ready' ? 'success' : 'info'">{{ qualityStatusLabel(rosterPreview.version.quality_status) }}</el-tag>
          <el-tag v-if="rosterPreview.report.late_submission" size="small" type="warning">逾期提交</el-tag>
        </div>
        <div class="review-body" v-html="renderMarkdown(rosterPreview.version.body_markdown)"></div>
      </template>
      <template #footer>
        <el-button @click="showRosterPreview = false">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.report-progress { display:grid; grid-template-columns:repeat(4,1fr); gap:12px; }
.report-progress > div { background:var(--bg-card); border:1px solid var(--border); border-radius:12px; padding:16px 20px; }
.report-progress span { display:block; color:var(--text-muted); font-size:12px; }
.report-progress strong { display:block; font-size:26px; font-variant-numeric:tabular-nums; margin-top:6px; }
.report-progress small { font-size:12px; font-weight:400; }
.compact-empty { padding:16px 0; color:var(--text-muted); }
@media(max-width:600px) { .report-progress { grid-template-columns:1fr 1fr; } }

.report {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.card {
  background: var(--bg-card);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-sm);
  padding: 1.25rem;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1rem;
}

.card-header h3 {
  font-size: 0.9375rem;
  font-weight: 600;
  letter-spacing: -0.01em;
  color: var(--text);
}

.actions {
  display: flex;
  gap: 0.5rem;
  align-items: center;
}

.period-meta {
  display: flex;
  gap: 1rem;
  font-size: 0.8rem;
  color: var(--text-muted);
}

.version-meta {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 0.75rem;
  font-size: 0.8rem;
  color: var(--text-muted);
}

.editor-header {
  display: flex;
  gap: 0.75rem;
  align-items: center;
  margin-bottom: 0.75rem;
}

.markdown-preview {
  line-height: 1.7;
  color: var(--text);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 1rem;
  background: var(--bg-deep);
  margin-bottom: 0.75rem;
}

.markdown-preview :deep(h1),
.markdown-preview :deep(h2),
.markdown-preview :deep(h3) {
  margin-top: 1em;
  margin-bottom: 0.5em;
  font-weight: 600;
}

.markdown-preview :deep(p) {
  margin-bottom: 0.75em;
}

.markdown-preview :deep(code) {
  background: var(--bg-deep);
  padding: 0.125em 0.375em;
  border-radius: 0.25rem;
  font-size: 0.875em;
  font-family: 'Courier New', monospace;
}

.markdown-preview :deep(pre) {
  background: var(--bg-card);
  padding: 1rem;
  border-radius: var(--radius);
  overflow-x: auto;
  margin-bottom: 1em;
}

.markdown-preview :deep(pre code) {
  background: none;
  padding: 0;
}

.markdown-preview :deep(ul),
.markdown-preview :deep(ol) {
  padding-left: 1.5em;
  margin-bottom: 0.75em;
}

.markdown-preview :deep(blockquote) {
  border-left: 3px solid var(--accent);
  padding-left: 1em;
  color: var(--text-secondary);
  margin-bottom: 0.75em;
}

.markdown-preview :deep(a) {
  color: var(--accent);
  text-decoration: none;
}

.markdown-preview :deep(a:hover) {
  text-decoration: underline;
}

.markdown-preview :deep(table) {
  width: 100%;
  border-collapse: collapse;
  margin-bottom: 1em;
}

.markdown-preview :deep(th),
.markdown-preview :deep(td) {
  border: 1px solid var(--border);
  padding: 0.5rem;
  text-align: left;
}

.markdown-preview :deep(th) {
  background: var(--bg-deep);
  font-weight: 600;
}

.parsed-panel,
.finding-panel {
  margin-top: 1rem;
  border-top: 1px solid var(--border);
  padding-top: 1rem;
}

.panel-title {
  font-size: 0.8rem;
  font-weight: 600;
  color: var(--text-secondary);
  margin-bottom: 0.5rem;
}

.parsed-section {
  margin-bottom: 0.5rem;
}

.parsed-heading {
  font-size: 0.875rem;
  font-weight: 600;
  color: var(--text);
}

.parsed-body {
  font-size: 0.8rem;
  color: var(--text-secondary);
  white-space: pre-wrap;
}

.finding {
  display: flex;
  align-items: flex-start;
  gap: 0.5rem;
  padding: 0.5rem 0;
  font-size: 0.8rem;
}

.finding-msg {
  color: var(--text);
}

.finding-suggestion {
  color: var(--text-muted);
}

.reject-note {
  margin-top: 0.75rem;
  padding: 0.5rem;
  background: var(--danger-bg);
  border-radius: 4px;
  font-size: 0.8rem;
  color: var(--danger);
}

.editor-lock {
  margin-bottom: 0.75rem;
  padding: 0.5rem;
  background: var(--accent-bg);
  border-radius: 4px;
  font-size: 0.8rem;
  color: var(--accent);
}

.lock-danger {
  background: var(--danger-bg);
  color: var(--danger);
}

.lock-warning {
  background: var(--warning-bg);
  color: var(--warning);
}

.pending-count {
  color: var(--text-muted);
  font-size: 0.875rem;
}

.review-list {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.review-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.75rem;
  background: var(--bg-deep);
  border: 1px solid var(--border);
  border-radius: var(--radius);
}

.review-info {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.review-name {
  font-weight: 500;
  color: var(--text);
}

.review-meta {
  font-size: 0.75rem;
  color: var(--text-muted);
}

.report-mine {
  padding: 1rem;
}

.notify-desc {
  font-size: 0.85rem;
  color: var(--text-secondary);
  margin-bottom: 1rem;
}

.notify-members {
  max-height: 45vh;
  overflow-y: auto;
  padding-right: 0.25rem;
}

.grade-group {
  margin-bottom: 1rem;
}

.grade-group :deep(.el-checkbox) {
  --el-checkbox-checked-bg-color: var(--accent);
  --el-checkbox-checked-input-border-color: var(--accent);
}

.grade-members {
  padding-left: 1.25rem;
  margin-top: 0.25rem;
  display: flex;
  flex-wrap: wrap;
  gap: 0.25rem 1rem;
}

.roster-list {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.roster-count {
  font-size: 0.8rem;
  color: var(--text-muted);
  margin-bottom: 0.25rem;
}

.roster-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.5rem 0.75rem;
  background: var(--bg-deep);
  border: 1px solid var(--border);
  border-radius: var(--radius);
}

.roster-name {
  font-weight: 500;
  color: var(--text);
}

.roster-time {
  font-size: 0.75rem;
  color: var(--text-muted);
}

.review-body {
  line-height: 1.7;
  color: var(--text);
  border-top: 1px solid var(--border);
  padding-top: 1rem;
}
@media (max-width: 768px) {
  .card { padding: 0.875rem; }
  .card-header, .editor-header { flex-wrap: wrap; gap: 0.5rem; }
  .actions { flex-wrap: wrap; }
  .period-meta { flex-wrap: wrap; gap: 0.5rem; }
  .review-row, .roster-row { flex-direction: column; align-items: flex-start; gap: 0.5rem; }
}
</style>
