import api from './index'

export interface AgentChatResponse {
  answer: string
  session_id: string
}

export const agentApi = {
  chat(message: string, sessionId = '') {
    return api.post<AgentChatResponse>('/agent/chat', {
      message,
      session_id: sessionId || undefined
    })
  }
}
