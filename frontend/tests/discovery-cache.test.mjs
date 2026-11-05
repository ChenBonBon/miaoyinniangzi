import { test } from 'node:test'
import assert from 'node:assert/strict'
import { DiscoveryController } from '../src/lib/discovery-controller.ts'
const entry=(id=1)=>({kind:'chart',id,title:`fixture ${id}`,group:'fixture',coverUrl:'',period:'2026_43',updated:'',total:1})
const catalog={kind:'chart',entries:[entry(),entry(2)]}
const collection=id=>({entry:entry(id),songs:[{id:`Song${id}`,source:'tx',title:'fixture',artist:'fixture'}]})
const flush=async()=>{for(let i=0;i<12;i++)await Promise.resolve()}
function setup(cached,now=1000000){let state,id=0;const requests=[];const controller=new DiscoveryController({cached,catalog:(kind,id)=>new Promise((resolve,reject)=>requests.push({kind,id,resolve,reject})),collection:(kind,number,period,id)=>new Promise((resolve,reject)=>requests.push({kind,number,period,id,resolve,reject})),cancel:async()=>true},value=>state=value,()=>`cache-${++id}`,()=>now);return{controller,requests,get state(){return state}}}
test('persisted stale catalog stays usable during refresh and after timeout',async()=>{
 const f=setup(async()=>({savedAt:1000,catalog}))
 const pending=f.controller.activate();await flush()
 assert.equal(f.state.catalog,catalog);assert.equal(f.state.status,'ready');assert.equal(f.state.refreshing,true);assert.equal(f.requests.length,1)
 f.requests[0].reject(Error('读取 QQ 音乐推荐超时，请重试'));await pending
 assert.equal(f.state.catalog,catalog);assert.equal(f.state.status,'ready');assert.match(f.state.error,/超时/);assert.equal(f.state.refreshing,false)
 const retry=f.controller.refresh();assert.equal(f.state.catalog,catalog);f.requests[1].resolve({kind:'chart',entries:[entry(2)]});await retry
 assert.equal(f.state.error,'');assert.equal(f.state.catalog.entries[0].id,2)
})
test('fresh persisted cache avoids QQ until explicit refresh and repeated refresh is coalesced',async()=>{
 const f=setup(async()=>({savedAt:999999,catalog}));await f.controller.activate();assert.equal(f.requests.length,0)
 const pending=f.controller.refresh(),again=f.controller.refresh();assert.equal(pending,again);assert.equal(f.requests.length,1);assert.equal(f.state.status,'ready')
 f.requests[0].resolve(catalog);await pending;assert.equal(f.state.refreshing,false)
})
test('cached collection survives malformed refresh without interrupting available songs',async()=>{
 const f=setup(async(_kind,id)=>id?{savedAt:1000,collection:collection(id)}:{savedAt:999999,catalog})
 await f.controller.activate();const pending=f.controller.open(entry());await flush()
 assert.equal(f.state.collection.songs[0].id,'Song1');assert.equal(f.state.status,'ready')
 f.requests[0].resolve({...collection(1),songs:[{...collection(1).songs[0],source:'other'}]});await pending
 assert.equal(f.state.collection.songs[0].id,'Song1');assert.equal(f.state.status,'ready');assert.match(f.state.error,/异常/)
})
test('late persisted cache cannot overwrite newer navigation or start an obsolete QQ request',async()=>{
 let release;const f=setup(async(_kind,id)=>id===1?new Promise(resolve=>release=resolve):id?{savedAt:999999,collection:collection(id)}:{savedAt:999999,catalog})
 await f.controller.activate();const old=f.controller.open(entry(1));await flush();await f.controller.open(entry(2))
 release({savedAt:999999,collection:collection(1)});await old
 assert.equal(f.state.selected.id,2);assert.equal(f.state.collection.songs[0].id,'Song2');assert.equal(f.requests.length,0)
})
test('expired, corrupt and unavailable snapshots fall back to QQ rather than fake empty success',async()=>{
 for(const cached of [async()=>({savedAt:1,catalog:{kind:'new',entries:[]}}),async()=>({savedAt:999999,collection:collection(1)}),async()=>{throw Error('cache unavailable')},async()=>({savedAt:1000000000,catalog})]){
  const f=setup(cached);const pending=f.controller.activate();await flush();assert.equal(f.state.catalog,null);assert.equal(f.requests.length,1)
  f.requests[0].reject(Error('network unavailable'));await pending;assert.equal(f.state.status,'error')
 }
})
test('snapshots older than the retention window are not displayed',async()=>{
 const f=setup(async()=>({savedAt:1000,catalog}),72*60*60*1000+2000)
 const pending=f.controller.activate();await flush();assert.equal(f.state.catalog,null);assert.equal(f.requests.length,1)
 f.requests[0].resolve(catalog);await pending
})
test('explicit refresh during a cache read still requests QQ even when the cache is fresh',async()=>{
 let release;const f=setup(()=>new Promise(resolve=>release=resolve));const pending=f.controller.activate();f.controller.refresh()
 release({savedAt:999999,catalog});await flush();assert.equal(f.requests.length,1)
 f.requests[0].resolve(catalog);await pending
})
test('an interrupted explicit refresh resumes on activation even with a still-fresh cache',async()=>{
 const f=setup(async()=>({savedAt:999999,catalog}));await f.controller.activate()
 const old=f.controller.refresh();f.controller.suspend();const current=f.controller.activate()
 assert.equal(f.requests.length,2);assert.equal(f.state.catalog,catalog)
 f.requests[1].resolve(catalog);await current;f.requests[0].resolve({kind:'chart',entries:[]});await old
 assert.equal(f.state.catalog,catalog)
})
test('suspension cancels refresh without clearing usable data and rejects its late result',async()=>{
 const f=setup(async()=>({savedAt:1000,catalog}));const pending=f.controller.activate();await flush();f.controller.suspend()
 f.requests[0].resolve({kind:'chart',entries:[]});await pending
 assert.equal(f.state.catalog,catalog);assert.equal(f.state.status,'ready');assert.equal(f.state.refreshing,false)
})
