<script setup lang="ts">
import { nextTick, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { agentApi, type AgentKnowledgeSource } from '@/api/agent'
import { knowledgeApi } from '@/api/knowledge'
import { useAuthStore } from '@/stores/auth'

interface ChatMessage {
  role: 'user' | 'assistant'
  content: string
  sources?: AgentKnowledgeSource[]
}

const authStore = useAuthStore()
const input = ref('')
const loading = ref(false)
const messages = ref<ChatMessage[]>([
  { role: 'assistant', content: '你好，我可以协助查询知识库、查看打卡状态，也可以按你的请求执行打卡操作。' }
])
const sessionId = ref('')
const messagesRef = ref<HTMLElement | null>(null)
const downloadingSources = ref<Record<number, boolean>>({})

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
    messages.value.push({ role: 'assistant', content: data.answer, sources: data.sources || [] })
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

function sourceExcerpt(source: AgentKnowledgeSource): string {
  return (source.excerpt || source.description || '').trim()
}

function sourceKindLabel(source: AgentKnowledgeSource): string {
  if (source.type === 'file' || source.source_kind === 'file') return '文件'
  if (source.type === 'link' || source.source_kind === 'link') return '链接'
  return '资料'
}

function sourceMatchLabel(source: AgentKnowledgeSource): string {
  if (source.match_mode === 'vector') return '语义匹配'
  if (source.match_mode === 'keyword') return '关键词匹配'
  if (source.match_mode === 'hybrid') return '混合匹配'
  return ''
}

function externalSourceURL(source: AgentKnowledgeSource): string {
  const raw = (source.url || '').trim()
  if (!raw) return ''
  try {
    const parsed = new URL(raw)
    if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') return ''
    if (parsed.username || parsed.password) return ''
    return parsed.href
  } catch {
    return ''
  }
}

function canDownloadSource(source: AgentKnowledgeSource): boolean {
  return source.id > 0 && Boolean(source.download_url) && (source.type === 'file' || source.source_kind === 'file')
}

function sourceFilename(source: AgentKnowledgeSource): string {
  const fallback = (source.name || '').trim() || `知识库文件-${source.id}`
  // The server controls the actual response content. This only supplies a
  // safe browser-side default filename for the Blob download.
  const cleaned = fallback.replace(/[\\/:*?"<>|\u0000-\u001f]/g, '_').trim()
  return cleaned || `知识库文件-${source.id}`
}

async function downloadSource(source: AgentKnowledgeSource) {
  if (!canDownloadSource(source) || downloadingSources.value[source.id]) return
  downloadingSources.value[source.id] = true
  try {
    // This API call goes through the shared Axios interceptor and therefore
    // carries the current bearer token (and refreshes it when necessary).
    const response = await knowledgeApi.downloadFile(source.id)
    const blob = response instanceof Blob ? response : (response as any).data
    if (!(blob instanceof Blob)) throw new Error('invalid download response')
    const objectURL = window.URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = objectURL
    link.download = sourceFilename(source)
    document.body.appendChild(link)
    link.click()
    link.remove()
    window.URL.revokeObjectURL(objectURL)
  } catch {
    ElMessage.error('知识库文件下载失败')
  } finally {
    delete downloadingSources.value[source.id]
  }
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
          <div class="message-content">
            <div class="message-bubble">{{ item.content }}</div>
            <div v-if="item.role === 'assistant' && item.sources?.length" class="source-list">
              <div class="source-list-title">参考资料</div>
              <div v-for="source in item.sources" :key="`${index}-${source.id}-${source.name}`" class="source-card">
                <div class="source-heading">
                  <span class="source-kind">{{ sourceKindLabel(source) }}</span>
                  <strong>{{ source.name || '未命名资料' }}</strong>
                </div>
                <p v-if="sourceExcerpt(source)" class="source-excerpt">{{ sourceExcerpt(source) }}</p>
                <div class="source-meta">
                  <span v-if="source.category_name">{{ source.category_name }}</span>
                  <span v-if="sourceMatchLabel(source)">{{ sourceMatchLabel(source) }}</span>
                </div>
                <div class="source-actions">
                  <el-button
                    v-if="canDownloadSource(source)"
                    text
                    size="small"
                    :loading="downloadingSources[source.id]"
                    @click="downloadSource(source)"
                  >下载文件</el-button>
                  <a
                    v-if="externalSourceURL(source)"
                    class="source-link"
                    :href="externalSourceURL(source)"
                    target="_blank"
                    rel="noopener noreferrer"
                  >打开链接</a>
                </div>
              </div>
            </div>
          </div>
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

.message-content {
  max-width: 82%;
}

.message-row.user .message-content {
  display: flex;
  justify-content: flex-end;
}

.source-list {
  margin-top: 0.55rem;
  display: grid;
  gap: 0.45rem;
}

.source-list-title {
  color: var(--text-secondary);
  font-size: 0.75rem;
  font-weight: 600;
}

.source-card {
  border: 1px solid var(--border);
  border-radius: 9px;
  padding: 0.55rem 0.65rem;
  background: var(--bg-card);
}

.source-heading {
  display: flex;
  align-items: center;
  gap: 0.45rem;
  color: var(--text);
  font-size: 0.8rem;
}

.source-kind {
  flex: none;
  color: var(--accent);
  font-size: 0.68rem;
  font-weight: 600;
}

.source-excerpt {
  margin: 0.35rem 0 0;
  color: var(--text-secondary);
  font-size: 0.76rem;
  line-height: 1.45;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.source-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 0.45rem;
  margin-top: 0.35rem;
  color: var(--text-muted);
  font-size: 0.68rem;
}

.source-actions {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  margin-top: 0.2rem;
}

.source-link {
  color: var(--accent);
  font-size: 0.75rem;
  text-decoration: none;
}

.source-link:hover {
  text-decoration: underline;
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

  .message-content {
    max-width: 92%;
  }
}
</style>
