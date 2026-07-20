<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { ElMessage } from 'element-plus'

const router = useRouter()
const authStore = useAuthStore()

const showLogin = ref(false)
const showRegister = ref(false)

const loginForm = ref({ username: '', password: '' })
const registerForm = ref({ username: '', password: '', nickname: '' })

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
  try {
    await authStore.register(registerForm.value.username, registerForm.value.password, registerForm.value.nickname)
    showRegister.value = false
    ElMessage.success('注册成功')
    router.push('/app')
  } catch {
    ElMessage.error('注册失败，用户名可能已存在')
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
        <el-form-item label="用户名">
          <el-input v-model="loginForm.username" placeholder="请输入用户名" />
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
        <el-form-item label="用户名">
          <el-input v-model="registerForm.username" placeholder="请输入用户名" />
        </el-form-item>
        <el-form-item label="昵称">
          <el-input v-model="registerForm.nickname" placeholder="请输入昵称" />
        </el-form-item>
        <el-form-item label="密码">
          <el-input v-model="registerForm.password" type="password" placeholder="请输入密码" show-password />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showRegister = false">取消</el-button>
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
</style>
