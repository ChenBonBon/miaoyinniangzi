import test from 'node:test'
import assert from 'node:assert/strict'
import {ForYouController,pickForYou} from '../src/lib/for-you-controller.ts'
const song=i=>({id:`Track${i}`,songId:String(100+i),source:'tx',title:`Song${i}`,artist:`Artist${i%6}`,artists:[{id:`Artist${i%6}`,name:'Artist'}],albumMid:`Album${i}`,album:'Album',coverUrl:'',duration:10,mediaMid:'',subtitle:'',availableQualities:['128k']})
const pool=Array.from({length:36},(_,i)=>song(i))
const result={songs:pool,reason:'ready',seedCount:1,skipped:0,warning:''}
const flush=()=>new Promise(resolve=>setImmediate(resolve))
test('for-you batches exclude queue/favorites, prefer unseen and never duplicate tracks',()=>{
 const first=pickForYou(pool,new Set(['Track0','numeric:101']),new Set(),new Set(),()=>.4)
 assert.equal(first.length,12);assert.equal(new Set(first.map(song=>song.id)).size,12)
 assert.equal(first.some(song=>song.id==='Track0'||song.id==='Track1'),false)
 const second=pickForYou(pool,new Set(),new Set(first.map(song=>song.id)),new Set(first.map(song=>song.id)),()=>.4)
 assert.equal(second.length,12);assert.equal(second.some(song=>first.some(old=>old.id===song.id)),false)
 assert.equal(pickForYou(pool.slice(0,3),new Set(),new Set(),new Set()).length,3)
})
test('next batch is local, return retains batch and refresh failure retains successful content',async()=>{
 let state,calls=0,clock=1,failed=false;const notices=[]
 const controller=new ForYouController({load:async()=>{calls++;if(failed)throw Error('network');return result},cancel:async()=>true},value=>state=value,(...args)=>notices.push(args),()=>`id${calls}`,()=>clock)
 controller.setFavorites([song(99)]);await controller.activate();const first=state.songs
 controller.suspend();await controller.activate();assert.equal(calls,1);assert.deepEqual(state.songs,first)
 controller.next();assert.equal(calls,1);assert.notDeepEqual(state.songs,first)
 const previous=state.songs;controller.setQueue(pool);controller.next();assert.deepEqual(state.songs,previous);assert.equal(calls,1)
 failed=true;clock+=3600001;await controller.activate()
 assert.deepEqual(state.songs,previous);assert.equal(state.status,'ready');assert.equal(notices.at(-1)[1],true)
})
test('favorite changes and page suspension fence late candidate replies',async()=>{
 const releases=[],canceled=[];let state
 const controller=new ForYouController({load:()=>new Promise(resolve=>releases.push(resolve)),cancel:async id=>{canceled.push(id);return true}},value=>state=value,()=>{},()=>`request${releases.length}`)
 controller.setFavorites([song(90)]);const old=controller.activate();await flush()
 controller.setFavorites([song(91)]);const current=controller.activate();await flush()
 releases[0](result);await old;assert.equal(state.songs.length,0)
 releases[1]({...result,songs:pool.slice(0,4)});await current;assert.equal(state.songs.length,4);assert.equal(canceled.length,1)
 const pending=controller.activate(true);await flush();controller.suspend();releases[2]({...result,songs:pool.slice(5,7)});await pending;assert.equal(state.songs.length,4)
})
test('invalid payload cannot become playable recommendations',async()=>{
 let state;const controller=new ForYouController({load:async()=>({...result,songs:[{...song(1),source:'fake'}]}),cancel:async()=>true},value=>state=value)
 controller.setFavorites([]);await controller.activate();assert.equal(state.status,'error');assert.equal(state.songs.length,0)
})
