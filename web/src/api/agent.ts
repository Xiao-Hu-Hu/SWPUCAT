import api from './index'

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
  token_count?: number
  created_at: string
}

export interface AgentConversationContext {
  conversation: AgentConversation
  messages: AgentConversationMessage[]
}

export const agentApi = {
  listConversations() {
    return api.get<AgentConversation[]>('/agent/conversations')
  },
  createConversation(title?: string) {
    return api.post<AgentConversation>('/agent/conversations', title ? { title } : {})
  },
  getConversation(sessionId: string) {
    return api.get<AgentConversationContext>(`/agent/conversations/${encodeURIComponent(sessionId)}`)
  },
  chat(message: string, sessionId = '') {
    return api.post<AgentChatResponse>('/agent/chat', {
      message,
      session_id: sessionId || undefined
    }, { timeout: 90000 })
  }
}
