import api from './index'

export const approvalApi = {
  listPending() {
    return api.get('/approvals')
  },
  submit(fileName: string, fileSize: string, fileKey: string, categoryId: number) {
    return api.post('/approvals', { file_name: fileName, file_size: fileSize, file_key: fileKey, category_id: categoryId })
  },
  approve(id: number) {
    return api.post(`/approvals/${id}/approve`)
  },
  reject(id: number) {
    return api.post(`/approvals/${id}/reject`)
  }
}
