import api from './index'

export const checkinApi = {
  getStatus() {
    return api.get('/checkin/status')
  },
  clockIn() {
    return api.post('/checkin/clock-in')
  },
  clockOut() {
    return api.post('/checkin/clock-out')
  },
  getRecords(limit = 10) {
    return api.get('/checkin/records', { params: { limit } })
  },
  getStats(period = 'week') {
    return api.get('/checkin/stats', { params: { period } })
  },
  getOnlineMembers() {
    return api.get('/checkin/online')
  },
  getTodayRecords() {
    return api.get('/checkin/today-records')
  },
  makeup(userId: number, date: string, minutes: number) {
    return api.post('/checkin/makeup', { user_id: userId, date, minutes })
  },
  getRequirements() {
    return api.get('/checkin/requirements')
  },
  setRequirements(requirements: { grade: number; minutes: number }[]) {
    return api.post('/checkin/requirements', { requirements })
  },
  publishReport() {
    return api.post('/checkin/report')
  }
}
