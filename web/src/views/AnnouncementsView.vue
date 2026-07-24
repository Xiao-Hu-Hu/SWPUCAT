<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { announcementApi } from '@/api/announcement'
import { memberApi } from '@/api/member'
import { useAuthStore } from '@/stores/auth'
import { ElMessage, ElMessageBox } from 'element-plus'
import { marked } from 'marked'

const authStore = useAuthStore()
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
const pageSize = ref(10)

const paginatedAnnouncements = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value
  return announcements.value.slice(start, start + pageSize.value)
})

onMounted(async () => {
  await loadAnnouncements()
})

async function loadAnnouncements() {
  try {
    const res = await announcementApi.list()
    announcements.value = res.data
  } catch {
    console.error('Failed to load announcements')
  }
}

function openCreate() {
  editingId.value = null
  form.value = { title: '', content: '', pinned: false }
  showDialog.value = true
}

function openEdit(ann: typeof announcements.value[0]) {
  editingId.value = ann.id
  form.value = { title: ann.title, content: ann.content, pinned: ann.pinned }
  showDialog.value = true
}

function openDetail(ann: typeof announcements.value[0]) {
  detailAnn.value = ann
  showDetail.value = true
}

function renderMarkdown(content: string) {
  return marked(content || '', { breaks: true })
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
      .sort((a, b) => b[0].localeCompare(a[0]))
      .map(([year, members]) => ({
        year,
        label: `${year}届`,
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
    if (editingId.value) {
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

async function handleNotify() {
  if (!notifyAnnId.value || selectedUserIds.value.length === 0) {
    ElMessage.warning('请至少选择一位成员')
    return
  }

  notifyLoading.value = true
  try {
    const res = await announcementApi.notify(notifyAnnId.value, selectedUserIds.value)
    const { sent = 0, failed = [] } = res.data || {}
    showNotifyDialog.value = false

    if (failed.length === 0) {
      ElMessage.success(`已成功通知 ${sent} 位成员`)
    } else {
      const failList = failed.map((f: { nickname: string; reason: string }) => `• ${f.nickname}（${f.reason}）`).join('\n')
      ElMessageBox.alert(
        `成功 ${sent} 人，失败 ${failed.length} 人：\n\n${failList}`,
        '通知发送结果',
        { type: 'warning', confirmButtonText: '知道了' }
      )
    }
  } catch {
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
</script>

<template>
  <div class="announcements">
    <div class="card">
      <div class="card-header">
        <h3>公告管理</h3>
        <el-button v-if="authStore.isCaptain" type="primary" @click="openCreate">
          <el-icon><Plus /></el-icon> 发布公告
        </el-button>
      </div>

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
          <div class="ann-content markdown-body" v-html="renderMarkdown(ann.content)"></div>
          <div class="ann-footer">
            <div class="ann-actions" v-if="authStore.isCaptain">
              <el-button size="small" @click="openEdit(ann)">编辑</el-button>
              <el-button size="small" type="success" @click="openNotify(ann)">通知</el-button>
              <el-button size="small" type="danger" @click="handleDelete(ann.id)">删除</el-button>
            </div>
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
          :page-sizes="[5, 10, 20, 50]"
          :total="announcements.length"
          layout="total, sizes, prev, pager, next, jumper"
          background
        />
      </div>
    </div>

    <!-- Create / Edit Dialog -->
    <el-dialog v-model="showDialog" :title="editingId ? '编辑公告' : '发布公告'" width="600px">
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
    <el-dialog v-model="showDetail" :title="detailAnn?.title || '公告详情'" width="700px" class="detail-dialog">
      <div v-if="detailAnn" class="detail-meta">
        <span>{{ detailAnn.author_name }}</span>
        <span>{{ detailAnn.created_at }}</span>
        <el-tag v-if="detailAnn.pinned" type="warning" size="small">置顶</el-tag>
      </div>
      <div v-if="detailAnn" class="detail-content markdown-body" v-html="renderMarkdown(detailAnn.content)"></div>
    </el-dialog>

    <!-- Notify Dialog -->
    <el-dialog v-model="showNotifyDialog" title="通知成员" width="500px">
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

/* Card list */
.announcements-list {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.announcement-item {
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
</style>
