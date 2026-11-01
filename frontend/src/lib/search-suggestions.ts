import { normalizeSearchQuery, requestID } from './search-controller.ts'

export interface SongSuggestion { id:string; source:'tx'; title:string; artist:string }
export interface SuggestionsAPI {
 suggest(query:string, id:string):Promise<SongSuggestion[]>
 cancel(id:string):Promise<boolean>
}
type SuggestionsBridge = { SuggestSongs:SuggestionsAPI['suggest']; CancelSongSuggestions:SuggestionsAPI['cancel'] }
function bridge():SuggestionsBridge {
 const host=window as Window & {go?:{main?:{App?:SuggestionsBridge}}}
 if(!host.go?.main?.App?.SuggestSongs) throw new Error('请在 macOS 桌面应用中获取歌曲建议。')
 return host.go.main.App
}
export const suggestionsAPI:SuggestionsAPI={suggest:(query,id)=>bridge().SuggestSongs(query,id),cancel:id=>bridge().CancelSongSuggestions(id)}
export interface SuggestionsState {query:string;status:'idle'|'loading'|'success'|'error';items:SongSuggestion[]}
export const initialSuggestionsState=():SuggestionsState=>({query:'',status:'idle',items:[]})

// Separate cancellation and generations from submitted searches. No keystroke
// is persisted as history. Bounded, one-minute memory cache only.
export class SuggestionsController {
 private revision=0
 private timer:ReturnType<typeof setTimeout>|null=null
 private activeID:string|null=null
 private queuedQuery=''
 private cache=new Map<string,{at:number;items:SongSuggestion[]}>()
 private api:SuggestionsAPI
 private publish:(state:SuggestionsState)=>void
 private makeID:()=>string
 private delay:number
 private now:()=>number
 constructor(api:SuggestionsAPI,publish:(state:SuggestionsState)=>void,makeID=requestID,delay=300,now=Date.now){
  this.api=api;this.publish=publish;this.makeID=makeID;this.delay=delay;this.now=now
 }
 private cancelWork(){
  this.revision++
  if(this.timer!==null){clearTimeout(this.timer);this.timer=null}
  const id=this.activeID;this.activeID=null
  if(id){try{void this.api.cancel(id).catch(()=>{})}catch{/* Late responses are still discarded. */}}
 }
 schedule(input:string){
  const query=normalizeSearchQuery(input)
  if(query && query===this.queuedQuery)return
  this.cancelWork();this.queuedQuery=query??''
  if(!query){this.publish(initialSuggestionsState());return}
  const hit=this.cache.get(query)
  if(hit&&this.now()-hit.at<60_000){this.publish({query,status:'success',items:hit.items});return}
  this.publish({query,status:'idle',items:[]})
  const revision=this.revision
  this.timer=setTimeout(()=>{this.timer=null;void this.fetch(query,revision)},this.delay)
 }
 private async fetch(query:string,revision:number){
  try{
   const id=this.makeID();this.activeID=id
   this.publish({query,status:'loading',items:[]})
   const items=await this.api.suggest(query,id)
   if(revision!==this.revision)return
   if(!Array.isArray(items)||items.length>8||items.some(item=>!item||item.source!=='tx'||typeof item.id!=='string'||!/^[A-Za-z0-9]{1,64}$/.test(item.id)||typeof item.title!=='string'||!normalizeSearchQuery(item.title)||typeof item.artist!=='string'||item.artist.length>1024)) throw new Error('Invalid suggestions')
   const unique=items.filter((item,index)=>items.findIndex(other=>other.id===item.id)===index)
   this.cache.delete(query);this.cache.set(query,{at:this.now(),items:unique})
   if(this.cache.size>10)this.cache.delete(this.cache.keys().next().value!)
   this.publish({query,status:'success',items:unique})
  }catch{
   if(revision===this.revision)this.publish({query,status:'error',items:[]})
  }finally{if(revision===this.revision)this.activeID=null}
 }
 close(){this.cancelWork();this.queuedQuery='';this.publish(initialSuggestionsState())}
}
