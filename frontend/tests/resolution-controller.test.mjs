import { test } from 'node:test'
import assert from 'node:assert/strict'
import { ResolutionController } from '../src/lib/resolution-controller.ts'

const song={id:'Song1',source:'tx'}
const result=(id='Song1')=>({songId:id,sourceId:'Source1',requestedQuality:'flac',quality:'flac',downgraded:false,url:'https://cdn.example/a?private=signed'})
function fixture(){
 const calls=[],canceled=[],notices=[];let state
 const controller=new ResolutionController({resolve:(song,id)=>new Promise((resolve,reject)=>calls.push({song,id,resolve,reject})),cancel:async id=>{canceled.push(id);return true}},value=>state=value,value=>notices.push(value),()=>`resolve-${calls.length}`)
 return {controller,calls,canceled,notices,get state(){return state}}
}
test('resolution records quality but never stores the signed address in UI state',async()=>{
 const f=fixture();const pending=f.controller.resolve(song)
 f.calls[0].resolve({...result(),quality:'320k',downgraded:true});await pending
 assert.equal(f.state.status,'success');assert.equal(f.state.quality,'320k');assert.equal(f.state.downgraded,true)
 assert.ok(!JSON.stringify(f.state).includes('signed'));assert.ok(!JSON.stringify(f.notices).includes('cdn.example'))
})
test('public HTTP media addresses from the validated backend are accepted without fetching',async()=>{
 const f=fixture();const pending=f.controller.resolve(song)
 f.calls[0].resolve({...result(),url:'http://cdn.example/song.flac?private=signed'});await pending
 assert.equal(f.state.status,'success');assert.ok(!JSON.stringify(f.state).includes('signed'))
})

test('new resolution cancels old and ignores late success and failures',async()=>{
 const f=fixture();const first=f.controller.resolve(song),second=f.controller.resolve({...song,id:'Song2'})
 assert.deepEqual(f.canceled,['resolve-0'])
 f.calls[1].resolve(result('Song2'));await second
 f.calls[0].reject(Error('old'));await first
 assert.equal(f.state.songId,'Song2');assert.equal(f.state.status,'success');assert.equal(f.notices.length,1)
})
test('cancel ignores late completion and backend errors remain retryable',async()=>{
 const f=fixture();const first=f.controller.resolve(song);f.controller.cancel();f.calls[0].resolve(result());await first
 assert.equal(f.state.status,'idle');assert.equal(f.notices.length,0)
 const second=f.controller.resolve(song);f.calls[1].reject(Error('请先启用音源'));await second
 assert.equal(f.state.error,'请先启用音源')
})
test('unexpected qualities, mismatched songs and unsafe media schemes are rejected',async()=>{
 for(const patch of [{quality:'hires'},{songId:'other'},{url:'file:///etc/passwd'},{url:'https://user:key@example.com/a'},{url:'not-a-url'}]){
  const f=fixture();const pending=f.controller.resolve(song);f.calls[0].resolve({...result(),...patch});await pending
  assert.equal(f.state.status,'error');assert.equal(f.notices.length,1)
 }
})
