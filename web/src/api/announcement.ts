import api from './index'

export const announcementApi = {
  list() {
    return api.get('/announcements')
  },
  create(title: string, content: string, pinned = false) {
    return api.post('/announcements', { title, content, pinned })
  },
  update(id: number, title: string, content: string, pinned?: boolean) {
    return api.put(`/announcements/${id}`, { title, content, pinned })
  },
  delete(id: number) {
    return api.delete(`/announcements/${id}`)
  },
  notify(id: number, userIds: number[]) {
    return api.post(`/announcements/${id}/notify`, { user_ids: userIds })
  }
}
