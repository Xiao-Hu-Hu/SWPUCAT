import assert from 'node:assert/strict'
import test from 'node:test'
import { normalizeCheckinStats } from '../src/utils/checkinChart.js'

test('normalizes and orders chart rows without dropping the administrator', () => {
  const result = normalizeCheckinStats([
    { user_id: 2, nickname: '成员乙', total_minutes: '125', total_hours: 2.08, days: 2.9 },
    { user_id: 1, nickname: '超级管理员', total_minutes: 60, total_hours: 1, days: 1 }
  ])

  assert.deepEqual(result, [
    { user_id: 1, nickname: '超级管理员', total_minutes: 60, total_hours: 1, days: 1 },
    { user_id: 2, nickname: '成员乙', total_minutes: 125, total_hours: 2.08, days: 2 }
  ])
})

test('drops invalid identities and clamps malformed numeric values', () => {
  const result = normalizeCheckinStats([
    null,
    { user_id: 0, nickname: '系统', total_minutes: 20 },
    { user_id: 7, nickname: ' ', total_minutes: -10, total_hours: 'bad', days: -2 }
  ])

  assert.deepEqual(result, [
    { user_id: 7, nickname: '成员 7', total_minutes: 0, total_hours: 0, days: 0 }
  ])
  assert.deepEqual(normalizeCheckinStats({ data: [] }), [])
})
