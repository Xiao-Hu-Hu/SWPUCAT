const assert = require('node:assert/strict')
const { test } = require('node:test')
const { readFileSync } = require('node:fs')
const { resolve } = require('node:path')
const vm = require('node:vm')
const ts = require('typescript')
const vue = require('vue')
const pinia = require('pinia')

function fixture() {
  const requests = [], saved = [], notices = [], timers = new Map()
  const auth = vue.reactive({ user: { id: 1 } })
  const imports = {
    vue, pinia,
    '@/stores/auth': { useAuthStore: () => auth },
    '@/api/knowledge': { knowledgeApi: { downloadFile(id, options) {
      let resolve, reject
      const promise = new Promise((yes, no) => { resolve = yes; reject = no })
      requests.push({ id, options, resolve, reject })
      return promise
    } } },
    '@/utils/saveDownload': { saveDownload(blob, name) {
      const item = { blob, name, released: false }
      saved.push(item)
      return () => { item.released = true }
    } },
    'element-plus': { ElNotification: options => notices.push(options) }
  }
  const source = readFileSync(resolve(__dirname, '../src/stores/downloads.ts'), 'utf8')
  const code = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 }
  }).outputText
  const exports = {}
  vm.runInNewContext(code, {
    exports, require: name => { assert.ok(name in imports, name); return imports[name] },
    Blob, AbortController,
    setTimeout(fn) { const id = {}; timers.set(id, fn); return id },
    clearTimeout(id) { timers.delete(id) }
  })
  const instance = pinia.createPinia()
  const store = exports.useDownloadStore(instance)
  return { store, requests, saved, notices, auth, timers, useStore: () => exports.useDownloadStore(instance) }
}

test('new page consumers share progress and reuse an active file request', async () => {
  const f = fixture()
  const first = f.store.download(7, '资料.pdf')
  f.requests[0].options.onProgress({ loaded: 40, total: 100 })
  const returnedPage = f.useStore()
  assert.equal(returnedPage.label(7), '下载中 40%')
  const repeated = returnedPage.download(7, '资料.pdf')
  assert.equal(f.requests.length, 1)
  f.requests[0].resolve(new Blob(['file']))
  await Promise.all([first, repeated])
  assert.equal(f.saved.length, 1)
  assert.equal(f.notices.length, 1)
  assert.equal(returnedPage.label(7), '下载')
  assert.equal(returnedPage.label(7, '点击下载'), '点击下载')
  assert.equal(returnedPage.tasks[7], undefined)
  assert.equal(f.notices[0].title, '下载完成')
  assert.equal(f.notices[0].position, 'top-right')
  assert.equal(f.notices[0].duration, 3000)
  assert.equal(returnedPage.isBusy(7), false)
})

test('unknown totals show received bytes, and 100 percent waits for the response', async () => {
  const f = fixture()
  const pending = f.store.download(8, '资料.pdf')
  f.requests[0].options.onProgress({ loaded: 2048 })
  assert.equal(f.store.label(8), '下载中 · 2.0 KB')
  f.requests[0].options.onProgress({ loaded: 4096, total: 4096 })
  assert.equal(f.store.label(8), '正在准备保存…')
  assert.equal(f.saved.length, 0)
  assert.equal(f.store.isBusy(8), true)
  f.requests[0].resolve({ data: new Blob(['file']) })
  await pending
  assert.equal(f.saved.length, 1)
})

test('failed downloads can retry without losing other concurrent tasks', async () => {
  const f = fixture()
  const first = f.store.download(7, 'A.pdf')
  const other = f.store.download(8, 'B.pdf')
  f.requests[0].reject(Error('offline'))
  await first
  assert.equal(f.store.label(7), '重试下载')
  assert.equal(f.store.isBusy(8), true)
  const retry = f.store.download(7, 'A.pdf')
  f.requests[1].resolve(new Blob(['B']))
  f.requests[2].resolve(new Blob(['A']))
  await Promise.all([other, retry])
  assert.equal(f.saved.length, 2)
})

test('logout cancels downloads and ignores late results and progress', async () => {
  const f = fixture()
  const pending = f.store.download(7, '资料.pdf')
  f.auth.user = null
  assert.equal(f.requests[0].options.signal.aborted, true)
  assert.equal(Object.keys(f.store.tasks).length, 0)
  f.requests[0].options.onProgress({ loaded: 80, total: 100 })
  f.requests[0].resolve(new Blob(['file']))
  await pending
  assert.equal(f.saved.length, 0)
  assert.equal(f.notices.length, 0)
})

test('old completion cleanup never removes a new download for the same file', async () => {
  const f = fixture()
  const first = f.store.download(7, '资料.pdf')
  f.requests[0].resolve(new Blob(['file']))
  await first
  const cleanup = [...f.timers.values()][0]
  const second = f.store.download(7, '资料.pdf')
  cleanup()
  assert.equal(f.saved[0].released, true)
  assert.equal(f.store.isBusy(7), true)
  f.auth.user = null
  f.requests[1].reject(Error('canceled'))
  await second
  assert.equal(f.notices.filter(item => item.type === 'error').length, 0)
})
