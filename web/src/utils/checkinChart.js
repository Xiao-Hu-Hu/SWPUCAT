/**
 * Convert the public check-in statistics response into safe chart rows.
 *
 * The API boundary is deliberately treated as unknown data here. Keeping the
 * conversion deterministic prevents a single malformed row from making
 * ECharts reject the complete series. Administrators are not removed by
 * display name: a deployment that only has the bootstrap administrator still
 * has one valid statistics row and should render it instead of a blank chart.
 *
 * @param {unknown} value
 * @returns {Array<{user_id: number, nickname: string, total_minutes: number, total_hours: number, days: number}>}
 */
export function normalizeCheckinStats(value) {
  if (!Array.isArray(value)) return []

  return value
    .map((item) => {
      const row = item && typeof item === 'object' ? item : {}
      const userId = Number(row.user_id)
      const minutes = Number(row.total_minutes)
      const hours = Number(row.total_hours)
      const days = Number(row.days)
      return {
        user_id: Number.isFinite(userId) ? userId : 0,
        nickname: String(row.nickname || '').trim() || `成员 ${userId}`,
        total_minutes: Number.isFinite(minutes) ? Math.max(0, minutes) : 0,
        total_hours: Number.isFinite(hours) ? Math.max(0, hours) : 0,
        days: Number.isFinite(days) ? Math.max(0, Math.trunc(days)) : 0
      }
    })
    .filter((item) => item.user_id > 0)
    .sort((left, right) => left.nickname.localeCompare(right.nickname, 'zh-CN') || left.user_id - right.user_id)
}
