<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { authApi } from '@/api/auth'
import { ElMessage } from 'element-plus'

const router = useRouter()
const authStore = useAuthStore()

const showLogin = ref(false)
const showRegister = ref(false)

const loginForm = ref({ username: '', password: '' })
const registerForm = ref({ username: '', password: '', nickname: '', email: '', verificationCode: '' })

// Captcha state
const captchaPassed = ref(false)
const captchaSliderValue = ref(0)
const captchaTarget = ref(Math.floor(Math.random() * 200) + 50)

// Verification code countdown
const codeCountdown = ref(0)
const canSendCode = computed(() => codeCountdown.value === 0 && registerForm.value.email)

let countdownTimer: number | null = null

function startCountdown() {
  codeCountdown.value = 60
  countdownTimer = window.setInterval(() => {
    codeCountdown.value--
    if (codeCountdown.value <= 0) {
      codeCountdown.value = 0
      if (countdownTimer) {
        clearInterval(countdownTimer)
        countdownTimer = null
      }
    }
  }, 1000)
}

async function handleSendCode() {
  if (!registerForm.value.email) {
    ElMessage.warning('请输入邮箱')
    return
  }
  try {
    await authApi.sendVerificationCode(registerForm.value.email)
    ElMessage.success('验证码已发送')
    startCountdown()
  } catch {
    ElMessage.error('发送验证码失败')
  }
}

function handleCaptchaChange(value: number) {
  if (Math.abs(value - captchaTarget.value) < 10) {
    captchaPassed.value = true
    ElMessage.success('验证通过')
  }
}

function resetCaptcha() {
  captchaPassed.value = false
  captchaSliderValue.value = 0
  captchaTarget.value = Math.floor(Math.random() * 200) + 50
}

async function handleLogin() {
  try {
    await authStore.login(loginForm.value.username, loginForm.value.password)
    showLogin.value = false
    ElMessage.success('登录成功')
    router.push('/app')
  } catch {
    ElMessage.error('登录失败，请检查用户名和密码')
  }
}

async function handleRegister() {
  if (!captchaPassed.value) {
    ElMessage.warning('请先完成滑块验证')
    return
  }
  try {
    await authStore.register(
      registerForm.value.username,
      registerForm.value.password,
      registerForm.value.nickname,
      registerForm.value.email,
      registerForm.value.verificationCode
    )
    showRegister.value = false
    ElMessage.success('注册成功')
    router.push('/app')
  } catch {
    ElMessage.error('注册失败，请检查输入信息')
  }
}
</script>

