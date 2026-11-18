<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { announcementApi } from '@/api/announcement'
import { announcementDraftApi, type AnnouncementDraft } from '@/api/announcementDraft'
import { memberApi } from '@/api/member'
import { useAuthStore } from '@/stores/auth'
import { ElMessage, ElMessageBox } from 'element-plus'
import { renderMarkdown } from '@/utils/markdown'
import { usePaginationLayout } from '@/composables/usePaginationLayout'

const loading = ref(true)
const loadError = ref('')
const authStore = useAuthStore()
const draftsLoading = ref(true)
const draftsError = ref('')
const drafts = ref<AnnouncementDraft[]>([])
const announcements = ref<Array<{
  id: number
  title: string
  content: string
  author_name: string
  pinned: boolean
  created_at: string
  updated_at: string
}>>([])

const showDialog = ref(false)
const editingId = ref<number | null>(null)
const editingDraftId = ref<number | null>(null)
const form = ref({ title: '', content: '', pinned: false })

const showDetail = ref(false)
const detailAnn = ref<typeof announcements.value[0] | null>(null)

// Notify
const showNotifyDialog = ref(false)
const notifyAnnId = ref<number | null>(null)
const notifyLoading = ref(false)
const selectedUserIds = ref<number[]>([])

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
  selected: boolean
}

const allMembers = ref<Member[]>([])
const gradeGroups = ref<GradeGroup[]>([])

const currentPage = ref(1)
const pageSize = ref(3)
const { layout: paginationLayout, pagerCount, small: paginationSmall } = usePaginationLayout()

const paginatedAnnouncements = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value
  return announcements.value.slice(start, start + pageSize.value)
})

onMounted(async () => {
  await Promise.all([loadAnnouncements(), ...(authStore.isCaptain || authStore.isSuperAdmin ? [loadDrafts()] : [])])
})

async function loadAnnouncements() {
  loading.value = true
  loadError.value = ''
  try {
    const res = await announcementApi.list()
    announcements.value = res.data
  } catch {
    loadError.value = '内容加载失败，请检查网络后重试'
  } finally {
    loading.value = false
  }
}

async function loadDrafts() {
  draftsLoading.value = true
  draftsError.value = ''
  try {
    const res = await announcementDraftApi.list()
    drafts.value = res.data || []
  } catch {
    draftsError.value = '草稿加载失败'
  } finally {
    draftsLoading.value = false
  }
}

function openCreate() {
  editingId.value = null
  editingDraftId.value = null
  form.value = { title: '', content: '', pinned: false }
  showDialog.value = true
}

function openEdit(ann: typeof announcements.value[0]) {
  editingId.value = ann.id
  editingDraftId.value = null
  form.value = { title: ann.title, content: ann.content, pinned: ann.pinned }
  showDialog.value = true
}

function openDraftEdit(draft: AnnouncementDraft) {
  editingId.value = null
  editingDraftId.value = draft.id
  form.value = { title: draft.title, content: draft.content, pinned: draft.pinned }
  showDialog.value = true
}

function openDetail(ann: typeof announcements.value[0]) {
  detailAnn.value = ann
  showDetail.value = true
}

async function openNotify(ann: typeof announcements.value[0]) {
  notifyAnnId.value = ann.id
  selectedUserIds.value = []
  gradeGroups.value.forEach(g => g.selected = false)
  await loadMembers()
  showNotifyDialog.value = true
}

