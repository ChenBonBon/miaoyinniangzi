import { test } from 'node:test'
import assert from 'node:assert/strict'
import { SearchController, initialSearchState } from '../src/lib/search-controller.ts'

const result = (query, page = 1, songs = []) => ({ query, page, pageSize:20, total:songs.length, hasMore:false, songs })
function deferred() { let resolve, reject; const promise = new Promise((yes, no) => { resolve = yes; reject = no }); return { promise, resolve, reject } }
function setup(search) {
  let state = initialSearchState(), sequence = 0
  const calls = [], canceled = []
  const controller = new SearchController({ search:(...args) => { calls.push(args); return search(...args) }, cancel:async id => { canceled.push(id); return true } }, value => state = value, () => `request-${++sequence}`)
  return { controller, calls, canceled, state:() => state }
}

test('history restore reuses a confirmed snapshot and rejects a pending late reply',async()=>{
 const pending=deferred(),f=setup(query=>query==='旧请求'?pending.promise:Promise.resolve(result(query)))
 await f.controller.search('已完成');const snapshot={...f.state()}
 const request=f.controller.search('旧请求')
 await f.controller.restore(snapshot)
 pending.resolve(result('旧请求'));await request
 assert.equal(f.state().query,'已完成');assert.equal(f.state().status,'success');assert.equal(f.calls.length,2)
})
test('history restore restarts only an unfinished metadata search',async()=>{
 const f=setup(async(query,page)=>result(query,page))
 await f.controller.restore({...initialSearchState(),status:'loading',query:'未完成',page:2})
 assert.equal(f.state().status,'success');assert.equal(f.state().page,2);assert.equal(f.calls.length,1)
})

test('invalid input does not call the bridge', async () => {
  const { controller, calls, state } = setup(async () => result('unused'))
  for (const query of ['', '  ', '曲\u0000名', '歌'.repeat(101)]) await controller.search(query)
  assert.equal(calls.length, 0)
  assert.match(state().fieldError, /1–100/)
  await controller.search('歌曲', 101)
  assert.equal(calls.length, 0)
  assert.equal(state().status, 'error')
})

test('a valid in-flight query clears an old field error without duplicating the request',async()=>{
  const pending=deferred()
  const {controller,calls,state}=setup(()=>pending.promise)
  const request=controller.search('歌曲')
  await controller.search('')
  assert.ok(state().fieldError)
  await controller.search('歌曲')
  assert.equal(state().fieldError,'')
  assert.equal(calls.length,1)
  pending.resolve(result('歌曲'));await request
})

test('successful searches trim input and distinguish empty success from errors', async () => {
  const { controller, calls, state } = setup(async (query, page) => result(query, page))
  await controller.search(' 周杰伦 ')
  assert.equal(calls[0][0], '周杰伦')
  assert.equal(state().status, 'success')
  assert.deepEqual(state().result.songs, [])
  assert.equal(state().error, '')
})

test('new requests cancel previous requests and stale results cannot overwrite newer results', async () => {
  const first = deferred(), second = deferred()
  const { controller, canceled, state } = setup(query => query === '旧歌' ? first.promise : second.promise)
  const one = controller.search('旧歌'), two = controller.search('新歌')
  assert.deepEqual(canceled, ['request-1'])
  second.resolve(result('新歌')); await two
  first.resolve(result('旧歌')); await one
  assert.equal(state().result.query, '新歌')
  assert.equal(state().status, 'success')
})

test('stale errors cannot replace a newer success', async () => {
  const first = deferred()
  const { controller, state } = setup(query => query === '旧歌' ? first.promise : Promise.resolve(result(query)))
  const old = controller.search('旧歌'); await controller.search('新歌')
  first.reject(new Error('old error')); await old
  assert.equal(state().status, 'success')
  assert.equal(state().error, '')
})

test('canceling clears loading and ignores late completion', async () => {
  const pending = deferred()
  const { controller, canceled, state } = setup(() => pending.promise)
  const request = controller.search('歌曲')
  controller.cancel()
  assert.equal(state().status, 'canceled')
  assert.deepEqual(canceled, ['request-1'])
  pending.resolve(result('歌曲')); await request
  assert.equal(state().status, 'canceled')
  assert.equal(state().result, null)
})

test('identical submissions in flight do not create duplicate requests', async () => {
  const pending = deferred()
  const { controller, calls } = setup(() => pending.promise)
  const request = controller.search('歌曲')
  await controller.search(' 歌曲 ')
  assert.equal(calls.length, 1)
  pending.resolve(result('歌曲')); await request
})

test('retry preserves the attempted query and page after a failure', async () => {
  let attempts = 0
  const { controller, calls, state } = setup(async (query, page) => { if (++attempts === 1) throw new Error('QQ 音乐暂时拒绝搜索请求'); return result(query, page) })
  await controller.search('歌手', 2)
  assert.equal(state().status, 'error')
  assert.match(state().error, /拒绝/)
  await controller.retry()
  assert.deepEqual(calls.map(args => args.slice(0,2)), [['歌手',2], ['歌手',2]])
  assert.equal(state().status, 'success')
})

test('unexpected providers and mismatched results are rejected', async () => {
  for (const response of [result('wrong query'), result('歌曲', 2), result('歌曲', 1, [{ id:'bad', source:'wy' }])]) {
    const { controller, state } = setup(async () => response)
    await controller.search('歌曲')
    assert.equal(state().status, 'error')
    assert.equal(state().result, null)
  }
})

test('submission hook records valid first-page searches without blocking the request', async () => {
  const recorded = [], pending = deferred()
  let state
  const controller = new SearchController({ search:(query,page) => page === 1 ? pending.promise : Promise.resolve(result(query,page)), cancel:async()=>true }, value=>state=value, ()=>crypto.randomUUID(), (query,page)=>{if(page===1)recorded.push(query)})
  await controller.search('  ')
  const first = controller.search(' 新歌 ')
  await controller.search('新歌')
  assert.deepEqual(recorded,['新歌'])
  pending.resolve(result('新歌'));await first
  await controller.search('新歌',2)
  assert.deepEqual(recorded,['新歌'])
  assert.equal(state.status,'success')
})

test('request ID failures do not leave a loading state', async () => {
  let state
  const controller = new SearchController({ search:async () => result('歌曲'), cancel:async () => true }, value => state = value, () => { throw new Error('no crypto') })
  await controller.search('歌曲')
  assert.equal(state.status, 'error')
  assert.match(state.error, /创建搜索请求/)
})
