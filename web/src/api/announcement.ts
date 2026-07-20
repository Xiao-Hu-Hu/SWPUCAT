import api from './index'

export const announcementApi = {
  list() {
    return api.get('/announcements')
  },
  create(title: string, content: string, pinned = false) {
    return api.post('/announcements', { title, content, pinned })
  },
  update(id: number, title: string, content: string) {
    return api.put(`/announcements/${id}`, { title, content })
  },
  delete(id: number) {
    return api.delete(`/announcements/${id}`)
  }
}
