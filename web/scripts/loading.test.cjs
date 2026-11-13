const assert = require('node:assert/strict')
const { test } = require('node:test')
const { readFileSync } = require('node:fs')
const { resolve } = require('node:path')
const vm = require('node:vm')
const ts = require('typescript')
const vue = require('vue')
const { parse, compileScript } = require('@vue/compiler-sfc')
function deferred() { let resolve, reject; const promise = new Promise((yes,no) => { resolve=yes; reject=no }); return {promise,resolve,reject} }
function fixture() {
  const requests=[]
  const descriptor=parse(readFileSync(resolve(__dirname,'../src/views/AnnouncementsView.vue'),'utf8')).descriptor
  const source=compileScript(descriptor,{id:'loading-test'}).content
  const code=ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2020}}).outputText
  let mounted
  const imports={
    vue:{...vue,onMounted:fn=>{mounted=fn}},
    '@/api/announcement':{announcementApi:{list:()=>{const request=deferred();requests.push(request);return request.promise}}},
    '@/api/announcementDraft':{announcementDraftApi:{}}, '@/api/member':{memberApi:{}},
    '@/stores/auth':{useAuthStore:()=>({isCaptain:false,isSuperAdmin:false})},
    'element-plus':{}, '@/utils/markdown':{},
    '@/composables/usePaginationLayout':{usePaginationLayout:()=>({})}
  }
  const exports={};vm.runInNewContext(code,{exports,require:name=>{if(!(name in imports))throw Error(name);return imports[name]},console})
  return {state:exports.default.setup({},{expose(){}}),requests,mount:()=>mounted()}
}
test('slow request keeps loading until actual announcements arrive', async()=>{
  const f=fixture();assert.equal(f.state.loading.value,true)
  const pending=f.mount();assert.equal(f.state.loading.value,true);assert.equal(f.state.announcements.value.length,0)
  f.requests[0].resolve({data:[{id:1,title:'团队公告'}]});await pending
  assert.equal(f.state.loading.value,false);assert.equal(f.state.announcements.value[0].title,'团队公告');assert.equal(f.state.loadError.value,'')
})
test('failed request exits loading and retry recovers to a genuine empty result',async()=>{
  const f=fixture();const first=f.mount();f.requests[0].reject(Error('offline'));await first
  assert.equal(f.state.loading.value,false);assert.ok(f.state.loadError.value)
  const retry=f.state.loadAnnouncements();assert.equal(f.state.loading.value,true);assert.equal(f.state.loadError.value,'')
  f.requests[1].resolve({data:[]});await retry
  assert.equal(f.state.loading.value,false);assert.equal(f.state.loadError.value,'');assert.equal(f.state.announcements.value.length,0)
})
test('refresh retains existing cards and failed refresh does not erase them',async()=>{
  const f=fixture();const first=f.mount();f.requests[0].resolve({data:[{id:1}]});await first
  const refresh=f.state.loadAnnouncements();assert.equal(f.state.loading.value,true);assert.equal(f.state.announcements.value.length,1)
  f.requests[1].reject(Error('timeout'));await refresh
  assert.equal(f.state.loading.value,false);assert.equal(f.state.announcements.value.length,1);assert.ok(f.state.loadError.value)
})

// Render the real state component to verify that pending and failure states never
// expose the empty-data slot, while refreshes preserve existing content.
const { renderToString } = require('@vue/server-renderer')
function stateComponent() {
  const descriptor=parse(readFileSync(resolve(__dirname,'../src/components/ContentState.vue'),'utf8')).descriptor
  const source=compileScript(descriptor,{id:'state-render',inlineTemplate:true}).content
  const code=ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2020}}).outputText
  const exports={};vm.runInNewContext(code,{exports,require:name=>{assert.equal(name,'vue');return vue}})
  return exports.default
}
async function renderState(props) {
  const component=stateComponent()
  const app=vue.createSSRApp({render:()=>vue.h(component,props,{default:()=>vue.h('p','empty-data-slot')})})
  app.component('el-button',{setup(_,ctx){return ()=>vue.h('button',ctx.slots.default?.())}})
  return renderToString(app)
}
test('pending and error render placeholders or retry, never a misleading empty state',async()=>{
  const pending=await renderState({loading:true})
  assert.ok(pending.includes('skeleton-card'));assert.ok(pending.includes('aria-busy="true"'));assert.ok(!pending.includes('empty-data-slot'))
  const failed=await renderState({loading:false,error:'加载失败'})
  assert.ok(failed.includes('重新加载'));assert.ok(!failed.includes('skeleton-card'));assert.ok(!failed.includes('empty-data-slot'))
  assert.ok((await renderState({loading:false})).includes('empty-data-slot'))
  assert.ok((await renderState({loading:true,hasContent:true})).includes('empty-data-slot'))
})
