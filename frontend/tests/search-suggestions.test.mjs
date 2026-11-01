import { test, afterEach } from 'node:test'
import assert from 'node:assert/strict'
import { SuggestionsController, suggestionsAPI } from '../src/lib/search-suggestions.ts'

const items = title => [{id:'Song1',source:'tx',title,artist:'歌手'}]
function fixture(t) {
 t.mock.timers.enable({apis:['setTimeout']})
 const calls=[],canceled=[];let state,clock=0
 const controller=new SuggestionsController({suggest:(query,id)=>new Promise((resolve,reject)=>calls.push({query,id,resolve,reject})),cancel:async id=>{canceled.push(id);return true}},next=>state=next,()=>`request-${calls.length}`,300,()=>clock)
 return {controller,calls,canceled,get state(){return state},advance:ms=>t.mock.timers.tick(ms),setClock:next=>clock=next}
}
afterEach(()=>{delete globalThis.window})

test('typeahead debounces 300ms, trims input, and never requests blank or invalid text',async t=>{
 const f=fixture(t)
 f.controller.schedule('');f.controller.schedule('x'.repeat(101));f.advance(500);assert.equal(f.calls.length,0)
 f.controller.schedule('周');f.advance(299);f.controller.schedule(' 周杰伦 ');f.advance(299);assert.equal(f.calls.length,0)
 f.advance(1);assert.equal(f.calls[0].query,'周杰伦')
 f.calls[0].resolve(items('晴天'));await Promise.resolve();assert.equal(f.state.status,'success')
})
test('new text cancels prior typeahead and rejects late completion',async t=>{
 const f=fixture(t)
 f.controller.schedule('第一首');f.advance(300)
 f.controller.schedule('第二首');f.advance(300)
 assert.deepEqual(f.canceled,['request-0'])
 f.calls[1].resolve(items('新的'));await Promise.resolve()
 f.calls[0].resolve(items('旧的'));await Promise.resolve()
 assert.equal(f.state.items[0].title,'新的')
 f.controller.schedule('第三首');f.advance(300);f.controller.close()
 f.calls[2].resolve(items('关闭后'));await Promise.resolve();assert.equal(f.state.status,'idle');assert.deepEqual(f.state.items,[])
})
test('closing before debounce performs no request and caches only successful results for one minute',async t=>{
 const f=fixture(t)
 f.controller.schedule('周杰伦');f.controller.close();f.advance(300);assert.equal(f.calls.length,0)
 f.controller.schedule('周杰伦');f.advance(300);f.calls[0].resolve(items('晴天'));await Promise.resolve()
 f.controller.close();f.controller.schedule('周杰伦');assert.equal(f.state.status,'success');assert.equal(f.calls.length,1)
 f.controller.close();f.setClock(60_000);f.controller.schedule('周杰伦');f.advance(300);assert.equal(f.calls.length,2)
})
test('typeahead failures and untrusted responses stay separate and can be retried',async t=>{
 const f=fixture(t)
 f.controller.schedule('歌曲');f.advance(300);f.calls[0].reject(Error('upstream error'));await Promise.resolve();assert.equal(f.state.status,'error')
 f.controller.close();f.controller.schedule('歌曲');f.advance(300);f.calls[1].resolve([{id:'Bad',source:'wy',title:'外部',artist:''}]);await Promise.resolve();assert.equal(f.state.status,'error')
 f.controller.close();f.controller.schedule('歌曲');f.advance(300);f.calls[2].resolve(items('正常'));await Promise.resolve();assert.equal(f.state.status,'success')
})
test('suggestion bridge does not fabricate data outside the desktop app',async()=>{
 globalThis.window={};assert.throws(()=>suggestionsAPI.suggest('歌曲','id'),/macOS/)
 const calls=[]
 globalThis.window={go:{main:{App:{SuggestSongs:async(...args)=>{calls.push(args);return []},CancelSongSuggestions:async id=>{calls.push(id);return true}}}}}
 assert.deepEqual(await suggestionsAPI.suggest('只想','request-1'),[])
 assert.equal(await suggestionsAPI.cancel('request-1'),true)
 assert.deepEqual(calls,[['只想','request-1'],'request-1'])
})
