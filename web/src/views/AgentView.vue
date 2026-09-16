<script setup lang="ts">
import { nextTick, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import {
  agentApi,
  type AgentConversation,
  type AgentConversationMessage,
  type AgentKnowledgeSource
} from '@/api/agent'
import { knowledgeApi } from '@/api/knowledge'
import { useAuthStore } from '@/stores/auth'
import { renderMarkdown } from '@/utils/markdown'

interface ChatMessage {
  role: 'user' | 'assistant'
  content: string
  createdAt: string
  sources?: AgentKnowledgeSource[]
}

const greetingMessage = '你好，我可以协助查询知识库、查看打卡状态，也可以按你的请求执行打卡操作。'

function newGreetingMessage(): ChatMessage {
  return { role: 'assistant', content: greetingMessage, createdAt: new Date().toISOString() }
}

const authStore = useAuthStore()
const input = ref('')
const loading = ref(false)
const historyLoading = ref(false)
const conversationsLoading = ref(false)
const creatingConversation = ref(false)
const conversations = ref<AgentConversation[]>([])
const messages = ref<ChatMessage[]>([newGreetingMessage()])
const sessionId = ref('')
const messagesRef = ref<HTMLElement | null>(null)
const downloadingSources = ref<Record<number, boolean>>({})
const historyRequestId = ref(0)
const editingSessionId = ref('')
const editingTitle = ref('')
const renamingSessionId = ref('')

const sessionStorageKey = () => `swpucat-agent-session:${authStore.user?.id || 'anonymous'}`

onMounted(() => {
  void initializeConversations()
})

async function initializeConversations() {
  conversationsLoading.value = true
  try {
    const res = await agentApi.listConversations()
    const items = Array.isArray(res.data) ? res.data : []
    conversations.value = items

    const storedSessionId = localStorage.getItem(sessionStorageKey()) || ''
    const preferred = items.find((item) => item.session_id === storedSessionId) || items[0]
    if (preferred) {
      await loadConversation(preferred.session_id)
    } else {
      resetDraft()
    }
  } catch {
    resetDraft()
    ElMessage.error('会话列表加载失败，请稍后重试')
  } finally {
    conversationsLoading.value = false
  }
}

async function scrollToBottom() {
  await nextTick()
  if (messagesRef.value) {
    messagesRef.value.scrollTop = messagesRef.value.scrollHeight
  }
}

function resetDraft() {
  sessionId.value = ''
  localStorage.removeItem(sessionStorageKey())
  messages.value = [newGreetingMessage()]
}

async function createNewConversation() {
  if (creatingConversation.value || conversationsLoading.value || loading.value || historyLoading.value || editingSessionId.value || renamingSessionId.value) return
  creatingConversation.value = true
  try {
    const res = await agentApi.createConversation()
    const created = res.data
    conversations.value = [created, ...conversations.value.filter((item) => item.session_id !== created.session_id)]
    sessionId.value = created.session_id
    localStorage.setItem(sessionStorageKey(), created.session_id)
    editingSessionId.value = ''
    messages.value = [newGreetingMessage()]
    await scrollToBottom()
  } catch {
    ElMessage.error('新建会话失败，请稍后重试')
  } finally {
    creatingConversation.value = false
  }
}

async function ensureConversation() {
  if (sessionId.value) return true
  try {
    const res = await agentApi.createConversation()
    const created = res.data
    conversations.value = [created, ...conversations.value.filter((item) => item.session_id !== created.session_id)]
    sessionId.value = created.session_id
    localStorage.setItem(sessionStorageKey(), created.session_id)
    return true
  } catch {
    ElMessage.error('无法创建会话，请稍后重试')
    return false
  }
}

async function selectConversation(item: AgentConversation) {
  if (item.session_id === sessionId.value && !historyLoading.value) return
  if (loading.value || historyLoading.value || creatingConversation.value || editingSessionId.value || renamingSessionId.value) return
  await loadConversation(item.session_id)
}

async function loadConversation(targetSessionId: string) {
  const requestId = ++historyRequestId.value
  historyLoading.value = true
  try {
    const res = await agentApi.getConversation(targetSessionId)
    if (requestId !== historyRequestId.value) return
    sessionId.value = res.data.conversation.session_id
    localStorage.setItem(sessionStorageKey(), sessionId.value)
    messages.value = res.data.messages
      .filter((item) => item.role === 'user' || item.role === 'assistant')
      .map(messageFromHistory)
    if (messages.value.length === 0) {
      messages.value = [newGreetingMessage()]
    }
  } catch (error: any) {
    if (requestId !== historyRequestId.value) return
    if (error?.response?.status === 404) {
      conversations.value = conversations.value.filter((item) => item.session_id !== targetSessionId)
      if (targetSessionId === sessionId.value) resetDraft()
      ElMessage.warning('该会话已不存在')
    } else {
      ElMessage.error('会话记录加载失败，请稍后重试')
    }
  } finally {
    if (requestId === historyRequestId.value) {
      // The message list is hidden while historyLoading is true. Turn the
      // placeholder off first, then wait for the real DOM before scrolling.
      historyLoading.value = false
      await scrollToBottom()
    }
  }
}

function messageFromHistory(message: AgentConversationMessage): ChatMessage {
  return {
    role: message.role === 'user' ? 'user' : 'assistant',
    content: message.content,
    createdAt: message.created_at,
    sources: message.role === 'assistant' ? parseSources(message.citations_json) : undefined
  }
}

function parseSources(value?: string): AgentKnowledgeSource[] {
  if (!value) return []
  try {
    const parsed = JSON.parse(value)
    return Array.isArray(parsed) ? parsed as AgentKnowledgeSource[] : []
  } catch {
    return []
  }
}

async function refreshConversationList() {
  try {
    const res = await agentApi.listConversations()
    conversations.value = Array.isArray(res.data) ? res.data : []
  } catch {
    // The current answer is already available. A later page refresh can retry
    // the list request without interrupting the completed chat turn.
  }
}

async function sendMessage() {
  const message = input.value.trim()
  if (!message || conversationsLoading.value || loading.value || historyLoading.value || editingSessionId.value || renamingSessionId.value) return
  if (!(await ensureConversation())) return

  const targetSessionId = sessionId.value
  messages.value.push({ role: 'user', content: message, createdAt: new Date().toISOString() })
  input.value = ''
  loading.value = true
  await scrollToBottom()
  try {
    const res = await agentApi.chat(message, targetSessionId)
    const data = res.data
    if (data.session_id && data.session_id !== sessionId.value) {
      sessionId.value = data.session_id
      localStorage.setItem(sessionStorageKey(), data.session_id)
    }
    messages.value.push({
      role: 'assistant',
      content: data.answer,
      createdAt: new Date().toISOString(),
      sources: data.sources || []
    })
    await refreshConversationList()
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

function conversationTitle(item: AgentConversation): string {
  return item.title?.trim() || '新会话'
}

function startRename(item: AgentConversation) {
  if (loading.value || historyLoading.value || creatingConversation.value) return
  editingSessionId.value = item.session_id
  editingTitle.value = conversationTitle(item)
}

function cancelRename() {
  editingSessionId.value = ''
  editingTitle.value = ''
}

async function saveRename(item: AgentConversation) {
  if (renamingSessionId.value || editingSessionId.value !== item.session_id) return
  const title = editingTitle.value.trim()
  if (!title) {
    ElMessage.warning('会话标题不能为空')
    editingTitle.value = conversationTitle(item)
    return
  }
  if (title === conversationTitle(item)) {
    cancelRename()
    return
  }
  renamingSessionId.value = item.session_id
  try {
    const res = await agentApi.updateConversationTitle(item.session_id, title)
    const updated = res.data
    const index = conversations.value.findIndex((conversation) => conversation.session_id === item.session_id)
    if (index >= 0) conversations.value[index] = { ...conversations.value[index], ...updated }
    cancelRename()
  } catch (error: any) {
    const detail = error?.response?.data?.message
    ElMessage.error(detail || '会话标题修改失败，请稍后重试')
  } finally {
    renamingSessionId.value = ''
  }
}

function handleRenameKeydown(event: KeyboardEvent, item: AgentConversation) {
  if (event.key === 'Enter') {
    event.preventDefault()
    void saveRename(item)
  } else if (event.key === 'Escape') {
    event.preventDefault()
    cancelRename()
  }
}

function formatMessageTime(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  const pad = (part: number) => String(part).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
}

function formatConversationDate(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return new Intl.DateTimeFormat('zh-CN', { month: 'numeric', day: 'numeric' }).format(date)
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
    </div>

    <div class="agent-layout">
      <aside class="conversation-panel">
        <div class="conversation-panel-header">
          <span class="conversation-panel-title">会话记录</span>
          <el-button text circle :loading="creatingConversation" :disabled="conversationsLoading || Boolean(editingSessionId) || Boolean(renamingSessionId)" aria-label="新建会话" @click="createNewConversation">
            <el-icon><Plus /></el-icon>
          </el-button>
        </div>
        <el-button
          class="new-conversation-button"
          type="primary"
          plain
          :loading="creatingConversation"
          :disabled="conversationsLoading || loading || historyLoading || Boolean(editingSessionId) || Boolean(renamingSessionId)"
          @click="createNewConversation"
        >
          <el-icon><Plus /></el-icon>
          新建会话
        </el-button>

        <div v-if="conversationsLoading" class="conversation-status">正在加载会话…</div>
        <div v-else-if="!conversations.length" class="conversation-status">暂无历史会话</div>
        <div v-else class="conversation-list">
          <div
            v-for="item in conversations"
            :key="item.session_id"
            class="conversation-item"
            :class="{ active: item.session_id === sessionId, editing: item.session_id === editingSessionId }"
            @click="selectConversation(item)"
          >
            <el-icon class="conversation-item-icon"><ChatDotRound /></el-icon>
            <span class="conversation-item-body">
              <el-input
                v-if="item.session_id === editingSessionId"
                v-model="editingTitle"
                class="conversation-title-input"
                size="small"
                maxlength="256"
                :disabled="renamingSessionId === item.session_id"
                autofocus
                @click.stop
                @keydown.stop="handleRenameKeydown($event, item)"
                @blur="saveRename(item)"
              />
              <span v-else class="conversation-item-title">{{ conversationTitle(item) }}</span>
              <span class="conversation-item-date">{{ formatConversationDate(item.last_active_at || item.created_at) }}</span>
            </span>
            <el-button
              v-if="item.session_id !== editingSessionId"
              class="conversation-edit-button"
              text
              circle
              size="small"
              :disabled="loading || historyLoading || creatingConversation || Boolean(renamingSessionId)"
              aria-label="修改会话标题"
              @click.stop="startRename(item)"
            >
              <el-icon><Edit /></el-icon>
            </el-button>
          </div>
        </div>
      </aside>

      <section class="chat-card" aria-label="智能助手对话">
        <div class="chat-header">
          <span class="chat-header-title">
            {{ conversations.find((item) => item.session_id === sessionId)?.title || '新会话' }}
          </span>
          <span v-if="historyLoading" class="chat-header-status">正在加载记录…</span>
        </div>
        <div ref="messagesRef" class="messages">
          <div v-if="historyLoading" class="history-placeholder">正在加载会话记录…</div>
          <template v-else>
            <div v-for="(item, index) in messages" :key="`${sessionId}-${index}-${item.createdAt}`" class="message-row" :class="item.role">
              <div class="message-content">
                <div v-if="item.role === 'assistant'" class="message-bubble assistant-bubble markdown-body" v-html="renderMarkdown(item.content)"></div>
                <div v-else class="message-bubble">{{ item.content }}</div>
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
                <time v-if="formatMessageTime(item.createdAt)" class="message-time" :datetime="item.createdAt">{{ formatMessageTime(item.createdAt) }}</time>
              </div>
            </div>
            <div v-if="loading" class="message-row assistant">
              <div class="message-content">
                <div class="message-bubble pending">正在思考…</div>
              </div>
            </div>
          </template>
        </div>
        <div class="composer">
          <el-input
            v-model="input"
            type="textarea"
            :rows="3"
            maxlength="4000"
            show-word-limit
            resize="none"
            :disabled="conversationsLoading || historyLoading || Boolean(editingSessionId) || Boolean(renamingSessionId)"
            placeholder="例如：帮我打卡上班，或者查一下本周的打卡情况"
            @keydown="handleKeydown"
          />
          <div class="composer-footer">
            <span>Enter 发送，Shift + Enter 换行</span>
            <el-button type="primary" :loading="loading" :disabled="conversationsLoading || historyLoading || Boolean(editingSessionId) || Boolean(renamingSessionId) || !input.trim()" @click="sendMessage">发送</el-button>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<style scoped>
.agent-page {
  max-width: 1180px;
  margin: 0 auto;
}

.agent-intro {
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

.agent-layout {
  display: grid;
  grid-template-columns: 238px minmax(0, 1fr);
  gap: 0.85rem;
  align-items: stretch;
}

.conversation-panel,
.chat-card {
  border: 1px solid var(--border);
  background: var(--bg-card);
  border-radius: var(--radius);
}

.conversation-panel {
  min-height: 620px;
  padding: 0.65rem;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.conversation-panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.15rem 0.15rem 0.55rem 0.35rem;
}

.conversation-panel-title {
  color: var(--text);
  font-size: 0.9rem;
  font-weight: 600;
}

.new-conversation-button {
  width: 100%;
  margin-bottom: 0.55rem;
}

.conversation-list {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 0.2rem;
  overflow-y: auto;
  padding-right: 0.1rem;
}

.conversation-item {
  width: 100%;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--text-secondary);
  display: flex;
  align-items: center;
  gap: 0.55rem;
  padding: 0.55rem 0.5rem;
  text-align: left;
  cursor: pointer;
  transition: background 0.2s, color 0.2s;
}

.conversation-item:hover {
  background: var(--bg-hover);
  color: var(--text);
}

.conversation-item.active {
  background: var(--accent-bg);
  color: var(--accent);
}

.conversation-item.editing {
  cursor: default;
}

.conversation-item-icon {
  flex: none;
}

.conversation-item-body {
  min-width: 0;
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 0.18rem;
}

.conversation-item-title {
  overflow: hidden;
  color: inherit;
  font-size: 0.8rem;
  line-height: 1.35;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.conversation-title-input {
  min-width: 0;
}

.conversation-edit-button {
  flex: none;
  opacity: 0;
  color: var(--text-muted);
  transition: opacity 0.2s, color 0.2s;
}

.conversation-item:hover .conversation-edit-button,
.conversation-edit-button:focus-visible {
  opacity: 1;
}

.conversation-edit-button:hover:not(:disabled) {
  color: var(--accent);
}

.conversation-item-date {
  color: var(--text-muted);
  font-size: 0.68rem;
}

.conversation-status {
  padding: 1.2rem 0.35rem;
  color: var(--text-muted);
  font-size: 0.78rem;
  text-align: center;
}

.chat-card {
  min-width: 0;
  overflow: hidden;
}

.chat-header {
  min-height: 46px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  border-bottom: 1px solid var(--border);
  padding: 0.55rem 0.9rem;
}

.chat-header-title {
  overflow: hidden;
  color: var(--text);
  font-size: 0.9rem;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.chat-header-status {
  flex: none;
  color: var(--text-muted);
  font-size: 0.72rem;
}

.messages {
  height: min(58vh, 560px);
  min-height: 320px;
  overflow-y: auto;
  padding: 0.5rem 0.9rem 1rem;
}

.history-placeholder {
  padding: 3rem 1rem;
  color: var(--text-muted);
  font-size: 0.85rem;
  text-align: center;
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

.assistant-bubble {
  white-space: normal;
}

.message-time {
  display: block;
  margin-top: 0.3rem;
  color: var(--text-muted);
  font-size: 0.68rem;
  line-height: 1.2;
}

.message-row.user .message-time {
  text-align: right;
}

.markdown-body {
  line-height: 1.65;
}

.markdown-body :deep(p) {
  margin: 0 0 0.65em;
}

.markdown-body :deep(p:last-child) {
  margin-bottom: 0;
}

.markdown-body :deep(h1),
.markdown-body :deep(h2),
.markdown-body :deep(h3),
.markdown-body :deep(h4) {
  margin: 0.7em 0 0.4em;
  font-weight: 600;
  line-height: 1.35;
}

.markdown-body :deep(h1:first-child),
.markdown-body :deep(h2:first-child),
.markdown-body :deep(h3:first-child),
.markdown-body :deep(h4:first-child) {
  margin-top: 0;
}

.markdown-body :deep(ul),
.markdown-body :deep(ol) {
  margin: 0.45em 0 0.7em;
  padding-left: 1.4em;
}

.markdown-body :deep(blockquote) {
  margin: 0.65em 0;
  border-left: 3px solid var(--accent);
  padding-left: 0.8em;
  color: var(--text-secondary);
}

.markdown-body :deep(code) {
  border-radius: 4px;
  background: var(--bg-deep);
  padding: 0.12em 0.35em;
  font-size: 0.9em;
}

.markdown-body :deep(pre) {
  max-width: 100%;
  margin: 0.7em 0;
  overflow-x: auto;
  border-radius: 7px;
  background: var(--bg-deep);
  padding: 0.75em;
}

.markdown-body :deep(pre code) {
  background: transparent;
  padding: 0;
}

.markdown-body :deep(table) {
  display: block;
  max-width: 100%;
  margin: 0.7em 0;
  overflow-x: auto;
  border-collapse: collapse;
  font-size: 0.88em;
}

.markdown-body :deep(th),
.markdown-body :deep(td) {
  border: 1px solid var(--border);
  padding: 0.35em 0.55em;
  text-align: left;
}

.markdown-body :deep(th) {
  background: var(--bg-deep);
}

.markdown-body :deep(a) {
  color: var(--accent);
  text-decoration: none;
}

.markdown-body :deep(a:hover) {
  text-decoration: underline;
}

.markdown-body :deep(img) {
  max-width: 100%;
  height: auto;
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
  padding: 1rem;
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

@media (max-width: 860px) {
  .agent-layout {
    grid-template-columns: 190px minmax(0, 1fr);
  }
}

@media (max-width: 680px) {
  .agent-layout {
    grid-template-columns: 1fr;
  }

  .conversation-panel {
    min-height: 0;
    max-height: 230px;
  }

  .conversation-list {
    max-height: 120px;
  }

  .message-bubble,
  .message-content {
    max-width: 92%;
  }
}
</style>