async function loadMembers() {
  try {
    const res = await memberApi.list()
    const members: Member[] = (res.data || res || []).filter((m: Member) => m.role !== 'super_admin')
    allMembers.value = members

    // Group by enrollment year
    const groups = new Map<string, Member[]>()
    for (const m of members) {
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
      .map(([year, members]) => ({
        year,
        label: year === '未知' ? '未知' : `${year}级`,
        members,
        selected: false
      }))
  } catch {
    allMembers.value = []
    gradeGroups.value = []
  }
}

function handleGroupToggle(group: GradeGroup) {
  for (const m of group.members) {
    const idx = selectedUserIds.value.indexOf(m.id)
    if (group.selected && idx === -1) {
      selectedUserIds.value.push(m.id)
    } else if (!group.selected && idx !== -1) {
      selectedUserIds.value.splice(idx, 1)
    }
  }
}

function handleMemberToggle(group: GradeGroup) {
  group.selected = group.members.every(m => selectedUserIds.value.includes(m.id))
}

async function handleSubmit() {
  try {
    if (editingDraftId.value) {
      await announcementDraftApi.update(editingDraftId.value, form.value.title, form.value.content, form.value.pinned)
      ElMessage.success('草稿已保存')
      showDialog.value = false
      await loadDrafts()
    } else if (editingId.value) {
      await announcementApi.update(editingId.value, form.value.title, form.value.content, form.value.pinned)
      ElMessage.success('更新成功')
      showDialog.value = false
      await loadAnnouncements()
    } else {
      const res = await announcementApi.create(form.value.title, form.value.content, form.value.pinned)
      ElMessage.success('创建成功')
      showDialog.value = false
      await loadAnnouncements()

      // Show notify dialog for new announcements
      const annId = res.data?.id
      if (annId) {
        notifyAnnId.value = annId
        selectedUserIds.value = []
        await loadMembers()
        showNotifyDialog.value = true
      }
    }
  } catch {
    ElMessage.error('操作失败')
  }
}

async function confirmDraft(draft: AnnouncementDraft) {
  try {
    await ElMessageBox.confirm('确认后仍需再次点击发布，确认保存这份公告草稿吗？', '确认草稿', { type: 'warning' })
    await announcementDraftApi.confirm(draft.id)
    ElMessage.success('草稿已确认，可发布')
    await loadDrafts()
  } catch {
    // Cancelled or failed; the dialog keeps the page quiet.
  }
}

async function publishDraft(draft: AnnouncementDraft) {
  try {
    await ElMessageBox.confirm('发布后将生成正式公告，是否继续？', '发布公告', { type: 'warning' })
    await announcementDraftApi.publish(draft.id)
    ElMessage.success('公告已发布')
    await Promise.all([loadDrafts(), loadAnnouncements()])
  } catch {
    // Cancelled or failed.
  }
}

async function discardDraft(draft: AnnouncementDraft) {
  try {
    await ElMessageBox.confirm('确定放弃这份公告草稿吗？', '放弃草稿', { type: 'warning' })
  } catch {
    return
  }
  try {
    await announcementDraftApi.discard(draft.id)
    ElMessage.success('草稿已放弃')
    await loadDrafts()
  } catch {
    ElMessage.error('放弃失败，草稿可能已发布或状态已变更')
  }
}

async function handleNotify() {
  if (!notifyAnnId.value || selectedUserIds.value.length === 0) {
    ElMessage.warning('请至少选择一位成员')
    return
  }

  notifyLoading.value = true
  try {
    const res = await announcementApi.notify(notifyAnnId.value, selectedUserIds.value)
    console.log('[Notify] response:', res)
    const { sent = 0, failed = [] } = res.data || {}
    showNotifyDialog.value = false

    if (!failed || failed.length === 0) {
      ElMessage.success(`已成功通知 ${sent} 位成员`)
    } else {
      const failList = failed.map((f: { nickname: string; reason: string }) => `• ${f.nickname}（${f.reason}）`).join('\n')
      ElMessageBox.alert(
        `成功 ${sent} 人，失败 ${failed.length} 人：\n\n${failList}`,
        '通知发送结果',
        { type: 'warning', confirmButtonText: '知道了' }
      )
    }
  } catch (err) {
    console.error('[Notify] error:', err)
    ElMessage.error('通知发送失败')
  } finally {
    notifyLoading.value = false
  }
}

async function handleDelete(id: number) {
  try {
    await ElMessageBox.confirm('确定要删除这条公告吗？', '提示', { type: 'warning' })
    await announcementApi.delete(id)
    ElMessage.success('删除成功')
    await loadAnnouncements()
  } catch {
    // Cancelled
  }
}
function announcementSummary(content: string, title: string) {
  return content.split('\n').filter(line => line.replace(/^#+\s*/, '').trim() !== title.trim()).join(' ')
    .replace(/!\[[^\]]*\]\([^)]*\)/g, '').replace(/\[([^\]]+)\]\([^)]*\)/g, '$1').replace(/[#*`>_~|]/g, '').trim().slice(0, 180) || '点击查看公告详情'
}
</script>

<template>
  <div class="announcements">
    <div class="card">
      <div class="card-header">
        <h3>团队公告</h3>
        <el-button v-if="authStore.isCaptain || authStore.isSuperAdmin" type="primary" @click="openCreate">
          <el-icon><Plus /></el-icon> 发布公告
        </el-button>
      </div>

      <section v-if="authStore.isCaptain || authStore.isSuperAdmin" class="draft-section" :class="{ 'is-empty': !draftsLoading && !draftsError && !drafts.length }">
        <div class="section-title">
          <span>待审核草稿</span>
          <el-tag v-if="!draftsLoading && !draftsError" size="small" type="info">{{ drafts.length }}</el-tag>
        </div>
        <ContentState :loading="draftsLoading" :error="draftsError" :has-content="drafts.length > 0" :rows="1" compact @retry="loadDrafts">
          <template #skeleton><div class="draft-loading skeleton-shell" aria-hidden="true"><span class="sk-block sk-meta"></span></div></template>
        <div v-if="drafts.length" class="draft-list">
          <div v-for="draft in drafts" :key="draft.id" class="draft-item">
            <div class="draft-main">
              <strong>{{ draft.title }}</strong>
              <span class="draft-time">更新于 {{ draft.updated_at }}</span>
            </div>
            <div class="draft-actions">
              <el-button size="small" :disabled="draft.status !== 'draft'" @click="openDraftEdit(draft)">编辑</el-button>
              <el-button size="small" type="success" :disabled="draft.status !== 'draft'" @click="confirmDraft(draft)">确认</el-button>
              <el-button size="small" type="primary" :disabled="draft.status !== 'confirmed'" @click="publishDraft(draft)">发布</el-button>
              <el-button size="small" type="danger" text @click="discardDraft(draft)">放弃</el-button>
            </div>
          </div>
        </div>
        <div v-else class="draft-empty">暂无待审核草稿</div>
        </ContentState>
      </section>

      <ContentState :loading="loading" :error="loadError" :has-content="announcements.length > 0" @retry="loadAnnouncements">
      <template #skeleton>
        <div class="announcements-list" aria-hidden="true">
          <article v-for="n in pageSize" :key="n" class="announcement-item skeleton-shell">
            <div class="ann-header"><div class="ann-title"><span class="sk-block sk-title"></span></div><div class="ann-meta"><span class="sk-block sk-meta"></span></div></div>
            <p class="ann-summary"><span class="sk-block sk-line"></span><span class="sk-block sk-line sk-line-short"></span></p>
            <div class="ann-footer"><span v-if="authStore.isCaptain || authStore.isSuperAdmin" class="sk-block sk-button"></span><span class="sk-block sk-button detail-btn"></span></div>
          </article>
        </div>
      <div class="pagination-wrapper" aria-hidden="true"><span class="sk-block sk-pagination"></span></div>
      </template>
      <div class="announcements-list">
        <div v-for="ann in paginatedAnnouncements" :key="ann.id" class="announcement-item">
          <div class="ann-header">
            <div class="ann-title">
              <el-tag v-if="ann.pinned" type="warning" size="small">置顶</el-tag>
              <span class="title-text">{{ ann.title }}</span>
            </div>
            <div class="ann-meta">
              <span>{{ ann.author_name }}</span>
              <span>{{ ann.created_at }}</span>
            </div>
          </div>
          <p class="ann-summary">{{ announcementSummary(ann.content, ann.title) }}</p>
          <div class="ann-footer">
            <el-dropdown v-if="authStore.isCaptain || authStore.isSuperAdmin" trigger="click">
              <el-button size="small" text>管理公告<el-icon><ArrowDown /></el-icon></el-button>
              <template #dropdown><el-dropdown-menu><el-dropdown-item @click="openEdit(ann)">编辑</el-dropdown-item><el-dropdown-item @click="openNotify(ann)">通知成员</el-dropdown-item><el-dropdown-item divided @click="handleDelete(ann.id)">删除公告</el-dropdown-item></el-dropdown-menu></template>
            </el-dropdown>
            <el-button class="detail-btn" size="small" type="primary" text @click="openDetail(ann)">
              查看详情
            </el-button>
          </div>
        </div>
        <el-empty v-if="announcements.length === 0" description="暂无公告" />
      </div>

      <div class="pagination-wrapper" v-if="announcements.length > 0">
        <el-pagination
          v-model:current-page="currentPage"
          v-model:page-size="pageSize"
          :page-sizes="[3, 5, 10, 20, 50]"
          :total="announcements.length"
          :layout="paginationLayout"
          :pager-count="pagerCount"
          :small="paginationSmall"
          background
        />
      </div>
      </ContentState>
    </div>

    <!-- Create / Edit Dialog -->
    <el-dialog align-center v-model="showDialog" :title="editingDraftId ? '编辑公告草稿' : (editingId ? '编辑公告' : '发布公告')" width="600px">
      <el-form :model="form" label-width="80px">
        <el-form-item label="标题">
          <el-input v-model="form.title" placeholder="请输入公告标题" />
        </el-form-item>
        <el-form-item label="内容">
          <el-input v-model="form.content" type="textarea" :rows="6" placeholder="支持 Markdown 格式" />
        </el-form-item>
        <el-form-item label="置顶">
          <el-switch v-model="form.pinned" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showDialog = false">取消</el-button>
        <el-button type="primary" @click="handleSubmit">确定</el-button>
      </template>
    </el-dialog>

    <!-- Detail Dialog -->
    <el-dialog align-center v-model="showDetail" :title="detailAnn?.title || '公告详情'" width="700px" class="detail-dialog">
      <div v-if="detailAnn" class="detail-meta">
        <span>{{ detailAnn.author_name }}</span>
        <span>{{ detailAnn.created_at }}</span>
        <el-tag v-if="detailAnn.pinned" type="warning" size="small">置顶</el-tag>
      </div>
      <div v-if="detailAnn" class="detail-content markdown-body" v-html="renderMarkdown(detailAnn.content)"></div>
    </el-dialog>

    <!-- Notify Dialog -->
    <el-dialog align-center v-model="showNotifyDialog" title="通知成员" width="500px">
      <p class="notify-desc">选择要发送邮件通知的成员：</p>
      <div class="notify-members">
        <div v-for="group in gradeGroups" :key="group.year" class="grade-group">
          <el-checkbox
            v-model="group.selected"
            @change="handleGroupToggle(group)"
            class="group-checkbox"
          >
            {{ group.label }}
          </el-checkbox>
          <div class="member-list">
            <el-checkbox
              v-for="m in group.members"
              :key="m.id"
              :model-value="selectedUserIds.includes(m.id)"
              @change="(val: boolean) => {
                if (val) selectedUserIds.push(m.id)
                else selectedUserIds.splice(selectedUserIds.indexOf(m.id), 1)
                handleMemberToggle(group)
              }"
              class="member-checkbox"
            >
              {{ m.nickname }}（{{ m.student_id }}）
            </el-checkbox>
          </div>
        </div>
      </div>
      <template #footer>
        <el-button @click="showNotifyDialog = false">跳过</el-button>
        <el-button type="primary" :loading="notifyLoading" @click="handleNotify">
          发送通知（{{ selectedUserIds.length }}人）
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.announcements {
  width: 100%;
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
  margin-bottom: 1.25rem;
}

