<script setup lang="ts">
import { nextTick, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { agentApi } from '@/api/agent'
import { useAuthStore } from '@/stores/auth'

interface ChatMessage {
  role: 'user' | 'assistant'
  content: string
}

const authStore = useAuthStore()
const input = ref('')
const loading = ref(false)
const messages = ref<ChatMessage[]>([
  { role: 'assistant', content: '你好，我可以协助查询知识库、查看打卡状态，也可以按你的请求执行打卡操作。' }
])
const sessionId = ref('')
const messagesRef = ref<HTMLElement | null>(null)

const sessionStorageKey = () => `swpucat-agent-session:${authStore.user?.id || 'anonymous'}`

onMounted(() => {
  sessionId.value = localStorage.getItem(sessionStorageKey()) || ''
})

async function scrollToBottom() {
  await nextTick()
  if (messagesRef.value) {
    messagesRef.value.scrollTop = messagesRef.value.scrollHeight
  }
}

async function sendMessage() {
  const message = input.value.trim()
  if (!message || loading.value) return

  messages.value.push({ role: 'user', content: message })
  input.value = ''
  loading.value = true
  await scrollToBottom()
  try {
    const res = await agentApi.chat(message, sessionId.value)
    const data = res.data
    sessionId.value = data.session_id
    localStorage.setItem(sessionStorageKey(), data.session_id)
    messages.value.push({ role: 'assistant', content: data.answer })
  } catch (error: any) {
    const detail = error?.response?.data?.message
    ElMessage.error(detail || 'Agent 暂时不可用，请稍后重试')
  } finally {
    loading.value = false
    await scrollToBottom()
  }
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Enter' && !event.shiftKey) {
    event.preventDefault()
    void sendMessage()
  }
}

function clearSession() {
  sessionId.value = ''
  localStorage.removeItem(sessionStorageKey())
  messages.value = [{ role: 'assistant', content: '会话已清空。你可以继续向我提问。' }]
}
</script>

<template>
  <div class="agent-page">
    <div class="agent-intro">
      <div>
        <h2>智能助手</h2>
        <p>用自然语言查询知识库和打卡信息，涉及写入操作时我会先按系统权限执行。</p>
      </div>
      <el-button plain @click="clearSession">清空会话</el-button>
    </div>

    <el-card class="chat-card" shadow="never">
      <div ref="messagesRef" class="messages">
        <div v-for="(item, index) in messages" :key="index" class="message-row" :class="item.role">
          <div class="message-bubble">{{ item.content }}</div>
        </div>
        <div v-if="loading" class="message-row assistant">
          <div class="message-bubble pending">正在思考…</div>
        </div>
      </div>
      <div class="composer">
        <el-input
          v-model="input"
          type="textarea"
          :rows="3"
          maxlength="4000"
          show-word-limit
          resize="none"
          placeholder="例如：帮我打卡上班，或者查一下本周的打卡情况"
          @keydown="handleKeydown"
        />
        <div class="composer-footer">
          <span>Enter 发送，Shift + Enter 换行</span>
          <el-button type="primary" :loading="loading" :disabled="!input.trim()" @click="sendMessage">发送</el-button>
        </div>
      </div>
    </el-card>
  </div>
</template>

<style scoped>
.agent-page {
  max-width: 960px;
  margin: 0 auto;
}

.agent-intro {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 1rem;
  margin-bottom: 1rem;
}

.agent-intro h2 {
  margin: 0 0 0.35rem;
  color: var(--text);
  font-size: 1.25rem;
}

.agent-intro p {
  margin: 0;
  color: var(--text-secondary);
  font-size: 0.875rem;
}

.chat-card {
  border: 1px solid var(--border);
  background: var(--bg-card);
}

.messages {
  height: min(58vh, 560px);
  min-height: 320px;
  overflow-y: auto;
  padding: 0.5rem 0.25rem 1rem;
}

.message-row {
  display: flex;
  margin: 0.75rem 0;
}

.message-row.user {
  justify-content: flex-end;
}

.message-bubble {
  max-width: 82%;
  padding: 0.7rem 0.9rem;
  border-radius: 12px;
  color: var(--text);
  background: var(--bg-hover);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  line-height: 1.6;
  font-size: 0.925rem;
}

.message-row.user .message-bubble {
  color: #fff;
  background: var(--accent);
}

.message-bubble.pending {
  color: var(--text-muted);
}

.composer {
  border-top: 1px solid var(--border);
  padding-top: 1rem;
}

.composer-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  margin-top: 0.65rem;
  color: var(--text-muted);
  font-size: 0.75rem;
}

@media (max-width: 640px) {
  .agent-intro {
    flex-direction: column;
  }

  .message-bubble {
    max-width: 92%;
  }
}
</style>
