<script setup lang="ts">
import { ref, computed, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { authApi } from '@/api/auth'
import { ElMessage } from 'element-plus'

const router = useRouter()
const authStore = useAuthStore()

const loginLoading = ref(false)
const loginError = ref('')
const showRegister = ref(false)

const loginForm = ref({ username: '', password: '' })
const registerForm = ref({ username: '', password: '', nickname: '', email: '', verificationCode: '', invitationCode: '' })

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
  if (loginLoading.value) return
  loginError.value = ''
  if (!/^\d{12}$/.test(loginForm.value.username.trim())) {
    loginError.value = '请输入12位数字学号'
    return
  }
  if (!loginForm.value.password) {
    loginError.value = '请输入密码'
    return
  }
  loginLoading.value = true
  try {
    await authStore.login(loginForm.value.username, loginForm.value.password)
    ElMessage.success('登录成功')
    router.push('/app')
  } catch {
    loginError.value = '登录失败，请检查学号和密码后重试'
  } finally {
    loginLoading.value = false
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
      registerForm.value.verificationCode,
      registerForm.value.invitationCode
    )
    showRegister.value = false
    ElMessage.success('注册成功')
    router.push('/app')
  } catch {
    ElMessage.error('注册失败，请检查输入信息')
  }
}
onUnmounted(() => { if (countdownTimer) clearInterval(countdownTimer) })
</script>

<template>
  <main class="login-page">
    <section class="login-intro">
      <img src="/association-logo.png" alt="西南石油大学计算机协会徽标" class="association-logo" />
      <p class="brand-name">西南石油大学计算机协会</p>
      <h1>把热爱，<br />写进每一天。</h1>
      <p>签到、协作、分享。让每一次投入都有记录，<br />让每一份经验都能传递。</p>
      <span class="intro-note">SWPUCAT · 成员协作平台</span>
    </section>
    <section class="login-card" aria-labelledby="login-title">
      <p class="login-eyebrow">MEMBER WORKSPACE</p>
      <h2 id="login-title">登录成员平台</h2>
      <p class="login-description">使用你的学号与密码继续</p>
      <el-form :model="loginForm" label-position="top" @submit.prevent="handleLogin">
        <el-form-item label="学号">
          <el-input v-model="loginForm.username" aria-label="学号" placeholder="请输入12位学号" maxlength="12" autocomplete="username" inputmode="numeric" size="large" />
        </el-form-item>
        <el-form-item label="密码">
          <el-input v-model="loginForm.password" aria-label="密码" type="password" placeholder="请输入密码" show-password autocomplete="current-password" size="large" />
        </el-form-item>
        <p v-if="loginError" class="login-error" role="alert">{{ loginError }}</p>
        <el-button class="login-submit" type="primary" size="large" native-type="submit" :loading="loginLoading">{{ loginLoading ? '正在登录' : '登录' }}</el-button>
      </el-form>
      <div class="register-entry">还没有账号？<el-button link type="primary" @click="showRegister = true">使用邀请码注册</el-button></div>
    </section>
    <!-- Register Dialog -->
    <el-dialog align-center v-model="showRegister" title="注册" width="400px">
      <el-form :model="registerForm" label-width="80px">
        <el-form-item label="邀请码">
          <el-input v-model="registerForm.invitationCode" placeholder="请输入6位邀请码" maxlength="6" />
        </el-form-item>
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
  </main>
</template>

<style scoped>
 .login-page { min-height:100dvh; display:grid; grid-template-columns:1.1fr 1fr; align-items:center; gap:clamp(40px,8vw,120px); max-width:1160px; margin:auto; padding:64px 40px; }
.login-intro { color:var(--text-secondary); }
.association-logo { width:64px; height:64px; object-fit:contain; margin-bottom:20px; }
.brand-name { font-weight:600; color:var(--text); }
.login-intro h1 { font-size:clamp(36px,4vw,54px); line-height:1.35; letter-spacing:-.04em; margin:28px 0 20px; color:#183b56; }
.login-intro > p { line-height:1.9; }
.intro-note { display:block; margin-top:48px; font-size:12px; letter-spacing:.08em; color:var(--text-muted); }
.login-card { padding:40px; background:white; border:1px solid var(--border); border-radius:20px; box-shadow:0 16px 48px #1645690a; }
.login-eyebrow { font-size:11px; color:var(--accent); letter-spacing:.16em; font-weight:600; }
.login-card h2 { font-size:26px; margin:12px 0 6px; }
.login-description { color:var(--text-muted); margin-bottom:28px; }
.login-submit { width:100%; margin-top:8px; }
.login-error { color:var(--danger); margin-bottom:16px; }
.register-entry { text-align:center; color:var(--text-muted); margin-top:24px; font-size:13px; }
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
@media (max-width:768px) {
 .login-page { grid-template-columns:1fr; grid-template-rows:auto auto; align-content:center; gap:28px; padding:32px 20px; max-width:500px; }
 .login-intro { text-align:center; }
 .association-logo { width:48px; height:48px; margin-bottom:8px; }
 .login-intro h1,.login-intro > p:not(.brand-name),.intro-note { display:none; }
 .login-card { padding:28px 24px; }
}
</style>
