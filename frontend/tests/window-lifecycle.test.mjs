import {test} from 'node:test'
import assert from 'node:assert/strict'
import {registerWindowLifecycle} from '../src/lib/window-lifecycle.ts'
const settle=()=>new Promise(resolve=>setTimeout(resolve,0))
test('real quit acknowledges only after playback persistence settles',async()=>{
 let handler,release,unsubscribed=false;const calls=[]
 globalThis.window={runtime:{EventsOn:(name,callback)=>{assert.equal(name,'app:before-quit');handler=callback;return()=>unsubscribed=true}},go:{main:{App:{AcknowledgeQuit:async id=>calls.push(id)}}}}
 const dispose=registerWindowLifecycle(()=>new Promise(resolve=>release=resolve))
 handler('7');await settle();assert.deepEqual(calls,[])
 release();await settle();assert.deepEqual(calls,['7']);dispose();assert.equal(unsubscribed,true)
})
test('invalid quit payloads cannot stop playback and flushing errors cannot trap exit',async()=>{
 let handler,flushes=0;const calls=[]
 globalThis.window={runtime:{EventsOn:(_,callback)=>{handler=callback;return()=>{}}},go:{main:{App:{AcknowledgeQuit:async id=>calls.push(id)}}}}
 registerWindowLifecycle(async()=>{flushes++;throw Error('failed')})
 handler('../bad');await settle();assert.equal(flushes,0)
 handler('8');await settle();assert.equal(flushes,1);assert.deepEqual(calls,['8'])
})
test('missing desktop runtime never flushes or changes playback',()=>{
 globalThis.window={};let flushed=false
 registerWindowLifecycle(async()=>{flushed=true})();assert.equal(flushed,false)
})
