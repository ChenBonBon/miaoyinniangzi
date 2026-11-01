import { test } from 'node:test'
import assert from 'node:assert/strict'
import { DownloadsController, canControlDownload } from '../src/lib/downloads-controller.ts'
const task = state => ({ id:'Task1',batch:'Batch1',song:{id:'Song1',source:'tx',title:'Song'},directory:'/Downloads',requested:'320k',quality:'320k',codec:'mp3',state,bytes:512,total:1024,file:'',message:'',created:1 })
function fixture(overrides={}, clock=undefined){let state;const notices=[],calls=[];const api={list:async()=>[],enqueue:async()=>({tasks:[task('queued')],existing:0}),pause:async id=>calls.push(['pause',id]),resume:async id=>calls.push(['resume',id]),remove:async(...args)=>calls.push(['remove',...args]),cancelBatch:async()=>{},retryBatch:async()=>{},control:async(...args)=>calls.push(['control',...args]),...overrides};const controller=new DownloadsController(api,value=>state=value,(...args)=>notices.push(args),clock);return {controller,calls,notices,get state(){return state}}}
test('download read failures are not an empty successful list',async()=>{const f=fixture({list:async()=>{throw Error('private')}});await f.controller.refresh();assert.equal(f.state.status,'error');assert.equal(f.notices.length,0)})
test('download actions pass explicit file deletion intent and refresh confirmed state',async()=>{const f=fixture({list:async()=>[task('success')]});await f.controller.refresh();await f.controller.remove('Task1');await f.controller.remove('Task1',true);assert.deepEqual(f.calls,[['remove','Task1',false],['remove','Task1',true]])})
test('enqueue reports queued tasks, not completed downloads',async()=>{const f=fixture({list:async()=>[task('queued')]});assert.equal(await f.controller.enqueue([]),true);assert.equal(f.state.tasks[0].state,'queued');assert.match(f.notices[0][0],/下载队列/)})
test('failed actions retain the last snapshot and clear pending state',async()=>{const f=fixture({list:async()=>[task('paused')],resume:async()=>{throw Error('继续失败')}});await f.controller.refresh();assert.equal(await f.controller.resume('Task1'),false);assert.equal(f.state.tasks[0].state,'paused');assert.deepEqual(f.state.pending,[])})
test('existing files produce a toast without a new task',async()=>{const f=fixture({enqueue:async()=>({tasks:[],existing:1}),list:async()=>[task('success')]});await f.controller.enqueue([]);assert.equal(f.state.tasks.length,1);assert.equal(f.state.tasks[0].state,'success');assert.match(f.notices[0][0],/目录中已有该歌曲/);assert.doesNotMatch(f.notices[0][0],/已加入/)})
test('speed uses confirmed byte deltas and excludes pause and preparation time',async()=>{
 let now=0,current={...task('downloading'),bytes:0,total:100000}
 const f=fixture({list:async()=>[{...current}]},()=>now)
 await f.controller.refresh();assert.deepEqual(f.state.speeds,{})
 now=1000;current.bytes=2048;await f.controller.refresh();assert.equal(f.state.speeds.Task1,2048)
 now=2000;await f.controller.refresh();assert.equal(f.state.speeds.Task1,0)
 current.state='paused';now=3000;await f.controller.refresh();assert.deepEqual(f.state.speeds,{})
 current.state='queued';now=9000;await f.controller.refresh();assert.deepEqual(f.state.speeds,{})
 current.state='downloading';now=10000;await f.controller.refresh();assert.deepEqual(f.state.speeds,{})
 now=11000;current.bytes+=1024;await f.controller.refresh();assert.equal(f.state.speeds.Task1,1024)
 now=12000;current.bytes=0;await f.controller.refresh();assert.deepEqual(f.state.speeds,{})
 current.state='success';now=13000;await f.controller.refresh();assert.deepEqual(f.state.speeds,{})
})
test('canceled records are not displayed even from an older bridge snapshot',async()=>{const f=fixture({list:async()=>[task('canceled'),{...task('success'),id:'Done'}]});await f.controller.refresh();assert.deepEqual(f.state.tasks.map(task=>task.id),['Done'])})
test('malformed download states are rejected',async()=>{for(const value of [null,[task('constructor')],[{...task('downloading'),bytes:-1}]]){const f=fixture({list:async()=>value});await f.controller.refresh();assert.equal(f.state.status,'error')}})

test('selection is explicit, deduplicated and never implicitly controls all tasks',async()=>{
 const f=fixture();assert.equal(await f.controller.control([],'pause'),false);assert.deepEqual(f.calls,[])
 assert.equal(await f.controller.control(['Task1','Task1'],'cancel'),true)
 assert.equal(await f.controller.control([],'start',true),true)
 assert.deepEqual(f.calls,[['control',['Task1'],'cancel',false],['control',[],'start',true]])
})
test('bulk failure retains snapshot and serializes duplicate bulk actions',async()=>{
 let release;const f=fixture({list:async()=>[task('paused')],control:()=>new Promise((resolve,reject)=>{release=()=>reject(Error('操作失败'))})})
 await f.controller.refresh();const pending=f.controller.control(['Task1'],'start')
 assert.deepEqual(f.state.pending,['download-control']);assert.equal(await f.controller.control([],'pause',true),false)
 release();assert.equal(await pending,false);assert.equal(f.state.tasks[0].state,'paused');assert.deepEqual(f.state.pending,[])
})
test('bulk eligibility excludes completed tasks and canceled tasks cannot restart',()=>{
 for(const state of ['success','skipped'])for(const action of ['start','pause','cancel'])assert.equal(canControlDownload(task(state),action),false)
 assert.equal(canControlDownload(task('canceled'),'start'),false);assert.equal(canControlDownload(task('canceled'),'cancel'),false)
 assert.equal(canControlDownload(task('queued'),'pause'),true);assert.equal(canControlDownload(task('paused'),'start'),true)
})