.card-header h3 {
  font-size: 0.9375rem;
  font-weight: 600;
  letter-spacing: -0.01em;
  color: var(--text);
}

.draft-section {
  margin-bottom: 1rem;
  padding: 0.625rem 0.75rem;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--bg-deep);
}

.section-title {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 0.5rem;
  color: var(--text);
  font-size: 0.875rem;
  font-weight: 600;
}

.draft-list {
  display: flex;
  flex-direction: column;
  gap: 0.375rem;
}

.draft-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  padding: 0.375rem 0.625rem;
  background: var(--bg-card);
  border: 1px solid var(--border);
  border-radius: var(--radius);
}

.draft-main {
  display: flex;
  align-items: baseline;
  gap: 0.5rem;
  min-width: 0;
}

.draft-main strong {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text);
  font-size: 0.875rem;
}

.draft-time {
  flex-shrink: 0;
  color: var(--text-muted);
  font-size: 0.75rem;
}

.draft-actions {
  display: flex;
  flex-shrink: 0;
  gap: 0.25rem;
}

.draft-empty {
  display: flex;
  align-items: center;
  padding: 0.375rem 0.625rem;
  border: 1px dashed var(--border);
  border-radius: var(--radius);
  color: var(--text-muted);
  font-size: 0.875rem;
}

