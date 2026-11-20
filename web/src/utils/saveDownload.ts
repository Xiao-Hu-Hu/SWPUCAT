/** Hand the received file to the browser and release its object URL after use. */
export function saveDownload(blob: Blob, filename: string): () => void {
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename.replace(/[\\/:*?"<>|\u0000-\u001f]/g, '_').trim() || '知识库文件'
  let cleanupTimer: ReturnType<typeof setTimeout> | undefined
  const cleanup = () => {
    clearTimeout(cleanupTimer)
    URL.revokeObjectURL(url)
  }
  try {
    document.body.appendChild(link)
    link.click()
    cleanupTimer = setTimeout(cleanup, 30000)
    return cleanup
  } catch (error) {
    cleanup()
    throw error
  } finally {
    link.remove()
  }
}
