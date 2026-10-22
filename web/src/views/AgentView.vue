<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  agentApi,
  type AgentActionProposal,
  type AgentActionResult,
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
  id?: number
  localId: string
  status: 'completed' | 'pending' | 'streaming' | 'failed' | 'stopped'
  clientTurnId?: string
  generationId?: string
  requestId?: string
  sourceMessage?: string
  regenerate?: boolean
  feedback?: boolean
  proposals?: AgentActionProposal[]
  actionResults?: Record<string, AgentActionResult>
}

const greetingMessage = '你好，我可以协助查询知识库、查看打卡状态；涉及打卡或创建草稿时，我会先给出待确认操作。'

function newGreetingMessage(): ChatMessage {
  return { role: 'assistant', content: greetingMessage, createdAt: new Date().toISOString(), localId: `greeting-${Date.now()}`, status: 'completed' }
}

const authStore = useAuthStore()
const input = ref('')
const loading = ref(false)
const historyLoading = ref(false)
const conversationsLoading = ref(false)
const creatingConversation = ref(false)
const conversations = ref<AgentConversation[]>([])
const sessionsOpen = ref(false)
const messages = ref<ChatMessage[]>([newGreetingMessage()])
const sessionId = ref('')
const messagesRef = ref<HTMLElement | null>(null)
const downloadingSources = ref<Record<number, boolean>>({})
const historyRequestId = ref(0)
const editingSessionId = ref('')
const editingTitle = ref('')
const renamingSessionId = ref('')
const deletingSessionId = ref('')
const activeAbortController = ref<AbortController | null>(null)
const feedbackSaving = ref<Record<string, boolean>>({})
const actionConfirming = ref<Record<string, boolean>>({})
const expandedSources = ref<Record<string, boolean>>({})

const sessionStorageKey = () => `swpucat-agent-session:${authStore.user?.id || 'anonymous'}`

onMounted(() => {
  void initializeConversations()
})

onBeforeUnmount(() => {
  activeAbortController.value?.abort()
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
  sessionsOpen.value = false
}

async function deleteConversation(item: AgentConversation) {
  if (loading.value || historyLoading.value || creatingConversation.value || editingSessionId.value || renamingSessionId.value || deletingSessionId.value) return
  try {
    await ElMessageBox.confirm(
      '删除后，会话消息、摘要和引用将永久清除且无法恢复；运行审计仅保留脱离会话标识的安全统计。',
      `删除“${conversationTitle(item)}”？`,
      { type: 'warning', confirmButtonText: '永久删除', cancelButtonText: '取消', confirmButtonClass: 'el-button--danger' }
    )
  } catch {
    return
  }

  deletingSessionId.value = item.session_id
  try {
    await agentApi.deleteConversation(item.session_id)
    const wasCurrent = sessionId.value === item.session_id
    conversations.value = conversations.value.filter((conversation) => conversation.session_id !== item.session_id)
    if (wasCurrent) {
      resetDraft()
      const next = conversations.value[0]
      if (next) await loadConversation(next.session_id)
    }
    ElMessage.success('会话已永久删除')
  } catch (error: any) {
    const detail = error?.response?.data?.message
    ElMessage.error(detail || '会话删除失败，请稍后重试')
  } finally {
    deletingSessionId.value = ''
  }
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
    let lastUserMessage = ''
    for (const item of messages.value) {
      if (item.role === 'user') {
        lastUserMessage = item.content
      } else if (item.clientTurnId) {
        item.sourceMessage = lastUserMessage
      }
    }
    if (messages.value.length === 0) {
      messages.value = [newGreetingMessage()]
    }
    recoverTokenlessProposals()
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
    sources: message.role === 'assistant' ? parseSources(message.citations_json) : undefined,
    id: message.id,
    localId: `message-${message.id}`,
    status: 'completed',
    clientTurnId: message.client_turn_id,
    generationId: message.generation_id,
    requestId: message.request_id,
    proposals: message.role === 'assistant' ? parseProposals(message.actions_json) : undefined,
    actionResults: {},
    sourceMessage: message.role === 'assistant' ? undefined : message.content
  }
}

