<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { memberApi } from '@/api/member'
import { checkinApi } from '@/api/checkin'
import { useAuthStore } from '@/stores/auth'
import { ElMessage, ElMessageBox } from 'element-plus'

const authStore = useAuthStore()
const members = ref<Array<{
  id: number
  nickname: string
  username: string
  student_id: string
  role: string
  joined_at: string
  checkin_count: number
}>>([])
const onlineMembers = ref<Array<{ user_id: number; nickname: string; online: boolean }>>([])

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

async function handleTransferCaptain(id: number) {
  try {
    await ElMessageBox.confirm('确定要转让队长权限吗？此操作不可撤销。', '提示', { type: 'warning' })
    await memberApi.transferCaptain(id)
    ElMessage.success('转让成功')
    await loadMembers()
  } catch {
    // Cancelled
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

      <el-table :data="members" style="width: 100%">
        <el-table-column prop="nickname" label="昵称" width="150">
          <template #default="{ row }">
            <div class="member-info">
              <span class="online-dot" :class="{ online: isOnline(row.id) }"></span>
              <el-avatar :size="32">{{ row.nickname[0] }}</el-avatar>
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
              @click="handleTransferCaptain(row.id)"
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
    </div>
  </div>
</template>

<style scoped>
.members {
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
</style>