/* Card list */
.announcements-list {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.announcement-item {
  min-height:157px;
  background: var(--bg-card);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 1rem;
  transition: border-color 0.15s ease;
}

.announcement-item:hover {
  border-color: var(--accent);
}

.ann-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 0.5rem;
}

.ann-title {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.9375rem;
  font-weight: 600;
  letter-spacing: -0.01em;
  color: var(--text);
  min-width: 0;
  flex: 1;
}

.title-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ann-meta {
  display: flex;
  gap: 1rem;
  font-size: 0.75rem;
  color: var(--text-muted);
  flex-shrink: 0;
  margin-left: 0.5rem;
}

/* Truncated content with markdown */
.ann-content {
  font-size: 0.875rem;
  line-height: 1.5;
  overflow: hidden;
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  text-overflow: ellipsis;
  margin-bottom: 0.75rem;
}

.ann-content :deep(p) {
  margin: 0;
}

.ann-content :deep(table),
.ann-content :deep(pre),
.ann-content :deep(blockquote) {
  display: none;
}

.ann-content :deep(h1),
.ann-content :deep(h2),
.ann-content :deep(h3) {
  font-size: inherit;
  font-weight: 600;
  margin: 0;
}

/* Footer with actions */
.ann-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.ann-actions {
  display: flex;
  gap: 0.5rem;
}

