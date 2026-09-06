<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import type { AxiosProgressEvent } from 'axios'
import { knowledgeApi } from '@/api/knowledge'
import { useAuthStore } from '@/stores/auth'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { UploadFile, UploadUserFile } from 'element-plus'
import { marked } from 'marked'

const authStore = useAuthStore()
const uploading = ref(false)
const uploadPhase = ref<'sending' | 'saving'>('sending')
const uploadPercent = ref<number | null>(0)
const uploadedBytes = ref(0)
const uploadTotal = ref<number | undefined>()
const uploadSpeed = ref(0)
const uploadRemaining = ref<number | undefined>()
const refreshingCount = ref(0)
let uploadController: AbortController | undefined
let uploadStartedAt = 0
let disposed = false
const downloading = ref<Record<number, number>>({})
const categories = ref<Array<{ id: number; name: string; is_system: boolean; count: number }>>([])
const items = ref<Array<{
  id: number
  type: string
  name: string
  description: string
  url: string
  file_size: string
  category_id: number
  uploader_id: number
  uploader_name: string
  approved: boolean
  created_at: string
}>>([])

const selectedCategory = ref<number | undefined>(undefined)
const searchQuery = ref('')

const currentPage = ref(1)
const pageSize = ref(10)

const paginatedItems = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value
  return items.value.slice(start, start + pageSize.value)
})

const showLinkDialog = ref(false)
const showCategoryDialog = ref(false)
const showUploadDialog = ref(false)
const showPreviewDialog = ref(false)
const previewItem = ref<{ name: string; description: string } | null>(null)
const linkForm = ref({ name: '', url: '', description: '', category_id: 0 })
const categoryForm = ref({ name: '' })
const uploadForm = ref({ description: '', category_id: 0 })
const selectedFile = ref<File | null>(null)
const fileList = ref<UploadUserFile[]>([])

onMounted(refreshKnowledge)

onBeforeUnmount(() => {
  disposed = true
  uploadController?.abort()
})

async function loadCategories() {
  try {
    const res = await knowledgeApi.listCategories()
    categories.value = res.data
    return true
  } catch {
    console.error('Failed to load categories')
    return false
  }
}

async function loadItems() {
  try {
    const res = await knowledgeApi.listItems(selectedCategory.value, searchQuery.value)
    items.value = res.data
    return true
  } catch {
    console.error('Failed to load items')
    return false
  }
}

async function refreshKnowledge() {
  refreshingCount.value++
  try {
    const results = await Promise.all([loadItems(), loadCategories()])
    if (!disposed && results.includes(false)) {
      ElMessage.warning('部分列表刷新失败，请刷新页面重试')
    }
  } finally {
    refreshingCount.value--
  }
}

function handleCategoryChange() {
  loadItems()
}

function handleSearch() {
  loadItems()
}

async function handleCreateLink() {
  try {
    await knowledgeApi.createLink(linkForm.value.name, linkForm.value.url, linkForm.value.category_id, linkForm.value.description)
    if (authStore.isCaptain || authStore.isSuperAdmin) {
      ElMessage.success('创建成功')
    } else {
      ElMessage.success('创建成功，等待队长审核')
    }
    showLinkDialog.value = false
    linkForm.value = { name: '', url: '', description: '', category_id: 0 }
    await loadItems()
  } catch {
    ElMessage.error('创建失败')
  }
}

async function handleCreateCategory() {
  try {
    await knowledgeApi.createCategory(categoryForm.value.name)
    ElMessage.success('创建成功')
    showCategoryDialog.value = false
    await loadCategories()
  } catch {
    ElMessage.error('创建失败')
  }
}

async function handleDeleteItem(id: number) {
  try {
    await ElMessageBox.confirm('确定要删除这个资源吗？', '提示', { type: 'warning' })
    await knowledgeApi.deleteItem(id)
    ElMessage.success('删除成功')
    await loadItems()
  } catch {
    // Cancelled
  }
}

async function handleDeleteCategory(id: number) {
  try {
    await ElMessageBox.confirm('确定要删除这个分类吗？', '提示', { type: 'warning' })
    await knowledgeApi.deleteCategory(id)
    ElMessage.success('删除成功')
    await loadCategories()
  } catch {
    ElMessage.error('删除失败，分类可能正在被使用')
  }
}

function handleFileChange(file: UploadFile) {
  selectedFile.value = file.raw ?? null
}

