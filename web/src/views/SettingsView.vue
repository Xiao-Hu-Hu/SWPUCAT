<script setup lang="ts">
import { ref } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { ElMessage } from 'element-plus'

const authStore = useAuthStore()

const profileForm = ref({
  nickname: authStore.user?.nickname || ''
})

function handleSaveProfile() {
  ElMessage.success('保存成功')
}
</script>

<template>
  <div class="settings">
    <div class="card">
      <h3>个人设置</h3>
      <el-form :model="profileForm" label-width="100px" style="max-width: 500px">
        <el-form-item label="用户名">
          <el-input :value="authStore.user?.username" disabled />
        </el-form-item>
        <el-form-item label="昵称">
          <el-input v-model="profileForm.nickname" placeholder="请输入昵称" />
        </el-form-item>
        <el-form-item label="角色">
          <el-tag :type="authStore.isCaptain ? 'warning' : 'info'">
            {{ authStore.isCaptain ? '队长' : '成员' }}
          </el-tag>
        </el-form-item>
        <el-form-item label="加入时间">
          <span>{{ authStore.user?.joined_at }}</span>
        </el-form-item>
        <el-form-item label="签到次数">
          <span>{{ authStore.user?.checkin_count }}</span>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="handleSaveProfile">保存</el-button>
        </el-form-item>
      </el-form>
    </div>
  </div>
</template>

<style scoped>
.settings {
  max-width: 600px;
}

.card {
  background: var(--bg-card);
  border: 1px solid var(--border);
  border-radius: 12px;
  padding: 1.5rem;
}

.card h3 {
  font-size: 1rem;
  font-weight: 600;
  margin-bottom: 1.5rem;
}
</style>
