import api from './index'

export const dashboardApi = {
  getDashboard() {
    return api.get('/dashboard')
  }
}
