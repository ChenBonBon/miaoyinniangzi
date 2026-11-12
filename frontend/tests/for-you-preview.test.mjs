import test from 'node:test'
import assert from 'node:assert/strict'
import {previewSongs,samplePreviewSongs} from '../src/lib/for-you-preview.ts'

test('static recommendation pool is exactly titles 1 through 50 without native song fields',()=>{
 assert.deepEqual(previewSongs.map(song=>song.title),Array.from({length:50},(_,index)=>String(index+1)))
 assert.equal(previewSongs.every(song=>!('source' in song)&&!('coverUrl' in song)),true)
})
test('sampling selects ten distinct pool entries without mutating the pool',()=>{
 const original=JSON.stringify(previewSongs)
 for(const random of [()=>0,()=>.999999,Math.random]){
  for(let round=0;round<50;round++){
   const songs=samplePreviewSongs(random)
   assert.equal(songs.length,10);assert.equal(new Set(songs.map(song=>song.id)).size,10)
   assert.equal(songs.every(song=>song.id>=1&&song.id<=50&&song.title===String(song.id)),true)
  }
 }
 assert.equal(JSON.stringify(previewSongs),original)
})