function parseProposals(value?: string): AgentActionProposal[] {
  if (!value) return []
  try {
    const parsed = JSON.parse(value)
    return Array.isArray(parsed) ? parsed as AgentActionProposal[] : []
  } catch {
    return []
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

  const clientTurnId = newTurnIdentifier()
  const generationId = newTurnIdentifier()
  messages.value.push({
    role: 'user', content: message, createdAt: new Date().toISOString(), localId: `user-${clientTurnId}`,
    status: 'completed', clientTurnId, generationId, sourceMessage: message
  })
  const assistantIndex = messages.value.push({
    role: 'assistant', content: '', createdAt: new Date().toISOString(), localId: `assistant-${clientTurnId}`,
    status: 'pending', clientTurnId, generationId, sourceMessage: message, regenerate: false, sources: []
  }) - 1
  input.value = ''
  await scrollToBottom()
  await runGeneration(assistantIndex)
}

function newTurnIdentifier(): string {
  const bytes = new Uint8Array(16)
  crypto.getRandomValues(bytes)
  return Array.from(bytes, (value) => value.toString(16).padStart(2, '0')).join('')
}

async function runGeneration(assistantIndex: number) {
  const target = messages.value[assistantIndex]
  if (!target || target.role !== 'assistant' || !target.sourceMessage || !target.clientTurnId || !target.generationId || loading.value) return
  const targetSessionId = sessionId.value
  const controller = new AbortController()
  activeAbortController.value = controller
  loading.value = true
  target.status = 'pending'
  const previousContent = target.content
  const previousSources = target.sources
  const previousMessageID = target.id
  const previousRequestID = target.requestId
  const regeneration = Boolean(target.regenerate)
  target.content = ''
  target.sources = []
  target.feedback = undefined
  try {
    const data = await agentApi.chatStream(
      target.sourceMessage,
      targetSessionId,
      target.clientTurnId,
      target.generationId,
      Boolean(target.regenerate),
      (delta) => {
        if (messages.value[assistantIndex] !== target) return
        target.status = 'streaming'
        target.content += delta
        void scrollToBottom()
      },
      controller.signal
    )
    if (data.session_id && data.session_id !== sessionId.value) {
      sessionId.value = data.session_id
      localStorage.setItem(sessionStorageKey(), data.session_id)
    }
    target.content = data.answer
    target.sources = data.sources || []
    target.proposals = data.proposals || []
    target.actionResults = {}
    target.createdAt = new Date().toISOString()
    target.id = data.message_id
    target.clientTurnId = data.client_turn_id || target.clientTurnId
    target.generationId = data.generation_id || target.generationId
    target.requestId = data.request_id
    target.status = data.conversation_persisted ? 'completed' : 'failed'
    target.regenerate = data.conversation_persisted ? false : regeneration
    if (!data.conversation_persisted) {
      ElMessage.warning('回答已生成，但会话记录保存失败；可点击重试再次保存')
    } else {
      await refreshConversationList()
    }
  } catch (error: any) {
    const stopped = controller.signal.aborted || error?.name === 'AbortError'
    if (target.regenerate && previousContent) {
      target.content = previousContent
      target.sources = previousSources
      target.id = previousMessageID
      target.requestId = previousRequestID
    }
    target.status = stopped ? 'stopped' : 'failed'
    if (stopped) {
      ElMessage.info('已停止生成，未保存本次部分回答')
    } else {
      const detail = error?.message || error?.response?.data?.message
      ElMessage.error(detail || '智能助手暂时不可用，请稍后重试')
    }
  } finally {
    if (activeAbortController.value === controller) activeAbortController.value = null
    loading.value = false
    await scrollToBottom()
  }
}

function proposalStableKey(proposal: AgentActionProposal): string {
  return `${proposal.tool}|${JSON.stringify(proposal.parameters || {})}`
}

function actionKey(item: ChatMessage, proposal: AgentActionProposal): string {
  return `${item.localId}|${proposalStableKey(proposal)}`
}

function actionExpired(proposal: AgentActionProposal): boolean {
  const expiresAt = new Date(proposal.expires_at).getTime()
  return !Number.isFinite(expiresAt) || expiresAt <= Date.now()
}

function actionToolLabel(tool: string): string {
  if (tool === 'clock_in') return '上班打卡'
  if (tool === 'clock_out') return '下班打卡'
  if (tool === 'draft_announcement') return '创建公告草稿'
  if (tool === 'review_knowledge_item') return '审核知识条目'
  if (tool === 'generate_invitation_code') return '生成邀请码'
  if (tool === 'save_evaluation_draft') return '保存考评草稿'
  return '待确认操作'
}

function actionParameter(proposal: AgentActionProposal, key: string): string {
  const value = proposal.parameters?.[key]
  return typeof value === 'string' || typeof value === 'number' ? String(value) : ''
}

function invitationCode(result?: AgentActionResult): string {
  const value = result?.data?.code
  return typeof value === 'string' ? value : ''
}

async function copyInvitationCode(result?: AgentActionResult) {
  const code = invitationCode(result)
  if (!code) return
  try {
    await navigator.clipboard.writeText(code)
    ElMessage.success('邀请码已复制')
  } catch {
    ElMessage.error('复制失败，请手动选择邀请码')
  }
}

function actionExpiryLabel(proposal: AgentActionProposal): string {
  const value = formatMessageTime(proposal.expires_at)
  return value ? `确认有效期至 ${value}` : '确认有效期未知'
}

async function confirmAction(item: ChatMessage, proposal: AgentActionProposal) {
  const key = actionKey(item, proposal)
  if (!proposal.token || loading.value || actionConfirming.value[key] || actionExpired(proposal)) return
  actionConfirming.value[key] = true
  try {
    const response = await agentApi.confirmAction(sessionId.value, proposal)
    const result = response.data
    item.actionResults ||= {}
    item.actionResults[proposalStableKey(proposal)] = result
    if (result.status === 'success') ElMessage.success(result.message)
    else if (result.status === 'pending') ElMessage.info(result.message)
    else ElMessage.error(result.message)
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.message || '操作确认失败，请稍后重试')
  } finally {
    delete actionConfirming.value[key]
  }
}

