import test from 'node:test'
import assert from 'node:assert/strict'
import { AppearanceController,validAppearance,resolveAppearance } from '../src/lib/appearance-controller.ts'
const flush=()=>new Promise(resolve=>setImmediate(resolve))
test('appearance modes resolve system preferences without changing manual selection',()=>{
 for(const mode of ['dark','light','system'])assert.equal(validAppearance(mode),true)
 assert.equal(validAppearance('auto'),false)
 assert.equal(resolveAppearance('system',true),'dark');assert.equal(resolveAppearance('system',false),'light')
 assert.equal(resolveAppearance('dark',false),'dark');assert.equal(resolveAppearance('light',true),'light')
})
test('appearance serializes rapid edits and returns to the confirmed mode after an in-flight write',async()=>{
 const writes=[],releases=[],remembered=[]
 const controller=new AppearanceController({load:async()=>'dark',save:mode=>{writes.push(mode);return new Promise(resolve=>releases.push(resolve))}},()=>{},'dark',mode=>remembered.push(mode))
 await controller.load();controller.select('light');controller.select('system');controller.select('dark')
 assert.deepEqual(writes,['light']);assert.equal(controller.state.mode,'dark')
 releases.shift()();await flush();assert.deepEqual(writes,['light','dark'])
 releases.shift()();await flush();assert.equal(controller.state.status,'saved');assert.deepEqual(remembered,['dark','light','dark'])
})
test('appearance load/save failures preserve the visible draft and require explicit retry',async()=>{
 let readFails=true,saveFails=true,writes=0
 const controller=new AppearanceController({load:async()=>{if(readFails)throw Error('private');return 'system'},save:async()=>{writes++;if(saveFails)throw Error('private')}},()=>{})
 await controller.load();controller.select('light');assert.equal(writes,0);assert.equal(controller.state.loaded,false)
 readFails=false;await controller.load();controller.select('light');await flush()
 assert.equal(controller.state.mode,'light');assert.equal(controller.state.status,'error');assert.equal(writes,1)
 saveFails=false;controller.retry();await flush();assert.equal(writes,2);assert.equal(controller.state.status,'saved')
})
test('appearance rejects malformed persisted preferences and does not save defaults on read failure',async()=>{
 let saves=0
 const controller=new AppearanceController({load:async()=>'invalid',save:async()=>{saves++}},()=>{},'light')
 await controller.load();assert.equal(controller.state.mode,'light');assert.equal(controller.state.loaded,false);assert.equal(saves,0)
})
