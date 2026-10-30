export interface CheckinChartStat {
  user_id: number
  nickname: string
  total_minutes: number
  total_hours: number
  days: number
}

export function normalizeCheckinStats(value: unknown): CheckinChartStat[]