// A persisted action card strips the confirmation token for privacy, so a
// reloaded proposal cannot be confirmed directly. Recovering asks the server for
// the stored outcome, or for a freshly re-signed token, so the card stays usable.
async function recoverProposal(item: ChatMessage, proposal: AgentActionProposal) {
  if (proposal.token || loading.value) return
  const key = actionKey(item, proposal)
  if (actionConfirming.value[key]) return
  actionConfirming.value[key] = true
  try {
    const response = await agentApi.recoverAction(sessionId.value, proposal)
    const recovered = response.data
    if (recovered.result) {
      item.actionResults ||= {}
      item.actionResults[proposalStableKey(proposal)] = recovered.result
    } else if (recovered.proposal?.token) {
      proposal.token = recovered.proposal.token
      proposal.expires_at = recovered.proposal.expires_at
    }
  } catch {
    // Keep the token-less card so it remains visibly expired/pending; a later
    // reload will retry the recovery.
  } finally {
    delete actionConfirming.value[key]
  }
}

function recoverTokenlessProposals() {
  for (const item of messages.value) {
    for (const proposal of item.proposals || []) {
      void recoverProposal(item, proposal)
    }
  }
}

function stopGeneration() {
  activeAbortController.value?.abort()
}

async function retryGeneration(index: number) {
  if (loading.value) return
  await runGeneration(index)
}

async function regenerateResponse(index: number) {
  const target = messages.value[index]
  if (!target || target.role !== 'assistant' || !target.clientTurnId || !target.sourceMessage || loading.value) return
  target.generationId = newTurnIdentifier()
  target.regenerate = true
  await runGeneration(index)
}

function isLastAssistant(index: number): boolean {
  for (let cursor = messages.value.length - 1; cursor > index; cursor--) {
    if (messages.value[cursor].role === 'assistant' && messages.value[cursor].clientTurnId) return false
  }
  return true
}

