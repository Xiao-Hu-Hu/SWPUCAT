<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { knowledgeApi } from '@/api/knowledge'
import { useAuthStore } from '@/stores/auth'
import { ElMessage } from 'element-plus'
import { marked } from 'marked'

const authStore = useAuthStore()
const items = ref<Array<{
  id: number
  type: string
  name: string
  description: string
  url: string
  file_key: string
  file_size: string
  category_id: number
  uploader_name: string
  approved: boolean
  rejected: boolean
  reject_reason: string
  reviewer_id: number
  reviewer_name: string
  created_at: string
}>>([])
const downloading = ref<Record<number, number>>({})
const showRejectDialog = ref(false)
const showPreviewDialog = ref(false)
const previewItem = ref<{ name: string; description: string } | null>(null)
const rejectItemId = ref<number | null>(null)
const rejectReason = ref('')

const isManager = computed(() => authStore.isCaptain || authStore.isSuperAdmin)

const currentPage = ref(1)
const pageSize = ref(10)

const paginatedItems = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value
  return items.value.slice(start, start + pageSize.value)
})

onMounted(async () => {
  await loadItems()
})

async function loadItems() {
  try {
    if (isManager.value) {
      const res = await knowledgeApi.listPendingItems()
      items.value = res.data
    } else {
      const res = await knowledgeApi.listUserItems()
      items.value = res.data
    }
  } catch {
    console.error('Failed to load items')
  }
}

async function handleApprove(id: number) {
  try {
    await knowledgeApi.approveItem(id)
    ElMessage.success('已批准')
    await loadItems()
  } catch {
    ElMessage.error('操作失败')
  }
}

function openRejectDialog(id: number) {
  rejectItemId.value = id
  rejectReason.value = ''
  showRejectDialog.value = true
}

async function handleReject() {
  if (!rejectItemId.value || !rejectReason.value.trim()) {
    ElMessage.warning('请输入拒绝理由')
    return
  }
  try {
    await knowledgeApi.rejectItem(rejectItemId.value, rejectReason.value.trim())
    ElMessage.success('已拒绝')
    showRejectDialog.value = false
    await loadItems()
  } catch {
    ElMessage.error('操作失败')
  }
}

async function handleDownloadFile(id: number, name: string) {
  if (downloading.value[id] !== undefined) return
  downloading.value[id] = 0
  try {
    const res = await knowledgeApi.downloadFile(id, (percent) => {
      downloading.value[id] = percent
    })
    const url = window.URL.createObjectURL(new Blob([res.data]))
    const link = document.createElement('a')
    link.href = url
    link.setAttribute('download', name)
    document.body.appendChild(link)
    link.click()
    link.remove()
    window.URL.revokeObjectURL(url)
    downloading.value[id] = 100
    setTimeout(() => {
      delete downloading.value[id]
    }, 1000)
  } catch {
    ElMessage.error('下载失败')
    delete downloading.value[id]
  }
}

function getStatusTag(item: any) {
  if (item.approved) return { type: 'success', text: '已通过' }
  if (item.rejected) return { type: 'danger', text: '已拒绝' }
  return { type: 'warning', text: '待审核' }
}

function openPreview(item: { name: string; description: string }) {
  previewItem.value = item
  showPreviewDialog.value = true
}

function renderMarkdown(md: string): string {
  return marked.parse(md) as string
}
</script>

