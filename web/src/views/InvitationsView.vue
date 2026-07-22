<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { invitationApi } from '@/api/invitation'
import { useAuthStore } from '@/stores/auth'
import { ElMessage } from 'element-plus'

const authStore = useAuthStore()
const codes = ref<Array<{
  id: number
  code: string
  type: string
  used: boolean
  expires_at: string
  created_at: string
}>>([])

const showGenerateDialog = ref(false)
const generateType = ref('member')

onMounted(async () => {
  await loadCodes()
})

async function loadCodes() {
  try {
    const res = await invitationApi.getMyCodes()
    codes.value = res.data || []
  } catch {
    console.error('Failed to load invitation codes')
  }
}

async function handleGenerate() {
  try {
    const res = await invitationApi.generateCode(generateType.value)
    ElMessage.success(`邀请码已生成: ${res.data.code}`)
    showGenerateDialog.value = false
    await loadCodes()
  } catch {
    ElMessage.error('生成邀请码失败')
  }
}

function copyCode(code: string) {
  navigator.clipboard.writeText(code)
  ElMessage.success('已复制到剪贴板')
}
</script>

<template>
  <div class="invitations">
    <div class="card">
      <div class="card-header">
        <h3>邀请码管理</h3>
        <el-button type="primary" @click="showGenerateDialog = true">
          生成邀请码
        </el-button>
      </div>

      <el-table :data="codes" style="width: 100%">
        <el-table-column prop="code" label="邀请码" width="120">
          <template #default="{ row }">
            <span class="code-text">{{ row.code }}</span>
            <el-button size="small" link @click="copyCode(row.code)">
              复制
            </el-button>
          </template>
        </el-table-column>
        <el-table-column prop="type" label="类型" width="100">
          <template #default="{ row }">
            <el-tag :type="row.type === 'captain' ? 'warning' : 'info'">
              {{ row.type === 'captain' ? '队长' : '成员' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="100">
          <template #default="{ row }">
            <el-tag v-if="row.used" type="success">已使用</el-tag>
            <el-tag v-else-if="new Date(row.expires_at) < new Date()" type="info">已过期</el-tag>
            <el-tag v-else type="warning">未使用</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="expires_at" label="过期时间" width="180" />
        <el-table-column prop="created_at" label="创建时间" width="180" />
      </el-table>
    </div>

    <el-dialog v-model="showGenerateDialog" title="生成邀请码" width="400px">
      <el-form label-width="80px">
        <el-form-item label="邀请类型">
          <el-radio-group v-model="generateType">
            <el-radio value="captain" :disabled="!authStore.isSuperAdmin">队长</el-radio>
            <el-radio value="member">成员</el-radio>
          </el-radio-group>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showGenerateDialog = false">取消</el-button>
        <el-button type="primary" @click="handleGenerate">生成</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.invitations {
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

.code-text {
  font-family: monospace;
  font-weight: 600;
  margin-right: 0.5rem;
}
</style>
