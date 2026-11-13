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
function stateComponent(globals = {}) {
  const descriptor=parse(readFileSync(resolve(__dirname,'../src/components/ContentState.vue'),'utf8')).descriptor
  const source=compileScript(descriptor,{id:'state-render',inlineTemplate:true}).content
  const code=ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2020}}).outputText
  const exports={};vm.runInNewContext(code,{exports,require:name=>{assert.equal(name,'vue');return vue},...globals})
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

// Mount the actual component with a deterministic clock. This exercises Vue's
// lifecycle and rendered branches without relying on wall-clock test delays.
function mountedState(initialProps = {}) {
  let now=0, nextId=0
  const timers=new Map()
  const component=stateComponent({
    setTimeout(fn,delay){const id=++nextId;timers.set(id,{fn,at:now+delay});return id},
    clearTimeout(id){timers.delete(id)}
  })
  function node(type,text=''){return {type,text,props:{},children:[],parent:null}}
  const renderer=vue.createRenderer({
    createElement:type=>node(type),createText:text=>node('#text',text),createComment:text=>node('#comment',text),
    setText(el,text){el.text=text},setElementText(el,text){el.text=text;el.children=[]},
    patchProp(el,key,oldValue,value){el.props[key]=value},
    insert(el,parent,anchor){if(el.parent){el.parent.children.splice(el.parent.children.indexOf(el),1)}el.parent=parent;const i=anchor?parent.children.indexOf(anchor):-1;parent.children.splice(i<0?parent.children.length:i,0,el)},
    remove(el){if(el.parent)el.parent.children.splice(el.parent.children.indexOf(el),1);el.parent=null},
    parentNode:el=>el.parent,nextSibling:el=>el.parent?.children[el.parent.children.indexOf(el)+1] || null
  })
  const props=vue.reactive({loading:true,...initialProps})
  const root=node('root')
  const app=renderer.createApp({render:()=>vue.h(component,props,{default:()=>vue.h('article',{'data-content':true},'actual-content')})})
  app.component('el-button',{setup(_,ctx){return ()=>vue.h('button',ctx.slots.default?.())}})
  app.mount(root)
  function find(predicate,el=root){return predicate(el)?el:el.children.map(child=>find(predicate,child)).find(Boolean)}
  return {
    findClass:name=>find(el=>String(el.props.class||'').split(' ').includes(name)),
    content:()=>find(el=>el.props['data-content']),
    pendingTimers:()=>timers.size,
    async update(values){Object.assign(props,values);await vue.nextTick()},
    async advance(ms){now+=ms;for(const [id,timer] of [...timers])if(timer.at<=now){timers.delete(id);timer.fn()}await vue.nextTick()},
    unmount:()=>app.unmount()
  }
}

test('fast responses skip visible skeletons and entrance animation, and cancel feedback',async()=>{
  const f=mountedState()
  assert.ok(f.findClass('feedback-pending'));assert.ok(!f.content())
  await f.advance(100);await f.update({loading:false,hasContent:true})
  assert.ok(f.content());assert.ok(!f.findClass('skeleton-list'));assert.ok(!f.findClass('content-reveal'))
  assert.equal(f.pendingTimers(),0)
  await f.advance(500);assert.ok(!f.findClass('refresh-note'));f.unmount()
})

test('slow responses reveal the reserved skeleton at 180ms and immediately replace it on success',async()=>{
  const f=mountedState()
  await f.advance(179);assert.ok(f.findClass('feedback-pending'))
  await f.advance(1);assert.ok(f.findClass('skeleton-list'));assert.ok(!f.findClass('feedback-pending'))
  await f.update({loading:false,hasContent:true})
  assert.ok(f.content());assert.ok(!f.findClass('skeleton-list'));assert.ok(f.findClass('content-reveal'));f.unmount()
})

test('refresh keeps the same content node, including a previously resolved empty result',async()=>{
  const f=mountedState()
  await f.update({loading:false});const content=f.content()
  await f.update({loading:true});assert.equal(f.content(),content);assert.ok(!f.findClass('refresh-note'))
  await f.advance(180);assert.equal(f.content(),content);assert.ok(f.findClass('refresh-note'));assert.ok(!f.findClass('skeleton-list'))
  await f.update({loading:false,error:'更新失败'});assert.equal(f.content(),content);assert.ok(f.findClass('load-error'))
  await f.update({loading:true,error:''});await f.advance(100);await f.update({loading:false})
  assert.equal(f.content(),content);assert.ok(!f.findClass('content-reveal'));f.unmount()
})

test('failure and unmount cancel delayed feedback; retry gets a fresh delay',async()=>{
  const f=mountedState()
  await f.advance(100);await f.update({loading:false,error:'加载失败'})
  assert.equal(f.pendingTimers(),0);assert.ok(f.findClass('load-error'));assert.ok(!f.content())
  await f.update({loading:true,error:''});await f.advance(179);assert.ok(f.findClass('feedback-pending'))
  f.unmount();assert.equal(f.pendingTimers(),0)
})
