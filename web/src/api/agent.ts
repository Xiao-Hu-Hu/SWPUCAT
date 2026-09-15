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

export const agentApi = {
  chat(message: string, sessionId = '') {
    return api.post<AgentChatResponse>('/agent/chat', {
      message,
      session_id: sessionId || undefined
    }, { timeout: 90000 })
  }
}
