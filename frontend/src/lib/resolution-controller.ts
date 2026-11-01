import type { Song } from './music'
import { requestID } from './search-controller.ts'

export interface Resolution { songId:string; sourceId:string; requestedQuality:string; quality:string; downgraded:boolean; url:string }
export interface ResolutionAPI { resolve(song:Song,id:string):Promise<Resolution>; cancel(id:string):Promise<boolean> }
export interface ResolutionState {status:'idle'|'loading'|'success'|'error';songId:string;quality:string;downgraded:boolean;error:string}
export const initialResolutionState=():ResolutionState=>({status:'idle',songId:'',quality:'',downgraded:false,error:''})
export class ResolutionController {
 private revision=0
 private activeID:string|null=null
 private api:ResolutionAPI
 private publish:(state:ResolutionState)=>void
 private notify:(state:ResolutionState)=>void
 private makeID:()=>string
 constructor(api:ResolutionAPI,publish:(state:ResolutionState)=>void,notify:(state:ResolutionState)=>void=()=>{},makeID=requestID){this.api=api;this.publish=publish;this.notify=notify;this.makeID=makeID}
 private cancelRequest(id:string){try{void this.api.cancel(id).catch(()=>{})}catch{/* Revision still discards late results. */}}
 async resolve(song:Song){
  const revision=++this.revision
  if(this.activeID)this.cancelRequest(this.activeID)
  this.activeID=null
  this.publish({...initialResolutionState(),status:'loading',songId:song.id})
  try{
   const id=this.makeID();this.activeID=id
   const result=await this.api.resolve(song,id)
   if(revision!==this.revision)return
   let url:URL
   try{if(!result||typeof result.url!=='string'||result.url.length>8192)throw Error();url=new URL(result.url)}catch{throw Error('音源返回异常解析结果，请重试')}
   if(result.songId!==song.id||!result.sourceId||!['flac','320k','128k'].includes(result.quality)||!['flac','320k','128k'].includes(result.requestedQuality)||result.downgraded!==(result.quality!==result.requestedQuality)||!['https:','http:'].includes(url.protocol)||url.username||url.password)throw Error('音源返回异常解析结果，请重试')
   // Do not retain signed URLs in UI state or write them to storage/logs.
   const state:ResolutionState={...initialResolutionState(),status:'success',songId:song.id,quality:result.quality,downgraded:result.downgraded}
   this.publish(state);this.notify(state)
  }catch(cause){
   if(revision!==this.revision)return
   const error=cause instanceof Error?cause.message:String(cause)
   const state:ResolutionState={...initialResolutionState(),status:'error',songId:song.id,error:error||'音源解析失败，请重试'}
   this.publish(state);this.notify(state)
  }finally{if(revision===this.revision)this.activeID=null}
 }
 cancel(){const id=this.activeID;this.activeID=null;this.revision++;if(id)this.cancelRequest(id);this.publish(initialResolutionState())}
}
