<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import type { AxiosProgressEvent } from 'axios'
import { knowledgeApi } from '@/api/knowledge'
import { useAuthStore } from '@/stores/auth'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { UploadFile, UploadUserFile } from 'element-plus'
import { renderMarkdown } from '@/utils/markdown'
import { usePaginationLayout } from '@/composables/usePaginationLayout'

const DESCRIPTION_TEMPLATE = `> 用途：一句话说明什么情况下使用这份资料
> 关键词：关键词一、关键词二、关键词三
> 适用对象：成员 / 队长 / 全体

## 内容摘要
用 2~3 句话概括这份资料回答什么问题、包含哪些核心内容。

## 使用要点
- 要点一：……
- 要点二：……

## 注意事项
- 说明限制、假设或边界（例如「以系统实时查询为准」）。`

const DESCRIPTION_EXAMPLE = `> 用途：查询上班打卡、下班签退的规则与异常处理
> 关键词：打卡、签退、工时、异常、补签
> 适用对象：全体成员

## 内容摘要
介绍打卡与签退的正常流程：同一天按时间顺序配对的上班—下班记录才会计入工时；日终会对未签退记录按 23:59:59 自动签退。

## 使用要点
- 开工执行上班打卡，收工执行下班打卡。
- 仍处于上班状态的记录，日终任务会按 23:59:59 自动签退。

## 注意事项
- 异常记录助手不会自行猜测或补齐，需队长/超级管理员核对补签。
- 具体工时要求请以系统实时查询为准。`

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
  source_origin: string
  source_key?: string
  revision: number
  created_at: string
}>>([])

const itemsLoading = ref(true)
const itemsError = ref('')
const categoriesLoading = ref(true)
const categoriesError = ref('')
let itemsRequest = 0
const selectedCategory = ref<number | undefined>(undefined)
const searchQuery = ref('')

const currentPage = ref(1)
const pageSize = ref(5)
const { layout: paginationLayout, pagerCount, small: paginationSmall } = usePaginationLayout()

const paginatedItems = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value
  return items.value.slice(start, start + pageSize.value)
})

const showLinkDialog = ref(false)
const showCategoryDialog = ref(false)
const showUploadDialog = ref(false)
const showPreviewDialog = ref(false)
const showLinkTemplateHelp = ref(false)
const showUploadTemplateHelp = ref(false)
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
  categoriesLoading.value = true
  categoriesError.value = ''
  try {
    const res = await knowledgeApi.listCategories()
    categories.value = res.data
    return true
  } catch {
    categoriesError.value = '分类加载失败'
    return false
  } finally {
    categoriesLoading.value = false
  }
}

