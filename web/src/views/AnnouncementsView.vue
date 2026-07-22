<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { announcementApi } from '@/api/announcement'
import { useAuthStore } from '@/stores/auth'
import { ElMessage, ElMessageBox } from 'element-plus'

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

async function handleSubmit() {
  try {
    if (editingId.value) {
      await announcementApi.update(editingId.value, form.value.title, form.value.content, form.value.pinned)
      ElMessage.success('更新成功')
    } else {
      await announcementApi.create(form.value.title, form.value.content, form.value.pinned)
      ElMessage.success('创建成功')
    }
    showDialog.value = false
    await loadAnnouncements()
  } catch {
    ElMessage.error('操作失败')
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
              <span>{{ ann.title }}</span>
            </div>
            <div class="ann-meta">
              <span>{{ ann.author_name }}</span>
              <span>{{ ann.created_at }}</span>
            </div>
          </div>
          <div class="ann-content">{{ ann.content }}</div>
          <div v-if="authStore.isCaptain" class="ann-actions">
            <el-button size="small" @click="openEdit(ann)">编辑</el-button>
            <el-button size="small" type="danger" @click="handleDelete(ann.id)">删除</el-button>
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

    <el-dialog v-model="showDialog" :title="editingId ? '编辑公告' : '发布公告'" width="600px">
      <el-form :model="form" label-width="80px">
        <el-form-item label="标题">
          <el-input v-model="form.title" placeholder="请输入公告标题" />
        </el-form-item>
        <el-form-item label="内容">
          <el-input v-model="form.content" type="textarea" :rows="6" placeholder="请输入公告内容" />
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
}

.ann-meta {
  display: flex;
  gap: 1rem;
  font-size: 0.75rem;
  color: var(--text-muted);
}

.ann-content {
  font-size: 0.875rem;
  color: var(--text-secondary);
  margin-bottom: 0.75rem;
  line-height: 1.5;
}

.ann-actions {
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
