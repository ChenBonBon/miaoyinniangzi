// Deliberately not a QQ Song: preview IDs must never enter native music APIs.
export interface PreviewSong { id:number; title:string; artist:string; albumTitle:string; artwork:string }
export const previewBatchSize=9
function artwork(index:number){
 const hue=(index*43+210)%360
 const svg=`<svg xmlns="http://www.w3.org/2000/svg" width="240" height="240" viewBox="0 0 240 240"><rect width="240" height="240" fill="hsl(${hue},30%,34%)"/><path d="M0 180L240 25V90L0 240Z" fill="white" opacity=".08"/><circle cx="120" cy="120" r="77" fill="black" opacity=".35"/><circle cx="120" cy="120" r="59" fill="none" stroke="white" stroke-opacity=".12"/><circle cx="120" cy="120" r="42" fill="none" stroke="white" stroke-opacity=".12"/><circle cx="120" cy="120" r="22" fill="hsl(${hue},30%,55%)"/><circle cx="120" cy="120" r="5" fill="black" opacity=".6"/></svg>`
 return `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`
}
export const previewSongs:readonly PreviewSong[]=Array.from({length:50},(_,index)=>({id:index+1,title:String(index+1),artist:`歌手 ${index+1}`,albumTitle:`专辑 ${index+1}`,artwork:artwork(index)}))
export function samplePreviewSongs(random:()=>number=Math.random):PreviewSong[]{
 const pool=[...previewSongs]
 for(let index=0;index<previewBatchSize;index++){
  const pick=index+Math.floor(random()*(pool.length-index))
  const current=pool[index];pool[index]=pool[pick];pool[pick]=current
 }
 return pool.slice(0,previewBatchSize)
}
