<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { approvalApi } from '@/api/approval'
import { ElMessage } from 'element-plus'

const approvals = ref<Array<{
  id: number
  file_name: string
  file_size: string
  category_id: number
  uploader_name: string
  status: string
  created_at: string
}>>([])

onMounted(async () => {
  await loadApprovals()
})

async function loadApprovals() {
  try {
    const res = await approvalApi.listPending()
    approvals.value = res.data
  } catch {
    console.error('Failed to load approvals')
  }
}

async function handleApprove(id: number) {
  try {
    await approvalApi.approve(id)
    ElMessage.success('已批准')
    await loadApprovals()
  } catch {
    ElMessage.error('操作失败')
  }
}

async function handleReject(id: number) {
  try {
    await approvalApi.reject(id)
    ElMessage.success('已拒绝')
    await loadApprovals()
  } catch {
    ElMessage.error('操作失败')
  }
}
</script>

<template>
  <div class="approvals">
    <div class="card">
      <div class="card-header">
        <h3>审批管理</h3>
        <span class="pending-count">{{ approvals.length }} 条待审批</span>
      </div>

      <el-table :data="approvals" style="width: 100%">
        <el-table-column prop="file_name" label="文件名" min-width="200" />
        <el-table-column prop="file_size" label="大小" width="100" />
        <el-table-column prop="uploader_name" label="上传者" width="120" />
        <el-table-column prop="created_at" label="上传时间" width="120" />
        <el-table-column label="操作" width="200">
          <template #default="{ row }">
            <el-button size="small" type="success" @click="handleApprove(row.id)">
              批准
            </el-button>
            <el-button size="small" type="danger" @click="handleReject(row.id)">
              拒绝
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-empty v-if="approvals.length === 0" description="暂无待审批项目" />
    </div>
  </div>
</template>

<style scoped>
.approvals {
  max-width: 1000px;
}

.card {
  background: var(--bg-card);
  border: 1px solid var(--border);
  border-radius: 12px;
  padding: 1.5rem;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1.5rem;
}

.card-header h3 {
  font-size: 1rem;
  font-weight: 600;
}

.pending-count {
  color: var(--text-muted);
  font-size: 0.875rem;
}
</style>
