import api, { streamPost } from './index'

/**
 * A user-facing knowledge source returned with an Agent answer.
 *
 * `download_url` is intentionally only metadata for the protected download
 * endpoint. The UI must use knowledgeApi.downloadFile instead of navigating to
 * this value directly, so the bearer token is always attached.
 */
export interface AgentKnowledgeSource {
  id: number
  type: string
  name: string
  description?: string
  url?: string
  download_url?: string
  category_name?: string
  source_kind?: string
  excerpt?: string
  score?: number
  match_mode?: string
  created_at?: string
}

export interface AgentChatResponse {
  answer: string
  session_id: string
  sources?: AgentKnowledgeSource[]
  proposals?: AgentActionProposal[]
  user_message_id?: number
  message_id?: number
  client_turn_id: string
  generation_id: string
  request_id?: string
  cached?: boolean
  conversation_persisted: boolean
}

export interface AgentActionProposal {
  tool: string
  title: string
  summary: string
  parameters: Record<string, unknown>
  token?: string
  expires_at: string
}

export interface AgentActionResult {
  tool: string
  status: 'success' | 'failed' | 'pending'
  title: string
  message: string
  data?: Record<string, unknown>
  replayed?: boolean
}

export interface AgentActionRecoverResult {
  result?: AgentActionResult
  proposal?: AgentActionProposal
}

export interface AgentConversation {
  id: number
  session_id: string
  title: string
  last_active_at: string
  created_at: string
}

export interface AgentConversationMessage {
  id: number
  role: 'system' | 'user' | 'assistant' | 'tool'
  content: string
  tool_name?: string
  tool_result?: string
  citations_json?: string
  actions_json?: string
  token_count?: number
  client_turn_id?: string
  generation_id?: string
  request_id?: string
  created_at: string
}

export interface AgentConversationContext {
  conversation: AgentConversation
  messages: AgentConversationMessage[]
}

export interface AgentStreamErrorEvent {
  type: 'error'
  code: number
  message: string
  failure_code?: string
  retryable?: boolean
}

interface AgentDeltaEvent {
  type: 'delta'
  delta: string
}

interface AgentDoneEvent {
  type: 'done'
  data: AgentChatResponse
}

type AgentStreamEvent = AgentDeltaEvent | AgentDoneEvent | AgentStreamErrorEvent

export class AgentStreamError extends Error {
  code: number
  failureCode?: string
  retryable: boolean

  constructor(event: AgentStreamErrorEvent) {
    super(event.message || '智能助手生成失败')
    this.name = 'AgentStreamError'
    this.code = event.code
    this.failureCode = event.failure_code
    this.retryable = Boolean(event.retryable)
  }
}

async function consumeAgentStream(response: Response, onDelta: (delta: string) => void): Promise<AgentChatResponse> {
  const contentType = response.headers.get('content-type')?.toLowerCase() || ''
  if (!contentType.startsWith('application/x-ndjson') || !response.body) {
    throw new Error('智能助手返回了无效的流式响应')
  }
  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  let totalBytes = 0
  const processLine = (line: string): AgentChatResponse | undefined => {
    if (!line.trim()) return
    let event: AgentStreamEvent
    try {
      event = JSON.parse(line) as AgentStreamEvent
    } catch {
      throw new Error('智能助手返回了无法解析的数据')
    }
    if (event.type === 'delta') {
      if (!event.delta) throw new Error('智能助手返回了空的数据片段')
      onDelta(event.delta)
      return
    }
    if (event.type === 'error') throw new AgentStreamError(event)
    if (event.type === 'done' && event.data?.answer && event.data?.session_id) return event.data
    throw new Error('智能助手返回了未知的流式事件')
  }
  try {
    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      totalBytes += value.byteLength
      if (totalBytes > 2 * 1024 * 1024) throw new Error('智能助手响应过大')
      buffer += decoder.decode(value, { stream: true })
      const lines = buffer.split('\n')
      buffer = lines.pop() || ''
      for (const line of lines) {
        const result = processLine(line)
        if (result) return result
      }
    }
    buffer += decoder.decode()
    const result = processLine(buffer)
    if (result) return result
    throw new Error('智能助手连接提前结束，请重试')
  } finally {
    reader.releaseLock()
  }
}

export const agentApi = {
  listConversations(signal?: AbortSignal) {
    return api.get<AgentConversation[]>('/agent/conversations', { signal })
  },
  createConversation(title?: string, signal?: AbortSignal) {
    return api.post<AgentConversation>('/agent/conversations', title ? { title } : {}, { signal })
  },
  getConversation(sessionId: string, signal?: AbortSignal) {
    return api.get<AgentConversationContext>(`/agent/conversations/${encodeURIComponent(sessionId)}`, { signal })
  },
  updateConversationTitle(sessionId: string, title: string) {
    return api.patch<AgentConversation>(`/agent/conversations/${encodeURIComponent(sessionId)}`, { title })
  },
  deleteConversation(sessionId: string) {
    return api.delete(`/agent/conversations/${encodeURIComponent(sessionId)}`)
  },
  chat(message: string, sessionId = '') {
    return api.post<AgentChatResponse>('/agent/chat', {
      message,
      session_id: sessionId || undefined
    }, { timeout: 195000 })
  },
  async chatStream(
    message: string,
    sessionId: string,
    clientTurnId: string,
    generationId: string,
    regenerate: boolean,
    onDelta: (delta: string) => void,
    signal?: AbortSignal
  ) {
    const response = await streamPost('/agent/chat/stream', {
      message,
      session_id: sessionId,
      client_turn_id: clientTurnId,
      generation_id: generationId,
      regenerate: regenerate || undefined
    }, signal)
    return consumeAgentStream(response, onDelta)
  },
  saveFeedback(messageId: number, requestId: string, helpful: boolean, note = '') {
    return api.post(`/agent/messages/${messageId}/feedback`, {
      request_id: requestId,
      helpful,
      note: note || undefined
    })
  },
  confirmAction(sessionId: string, proposal: AgentActionProposal) {
    return api.post<AgentActionResult>('/agent/actions/confirm', {
      session_id: sessionId,
      tool: proposal.tool,
      token: proposal.token || '',
      parameters: proposal.parameters
    }, { timeout: 30000 })
  },
  recoverAction(sessionId: string, proposal: AgentActionProposal) {
    return api.post<AgentActionRecoverResult>('/agent/actions/recover', {
      session_id: sessionId,
      tool: proposal.tool,
      title: proposal.title,
      summary: proposal.summary,
      parameters: proposal.parameters
    }, { timeout: 30000 })
  }
}
