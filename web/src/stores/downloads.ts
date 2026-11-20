import { defineStore } from 'pinia'
import { ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { knowledgeApi } from '@/api/knowledge'
import { useAuthStore } from '@/stores/auth'
import { saveDownload } from '@/utils/saveDownload'

interface DownloadTask {
  name: string
  status: 'downloading' | 'saving' | 'completed' | 'failed'
  loaded: number
  total?: number
  percent?: number
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${Math.round(bytes)} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

export const useDownloadStore = defineStore('downloads', () => {
  const auth = useAuthStore()
  const tasks = ref<Record<number, DownloadTask>>({})
  const active = new Map<number, { controller: AbortController; promise: Promise<void> }>()
  const cleanupTimers = new Set<ReturnType<typeof setTimeout>>()
  const savedFiles = new Set<() => void>()

  watch(() => auth.user?.id, () => {
    for (const task of active.values()) task.controller.abort()
    active.clear()
    for (const timer of cleanupTimers) clearTimeout(timer)
    cleanupTimers.clear()
    for (const cleanup of savedFiles) cleanup()
    savedFiles.clear()
    tasks.value = {}
  }, { flush: 'sync' })

  function isBusy(id: number): boolean {
    return tasks.value[id]?.status === 'downloading' || tasks.value[id]?.status === 'saving'
  }

  function label(id: number, fallback = '下载'): string {
    const task = tasks.value[id]
    if (!task) return fallback
    if (task.status === 'failed') return '重试下载'
    if (task.status === 'completed') return '已交给浏览器保存'
    if (task.status === 'saving') return '正在准备保存…'
    if (task.percent !== undefined) return `下载中 ${task.percent}%`
    return task.loaded > 0 ? `下载中 · ${formatBytes(task.loaded)}` : '下载中…'
  }

  function download(id: number, name: string): Promise<void> {
    const existing = active.get(id)
    if (existing) return existing.promise
    if (!auth.user?.id) return Promise.resolve()

    const controller = new AbortController()
    tasks.value[id] = { name, status: 'downloading', loaded: 0 }
    const task = tasks.value[id]
    const isCurrent = () => tasks.value[id] === task && !controller.signal.aborted

    const promise = (async () => {
      try {
        const response = await knowledgeApi.downloadFile(id, {
          signal: controller.signal,
          onProgress: (event) => {
            if (!isCurrent()) return
            task.loaded = event.loaded
            task.total = event.total && event.total > 0 ? event.total : undefined
            task.percent = task.total ? Math.min(100, Math.floor(task.loaded * 100 / task.total)) : undefined
            task.status = task.percent === 100 ? 'saving' : 'downloading'
          }
        })
        if (!isCurrent()) return
        const blob = response instanceof Blob ? response : response.data
        if (!(blob instanceof Blob)) throw new Error('Invalid download response')
        const releaseFile = saveDownload(blob, task.name)
        savedFiles.add(releaseFile)
        task.status = 'completed'
        ElMessage.success(`${task.name} 已交给浏览器保存`)
        const timer = setTimeout(() => {
          if (tasks.value[id] === task) delete tasks.value[id]
          releaseFile()
          savedFiles.delete(releaseFile)
          cleanupTimers.delete(timer)
        }, 30000)
        cleanupTimers.add(timer)
      } catch {
        if (!isCurrent()) return
        task.status = 'failed'
        ElMessage.error(`${task.name} 下载失败，请重试`)
      } finally {
        if (active.get(id)?.controller === controller) active.delete(id)
      }
    })()
    active.set(id, { controller, promise })
    return promise
  }

  return { tasks, isBusy, label, download }
})
