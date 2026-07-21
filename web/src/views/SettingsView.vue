<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { userApi } from '@/api/user'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useRouter } from 'vue-router'

const authStore = useAuthStore()
const router = useRouter()
const email = ref('')
const showPasswordDialog = ref(false)

const passwordForm = ref({
  oldPassword: '',
  newPassword: '',
  confirmPassword: '',
  verificationCode: ''
})

const countdown = ref(0)
let timer: ReturnType<typeof setInterval> | null = null

const passwordSame = ref(false)
const nickname = ref(authStore.user?.nickname || '')

function checkPasswordSame() {
  passwordSame.value = passwordForm.value.oldPassword.length > 0 &&
    passwordForm.value.newPassword.length > 0 &&
    passwordForm.value.oldPassword === passwordForm.value.newPassword
}

onMounted(async () => {
  try {
    const res = await userApi.getProfile()
    email.value = res.data.email || ''
  } catch {
    console.error('Failed to load profile')
  }
})

async function handleSaveProfile() {
  try {
    await userApi.updateProfile(nickname.value)
    if (authStore.user) {
      authStore.user.nickname = nickname.value
      localStorage.setItem('user', JSON.stringify(authStore.user))
    }
    ElMessage.success('保存成功')
  } catch {
    ElMessage.error('保存失败')
  }
}

function openPasswordDialog() {
  passwordForm.value = {
    oldPassword: '',
    newPassword: '',
    confirmPassword: '',
    verificationCode: ''
  }
  countdown.value = 0
  if (timer) {
    clearInterval(timer)
    timer = null
  }
  showPasswordDialog.value = true
}

async function handleSendCode() {
  if (!email.value) {
    ElMessage.warning('未绑定邮箱')
    return
  }
  if (countdown.value > 0) return

  try {
    await userApi.sendVerificationCode(email.value)
    ElMessage.success('验证码已发送')
    countdown.value = 60
    timer = setInterval(() => {
      countdown.value--
      if (countdown.value <= 0 && timer) {
        clearInterval(timer)
        timer = null
      }
    }, 1000)
  } catch {
    ElMessage.error('发送验证码失败')
  }
}

async function handleChangePassword() {
  if (!passwordForm.value.oldPassword) {
    ElMessage.warning('请输入旧密码')
    return
  }
  if (!passwordForm.value.newPassword) {
    ElMessage.warning('请输入新密码')
    return
  }
  if (passwordForm.value.newPassword.length < 6) {
    ElMessage.warning('新密码长度不能少于6位')
    return
  }
  if (passwordForm.value.oldPassword === passwordForm.value.newPassword) {
    ElMessage.warning('新密码不能与旧密码相同')
    return
  }
  if (passwordForm.value.newPassword !== passwordForm.value.confirmPassword) {
    ElMessage.warning('两次输入的密码不一致')
    return
  }
  if (!passwordForm.value.verificationCode) {
    ElMessage.warning('请输入验证码')
    return
  }

  try {
    await userApi.changePassword(
      passwordForm.value.oldPassword,
      passwordForm.value.newPassword,
      passwordForm.value.verificationCode
    )
    showPasswordDialog.value = false
    await ElMessageBox.alert('密码修改成功，请重新登录', '提示', {
      confirmButtonText: '确定',
      type: 'success'
    })
    await authStore.logout()
    router.push('/login')
  } catch (err: any) {
    ElMessage.error(err.response?.data?.message || '修改失败')
  }
}
</script>

<template>
  <div class="settings">
    <div class="card">
      <h3>个人信息</h3>
      <el-form label-width="100px" style="max-width: 500px">
        <el-form-item label="用户名">
          <el-input :value="authStore.user?.username" disabled />
        </el-form-item>
        <el-form-item label="昵称">
          <el-input v-model="nickname" placeholder="请输入昵称" />
        </el-form-item>
        <el-form-item label="邮箱">
          <el-input :value="email" disabled />
        </el-form-item>
        <el-form-item label="角色">
          <el-tag :type="authStore.isSuperAdmin ? 'danger' : authStore.isCaptain ? 'warning' : 'info'">
            {{ authStore.isSuperAdmin ? '超级管理员' : authStore.isCaptain ? '队长' : '成员' }}
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
          <el-button @click="openPasswordDialog">修改密码</el-button>
        </el-form-item>
      </el-form>
    </div>

    <el-dialog v-model="showPasswordDialog" title="修改密码" width="500px">
      <el-form :model="passwordForm" label-width="100px">
        <el-form-item label="旧密码">
          <el-input
            v-model="passwordForm.oldPassword"
            type="password"
            placeholder="请输入旧密码"
            show-password
            @input="checkPasswordSame"
          />
        </el-form-item>
        <el-form-item label="新密码">
          <el-input
            v-model="passwordForm.newPassword"
            type="password"
            placeholder="请输入新密码（至少6位）"
            show-password
            @input="checkPasswordSame"
          />
          <div v-if="passwordSame" class="warning-text">新密码不能与旧密码相同</div>
        </el-form-item>
        <el-form-item label="确认密码">
          <el-input
            v-model="passwordForm.confirmPassword"
            type="password"
            placeholder="请再次输入新密码"
            show-password
          />
        </el-form-item>
        <el-form-item label="验证码">
          <div class="code-input">
            <el-input
              v-model="passwordForm.verificationCode"
              placeholder="请输入邮箱验证码"
              maxlength="6"
            />
            <el-button
              type="primary"
              :disabled="countdown > 0"
              @click="handleSendCode"
            >
              {{ countdown > 0 ? `${countdown}秒后重试` : '发送验证码' }}
            </el-button>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showPasswordDialog = false">取消</el-button>
        <el-button type="primary" @click="handleChangePassword">确认修改</el-button>
      </template>
    </el-dialog>
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

.code-input {
  display: flex;
  gap: 0.5rem;
  width: 100%;
}

.code-input .el-input {
  flex: 1;
}

.warning-text {
  color: #e6a23c;
  font-size: 12px;
  margin-top: 4px;
}
</style>