<template>
  <div class="approvals">
    <div class="card">
      <div class="card-header">
        <h3>{{ isManager ? '审批管理' : '我的申请' }}</h3>
        <span class="pending-count" v-if="isManager">{{ items.filter(i => !i.approved && !i.rejected).length }} 条待审批</span>
      </div>

      <div class="approval-list">
        <div v-for="item in paginatedItems" :key="item.id" class="approval-card">
          <div class="item-icon">
            <el-icon :size="24">
              <Link v-if="item.type === 'link'" />
              <Document v-else />
            </el-icon>
          </div>
          <div class="item-info">
            <div class="item-name">
              <a v-if="item.type === 'link'" :href="item.url" target="_blank">{{ item.name }}</a>
              <span v-else>{{ item.name }}</span>
            </div>
            <div class="item-meta">
              <span v-if="isManager">{{ item.uploader_name }}</span>
              <span>{{ item.created_at }}</span>
              <span v-if="item.file_size">{{ item.file_size }}</span>
            </div>
            <div v-if="item.rejected && item.reject_reason" class="reject-reason">
              拒绝理由：{{ item.reject_reason }}
            </div>
            <div v-if="(item.approved || item.rejected) && item.reviewer_name" class="reviewer-info">
              审批人：{{ item.reviewer_name }}
            </div>
          </div>
          <div class="item-actions">
            <el-tag :type="getStatusTag(item).type" size="small">
              {{ getStatusTag(item).text }}
            </el-tag>
            <el-button v-if="item.description" size="small" @click="openPreview(item)">
              查看说明
            </el-button>
            <el-button
              v-if="item.type === 'file' && !item.rejected"
              size="small"
              type="primary"
              :loading="downloading[item.id] !== undefined && downloading[item.id] < 100"
              :disabled="downloading[item.id] !== undefined"
              @click="handleDownloadFile(item.id, item.name)"
            >
              {{ downloading[item.id] !== undefined ? (downloading[item.id] < 100 ? `${downloading[item.id]}%` : '完成') : '下载' }}
            </el-button>
            <template v-if="isManager && !item.approved && !item.rejected">
              <el-button size="small" type="success" @click="handleApprove(item.id)">
                通过
              </el-button>
              <el-button size="small" type="danger" @click="openRejectDialog(item.id)">
                拒绝
              </el-button>
            </template>
          </div>
        </div>
        <el-empty v-if="items.length === 0" :description="isManager ? '暂无待审批项目' : '暂无申请记录'" />
      </div>

      <div class="pagination-wrapper" v-if="items.length > 0">
        <el-pagination
          v-model:current-page="currentPage"
          v-model:page-size="pageSize"
          :page-sizes="[5, 10, 20, 50]"
          :total="items.length"
          layout="total, sizes, prev, pager, next, jumper"
          background
        />
      </div>
    </div>

    <el-dialog v-model="showRejectDialog" title="拒绝理由" width="500px">
      <el-form label-width="80px">
        <el-form-item label="拒绝理由">
          <el-input
            v-model="rejectReason"
            type="textarea"
            :rows="3"
            placeholder="请输入拒绝理由，将通知给上传者"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showRejectDialog = false">取消</el-button>
        <el-button type="danger" @click="handleReject">确认拒绝</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="showPreviewDialog" :title="previewItem?.name || '资源说明'" width="600px">
      <div v-if="previewItem" class="markdown-preview" v-html="renderMarkdown(previewItem.description)"></div>
      <template #footer>
        <el-button @click="showPreviewDialog = false">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.approvals {
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
  margin-bottom: 1.5rem;
}

.card-header h3 {
  font-size: 0.9375rem;
  font-weight: 600;
  letter-spacing: -0.01em;
  color: var(--text);
}

.pending-count {
  color: var(--text-muted);
  font-size: 0.875rem;
}

.approval-list {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.approval-card {
  display: flex;
  align-items: flex-start;
  gap: 1rem;
  padding: 1rem;
  background: var(--bg-card);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  transition: border-color 0.2s;
}

.approval-card:hover {
  border-color: var(--accent);
}

.item-icon {
  width: 48px;
  height: 48px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--accent-bg);
  border-radius: var(--radius);
  color: var(--accent);
  flex-shrink: 0;
}

.item-info {
  flex: 1;
  min-width: 0;
}

.item-name {
  display: block;
  font-weight: 500;
  margin-bottom: 0.25rem;
}

.item-name a {
  color: var(--accent);
  text-decoration: none;
}

.item-name a:hover {
  text-decoration: underline;
}

.item-meta {
  display: flex;
  gap: 1rem;
  font-size: 0.75rem;
  color: var(--text-muted);
}

.reject-reason {
  margin-top: 0.5rem;
  padding: 0.5rem;
  background: var(--danger-bg);
  border-radius: 4px;
  font-size: 0.8rem;
  color: var(--danger);
}

.reviewer-info {
  margin-top: 0.5rem;
  font-size: 0.8rem;
  color: var(--text-muted);
}

.item-actions {
  display: flex;
  gap: 0.5rem;
  align-items: center;
  flex-shrink: 0;
}

.pagination-wrapper {
  display: flex;
  justify-content: center;
  margin-top: 1.25rem;
  padding-top: 1rem;
  border-top: 1px solid var(--border);
}

.markdown-preview {
  line-height: 1.7;
  color: var(--text);
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
  background: var(--bg-deep);
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
</style>
