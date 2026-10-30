import api from './index'

export interface AnnouncementDraft {
  id: number
  status: 'draft' | 'confirmed' | 'published' | 'discarded'
  title: string
  content: string
  pinned: boolean
  source_json?: string
  recipient_ids: number[]
  email_requested: boolean
  email_result_json?: string
  announcement_id?: number
  created_by: number
  created_at: string
  updated_at: string
  confirmed_at?: string
  published_at?: string
}

export const announcementDraftApi = {
  list(status = '') {
    return api.get<AnnouncementDraft[]>('/announcement-drafts', { params: { status } })
  },
  update(id: number, title: string, content: string, pinned: boolean) {
    return api.put<AnnouncementDraft>(`/announcement-drafts/${id}`, { title, content, pinned })
  },
  confirm(id: number) {
    return api.post<AnnouncementDraft>(`/announcement-drafts/${id}/confirm`)
  },
  publish(id: number) {
    return api.post<AnnouncementDraft>(`/announcement-drafts/${id}/publish`)
  },
  discard(id: number) {
    return api.post(`/announcement-drafts/${id}/discard`)
  }
}
