import { defineStore } from 'pinia'
import { computed, ref, shallowRef, watch } from 'vue'
import { agentApi, type AgentActionProposal, type AgentActionResult, type AgentConversation, type AgentKnowledgeSource } from '@/api/agent'
import { useAuthStore } from '@/stores/auth'

export interface ChatMessage {
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

type GenerationOutcome = { sessionId: string } & (
  { kind: 'completed' | 'stopped' | 'unsaved' } | { kind: 'failed'; message: string }
)

export function newGreetingMessage(): ChatMessage {
  return {
    role: 'assistant', content: '你好，我可以协助查询知识库、查看打卡状态；涉及打卡或创建草稿时，我会先给出待确认操作。',
    createdAt: new Date().toISOString(), localId: 'greeting', status: 'completed'
  }
}

export function newTurnIdentifier(): string {
  const bytes = new Uint8Array(16)
  crypto.getRandomValues(bytes)
  return Array.from(bytes, (value) => value.toString(16).padStart(2, '0')).join('')
}

export const useAgentStore = defineStore('agent', () => {
  const auth = useAuthStore()
  const sessionId = ref('')
  const messages = ref<ChatMessage[]>([newGreetingMessage()])
  const conversations = ref<AgentConversation[]>([])
  const controller = shallowRef<AbortController | null>(null)
  const loading = computed(() => controller.value !== null)
  const outcome = shallowRef<GenerationOutcome | null>(null)

  const sessionStorageKey = () => `swpucat-agent-session:${auth.user?.id || 'anonymous'}`

  function activateSession(id: string) {
    sessionId.value = id
    if (id) localStorage.setItem(sessionStorageKey(), id)
    else localStorage.removeItem(sessionStorageKey())
  }

  function stopGeneration() {
    controller.value?.abort()
  }

  watch(() => auth.user?.id, () => {
    stopGeneration()
    controller.value = null
    sessionId.value = ''
    messages.value = [newGreetingMessage()]
    conversations.value = []
    outcome.value = null
  }, { flush: 'sync' })

  async function sendMessage(message: string) {
    if (!message) return
    await run(async (signal) => {
      if (!sessionId.value) {
        const { data: created } = await agentApi.createConversation(undefined, signal)
        if (signal.aborted) return
        activateSession(created.session_id)
        conversations.value = [created, ...conversations.value.filter(item => item.session_id !== created.session_id)]
      }

      const clientTurnId = newTurnIdentifier()
      const generationId = newTurnIdentifier()
      messages.value.push({
        role: 'user', content: message, createdAt: new Date().toISOString(), localId: `user-${clientTurnId}`,
        status: 'completed', clientTurnId, generationId, sourceMessage: message
      }, {
        role: 'assistant', content: '', createdAt: new Date().toISOString(), localId: `assistant-${clientTurnId}`,
        status: 'pending', clientTurnId, generationId, sourceMessage: message, sources: []
      })
      return messages.value[messages.value.length - 1]
    })
  }

  async function generateResponse(target: ChatMessage) {
    await run(() => target)
  }

  async function run(prepare: (signal: AbortSignal) => ChatMessage | undefined | Promise<ChatMessage | undefined>) {
    const ownerId = auth.user?.id
    if (!ownerId || loading.value) return
    const active = new AbortController()
    controller.value = active
    outcome.value = null
    const isCurrent = () => controller.value === active && auth.user?.id === ownerId
    let target: ChatMessage | undefined
    let previous: ChatMessage | undefined

    try {
      target = await prepare(active.signal)
      if (!isCurrent()) return
      if (active.signal.aborted) {
        if (target?.status === 'pending') target.status = 'stopped'
        outcome.value = { kind: 'stopped', sessionId: sessionId.value }
        return
      }
      if (!target) return
      const { sourceMessage, clientTurnId, generationId } = target
      const targetSessionId = sessionId.value
      if (!sourceMessage || !clientTurnId || !generationId || !targetSessionId) return
      const answer = target
      previous = { ...answer }
      answer.status = 'pending'
      answer.content = ''
      answer.sources = []
      answer.feedback = undefined

      const data = await agentApi.chatStream(
        sourceMessage, targetSessionId, clientTurnId, generationId, Boolean(answer.regenerate),
        (delta) => {
          if (!isCurrent() || active.signal.aborted) return
          answer.status = 'streaming'
          answer.content += delta
        },
        active.signal
      )
      if (!isCurrent()) return
      answer.content = data.answer
      answer.sources = data.sources || []
      answer.proposals = data.proposals || []
      answer.actionResults = {}
      answer.createdAt = new Date().toISOString()
      answer.id = data.message_id
      answer.clientTurnId = data.client_turn_id || answer.clientTurnId
      answer.generationId = data.generation_id || answer.generationId
      answer.requestId = data.request_id
      answer.status = data.conversation_persisted ? 'completed' : 'failed'
      if (data.conversation_persisted) {
        answer.regenerate = false
        outcome.value = { kind: 'completed', sessionId: targetSessionId }
        try {
          const { data: items } = await agentApi.listConversations(active.signal)
          if (isCurrent() && Array.isArray(items)) conversations.value = items
        } catch {
          // A list failure does not change the saved answer.
        }
      } else {
        outcome.value = { kind: 'unsaved', sessionId: targetSessionId }
      }
    } catch (error) {
      if (!isCurrent()) return
      const stopped = active.signal.aborted
      if (target) {
        if (previous?.regenerate) Object.assign(target, previous)
        target.status = stopped ? 'stopped' : 'failed'
      }
      outcome.value = stopped
        ? { kind: 'stopped', sessionId: sessionId.value }
        : { kind: 'failed', sessionId: sessionId.value, message: error instanceof Error ? error.message : '智能助手暂时不可用，请稍后重试' }
    } finally {
      if (isCurrent()) controller.value = null
    }
  }

  return { messages, conversations, sessionId, loading, outcome, sessionStorageKey, activateSession, sendMessage, generateResponse, stopGeneration }
})
