import { test } from 'node:test'
import assert from 'node:assert/strict'
import { DetailController } from '../src/lib/detail-controller.ts'
const value=(kind,id,page=1)=>({kind,id,title:'detail',coverUrl:'',artists:[],date:'',songs:[],albums:[],albumPage:page,totalAlbums:0,hasMoreAlbums:false})
function fixture(get){let state,sequence=0;const canceled=[],calls=[];const controller=new DetailController({get:async(...args)=>{calls.push(args);return get(...args)},cancel:async id=>canceled.push(id)},next=>state=next,()=>`Request${++sequence}`);return {controller,calls,canceled,get state(){return state}}}
test('metadata detail caching is bounded by route and explicit refresh bypasses reuse',async()=>{
 const f=fixture(async(kind,id,page)=>value(kind,id,page));await f.controller.open('artist','Artist1');await f.controller.open('artist','Artist1');assert.equal(f.calls.length,1)
 await f.controller.open('artist','Artist1',2);assert.equal(f.calls.length,2)
 await f.controller.open('artist','Artist1',1,true);assert.equal(f.calls.length,3)
})
test('switching details cancels old metadata and late results cannot replace the current route',async()=>{
 let release;const f=fixture((kind,id,page)=>id==='Old'?new Promise(resolve=>release=()=>resolve(value(kind,id,page))):Promise.resolve(value(kind,id,page)))
 const pending=f.controller.open('artist','Old');await f.controller.open('album','New');release();await pending
 assert.equal(f.state.value.id,'New');assert.deepEqual(f.canceled,['Request1'])
})
test('detail errors differ from an empty successful track list and can explicitly retry',async()=>{
 let fail=true;const f=fixture(async(kind,id,page)=>{if(fail)throw Error('failed');return value(kind,id,page)})
 await f.controller.open('album','Album1');assert.equal(f.state.status,'error');assert.equal(f.state.value,null)
 fail=false;await f.controller.open('album','Album1',1,true);assert.equal(f.state.status,'ready');assert.deepEqual(f.state.value.songs,[])
})
test('invalid identifiers and mismatched response identities do not enter the cache',async()=>{
 const f=fixture(async()=>value('album','Other'));await f.controller.open('album','../bad');assert.equal(f.calls.length,0)
 await f.controller.open('album','Album1');assert.equal(f.state.status,'error')
})
test('incomplete route navigation can be suspended and resumed without playback APIs',async()=>{
 let release;const f=fixture((kind,id,page)=>new Promise(resolve=>release=()=>resolve(value(kind,id,page))))
 const pending=f.controller.open('artist','Artist1');f.controller.suspend();release();await pending
 assert.equal(f.state.status,'idle');assert.equal(f.state.value,null)
})
