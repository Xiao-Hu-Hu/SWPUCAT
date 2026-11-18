<script setup lang="ts">
import { ref, onMounted, computed, onUnmounted } from 'vue'
import { invitationApi } from '@/api/invitation'
import { useAuthStore } from '@/stores/auth'
import { ElMessage } from 'element-plus'

const loading = ref(true)
const loadError = ref('')
const authStore = useAuthStore()
const codes = ref<Array<{
  id: number
  code: string
  type: string
  used: boolean
  expires_at: string
  created_at: string
}>>([])

const statusFilter = ref('valid')
const currentTime = ref(Date.now())
const clockTimer = window.setInterval(() => { currentTime.value = Date.now() }, 30000)
onUnmounted(() => window.clearInterval(clockTimer))
function codeStatus(code: { used: boolean; expires_at: string }) { return code.used ? 'used' : new Date(code.expires_at).getTime() <= currentTime.value ? 'expired' : 'valid' }
const filteredCodes = computed(() => codes.value.filter(code => !statusFilter.value || codeStatus(code) === statusFilter.value))
function formatDate(value: string) { const date = new Date(value); return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString('zh-CN', { hour12:false }) }
const showGenerateDialog = ref(false)
const generateType = ref('member')

onMounted(async () => {
  await loadCodes()
})

async function loadCodes() {
  loading.value = true
  loadError.value = ''
  try {
    const res = await invitationApi.getMyCodes()
    codes.value = res.data || []
  } catch {
    loadError.value = '内容加载失败，请检查网络后重试'
  } finally {
    loading.value = false
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

async function copyCode(code: string) {
  try { await navigator.clipboard.writeText(code); ElMessage.success('已复制到剪贴板') } catch { ElMessage.error('复制失败，请手动选择邀请码复制') }
}
</script>

<template>
  <div class="invitations">
    <div class="card">
      <div class="card-header">
        <h3>成员邀请</h3>
        <el-button type="primary" @click="showGenerateDialog = true">
          生成邀请码
        </el-button>
      </div>

      <el-radio-group v-model="statusFilter" class="code-filter" aria-label="邀请码状态"><el-radio-button value="valid">有效</el-radio-button><el-radio-button value="used">已使用</el-radio-button><el-radio-button value="expired">已过期</el-radio-button><el-radio-button value="">全部</el-radio-button></el-radio-group>
      <ContentState :loading="loading" :error="loadError" :has-content="codes.length > 0" compact @retry="loadCodes">
      <template #skeleton>
        <TableSkeleton :columns="[{label:'邀请码',minWidth:170},{label:'类型',width:100},{label:'状态',width:100},{label:'过期时间',minWidth:190},{label:'创建时间',minWidth:190}]" :row-height="48" />
      </template>
      <el-table :data="filteredCodes" empty-text="当前分类暂无邀请码" style="width: 100%">
        <el-table-column prop="code" label="邀请码" min-width="170">
          <template #default="{ row }">
            <span class="code-text">{{ row.code }}</span>
            <el-button size="small" link :disabled="codeStatus(row) !== 'valid'" @click="copyCode(row.code)">
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
            <el-tag v-else-if="codeStatus(row) === 'expired'" type="info">已过期</el-tag>
            <el-tag v-else type="warning">未使用</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="expires_at" label="过期时间" min-width="190"><template #default="{ row }">{{ formatDate(row.expires_at) }}</template></el-table-column>
        <el-table-column prop="created_at" label="创建时间" min-width="190"><template #default="{ row }">{{ formatDate(row.created_at) }}</template></el-table-column>
      </el-table>
      </ContentState>
    </div>

    <el-dialog align-center v-model="showGenerateDialog" title="生成邀请码" width="400px">
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
.code-filter { margin-bottom:20px; }
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
@media (max-width: 768px) {
  .card { padding: 0.875rem; }
  .card-header { flex-wrap: wrap; gap: 0.5rem; margin-bottom: 1rem; }
}
</style>
