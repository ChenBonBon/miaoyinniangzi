import { test, afterEach } from 'node:test'
import assert from 'node:assert/strict'
import { HistoryController, initialHistoryState, historyAPI } from '../src/lib/search-history.ts'

const deferred = () => { let resolve; const promise = new Promise(yes => resolve = yes); return { promise, resolve } }
const settle = async () => { for (let i=0;i<4;i++) await Promise.resolve() }
function setup(overrides = {}) {
  let state = initialHistoryState(), stored = ['旧歌']
  const calls=[]
  const api = {
    list:async () => { calls.push('list'); return [...stored] },
    record:async query => { calls.push(`record:${query}`); stored = [query,...stored.filter(item => item !== query)].slice(0,20); return [...stored] },
    remove:async query => { calls.push(`remove:${query}`); stored = stored.filter(item => item !== query); return [...stored] },
    clear:async () => { calls.push('clear'); stored=[]; return [] },
    ...overrides,
  }
  return { controller:new HistoryController(api, value => state=value), state:() => state, calls }
}

test('history loads, records trimmed terms, removes and clears', async () => {
  const {controller,state}=setup()
  await controller.load(); assert.deepEqual(state().items,['旧歌'])
  await controller.record(' 新歌 '); assert.deepEqual(state().items,['新歌','旧歌'])
  await controller.record('旧歌'); assert.deepEqual(state().items,['旧歌','新歌'])
  await controller.remove('旧歌'); assert.deepEqual(state().items,['新歌'])
  await controller.clear(); assert.deepEqual(state().items,[])
  assert.equal(state().busy,false)
})

test('a slow initial load cannot overwrite a later record', async () => {
  const pending=deferred()
  const {controller,state,calls}=setup({ list:()=>pending.promise })
  const load=controller.load(), record=controller.record('新歌')
  await settle(); assert.deepEqual(calls,[])
  pending.resolve(['旧歌']);await load;await record
  assert.deepEqual(state().items,['新歌','旧歌'])
})

test('clearing during a pending record stays cleared', async () => {
  const pending=deferred()
  const {controller,state}=setup({ record:()=>pending.promise })
  const record=controller.record('新歌'), clear=controller.clear()
  await settle();pending.resolve(['新歌','旧歌']);await record;await clear
  assert.deepEqual(state().items,[])
  assert.equal(state().busy,false)
})

test('a pending record satisfies initial loading without another read', async () => {
  const {controller,calls}=setup()
  const record=controller.record('新歌'), load=controller.load()
  await record;await load;await controller.load()
  assert.deepEqual(calls,['record:新歌'])
})

test('failed edits preserve visible history and the queue remains usable', async () => {
  const {controller,state}=setup({ remove:async()=>{throw Error('disk unavailable')} })
  await controller.load();await controller.remove('旧歌')
  assert.deepEqual(state().items,['旧歌'])
  assert.match(state().error,/disk unavailable/)
  await controller.clear();assert.deepEqual(state().items,[]);assert.equal(state().error,'')
})

test('invalid history input is ignored without invoking persistence', async () => {
  const {controller,calls}=setup()
  for(const query of ['', ' ', '歌'.repeat(101), '歌\u0000名'])await controller.record(query)
  assert.deepEqual(calls,[])
})

afterEach(()=>{delete globalThis.window})
test('history bridge requires desktop persistence rather than browser fake data',()=>{
  globalThis.window={}
  assert.throws(()=>historyAPI.list(),/macOS/)
})
