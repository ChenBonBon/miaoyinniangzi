import type { Song, Artist } from './music'
export type DetailKind='artist'|'album'
export interface AlbumSummary { id:string;title:string;artist:string;coverUrl:string;date:string }
export interface MusicDetail { kind:DetailKind;id:string;title:string;description?:string;coverUrl:string;artists:Artist[];date:string;songs:Song[];albums:AlbumSummary[];albumPage:number;totalAlbums:number;hasMoreAlbums:boolean }
export interface DetailAPI { get(kind:DetailKind,id:string,page:number,requestID:string):Promise<MusicDetail>;cancel(requestID:string):Promise<unknown> }
export interface DetailState { status:'idle'|'loading'|'ready'|'error';value:MusicDetail|null;error:string }
export class DetailController {
 private state:DetailState={status:'idle',value:null,error:''}
 private revision=0
 private request=''
 private key=''
 private pending:Promise<void>|null=null
 private cache=new Map<string,{time:number;value:MusicDetail}>()
 private api:DetailAPI
 private publish:(state:DetailState)=>void
 private makeID:()=>string
 constructor(api:DetailAPI,publish:(state:DetailState)=>void,makeID=()=>crypto.randomUUID()){this.api=api;this.publish=publish;this.makeID=makeID}
 private update(patch:Partial<DetailState>){this.state={...this.state,...patch};this.publish(this.state)}
 suspend(){this.revision++;const id=this.request;this.request='';if(id)void this.api.cancel(id).catch(()=>{});if(this.state.status==='loading')this.update({status:'idle'})}
 open(kind:DetailKind,id:string,page=1,force=false):Promise<void>{
  if(!['artist','album'].includes(kind)||!/^[a-zA-Z0-9]{1,64}$/.test(id)||!Number.isInteger(page)||page<1||page>100||kind==='album'&&page!==1)return Promise.resolve()
  const key=`${kind}:${id}:${page}`
  if(!force&&this.key===key&&this.pending&&this.state.status==='loading')return this.pending
  this.suspend();this.key=key;const revision=this.revision,cached=this.cache.get(key)
  if(!force&&cached&&Date.now()-cached.time<300000){this.cache.delete(key);this.cache.set(key,cached);this.update({value:cached.value,status:'ready',error:''});return Promise.resolve()}
  this.update({value:null,status:'loading',error:''})
  this.pending=(async()=>{
   try{
    const request=this.makeID();this.request=request
    const value=await this.api.get(kind,id,page,request)
    if(revision!==this.revision)return
    if(!value||value.kind!==kind||value.id!==id||value.albumPage!==page||!value.title||!Array.isArray(value.songs)||value.songs.length>1000||value.songs.some(song=>!song||song.source!=='tx'||!song.id)||!Array.isArray(value.albums)||value.albums.length>20||value.albums.some(album=>!album||!/^[a-zA-Z0-9]{1,64}$/.test(album.id)||!album.title))throw Error('invalid detail')
    this.cache.delete(key);this.cache.set(key,{time:Date.now(),value});while(this.cache.size>8)this.cache.delete(this.cache.keys().next().value!)
    this.update({value,status:'ready',error:''})
   }catch{if(revision===this.revision)this.update({status:'error',error:'无法读取 QQ 音乐详情，请稍后重试'})}
   finally{if(revision===this.revision){this.request='';this.pending=null}}
  })()
  return this.pending!
 }
}
