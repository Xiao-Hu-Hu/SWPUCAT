const assert = require('node:assert/strict')
const { test } = require('node:test')
const { readFileSync } = require('node:fs')
const { resolve } = require('node:path')
const vm = require('node:vm')
const ts = require('typescript')
const vue = require('vue')
const { parse, compileScript } = require('@vue/compiler-sfc')

// Compile the real component and exercise its upload behavior with a controllable
// transport. No browser, backend, or additional test dependencies are required.
function evaluate(source, imports) {
  const js = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 }
  }).outputText
  const exports = {}
  vm.runInNewContext(js, {
    exports, require: (name) => {
      if (!(name in imports)) throw new Error(`Unexpected import: ${name}`)
      return imports[name]
    },
    AbortController, FormData, performance, console, setTimeout
  })
  return exports
}

function deferred() {
  let resolve, reject
  const promise = new Promise((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}

const descriptor = parse(readFileSync(resolve(__dirname, '../src/views/KnowledgeView.vue'), 'utf8')).descriptor
const componentSource = compileScript(descriptor, { id: 'upload-regression' }).content

function fixture() {
  const upload = deferred()
  const items = deferred()
  const categories = deferred()
  const messages = []
  const calls = []
  let options, unmount
  const api = {
    uploadFile: (_file, _category, _description, opts) => {
      options = opts
      opts.signal.addEventListener('abort', () => upload.reject({ code: 'ERR_CANCELED' }), { once: true })
      return upload.promise
    },
    listItems: () => { calls.push('items'); return items.promise },
    listCategories: () => { calls.push('categories'); return categories.promise }
  }
  const component = evaluate(componentSource, {
    vue: { ...vue, onMounted: () => {}, onBeforeUnmount: (fn) => { unmount = fn } },
    '@/api/knowledge': { knowledgeApi: api },
    '@/stores/auth': { useAuthStore: () => ({ isCaptain: true }) },
    'element-plus': {
      ElMessage: Object.fromEntries(['success', 'error', 'warning', 'info'].map(kind => [kind, text => messages.push({ kind, text })])),
      ElMessageBox: {}
    },
    marked: { marked: { parse: text => text } }
  }).default
  const state = component.setup({}, { expose() {} })
  const file = new File(['test file'], 'test.pdf')
  state.handleFileChange({ raw: file })
  state.uploadForm.value.category_id = 2
  return { state, upload, items, categories, messages, calls, file, get options() { return options }, unmount: () => unmount() }
}

const flush = () => new Promise(resolve => setImmediate(resolve))

test('the API sends the original file with progress and cancellation options', () => {
  let sent
  const { knowledgeApi } = evaluate(readFileSync(resolve(__dirname, '../src/api/knowledge.ts'), 'utf8'), {
    './index': { default: { post: (...args) => { sent = args } } }
  })
  const file = new File(['payload'], '资料.pdf')
  const controller = new AbortController()
  const progress = () => {}
  knowledgeApi.uploadFile(file, 2, '说明', { signal: controller.signal, onProgress: progress })
  assert.equal(sent[0], '/knowledge/files')
  assert.equal(sent[1].get('file'), file)
  assert.equal(sent[1].get('category_id'), '2')
  assert.equal(sent[1].get('description'), '说明')
  assert.equal(sent[2].signal, controller.signal)
  assert.equal(sent[2].onUploadProgress, progress)
  assert.equal(sent[2].headers, undefined)
})

test('100% means waiting for the server; refreshes run together after upload finishes', async () => {
  const f = fixture()
  const task = f.state.handleUploadFile()
  f.options.onProgress({ loaded: 50, total: 100, rate: 10, estimated: 5 })
  assert.equal(f.state.uploadPercent.value, 50)
  assert.equal(f.state.uploadSpeed.value, 10)
  assert.equal(f.state.uploadRemaining.value, 5)
  f.options.onProgress({ loaded: 100, total: 100 })
  assert.equal(f.state.uploadPhase.value, 'saving')
  assert.equal(f.state.uploading.value, true)
  assert.equal(f.messages.length, 0)
  f.upload.resolve({})
  await task
  assert.equal(f.state.uploading.value, false)
  assert.equal(f.state.refreshingCount.value, 1)
  assert.deepEqual(f.calls, ['items', 'categories'])
  assert.equal(f.state.selectedFile.value, null)
  f.items.resolve({ data: [] })
  f.categories.resolve({ data: [] })
  await flush()
  assert.equal(f.state.refreshingCount.value, 0)
})

test('unknown totals show bytes without inventing a percentage', async () => {
  const f = fixture()
  const task = f.state.handleUploadFile()
  f.options.onProgress({ loaded: 100 })
  assert.equal(f.state.uploadPercent.value, null)
  assert.equal(f.state.uploadedBytes.value, 100)
  assert.equal(f.state.uploadPhase.value, 'sending')
  f.upload.reject({ code: 'ERR_NETWORK' })
  await task
  assert.equal(f.state.uploading.value, false)
  assert.equal(f.state.selectedFile.value, f.file)
  assert.equal(f.messages.at(-1).kind, 'error')
})

test('cancel aborts the request and preserves the selected file for retry', async () => {
  const f = fixture()
  const task = f.state.handleUploadFile()
  f.state.handleCancelUpload()
  assert.equal(f.options.signal.aborted, true)
  await task
  assert.equal(f.state.uploading.value, false)
  assert.equal(f.state.selectedFile.value, f.file)
  assert.equal(f.messages.some(m => m.kind === 'error' || m.kind === 'success'), false)
  f.items.resolve({ data: [] })
  f.categories.resolve({ data: [] })
  await flush()
})

test('leaving the page aborts the request without a stray notification or refresh', async () => {
  const f = fixture()
  const task = f.state.handleUploadFile()
  f.unmount()
  await task
  assert.equal(f.options.signal.aborted, true)
  assert.equal(f.messages.length, 0)
  assert.equal(f.calls.length, 0)
})