<template>
  <div class="home">
    <header class="hero">
      <div class="hero-content">
        <h1 class="title">SWPUCAT</h1>
        <p class="subtitle">软件工作室成员管理平台</p>
        <div class="actions">
          <el-button type="primary" size="large" @click="showLogin = true">
            登录
          </el-button>
          <el-button size="large" @click="showRegister = true">
            注册
          </el-button>
        </div>
      </div>
    </header>

    <section class="features">
      <div class="feature-grid">
        <div class="feature-card">
          <el-icon :size="48" color="var(--accent)"><Clock /></el-icon>
          <h3>签到打卡</h3>
          <p>便捷的签到打卡功能，记录每日出勤</p>
        </div>
        <div class="feature-card">
          <el-icon :size="48" color="var(--violet)"><Bell /></el-icon>
          <h3>公告管理</h3>
          <p>及时发布和查看工作室公告</p>
        </div>
        <div class="feature-card">
          <el-icon :size="48" color="var(--success)"><Folder /></el-icon>
          <h3>知识库</h3>
          <p>共享学习资料和项目文档</p>
        </div>
        <div class="feature-card">
          <el-icon :size="48" color="var(--warning)"><User /></el-icon>
          <h3>成员管理</h3>
          <p>管理工作室成员信息</p>
        </div>
      </div>
    </section>

    <!-- Login Dialog -->
    <el-dialog v-model="showLogin" title="登录" width="400px">
      <el-form :model="loginForm" label-width="80px">
        <el-form-item label="学号">
          <el-input v-model="loginForm.username" placeholder="请输入12位学号" maxlength="12" />
        </el-form-item>
        <el-form-item label="密码">
          <el-input v-model="loginForm.password" type="password" placeholder="请输入密码" show-password />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showLogin = false">取消</el-button>
        <el-button type="primary" @click="handleLogin">登录</el-button>
      </template>
    </el-dialog>

    <!-- Register Dialog -->
    <el-dialog v-model="showRegister" title="注册" width="400px">
      <el-form :model="registerForm" label-width="80px">
        <el-form-item label="学号">
          <el-input v-model="registerForm.username" placeholder="请输入12位学号 (如: 202431060420)" maxlength="12" />
        </el-form-item>
        <el-form-item label="昵称">
          <el-input v-model="registerForm.nickname" placeholder="请输入昵称" />
        </el-form-item>
        <el-form-item label="密码">
          <el-input v-model="registerForm.password" type="password" placeholder="请输入密码" show-password />
        </el-form-item>
        <el-form-item label="邮箱">
          <el-input v-model="registerForm.email" placeholder="请输入邮箱" />
        </el-form-item>
        <el-form-item label="验证码">
          <div class="code-input">
            <el-input v-model="registerForm.verificationCode" placeholder="请输入6位验证码" maxlength="6" />
            <el-button
              type="primary"
              :disabled="!canSendCode"
              @click="handleSendCode"
            >
              {{ codeCountdown > 0 ? `${codeCountdown}秒后重发` : '发送验证码' }}
            </el-button>
          </div>
        </el-form-item>
        <el-form-item label="验证">
          <div class="captcha-container" v-if="!captchaPassed">
            <div class="captcha-track">
              <div class="captcha-slider" :style="{ left: captchaSliderValue + 'px' }"></div>
              <div class="captcha-target" :style="{ left: captchaTarget + 'px' }"></div>
            </div>
            <el-slider
              v-model="captchaSliderValue"
              :max="300"
              :show-tooltip="false"
              @change="handleCaptchaChange"
            />
            <p class="captcha-hint">拖动滑块到目标位置完成验证</p>
          </div>
          <div v-else class="captcha-success">
            <el-tag type="success">验证通过</el-tag>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showRegister = false; resetCaptcha()">取消</el-button>
        <el-button type="primary" @click="handleRegister">注册</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.home {
  min-height: 100vh;
}

.hero {
  min-height: 60vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, var(--bg-deep) 0%, #0f172a 100%);
}

.hero-content {
  text-align: center;
}

.title {
  font-size: 4rem;
  font-weight: 800;
  background: linear-gradient(135deg, var(--accent), var(--violet));
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  margin-bottom: 1rem;
}

.subtitle {
  font-size: 1.5rem;
  color: var(--text-muted);
  margin-bottom: 2rem;
}

.actions {
  display: flex;
  gap: 1rem;
  justify-content: center;
}

.features {
  padding: 4rem 2rem;
  background: var(--bg-card);
}

.feature-grid {
  max-width: 1200px;
  margin: 0 auto;
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
  gap: 2rem;
}

.feature-card {
  background: var(--bg-deep);
  border: 1px solid var(--border);
  border-radius: 12px;
  padding: 2rem;
  text-align: center;
  transition: transform 0.2s;
}

.feature-card:hover {
  transform: translateY(-4px);
}

.feature-card h3 {
  margin: 1rem 0 0.5rem;
  font-size: 1.25rem;
}

.feature-card p {
  color: var(--text-muted);
}

.code-input {
  display: flex;
  gap: 0.5rem;
  width: 100%;
}

.code-input .el-input {
  flex: 1;
}

.captcha-container {
  width: 100%;
}

.captcha-track {
  position: relative;
  height: 30px;
  background: var(--bg-deep);
  border-radius: 4px;
  margin-bottom: 0.5rem;
  overflow: hidden;
}

.captcha-slider {
  position: absolute;
  top: 0;
  width: 30px;
  height: 100%;
  background: var(--accent);
  border-radius: 4px;
  transition: left 0.1s;
}

.captcha-target {
  position: absolute;
  top: 0;
  width: 30px;
  height: 100%;
  background: var(--success);
  border-radius: 4px;
  opacity: 0.5;
}

.captcha-hint {
  font-size: 0.75rem;
  color: var(--text-muted);
  margin-top: 0.25rem;
}

.captcha-success {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}
</style>
