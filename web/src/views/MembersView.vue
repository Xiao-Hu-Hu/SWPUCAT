<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { memberApi } from '@/api/member'
import { checkinApi } from '@/api/checkin'
import { userApi } from '@/api/user'
import { useAuthStore } from '@/stores/auth'
import { ElMessage, ElMessageBox } from 'element-plus'

const authStore = useAuthStore()
const members = ref<Array<{
  id: number
  nickname: string
  username: string
  student_id: string
  avatar: string
  role: string
  joined_at: string
  checkin_count: number
}>>([])
const onlineMembers = ref<Array<{ user_id: number; nickname: string; online: boolean }>>([])

const currentPage = ref(1)
const pageSize = ref(10)

const paginatedMembers = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value
  return members.value.slice(start, start + pageSize.value)
})

const showTransferDialog = ref(false)
const transferTargetId = ref(0)
const transferTargetName = ref('')
const transferCode = ref('')
const transferCountdown = ref(0)
let transferTimer: ReturnType<typeof setInterval> | null = null

onMounted(async () => {
  await loadMembers()
  await loadOnlineMembers()
})

async function loadMembers() {
  try {
    const res = await memberApi.list()
    members.value = res.data
  } catch {
    console.error('Failed to load members')
  }
}

async function loadOnlineMembers() {
  try {
    const res = await checkinApi.getOnlineMembers()
    onlineMembers.value = res.data || []
  } catch {
    console.error('Failed to load online members')
  }
}

function isOnline(userId: number) {
  return onlineMembers.value.find(m => m.user_id === userId)?.online || false
}

function openTransferDialog(id: number, nickname: string) {
  transferTargetId.value = id
  transferTargetName.value = nickname
  transferCode.value = ''
  transferCountdown.value = 0
  if (transferTimer) {
    clearInterval(transferTimer)
    transferTimer = null
  }
  showTransferDialog.value = true
}

async function handleSendTransferCode() {
  if (transferCountdown.value > 0) return
  try {
    const res = await userApi.getProfile()
    const email = res.data.email
    if (!email) {
      ElMessage.warning('未绑定邮箱')
      return
    }
    await userApi.sendVerificationCode(email)
    ElMessage.success('验证码已发送')
    transferCountdown.value = 60
    transferTimer = setInterval(() => {
      transferCountdown.value--
      if (transferCountdown.value <= 0 && transferTimer) {
        clearInterval(transferTimer)
        transferTimer = null
      }
    }, 1000)
  } catch {
    ElMessage.error('发送验证码失败')
  }
}

async function handleConfirmTransfer() {
  if (!transferCode.value) {
    ElMessage.warning('请输入验证码')
    return
  }
  try {
    await memberApi.transferCaptain(transferTargetId.value, transferCode.value)
    showTransferDialog.value = false
    await ElMessageBox.alert('队长权限已转让，请重新登录以更新权限。', '转让成功', {
      confirmButtonText: '确定',
      type: 'success'
    })
    await authStore.logout()
    window.location.href = '/'
  } catch (err: any) {
    ElMessage.error(err.response?.data?.message || '转让失败')
  }
}

async function handleRemoveMember(id: number) {
  try {
    await ElMessageBox.confirm('确定要移除该成员吗？', '提示', { type: 'warning' })
    await memberApi.removeMember(id)
    ElMessage.success('移除成功')
    await loadMembers()
  } catch {
    // Cancelled
  }
}
</script>

<template>
  <div class="members">
    <div class="card">
      <div class="card-header">
        <h3>成员管理</h3>
        <span class="member-count">共 {{ members.length }} 人</span>
      </div>

      <el-table :data="paginatedMembers" style="width: 100%">
        <el-table-column prop="nickname" label="昵称" width="150">
          <template #default="{ row }">
            <div class="member-info">
              <span class="online-dot" :class="{ online: isOnline(row.id) }"></span>
              <el-avatar :size="32" :src="row.avatar ? `/api/avatar/${row.avatar}` : undefined">{{ row.nickname[0] }}</el-avatar>
              <span>{{ row.nickname }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="student_id" label="学号" width="140" />
        <el-table-column prop="role" label="角色" width="100">
          <template #default="{ row }">
            <el-tag :type="row.role === 'captain' ? 'warning' : 'info'">
              {{ row.role === 'captain' ? '队长' : '成员' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="joined_at" label="加入时间" width="120" />
        <el-table-column prop="checkin_count" label="签到次数" width="100" />
        <el-table-column label="操作" width="200" v-if="authStore.isCaptain">
          <template #default="{ row }">
            <el-button
              v-if="row.role !== 'captain'"
              size="small"
              @click="openTransferDialog(row.id, row.nickname)"
            >
              转让队长
            </el-button>
            <el-button
              v-if="row.role !== 'captain'"
              size="small"
              type="danger"
              @click="handleRemoveMember(row.id)"
            >
              移除
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="pagination-wrapper" v-if="members.length > 0">
        <el-pagination
          v-model:current-page="currentPage"
          v-model:page-size="pageSize"
          :page-sizes="[5, 10, 20, 50]"
          :total="members.length"
          layout="total, sizes, prev, pager, next, jumper"
          background
        />
      </div>
    </div>

    <el-dialog v-model="showTransferDialog" title="转让队长权限" width="450px">
      <p style="margin-bottom: 1rem;">确定要将队长权限转让给 <strong>{{ transferTargetName }}</strong> 吗？此操作不可撤销。</p>
      <el-form label-width="80px">
        <el-form-item label="验证码">
          <div class="code-input">
            <el-input
              v-model="transferCode"
              placeholder="请输入邮箱验证码"
              maxlength="6"
            />
            <el-button
              type="primary"
              :disabled="transferCountdown > 0"
              @click="handleSendTransferCode"
            >
              {{ transferCountdown > 0 ? `${transferCountdown}秒后重试` : '发送验证码' }}
            </el-button>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showTransferDialog = false">取消</el-button>
        <el-button type="warning" @click="handleConfirmTransfer">确认转让</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.members {
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

.member-count {
  color: var(--text-muted);
  font-size: 0.875rem;
}

.member-info {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.online-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--text-muted);
}

.online-dot.online {
  background: var(--success);
  box-shadow: 0 0 6px var(--success);
}

.code-input {
  display: flex;
  gap: 0.5rem;
  width: 100%;
}

.code-input .el-input {
  flex: 1;
}

.pagination-wrapper {
  display: flex;
  justify-content: center;
  margin-top: 1.25rem;
  padding-top: 1rem;
  border-top: 1px solid var(--border);
}
</style>