async function copyResponse(item: ChatMessage) {
  if (!item.content) return
  try {
    await navigator.clipboard.writeText(item.content)
    ElMessage.success('回答已复制')
  } catch {
    ElMessage.error('复制失败，请手动选择文本')
  }
}

async function saveFeedback(item: ChatMessage, helpful: boolean) {
  if (!item.id || !item.requestId || item.status !== 'completed') return
  const key = item.localId
  if (feedbackSaving.value[key]) return
  feedbackSaving.value[key] = true
  try {
    await agentApi.saveFeedback(item.id, item.requestId, helpful)
    item.feedback = helpful
    ElMessage.success('感谢你的反馈')
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.message || '反馈保存失败，请稍后重试')
  } finally {
    delete feedbackSaving.value[key]
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
  if (source.type === 'article') return '官方文章'
  if (source.type === 'file' || source.source_kind === 'file') return '文件'
  if (source.type === 'link' || source.source_kind === 'link') return '链接'
  return '资料'
}

function sourceMatchLabel(source: AgentKnowledgeSource): string {
  if (source.match_mode === 'vector') return '语义匹配'
  if (source.match_mode === 'keyword') return '关键词匹配'
  if (source.match_mode === 'hybrid') return '混合匹配'
  if (source.match_mode === 'metadata_keyword') return '元数据匹配（降级）'
  return ''
}

function isSourcesExpanded(localId: string): boolean {
  return Boolean(expandedSources.value[localId])
}

