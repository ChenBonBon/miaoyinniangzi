// Deliberately not a QQ Song: preview IDs must never enter playback/favorites APIs.
export interface PreviewSong { id:number; title:string; artist:string }
export const previewSongs:readonly PreviewSong[]=Array.from({length:50},(_,index)=>({id:index+1,title:String(index+1),artist:`歌手 ${index+1}`}))
export function samplePreviewSongs(random:()=>number=Math.random):PreviewSong[]{
 const pool=[...previewSongs]
 for(let index=0;index<10;index++){
  const pick=index+Math.floor(random()*(pool.length-index))
  const current=pool[index];pool[index]=pool[pick];pool[pick]=current
 }
 return pool.slice(0,10)
}