function handleFileRemove() {
  selectedFile.value = null
}

function resetUploadForm() {
  selectedFile.value = null
  fileList.value = []
  uploadForm.value = { description: '', category_id: 0 }
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${Math.round(bytes)} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

const remainingText = computed(() => {
  const seconds = uploadRemaining.value
  if (seconds === undefined || !Number.isFinite(seconds)) return '正在估算剩余时间'
  if (seconds < 60) return `预计剩余 ${Math.max(1, Math.ceil(seconds))} 秒`
  return `预计剩余 ${Math.ceil(seconds / 60)} 分钟`
})

function handleUploadProgress(event: AxiosProgressEvent) {
  if (disposed || !uploading.value || uploadController?.signal.aborted) return
  // A token refresh may retry the request; restart speed estimation if bytes reset.
  if (event.loaded < uploadedBytes.value) uploadStartedAt = performance.now()
  uploadedBytes.value = event.loaded
  uploadTotal.value = event.total
  const ratio = event.total ? Math.min(1, event.loaded / event.total) : undefined
  uploadPercent.value = ratio === undefined ? null : Math.floor(ratio * 100)
  uploadPhase.value = ratio === 1 ? 'saving' : 'sending'
  const elapsed = (performance.now() - uploadStartedAt) / 1000
  uploadSpeed.value = event.rate ?? (elapsed > 0 ? event.loaded / elapsed : 0)
  uploadRemaining.value = event.estimated ?? (event.total && uploadSpeed.value > 0
    ? Math.max(0, event.total - event.loaded) / uploadSpeed.value : undefined)
}

function handleCancelUpload() {
  if (uploading.value) {
    uploadController?.abort()
  } else {
    showUploadDialog.value = false
  }
}

async function handleUploadFile() {
  if (uploading.value) return
  if (!selectedFile.value) {
    ElMessage.warning('请选择文件')
    return
  }
  if (!uploadForm.value.category_id) {
    ElMessage.warning('请选择分类')
    return
  }
  const MAX_SIZE = 1024 * 1024 * 1024
  if (selectedFile.value.size > MAX_SIZE) {
    ElMessage.error('文件大小超过 1GB 限制')
    return
  }
  uploading.value = true
  uploadPhase.value = 'sending'
  uploadPercent.value = 0
  uploadedBytes.value = 0
  uploadTotal.value = undefined
  uploadSpeed.value = 0
  uploadRemaining.value = undefined
  uploadStartedAt = performance.now()
  const controller = new AbortController()
  uploadController = controller
  let shouldRefresh = false
  try {
    await knowledgeApi.uploadFile(selectedFile.value, uploadForm.value.category_id, uploadForm.value.description, {
      onProgress: (event) => {
        if (uploadController === controller) handleUploadProgress(event)
      },
      signal: controller.signal
    })
    if (disposed) return
    if (authStore.isCaptain || authStore.isSuperAdmin) {
      ElMessage.success('上传成功')
    } else {
      ElMessage.success('上传成功，等待队长审核')
    }
    showUploadDialog.value = false
    resetUploadForm()
    shouldRefresh = true
  } catch (err: any) {
    if (disposed) return
    if (err?.code === 'ERR_CANCELED') {
      ElMessage.info(uploadPhase.value === 'saving'
        ? '已停止等待，正在刷新列表以确认文件是否已保存' : '已停止上传请求')
      shouldRefresh = true
    } else if (err?.code === 'ECONNABORTED') {
      ElMessage.error('上传超时，请检查网络或重试')
    } else if (err?.response?.status === 413) {
      ElMessage.error(err.response?.data?.message || '文件过大，上传失败')
    } else {
      ElMessage.error('上传失败')
    }
  } finally {
    uploading.value = false
    uploadController = undefined
  }
  if (shouldRefresh && !disposed) void refreshKnowledge()
}

function openPreview(item: { name: string; description: string }) {
  previewItem.value = item
  showPreviewDialog.value = true
}

function renderMarkdown(md: string): string {
  return marked.parse(md) as string
}

async function handleDownloadFile(id: number, name: string) {
  if (downloading.value[id] !== undefined) return
  downloading.value[id] = 0
  try {
    const res = await knowledgeApi.downloadFile(id, (percent) => {
      downloading.value[id] = percent
    })
    const url = window.URL.createObjectURL(new Blob([res]))
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
</script>

<template>
  <div class="knowledge">
    <div class="sidebar">
      <div class="card">
        <div class="card-header">
          <h3>分类</h3>
          <el-button v-if="authStore.isCaptain" size="small" @click="showCategoryDialog = true">
            <el-icon><Plus /></el-icon>
          </el-button>
        </div>
        <div class="category-list">
          <div
            class="category-item"
            :class="{ active: !selectedCategory }"
            @click="selectedCategory = undefined; handleCategoryChange()"
          >
            <span>全部</span>
          </div>
          <div
            v-for="cat in categories"
            :key="cat.id"
            class="category-item"
            :class="{ active: selectedCategory === cat.id }"
            @click="selectedCategory = cat.id; handleCategoryChange()"
          >
            <span>{{ cat.name }}</span>
            <span class="count">{{ cat.count }}</span>
            <el-button
              v-if="authStore.isCaptain && !cat.is_system"
              size="small"
              type="danger"
              link
              @click.stop="handleDeleteCategory(cat.id)"
            >
              <el-icon><Delete /></el-icon>
            </el-button>
          </div>
        </div>
      </div>
    </div>

    <div class="content">
      <div class="card">
        <div class="card-header">
          <h3>知识库 <span v-if="refreshingCount > 0" class="refresh-status" role="status">正在刷新列表…</span></h3>
          <div class="actions">
            <el-input
              v-model="searchQuery"
              placeholder="搜索资源..."
              style="width: 200px"
              @keyup.enter="handleSearch"
            >
              <template #prefix>
                <el-icon><Search /></el-icon>
              </template>
            </el-input>
            <el-button type="primary" @click="showLinkDialog = true">
              <el-icon><Link /></el-icon> 添加链接
            </el-button>
            <el-button type="success" @click="showUploadDialog = true" :disabled="uploading">
              <el-icon><Upload /></el-icon> 上传文件
            </el-button>
          </div>
        </div>

        <div class="items-list">
          <div v-for="item in paginatedItems" :key="item.id" class="item-card">
            <div class="item-icon">
              <el-icon :size="24">
                <Link v-if="item.type === 'link'" />
                <Document v-else />
              </el-icon>
            </div>
            <div class="item-info">
              <a v-if="item.type === 'link'" :href="item.url" target="_blank" class="item-name">
                {{ item.name }}
              </a>
              <span v-else class="item-name">{{ item.name }}</span>
              <div class="item-meta">
                <span>{{ item.uploader_name }}</span>
                <span>{{ item.created_at }}</span>
                <span v-if="item.file_size">{{ item.file_size }}</span>
              </div>
            </div>
            <div class="item-actions" v-if="item.description">
              <el-button size="small" @click="openPreview(item)">
                查看说明
              </el-button>
            </div>
            <div class="item-actions">
              <el-button
                v-if="item.type === 'file'"
                size="small"
                type="primary"
                :loading="downloading[item.id] !== undefined && downloading[item.id] < 100"
                :disabled="downloading[item.id] !== undefined"
                @click="handleDownloadFile(item.id, item.name)"
              >
                {{ downloading[item.id] !== undefined ? (downloading[item.id] < 100 ? `${downloading[item.id]}%` : '完成') : '下载' }}
              </el-button>
              <el-button
                v-if="authStore.isCaptain || authStore.user?.id === item.uploader_id"
                size="small"
                type="danger"
                @click="handleDeleteItem(item.id)"
              >
                删除
              </el-button>
            </div>
          </div>
          <el-empty v-if="items.length === 0" description="暂无资源" />
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
    </div>

    <el-dialog v-model="showLinkDialog" title="添加链接" width="500px">
      <el-form :model="linkForm" label-width="80px">
        <el-form-item label="名称">
          <el-input v-model="linkForm.name" placeholder="请输入链接名称" />
        </el-form-item>
        <el-form-item label="URL">
          <el-input v-model="linkForm.url" placeholder="请输入链接地址" />
        </el-form-item>
        <el-form-item label="分类">
          <el-select v-model="linkForm.category_id" placeholder="请选择分类">
            <el-option v-for="cat in categories" :key="cat.id" :label="cat.name" :value="cat.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="描述">
          <el-input
            v-model="linkForm.description"
            type="textarea"
            :rows="4"
            placeholder="请输入描述（支持Markdown格式，0-1000字）"
            maxlength="1000"
            show-word-limit
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showLinkDialog = false">取消</el-button>
        <el-button type="primary" @click="handleCreateLink">确定</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="showCategoryDialog" title="添加分类" width="400px">
      <el-form :model="categoryForm" label-width="80px">
        <el-form-item label="名称">
          <el-input v-model="categoryForm.name" placeholder="请输入分类名称" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCategoryDialog = false">取消</el-button>
        <el-button type="primary" @click="handleCreateCategory">确定</el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="showUploadDialog"
      title="上传文件"
      width="500px"
      :close-on-click-modal="!uploading"
      :close-on-press-escape="!uploading"
      :show-close="!uploading"
      @closed="resetUploadForm"
    >
      <el-form :model="uploadForm" label-width="80px" :disabled="uploading">
        <el-form-item label="文件">
          <el-upload
            v-model:file-list="fileList"
            :auto-upload="false"
            :limit="1"
            :disabled="uploading"
            :on-change="handleFileChange"
            :on-remove="handleFileRemove"
            :on-exceed="() => ElMessage.warning('只能上传一个文件')"
          >
            <el-button type="primary">选择文件</el-button>
            <template #tip>
              <div class="el-upload__tip">支持 PDF、Word、Markdown、ZIP、EXE 等格式</div>
            </template>
          </el-upload>
        </el-form-item>
        <el-form-item label="分类">
          <el-select v-model="uploadForm.category_id" placeholder="请选择分类">
            <el-option v-for="cat in categories" :key="cat.id" :label="cat.name" :value="cat.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="描述">
          <el-input
            v-model="uploadForm.description"
            type="textarea"
            :rows="4"
            placeholder="请输入描述（支持Markdown格式，0-1000字）"
            maxlength="1000"
            show-word-limit
          />
        </el-form-item>
      </el-form>
      <div v-if="uploading" class="upload-progress" aria-live="polite">
        <div class="upload-stage">{{ uploadPhase === 'saving' ? '文件已发送，正在等待服务器保存…' : '正在上传文件' }}</div>
        <el-progress
          :percentage="uploadPercent ?? 0"
          :indeterminate="uploadPercent === null"
          :show-text="uploadPercent !== null"
        />
        <div class="upload-details">
          <span>已发送 {{ formatBytes(uploadedBytes) }}<template v-if="uploadTotal"> / {{ formatBytes(uploadTotal) }}</template></span>
          <span v-if="uploadPhase === 'sending'">{{ formatBytes(uploadSpeed) }}/s</span>
        </div>
        <div v-if="uploadPhase === 'sending'" class="upload-remaining">{{ remainingText }}</div>
      </div>
      <template #footer>
        <el-button @click="handleCancelUpload">{{ uploading ? (uploadPhase === 'saving' ? '停止等待' : '取消上传') : '取消' }}</el-button>
        <el-button type="primary" @click="handleUploadFile" :loading="uploading">
          {{ uploading ? (uploadPhase === 'saving' ? '保存中...' : '上传中...') : '上传' }}
        </el-button>
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
.upload-progress {
  margin-top: 1rem;
}

.upload-stage {
  margin-bottom: 0.5rem;
  color: var(--text);
}

.upload-details {
  display: flex;
  justify-content: space-between;
  margin-top: 0.5rem;
}

.upload-details,
.upload-remaining,
.refresh-status {
  color: var(--text-secondary);
  font-size: 0.75rem;
  font-weight: normal;
}

.upload-remaining {
  margin-top: 0.25rem;
}

.refresh-status {
  margin-left: 0.5rem;
}

.knowledge {
  display: flex;
  gap: 1rem;
  width: 100%;
}

.sidebar {
  width: 200px;
  flex-shrink: 0;
}

.content {
  flex: 1;
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
  gap: 0.75rem;
}

.category-list {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.category-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.75rem;
  border-radius: var(--radius);
  cursor: pointer;
  transition: background 0.2s;
  color: var(--text-secondary);
}

.category-item:hover {
  background: var(--bg-hover);
}

.category-item.active {
  background: var(--accent);
  color: #ffffff;
}

.count {
  font-size: 0.75rem;
  color: var(--text-muted);
}

.items-list {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.item-card {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 1rem;
  background: var(--bg-deep);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  transition: box-shadow 0.2s;
}

.item-card:hover {
  box-shadow: var(--shadow-md);
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
}

.item-info {
  flex: 1;
}

.item-name {
  display: block;
  font-weight: 500;
  color: var(--text);
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

.item-actions {
  display: flex;
  gap: 0.5rem;
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