async function loadItems() {
  const request = ++itemsRequest
  itemsLoading.value = true
  itemsError.value = ''
  try {
    const res = await knowledgeApi.listItems(selectedCategory.value, searchQuery.value)
    if (request === itemsRequest) items.value = res.data || []
    return true
  } catch {
    if (request === itemsRequest) itemsError.value = '资源加载失败，请重试'
    return false
  } finally {
    if (request === itemsRequest) itemsLoading.value = false
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
    const res: any = await knowledgeApi.createLink(linkForm.value.name, linkForm.value.url, linkForm.value.category_id, linkForm.value.description)
    if (authStore.isCaptain || authStore.isSuperAdmin) {
      ElMessage.success('创建成功')
    } else {
      ElMessage.success('创建成功，等待队长审核')
    }
    if (res?.warnings?.length) ElMessage.warning(res.warnings[0])
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
    const res: any = await knowledgeApi.uploadFile(selectedFile.value, uploadForm.value.category_id, uploadForm.value.description, {
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
    if (res?.warnings?.length) ElMessage.warning(res.warnings[0])
    showUploadDialog.value = false
    resetUploadForm()
    shouldRefresh = true
  } catch (err: any) {
    if (disposed) return
    if (err?.code === 'ERR_CANCELED') {
      const phase: string = uploadPhase.value
      ElMessage.info(phase === 'saving'
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

async function handleDownloadFile(id: number, name: string) {
  if (downloading.value[id] !== undefined) return
  downloading.value[id] = 0
  try {
    const res = await knowledgeApi.downloadFile(id, (percent) => {
      downloading.value[id] = percent
    })
    const blob = res instanceof Blob ? res : (res as any).data
    const url = window.URL.createObjectURL(blob instanceof Blob ? blob : new Blob([blob]))
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
          <el-button v-if="authStore.isCaptain || authStore.isSuperAdmin" size="small" aria-label="添加分类" @click="showCategoryDialog = true">
            <el-icon><Plus /></el-icon><span>添加</span>
          </el-button>
        </div>
        <ContentState class="category-state" :loading="categoriesLoading" :error="categoriesError" :has-content="categories.length > 0" :rows="1" compact @retry="loadCategories">
      <template #skeleton>
        <div class="category-list" aria-hidden="true"><div v-for="n in 4" :key="n" class="category-item skeleton-shell"><span class="sk-block sk-meta"></span></div></div>
      </template>
        <div class="category-list">
          <div
            class="category-item" role="button" tabindex="0"
            :class="{ active: !selectedCategory }"
            @click="selectedCategory = undefined; handleCategoryChange()" @keydown.enter.self="selectedCategory = undefined; handleCategoryChange()" @keydown.space.prevent.self="selectedCategory = undefined; handleCategoryChange()"
          >
            <span>全部</span>
          </div>
          <div
            v-for="cat in categories"
            :key="cat.id"
            class="category-item" role="button" tabindex="0"
            :class="{ active: selectedCategory === cat.id }"
            @click="selectedCategory = cat.id; handleCategoryChange()" @keydown.enter.self="selectedCategory = cat.id; handleCategoryChange()" @keydown.space.prevent.self="selectedCategory = cat.id; handleCategoryChange()"
          >
            <span>{{ cat.name }}</span>
            <span class="count">{{ cat.count }}</span>
            <el-button
              v-if="(authStore.isCaptain || authStore.isSuperAdmin) && !cat.is_system"
              size="small"
              type="danger"
              link
              @click.stop="handleDeleteCategory(cat.id)"
            >
              <el-icon><Delete /></el-icon>
            </el-button>
          </div>
        </div>
        </ContentState>
      </div>
    </div>

    <div class="content">
      <div class="card">
        <div class="card-header">
          <h3>共享资源</h3>
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
            <el-button @click="showLinkDialog = true">
              <el-icon><Link /></el-icon> 添加链接
            </el-button>
            <el-button type="primary" @click="showUploadDialog = true" :disabled="uploading">
              <el-icon><Upload /></el-icon> 上传文件
            </el-button>
          </div>
        </div>

        <ContentState :loading="itemsLoading" :error="itemsError" :has-content="items.length > 0" @retry="loadItems">
      <template #skeleton>
        <div class="items-list" aria-hidden="true">
          <div v-for="n in pageSize" :key="n" class="item-card skeleton-shell">
            <div class="item-icon"><span class="sk-block sk-icon"></span></div>
            <div class="item-info"><div class="item-name"><span class="sk-block sk-title"></span></div><div class="item-meta"><span class="sk-block sk-meta"></span></div></div>
            <div class="item-card-actions"><span class="sk-block sk-button"></span><span class="sk-block sk-button"></span></div>
          </div>
        </div>
      <div class="pagination-wrapper" aria-hidden="true"><span class="sk-block sk-pagination"></span></div>
      </template>
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
                <el-tag v-if="item.source_origin === 'official_catalog'" size="small" type="success">官方 · v{{ item.revision }}</el-tag>
              </div>
            </div>
            <div class="item-card-actions">
              <el-button v-if="item.description" size="small" @click="openPreview(item)">
                查看说明
              </el-button>
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
              <el-dropdown v-if="item.source_origin !== 'official_catalog' && (authStore.isCaptain || authStore.user?.id === item.uploader_id)" trigger="click">
                <el-button size="small" text :aria-label="`更多操作：${item.name}`">更多</el-button>
                <template #dropdown><el-dropdown-menu><el-dropdown-item @click="handleDeleteItem(item.id)">删除资源</el-dropdown-item></el-dropdown-menu></template>
              </el-dropdown>
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
            :layout="paginationLayout"
            :pager-count="pagerCount"
            :small="paginationSmall"
            background
          />
        </div>
        </ContentState>
      </div>
    </div>

    <el-dialog align-center v-model="showLinkDialog" title="添加链接" width="500px">
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
            placeholder="请按规范模板填写：用途 / 关键词 / 摘要 / 要点 / 注意事项"
            maxlength="1000"
            show-word-limit
          />
          <el-button link type="primary" size="small" class="template-toggle" @click="showLinkTemplateHelp = !showLinkTemplateHelp">
            {{ showLinkTemplateHelp ? '收起模板' : '查看模板' }}
          </el-button>
          <div v-if="showLinkTemplateHelp" class="markdown-preview template-help" v-html="renderMarkdown(DESCRIPTION_TEMPLATE + '\n\n---\n\n**填写示例**\n\n' + DESCRIPTION_EXAMPLE)"></div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showLinkDialog = false">取消</el-button>
        <el-button type="primary" @click="handleCreateLink">确定</el-button>
      </template>
    </el-dialog>

    <el-dialog align-center v-model="showCategoryDialog" title="添加分类" width="400px">
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

    <el-dialog align-center
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
            placeholder="请按规范模板填写：用途 / 关键词 / 摘要 / 要点 / 注意事项"
            maxlength="1000"
            show-word-limit
          />
          <el-button link type="primary" size="small" class="template-toggle" @click="showUploadTemplateHelp = !showUploadTemplateHelp">
            {{ showUploadTemplateHelp ? '收起模板' : '查看模板' }}
          </el-button>
          <div v-if="showUploadTemplateHelp" class="markdown-preview template-help" v-html="renderMarkdown(DESCRIPTION_TEMPLATE + '\n\n---\n\n**填写示例**\n\n' + DESCRIPTION_EXAMPLE)"></div>
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

    <el-dialog align-center v-model="showPreviewDialog" :title="previewItem?.name || '资源说明'" width="600px">
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
.upload-remaining {
  color: var(--text-secondary);
  font-size: 0.75rem;
  font-weight: normal;
}

.upload-remaining {
  margin-top: 0.25rem;
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

.item-card-actions {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex-shrink: 0;
}

.pagination-wrapper {
  display: flex;
  justify-content: center;
  margin-top: 1.25rem;
  padding-top: 1rem;
  border-top: 1px solid var(--border);
}

.template-toggle {
  margin-top: 0.25rem;
  height: auto;
}

.template-help {
  margin-top: 0.5rem;
  padding: 0.75rem;
  border: 1px dashed var(--border);
  border-radius: var(--radius);
  background: var(--bg-deep);
  max-height: 24rem;
  overflow-y: auto;
  font-size: 0.85rem;
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
@media (max-width: 768px) {
  .knowledge { flex-direction: column; gap: 0.75rem; }
  .sidebar { width: 100%; }
  .sidebar .card { padding: 0.5rem 0.625rem; }
  .sidebar .card-header { margin-bottom: 0.5rem; }
  .category-list { display: flex; flex-direction: row; flex-wrap: nowrap; overflow-x: auto; gap: 0.375rem; padding-bottom: 0.25rem; -webkit-overflow-scrolling: touch; }
  .category-item { flex: 0 0 auto; justify-content: flex-start; gap: 0.375rem; padding: 0.25rem 0.625rem; border-radius: 999px; background: var(--bg-deep); }
  .category-item.active { background: var(--accent); color: #fff; }
  .category-item .count { margin-left: 0.25rem; }
  .card { padding: 0.875rem; }
  .content .card-header { flex-direction: column; align-items: stretch; gap: 0.75rem; }
  .actions { width: 100%; gap: 0.5rem; flex-wrap: wrap; }
  .actions .el-input { width: 100% !important; flex: 1 1 100%; }
  .actions .el-button { flex: 1 1 auto; }
  .item-card { display: grid; grid-template-columns: auto 1fr; grid-template-areas: "icon info" "actions actions"; gap: 0.5rem 0.75rem; align-items: start; }
  .item-icon { grid-area: icon; }
  .item-info { grid-area: info; }
  .item-card-actions { grid-area: actions; justify-content: flex-end; flex-wrap: wrap; }
  .item-meta { flex-wrap: wrap; gap: 0.25rem 0.75rem; }
  .item-name { max-width: 100%; overflow: hidden; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; }
  .upload-details { flex-wrap: wrap; gap: 0.5rem; }
}

.content,.item-info { min-width:0; }
.item-name { overflow-wrap:anywhere; }
.item-card-actions { gap:6px; }
.category-item:focus-visible { outline:2px solid var(--accent); outline-offset:2px; }
@media(max-width:768px) {
 .sidebar .card { display:flex; align-items:center; gap:12px; }
 .sidebar .card-header { flex:none; gap:6px; margin:0; }
 .sidebar .card-header h3 { display:none; }
 .category-state { min-width:0; flex:1; }
 .category-list { min-width:0; flex:1; }
 .actions .el-button { margin-left:0; }
}

.item-card { min-height:82px; }
.item-icon { flex-shrink:0; }
.item-name { min-height:22.4px; }
.item-name .sk-title { height:22.4px; }
@media(max-width:768px) {
 .item-card { min-height:144px; }
 .item-name { min-height:44.8px; }
 .category-item .sk-meta { width:56px; height:22.4px; }
}
</style>
