import api from './index'

export interface CostEstimate {
  known: boolean
  micro_usd?: number
}

export interface AgentAuditRun {
  id: number
  request_id: string
  user_id?: number
  session_id?: string
  actor_role?: string
  status: string
  telemetry_status: string
  http_status?: number
  failure_category?: string
  failure_code?: string
  duration_ms: number
  model_call_count?: number
  tool_call_count?: number
  prompt_tokens?: number
  completion_tokens?: number
  total_tokens?: number
  cached_tokens?: number
  estimated_cost: CostEstimate
  created_at: string
}

export interface AgentAuditToolCall {
  sequence: number
  name: string
  is_write: boolean
  status: string
  duration_ms: number
  arguments_json: string
  result_json: string
  arguments_redacted: boolean
  result_redacted: boolean
  error_category?: string
  error_code?: string
}

export interface AgentAuditDetail extends AgentAuditRun {
  tool_calls: AgentAuditToolCall[]
}

export interface AgentAuditPage {
  items: AgentAuditRun[]
  total: number
  page: number
  page_size: number
}

export interface AgentAuditSummary {
  from: string
  to: string
  total_runs: number
  success_runs: number
  success_rate: number
  average_duration_ms: number
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
  cached_tokens: number
  runs_with_token_usage: number
  estimated_cost: CostEstimate
  cost_known_runs: number
  status_counts: Record<string, number>
  tool_counts: Record<string, number>
}

export interface AgentAuditQuery {
  from: string
  to: string
  status?: string
  page?: number
  page_size?: number
}

export const agentAuditApi = {
  list(params: AgentAuditQuery) {
    return api.get<AgentAuditPage>('/agent/audits', { params })
  },
  summary(params: AgentAuditQuery) {
    return api.get<AgentAuditSummary>('/agent/audits/summary', { params })
  },
  detail(id: number) {
    return api.get<AgentAuditDetail>(`/agent/audits/${id}`)
  }
}