.detail-btn {
  margin-left: auto;
}

/* Detail dialog */
.detail-meta {
  display: flex;
  align-items: center;
  gap: 1rem;
  font-size: 0.8125rem;
  color: var(--text-muted);
  margin-bottom: 1rem;
  padding-bottom: 0.75rem;
  border-bottom: 1px solid var(--border);
}

.detail-content {
  font-size: 0.9375rem;
  line-height: 1.75;
  color: var(--text);
}

/* Markdown styles */
.markdown-body :deep(h1),
.markdown-body :deep(h2),
.markdown-body :deep(h3) {
  margin-top: 1em;
  margin-bottom: 0.5em;
  font-weight: 600;
  line-height: 1.3;
}

.markdown-body :deep(h1) { font-size: 1.5em; }
.markdown-body :deep(h2) { font-size: 1.3em; }
.markdown-body :deep(h3) { font-size: 1.1em; }

.markdown-body :deep(p) {
  margin-bottom: 0.75em;
}

.markdown-body :deep(table) {
  width: 100%;
  border-collapse: collapse;
  margin: 1em 0;
  font-size: 0.875em;
}

.markdown-body :deep(th),
.markdown-body :deep(td) {
  border: 1px solid var(--border);
  padding: 0.5em 0.75em;
  text-align: left;
}