function toggleSources(localId: string) {
  expandedSources.value[localId] = !expandedSources.value[localId]
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
      <div class="sessions-mask" :class="{ show: sessionsOpen }" @click="sessionsOpen = false"></div>
      <aside class="conversation-panel" :class="{ 'drawer-open': sessionsOpen }">
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
            <el-button
              v-if="item.session_id !== editingSessionId"
              class="conversation-delete-button"
              text
              circle
              size="small"
              :loading="deletingSessionId === item.session_id"
              :disabled="loading || historyLoading || creatingConversation || Boolean(renamingSessionId) || Boolean(deletingSessionId)"
              aria-label="永久删除会话"
              @click.stop="deleteConversation(item)"
            >
              <el-icon><Delete /></el-icon>
            </el-button>
          </div>
        </div>
      </aside>

      <section class="chat-card" aria-label="智能助手对话">
        <div class="chat-header">
          <button class="sessions-toggle" aria-label="会话记录" @click="sessionsOpen = true">
            <el-icon :size="18"><ChatDotRound /></el-icon>
          </button>
          <span class="chat-header-title">
            {{ conversations.find((item) => item.session_id === sessionId)?.title || '新会话' }}
          </span>
          <span v-if="historyLoading" class="chat-header-status">正在加载记录…</span>
        </div>
        <div ref="messagesRef" class="messages">
          <div v-if="historyLoading" class="history-placeholder">正在加载会话记录…</div>
          <template v-else>
            <div v-for="(item, index) in messages" :key="`${sessionId}-${item.localId}`" class="message-row" :class="item.role">
              <div class="message-content">
                <div v-if="item.role === 'assistant' && item.content" class="message-bubble assistant-bubble markdown-body" v-html="renderMarkdown(item.content)"></div>
                <div v-else-if="item.role === 'assistant'" class="message-bubble pending">
                  {{ item.status === 'pending' ? '正在思考…' : '暂未生成回答' }}
                </div>
                <div v-else class="message-bubble">{{ item.content }}</div>
                <div v-if="item.role === 'assistant' && item.sources?.length" class="source-list">
                  <button
                    class="source-list-toggle"
                    :class="{ expanded: isSourcesExpanded(item.localId) }"
                    :aria-expanded="isSourcesExpanded(item.localId)"
                    @click="toggleSources(item.localId)"
                  >
                    <span class="source-list-title">参考资料</span>
                    <el-icon class="source-list-arrow"><ArrowRight /></el-icon>
                  </button>
                  <div class="source-collapse" :class="{ expanded: isSourcesExpanded(item.localId) }">
                    <div class="source-cards">
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
                <div v-if="item.role === 'assistant' && item.proposals?.length" class="action-proposal-list">
                  <article v-for="proposal in item.proposals" :key="actionKey(item, proposal)" class="action-card">
                    <div class="action-card-heading">
                      <span class="action-card-kind">{{ actionToolLabel(proposal.tool) }}</span>
                      <strong>{{ proposal.title }}</strong>
                    </div>
                    <p>{{ proposal.summary }}</p>
                    <dl v-if="proposal.tool === 'draft_announcement'" class="action-parameters">
                      <div><dt>标题</dt><dd>{{ proposal.parameters.title }}</dd></div>
                      <div><dt>正文</dt><dd class="action-content-preview">{{ proposal.parameters.content }}</dd></div>
                      <div><dt>置顶</dt><dd>{{ proposal.parameters.pinned ? '是' : '否' }}</dd></div>
                    </dl>
                    <dl v-else-if="proposal.tool === 'review_knowledge_item'" class="action-parameters">
                      <div><dt>条目</dt><dd>#{{ actionParameter(proposal, 'item_id') }}</dd></div>
                      <div><dt>决定</dt><dd>{{ actionParameter(proposal, 'decision') === 'approve' ? '批准' : '拒绝' }}</dd></div>
                      <div v-if="actionParameter(proposal, 'reason')"><dt>原因</dt><dd>{{ actionParameter(proposal, 'reason') }}</dd></div>
                    </dl>
                    <dl v-else-if="proposal.tool === 'generate_invitation_code'" class="action-parameters">
                      <div><dt>类型</dt><dd>{{ actionParameter(proposal, 'type') === 'captain' ? '队长邀请码' : '成员邀请码' }}</dd></div>
                    </dl>
                    <dl v-else-if="proposal.tool === 'save_evaluation_draft'" class="action-parameters">
                      <div><dt>方式</dt><dd>{{ actionParameter(proposal, 'mode') === 'create' ? '创建草稿' : '更新草稿' }}</dd></div>
                      <div v-if="actionParameter(proposal, 'snapshot_id')"><dt>快照</dt><dd>#{{ actionParameter(proposal, 'snapshot_id') }}</dd></div>
                      <div v-if="actionParameter(proposal, 'draft_id')"><dt>草稿</dt><dd>#{{ actionParameter(proposal, 'draft_id') }}</dd></div>
                      <div><dt>标题</dt><dd>{{ actionParameter(proposal, 'title') }}</dd></div>
                      <div><dt>总结</dt><dd class="action-content-preview">{{ actionParameter(proposal, 'summary_markdown') }}</dd></div>
                    </dl>
                    <div class="action-card-expiry" :class="{ expired: actionExpired(proposal) }">{{ actionExpiryLabel(proposal) }}</div>
                    <div v-if="item.actionResults?.[proposalStableKey(proposal)]" class="action-result" :class="item.actionResults[proposalStableKey(proposal)].status">
                      <strong>{{ item.actionResults[proposalStableKey(proposal)].title }}</strong>
                      <span>{{ item.actionResults[proposalStableKey(proposal)].message }}</span>
                      <div v-if="proposal.tool === 'generate_invitation_code' && invitationCode(item.actionResults[proposalStableKey(proposal)])" class="invitation-code-result">
                        <code>{{ invitationCode(item.actionResults[proposalStableKey(proposal)]) }}</code>
                        <el-button text size="small" @click="copyInvitationCode(item.actionResults[proposalStableKey(proposal)])">复制邀请码</el-button>
                      </div>
                      <small v-if="item.actionResults[proposalStableKey(proposal)].replayed">已返回此前执行结果</small>
                    </div>
                    <el-button
                      v-else
                      type="primary"
                      size="small"
                      :loading="actionConfirming[actionKey(item, proposal)]"
                      :disabled="loading || !proposal.token || actionExpired(proposal)"
                      @click="confirmAction(item, proposal)"
                    >{{ !proposal.token ? '请重新生成提议' : actionExpired(proposal) ? '确认已过期' : '确认执行' }}</el-button>
                  </article>
                </div>
                <div v-if="item.role === 'assistant' && item.clientTurnId" class="message-actions">
                  <span v-if="item.status === 'streaming'" class="message-state">正在生成</span>
                  <span v-else-if="item.status === 'failed'" class="message-state error">生成失败</span>
                  <span v-else-if="item.status === 'stopped'" class="message-state">已停止，未保存</span>
                  <el-button v-if="item.content" text size="small" @click="copyResponse(item)">复制</el-button>
                  <el-button
                    v-if="item.status === 'failed' || item.status === 'stopped'"
                    text
                    size="small"
                    :disabled="loading"
                    @click="retryGeneration(index)"
                  >重试</el-button>
                  <el-button
                    v-if="item.status === 'completed' && isLastAssistant(index)"
                    text
                    size="small"
                    :disabled="loading"
                    @click="regenerateResponse(index)"
                  >重新生成</el-button>
                  <template v-if="item.status === 'completed' && item.id && item.requestId">
                    <el-button
                      text
                      size="small"
                      :type="item.feedback === true ? 'primary' : ''"
                      :loading="feedbackSaving[item.localId]"
                      @click="saveFeedback(item, true)"
                    >有用</el-button>
                    <el-button
                      text
                      size="small"
                      :type="item.feedback === false ? 'danger' : ''"
                      :loading="feedbackSaving[item.localId]"
                      @click="saveFeedback(item, false)"
                    >无用</el-button>
                  </template>
                </div>
                <time v-if="formatMessageTime(item.createdAt)" class="message-time" :datetime="item.createdAt">{{ formatMessageTime(item.createdAt) }}</time>
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
            <el-button v-if="loading" type="danger" plain @click="stopGeneration">停止生成</el-button>
            <el-button v-else type="primary" :disabled="conversationsLoading || historyLoading || Boolean(editingSessionId) || Boolean(renamingSessionId) || !input.trim()" @click="sendMessage">发送</el-button>
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
  /* Cap the panel at the chat card's height so many sessions scroll internally
     instead of stretching the row; mirrors the .messages min(58vh,560px)/320px. */
  max-height: calc(max(min(58vh, 560px), 320px) + 194px);
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
  min-height: 0;
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

.conversation-delete-button {
  flex: none;
  opacity: 0;
  color: var(--text-muted);
  transition: opacity 0.2s, color 0.2s;
}

.conversation-item:hover .conversation-edit-button,
.conversation-edit-button:focus-visible,
.conversation-item:hover .conversation-delete-button,
.conversation-delete-button:focus-visible {
  opacity: 1;
}

.conversation-edit-button:hover:not(:disabled) {
  color: var(--accent);
}

.conversation-delete-button:hover:not(:disabled) {
  color: var(--danger);
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

.message-actions {
  min-height: 28px;
  margin-top: 0.25rem;
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 0.1rem;
}

.message-state {
  margin-right: 0.3rem;
  color: var(--text-muted);
  font-size: 0.7rem;
}

.message-state.error {
  color: var(--danger);
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

.markdown-body :deep(.md-source-label) {
  color: #2563eb;
  font-weight: 600;
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
  flex-direction: column;
  justify-content: flex-end;
  align-items: flex-end;
}

.source-list {
  margin-top: 0.55rem;
  display: grid;
  gap: 0.45rem;
}

.source-list-toggle {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  width: 100%;
  padding: 0;
  border: none;
  background: transparent;
  cursor: pointer;
  text-align: left;
  color: inherit;
}

.source-list-title {
  color: var(--text-secondary);
  font-size: 0.75rem;
  font-weight: 600;
}

.source-list-toggle:hover .source-list-title {
  color: var(--text);
}

.source-list-arrow {
  color: var(--text-muted);
  font-size: 0.8rem;
  transition: transform 0.28s ease;
}

.source-list-toggle.expanded .source-list-arrow {
  transform: rotate(90deg);
}

.source-collapse {
  display: grid;
  grid-template-rows: 0fr;
  opacity: 0;
  transition: grid-template-rows 0.28s ease, opacity 0.25s ease;
}

.source-collapse.expanded {
  grid-template-rows: 1fr;
  opacity: 1;
}

.source-collapse > .source-cards {
  overflow: hidden;
  min-height: 0;
  display: grid;
  gap: 0.45rem;
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

.action-proposal-list {
  margin-top: 0.65rem;
  display: grid;
  gap: 0.55rem;
}

.action-card {
  border: 1px solid color-mix(in srgb, var(--accent) 35%, var(--border));
  border-radius: 10px;
  background: var(--accent-bg);
  padding: 0.75rem;
}

.action-card-heading {
  display: flex;
  align-items: center;
  gap: 0.45rem;
  color: var(--text);
  font-size: 0.84rem;
}

.action-card-kind {
  border-radius: 999px;
  background: var(--bg-card);
  padding: 0.15rem 0.45rem;
  color: var(--accent);
  font-size: 0.68rem;
  font-weight: 600;
}

.action-card p {
  margin: 0.5rem 0;
  color: var(--text-secondary);
  font-size: 0.78rem;
  line-height: 1.5;
}

.action-parameters {
  margin: 0.5rem 0;
  display: grid;
  gap: 0.3rem;
}

.action-parameters > div {
  display: grid;
  grid-template-columns: 3rem minmax(0, 1fr);
  gap: 0.45rem;
  font-size: 0.75rem;
}

.action-parameters dt {
  color: var(--text-muted);
}

.action-parameters dd {
  margin: 0;
  color: var(--text);
  overflow-wrap: anywhere;
}

.action-content-preview {
  max-height: 7.5rem;
  overflow-y: auto;
  white-space: pre-wrap;
}

.action-card-expiry {
  margin-bottom: 0.45rem;
  color: var(--text-muted);
  font-size: 0.68rem;
}

.action-card-expiry.expired {
  color: var(--danger);
}

.action-result {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
  border-radius: 8px;
  background: var(--bg-card);
  padding: 0.55rem 0.65rem;
  color: var(--text-secondary);
  font-size: 0.75rem;
}

.action-result.success strong {
  color: var(--success);
}

.action-result.failed strong {
  color: var(--danger);
}

.invitation-code-result {
  display: flex;
  align-items: center;
  gap: 0.45rem;
}

.invitation-code-result code {
  border-radius: 6px;
  background: var(--bg);
  padding: 0.25rem 0.45rem;
  color: var(--text);
  font-size: 1rem;
  letter-spacing: 0.12em;
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

.sessions-toggle { display: none; }
.sessions-mask { display: none; }

@media (max-width: 860px) {
  .agent-layout {
    grid-template-columns: 190px minmax(0, 1fr);
  }
}

@media (max-width: 680px) {
  .agent-layout {
    grid-template-columns: 1fr;
  }

  .sessions-toggle {
    display: inline-flex;
  }
  .sessions-mask {
    display: block;
  }
  .conversation-panel {
    position: fixed;
    top: 0;
    left: 0;
    bottom: 0;
    width: min(300px, 82vw);
    z-index: 40;
    padding: 0.5rem;
    min-height: 0;
    max-height: none;
    transform: translateX(-100%);
    transition: transform 0.25s cubic-bezier(.2, .8, .2, 1);
    box-shadow: var(--shadow-md);
  }
  .conversation-panel.drawer-open {
    transform: translateX(0);
  }
  .sessions-mask {
    position: fixed;
    inset: 0;
    background: rgba(24, 24, 27, 0.45);
    z-index: 30;
    opacity: 0;
    pointer-events: none;
    transition: opacity 0.2s;
  }
  .sessions-mask.show {
    opacity: 1;
    pointer-events: auto;
  }
  .conversation-list {
    max-height: none;
  }
  .message-bubble,
  .message-content {
    max-width: 92%;
  }
}
@media (max-width: 480px) {
  .messages { height: 52vh; min-height: 260px; }
  .composer { padding: 0.75rem; }
}
</style>
