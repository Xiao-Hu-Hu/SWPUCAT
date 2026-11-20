import api from './index'
import type { AxiosProgressEvent } from 'axios'

interface UploadOptions {
  onProgress?: (event: AxiosProgressEvent) => void
  signal?: AbortSignal
}

interface DownloadOptions {
  onProgress?: (event: AxiosProgressEvent) => void
  signal?: AbortSignal
}

export const knowledgeApi = {
  listItems(categoryId?: number, search?: string) {
    return api.get('/knowledge/items', { params: { category_id: categoryId, search } })
  },
  getItem(id: number) {
    return api.get(`/knowledge/items/${id}`)
  },
  createLink(name: string, url: string, categoryId: number, description?: string) {
    return api.post('/knowledge/links', { name, url, category_id: categoryId, description: description || '' })
  },
  uploadFile(file: File, categoryId: number, description?: string, options: UploadOptions = {}) {
    const formData = new FormData()
    formData.append('file', file)
    formData.append('category_id', categoryId.toString())
    if (description) {
      formData.append('description', description)
    }
    return api.post('/knowledge/files', formData, {
      timeout: 0,
      onUploadProgress: options.onProgress,
      signal: options.signal
    })
  },
  deleteItem(id: number) {
    return api.delete(`/knowledge/items/${id}`)
  },
  downloadFile(id: number, options: DownloadOptions = {}) {
    return api.get<Blob>(`/knowledge/download/${id}`, {
      responseType: 'blob',
      timeout: 0,
      onDownloadProgress: options.onProgress,
      signal: options.signal
    })
  },
  listCategories() {
    return api.get('/knowledge/categories')
  },
  createCategory(name: string) {
    return api.post('/knowledge/categories', { name })
  },
  deleteCategory(id: number) {
    return api.delete(`/knowledge/categories/${id}`)
  },
  listPendingItems() {
    return api.get('/knowledge/items/pending')
  },
  listQuarantinedItems() {
    return api.get('/knowledge/items/quarantined')
  },
  quarantineItem(id: number, reason: string) {
    return api.post(`/knowledge/items/${id}/quarantine`, { reason })
  },
  restoreItem(id: number, reason: string) {
    return api.post(`/knowledge/items/${id}/restore`, { reason })
  },
  approveItem(id: number) {
    return api.put(`/knowledge/items/${id}/approve`)
  },
  rejectItem(id: number, reason: string) {
    return api.put(`/knowledge/items/${id}/reject`, { reason })
  },
  listUserItems() {
    return api.get('/knowledge/items/my')
  }
}
