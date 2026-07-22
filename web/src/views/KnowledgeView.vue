<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { knowledgeApi } from '@/api/knowledge'
import { useAuthStore } from '@/stores/auth'
import { ElMessage, ElMessageBox } from 'element-plus'

const authStore = useAuthStore()
const uploading = ref(false)
const downloading = ref<Record<number, number>>({})
const categories = ref<Array<{ id: number; name: string; is_system: boolean; count: number }>>([])
const items = ref<Array<{
  id: number
  type: string
  name: string
  url: string
  file_size: string
  category_id: number
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
const linkForm = ref({ name: '', url: '', category_id: 0 })
const categoryForm = ref({ name: '' })
const uploadForm = ref({ category_id: 0 })
const selectedFile = ref<File | null>(null)

onMounted(async () => {
  await loadCategories()
  await loadItems()
})

async function loadCategories() {
  try {
    const res = await knowledgeApi.listCategories()
    categories.value = res.data
  } catch {
    console.error('Failed to load categories')
  }
}

async function loadItems() {
  try {
    const res = await knowledgeApi.listItems(selectedCategory.value, searchQuery.value)
    items.value = res.data
  } catch {
    console.error('Failed to load items')
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
    await knowledgeApi.createLink(linkForm.value.name, linkForm.value.url, linkForm.value.category_id)
    if (authStore.isCaptain || authStore.isSuperAdmin) {
      ElMessage.success('创建成功')
    } else {
      ElMessage.success('创建成功，等待队长审核')
    }
    showLinkDialog.value = false
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

function handleFileChange(file: any) {
  selectedFile.value = file.raw
  return false
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
  uploading.value = true
  try {
    await knowledgeApi.uploadFile(selectedFile.value, uploadForm.value.category_id)
    if (authStore.isCaptain || authStore.isSuperAdmin) {
      ElMessage.success('上传成功')
    } else {
      ElMessage.success('上传成功，等待队长审核')
    }
    showUploadDialog.value = false
    selectedFile.value = null
    await loadItems()
    await loadCategories()
  } catch {
    ElMessage.error('上传失败')
  } finally {
    uploading.value = false
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
          <h3>知识库</h3>
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

    <el-dialog v-model="showUploadDialog" title="上传文件" width="500px">
      <el-form :model="uploadForm" label-width="80px">
        <el-form-item label="文件">
          <el-upload
            :auto-upload="false"
            :limit="1"
            :on-change="handleFileChange"
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
      </el-form>
      <template #footer>
        <el-button @click="showUploadDialog = false" :disabled="uploading">取消</el-button>
        <el-button type="primary" @click="handleUploadFile" :loading="uploading">
          {{ uploading ? '上传中...' : '上传' }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
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
</style>
