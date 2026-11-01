import { test } from 'node:test'
import assert from 'node:assert/strict'
import { loadSongCredits } from '../src/lib/song-credits.ts'
test('legacy credit lookup is explicit, single flight and cached',async()=>{
 let count=0,release
 globalThis.window={go:{main:{App:{GetSongCredits:async id=>{count++;await new Promise(resolve=>release=resolve);return{id,artists:[{id:'Artist1',name:'artist'}],albumMid:'Album1'}}}}}}
 assert.equal(count,0)
 const first=loadSongCredits('Legacy1'),second=loadSongCredits('Legacy1');assert.equal(count,1)
 release();assert.deepEqual(await first,await second);await loadSongCredits('Legacy1');assert.equal(count,1)
})
test('failed or mismatched credits are never cached and invalid IDs do not call the bridge',async()=>{
 let count=0,wrong=true
 globalThis.window={go:{main:{App:{GetSongCredits:async id=>{count++;return{id:wrong?'Other':id,artists:[],albumMid:''}}}}}}
 await assert.rejects(loadSongCredits('../bad'));assert.equal(count,0)
 await assert.rejects(loadSongCredits('Legacy2'));wrong=false
 await loadSongCredits('Legacy2');assert.equal(count,2)
})
