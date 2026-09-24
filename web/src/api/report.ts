import api from './index'

export interface ReportPeriod {
  id: number
  period_key: string
  starts_on: string
  ends_on: string
  timezone: string
  due_at?: string
  status: string
  roster_version: number
  member_count: number
}
export interface PeriodMember {
  user_id: number
  display_name: string
  required: boolean
  exempt: boolean
  exemption_reason?: string
}

export interface RosterEntry {
  user_id: number
  display_name: string
  required: boolean
  exempt: boolean
  exemption_reason?: string
  report_id?: number
  report_status?: string
  submitted_at?: string
}

export interface Report {
  id: number
  period_id: number
  report_type: string
  author_id: number
  author_name: string
  status: string
  visibility: string
  current_version: number
  late_submission?: boolean
  created_at: string
  updated_at: string
}

export interface ReportAsset {
  report_version_id: number
  knowledge_item_id: number
  asset_role: string
  locator?: string
  note?: string
}

export interface ReportSection {
  kind: string
  heading: string
  content: string
  start_line: number
  end_line: number
}

export interface ParsedReport {
  parser_version: string
  sections: ReportSection[]
  links: Array<{ url: string; line: number; section?: string }>
  warnings?: string[]
}

export interface QualityFinding {
  index: number
  kind: string
  message: string
  suggestion: string
  start_line?: number
  end_line?: number
  original?: string
}

export interface QualityResult {
  status: string
  findings: QualityFinding[]
}

export interface ReportVersion {
  id: number
  report_id: number
  version: number
  status: string
  title: string
  body_markdown: string
  content_hash?: string
  parse_status?: string
  parse_error?: string
  parser_version?: string
  parsed_json?: ParsedReport
  quality_status?: string
  quality_json?: QualityResult
  quality_checked_at?: string
  submitted_at?: string
  created_at: string
  assets?: ReportAsset[]
}

export interface CreatePeriodPayload {
  period_key: string
  starts_on: string
  ends_on: string
  timezone?: string
  due_at?: string
}

export interface AddMemberPayload {
  user_id: number
  display_name: string
  required: boolean
  exempt: boolean
  exemption_reason?: string
}

export interface CreateReportPayload {
  period_id: number
  report_type: string
}

export interface CreateVersionPayload {
  expected_version: number
  title: string
  body_markdown: string
}

export interface AttachAssetPayload {
  knowledge_item_id: number
  asset_role: string
  locator?: string
  note?: string
}

export const reportApi = {
  listPeriods() {
    return api.get<ReportPeriod[]>('/report-periods')
  },
  listRoster(periodId: number) {
    return api.get<RosterEntry[]>(`/report-periods/${periodId}/members`)
  },
  isPeriodMember(periodId: number) {
    return api.get<{ is_member: boolean }>(`/report-periods/${periodId}/is-member`)
  },
  createPeriod(data: CreatePeriodPayload) {
    return api.post<ReportPeriod>('/report-periods', data)
  },
  addPeriodMember(periodId: number, data: AddMemberPayload) {
    return api.post(`/report-periods/${periodId}/members`, data)
  },
  listReports(params?: { period_id?: number; status?: string; page?: number; page_size?: number }) {
    return api.get<Report[]>('/reports', { params })
  },
  getReport(id: number) {
    return api.get<Report>(`/reports/${id}`)
  },
  createReport(data: CreateReportPayload) {
    return api.post<Report>('/reports', data)
  },
  createVersion(reportId: number, data: CreateVersionPayload) {
    return api.post<ReportVersion>(`/reports/${reportId}/versions`, data)
  },
  getVersion(reportId: number, version: number) {
    return api.get<ReportVersion>(`/reports/${reportId}/versions/${version}`)
  },
  getSubmittedVersion(reportId: number) {
    return api.get<ReportVersion>(`/reports/${reportId}/submitted-version`)
  },
  submitVersion(reportId: number, version: number) {
    return api.post<ReportVersion>(`/reports/${reportId}/versions/${version}/submit`)
  },
  withdrawVersion(reportId: number, version: number) {
    return api.post<ReportVersion>(`/reports/${reportId}/versions/${version}/withdraw`)
  },
  reviewVersion(reportId: number, version: number, approved: boolean) {
    return api.post<ReportVersion>(`/reports/${reportId}/versions/${version}/review`, { approved })
  },
  checkQuality(reportId: number, version: number) {
    return api.post<ReportVersion>(`/reports/${reportId}/versions/${version}/check`)
  },
  attachAsset(reportId: number, version: number, data: AttachAssetPayload) {
    return api.post<ReportAsset>(`/reports/${reportId}/versions/${version}/assets`, data)
  },
  decideQuality(reportId: number, version: number, finding: number, accepted: boolean, note?: string) {
    return api.post(`/reports/${reportId}/versions/${version}/quality/${finding}/decision`, { accepted, note: note || undefined })
  }
}
