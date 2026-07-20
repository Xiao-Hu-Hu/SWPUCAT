import api from './index'

export const knowledgeApi = {
  listItems(categoryId?: number, search?: string) {
    return api.get('/knowledge/items', { params: { category_id: categoryId, search } })
  },
  getItem(id: number) {
    return api.get(`/knowledge/items/${id}`)
  },
  createLink(name: string, url: string, categoryId: number) {
    return api.post('/knowledge/links', { name, url, category_id: categoryId })
  },
  uploadFile(fileName: string, fileSize: string, fileKey: string, categoryId: number) {
    return api.post('/knowledge/files', { file_name: fileName, file_size: fileSize, file_key: fileKey, category_id: categoryId })
  },
  deleteItem(id: number) {
    return api.delete(`/knowledge/items/${id}`)
  },
  listCategories() {
    return api.get('/knowledge/categories')
  },
  createCategory(name: string) {
    return api.post('/knowledge/categories', { name })
  },
  deleteCategory(id: number) {
    return api.delete(`/knowledge/categories/${id}`)
  }
}