.markdown-body :deep(th) {
  background: var(--bg-deep);
  font-weight: 600;
}

.markdown-body :deep(code) {
  background: var(--bg-deep);
  padding: 0.15em 0.4em;
  border-radius: 4px;
  font-size: 0.9em;
}

.markdown-body :deep(pre) {
  background: var(--bg-deep);
  padding: 1em;
  border-radius: var(--radius);
  overflow-x: auto;
  margin: 1em 0;
}

.markdown-body :deep(pre code) {
  background: none;
  padding: 0;
}

.markdown-body :deep(ul),
.markdown-body :deep(ol) {
  padding-left: 1.5em;
  margin-bottom: 0.75em;
}

.markdown-body :deep(blockquote) {
  border-left: 3px solid var(--accent);
  padding-left: 1em;
  color: var(--text-secondary);
  margin: 1em 0;
}

.markdown-body :deep(hr) {
  border: none;
  border-top: 1px solid var(--border);
  margin: 1.5em 0;
}

.markdown-body :deep(a) {
  color: var(--accent);
  text-decoration: none;
}

.markdown-body :deep(a:hover) {
  text-decoration: underline;
}

.pagination-wrapper {
  display: flex;
  justify-content: center;
  margin-top: 1.25rem;
  padding-top: 1rem;
  border-top: 1px solid var(--border);
}

/* Notify dialog */
.notify-desc {
  font-size: 0.875rem;
  color: var(--text-secondary);
  margin-bottom: 1rem;
}

.notify-members {
  max-height: 400px;
  overflow-y: auto;
  padding-right: 0.5rem;
}

.grade-group {
  margin-bottom: 1rem;
}

.group-checkbox {
  font-weight: 600;
  margin-bottom: 0.5rem;
}

.member-list {
  display: flex;
  flex-direction: column;
  gap: 0.375rem;
  padding-left: 1.5rem;
}

.member-checkbox {
  font-size: 0.875rem;
}

@media (max-width: 640px) {
  .draft-item {
    align-items: flex-start;
    flex-direction: column;
  }

  .draft-main {
    width: 100%;
  }

  .draft-actions {
    width: 100%;
    justify-content: flex-end;
  }
}
@media (max-width: 768px) {
  .card { padding: 0.875rem; }
  .card-header { flex-wrap: wrap; gap: 0.5rem; }
  .ann-header { flex-wrap: wrap; gap: 0.5rem; }
  .ann-meta { flex-wrap: wrap; gap: 0.5rem; }
  .ann-actions { flex-wrap: wrap; gap: 0.5rem; }
  .section-title { flex-wrap: wrap; gap: 0.5rem; }
}

.ann-summary { color:var(--text-secondary); display:-webkit-box; -webkit-line-clamp:2; -webkit-box-orient:vertical; overflow:hidden; line-height:1.8; min-height:3.6em; margin:12px 0; overflow-wrap:anywhere; }
.draft-section.is-empty { display:flex; align-items:center; justify-content:space-between; gap:12px; padding:12px 16px; }
.draft-section.is-empty .section-title { margin:0; }
.draft-section.is-empty .draft-empty { padding:0; }

.draft-loading { min-height:22.4px; display:flex; align-items:center; }
.draft-section { min-height:58.4px; }
.draft-section:has(.draft-loading) { display:flex; align-items:center; justify-content:space-between; gap:12px; padding:12px 16px; }
.draft-section:has(.draft-loading) .section-title { margin:0; }

@media(max-width:768px) { .announcement-item { min-height:184px; } .ann-header { flex-direction:column; gap:8px; } .ann-title { width:100%; min-height:24px; } .ann-meta { margin-left:0; } }
</style>
