import type { Song } from './music'
import { requestID } from './search-controller.ts'
export type DiscoveryKind = 'chart' | 'new'
export interface DiscoveryEntry { kind: DiscoveryKind; id: number; title: string; group: string; groupId?: number; coverUrl: string; period: string; updated: string; total: number }
export interface DiscoveryCatalog { kind: DiscoveryKind; entries: DiscoveryEntry[] }
export interface DiscoveryCollection { entry: DiscoveryEntry; songs: Song[] }
export interface DiscoverySnapshot { savedAt: number; catalog?: DiscoveryCatalog; collection?: DiscoveryCollection }
export interface DiscoveryAPI {
 catalog(kind: DiscoveryKind, requestID: string): Promise<DiscoveryCatalog>
 collection(kind: DiscoveryKind, id: number, period: string, requestID: string): Promise<DiscoveryCollection>
 cached?(kind: DiscoveryKind, id: number, period: string): Promise<DiscoverySnapshot>
 cancel(requestID: string): Promise<boolean>
}
export interface DiscoveryState { kind: DiscoveryKind; catalog: DiscoveryCatalog | null; selected: DiscoveryEntry | null; collection: DiscoveryCollection | null; status: 'idle' | 'loading' | 'ready' | 'error'; error: string; refreshing: boolean }
export const initialDiscoveryState = (): DiscoveryState => ({ kind:'chart',catalog:null,selected:null,collection:null,status:'idle',error:'',refreshing:false })
export const discoveryKey = (entry: DiscoveryEntry) => `${entry.kind}:${entry.id}:${entry.period}`
const validEntry = (entry: DiscoveryEntry,kind:DiscoveryKind) => entry && entry.kind===kind && Number.isInteger(entry.id) && entry.id>0 && entry.id<=1000000 && typeof entry.title==='string' && !!entry.title && typeof entry.period==='string' && /^[a-zA-Z0-9_-]{0,32}$/.test(entry.period)
const validCatalog = (value:DiscoveryCatalog,kind:DiscoveryKind) => value && value.kind===kind && Array.isArray(value.entries) && value.entries.length<=128 && value.entries.every(entry=>validEntry(entry,kind)) && new Set(value.entries.map(entry=>entry.id)).size===value.entries.length
const validCollection = (value:DiscoveryCollection,entry:DiscoveryEntry) => value && validEntry(value.entry,entry.kind) && value.entry.id===entry.id && (entry.period===''||value.entry.period===entry.period) && Array.isArray(value.songs) && value.songs.length<=1000 && value.songs.every(song=>song&&song.source==='tx'&&typeof song.id==='string'&&!!song.id&&typeof song.title==='string'&&!!song.title) && new Set(value.songs.map(song=>song.id)).size===value.songs.length
const freshFor=300000, retainFor=72*60*60*1000
interface Cached<T> { time:number; value:T }
export class DiscoveryController {
 private api:DiscoveryAPI
 private publish:(state:DiscoveryState)=>void
 private makeID:()=>string
 private clock:()=>number
 private state=initialDiscoveryState()
 private revision=0
 private activeID:string|null=null
 private pending:Promise<void>|null=null
 private forceRequested=false
 private interruptedKey:string|null=null
 private catalogs=new Map<DiscoveryKind,Cached<DiscoveryCatalog>>()
 private collections=new Map<string,Cached<DiscoveryCollection>>()
 constructor(api:DiscoveryAPI,publish:(state:DiscoveryState)=>void,makeID=requestID,clock=Date.now){this.api=api;this.publish=publish;this.makeID=makeID;this.clock=clock}
 private update(patch:Partial<DiscoveryState>){this.state={...this.state,...patch};this.publish(this.state)}
 private hasData(){return this.state.selected?this.state.collection!==null:this.state.catalog!==null}
 private usable(time:number){return Number.isFinite(time)&&time>=0&&time<=this.clock()+60000&&this.clock()-time<=retainFor}
 private fresh(time:number){return this.usable(time)&&this.clock()-time<freshFor}
 private cancel(){
  this.revision++;const id=this.activeID;this.activeID=null;this.pending=null;this.forceRequested=false
  if(id){try{void this.api.cancel(id).catch(()=>{})}catch{}}
 }
 activate(){return this.pending??(this.state.selected?this.loadCollection(false):this.loadCatalog(false))}
 async selectKind(kind:DiscoveryKind){
  if(kind===this.state.kind&&!this.state.selected&&(this.state.status==='ready'||this.state.status==='loading'))return
  this.cancel();this.update({kind,catalog:null,selected:null,collection:null,status:'idle',error:'',refreshing:false})
  await this.loadCatalog(false)
 }
 async open(entry:DiscoveryEntry){
  if(!validEntry(entry,this.state.kind))return
  if(this.state.selected&&discoveryKey(this.state.selected)===discoveryKey(entry)&&this.pending)return
  this.cancel();this.update({selected:entry,collection:null,status:'idle',error:'',refreshing:false})
  await this.loadCollection(false)
 }
 async back(){
  this.cancel();this.update({selected:null,collection:null,status:'idle',error:'',refreshing:false})
  await this.loadCatalog(false)
 }
 refresh(){this.forceRequested=true;return this.pending??(this.state.selected?this.loadCollection(true):this.loadCatalog(true))}
 suspend(){if(this.pending)this.interruptedKey=this.state.selected?`collection:${discoveryKey(this.state.selected)}`:`catalog:${this.state.kind}`;this.cancel();this.update({status:this.hasData()?'ready':this.state.status==='loading'?'idle':this.state.status,refreshing:false})}
 private async snapshot(kind:DiscoveryKind,id:number,period:string){
  try{const value=await this.api.cached?.(kind,id,period);return value&&value.savedAt>0&&this.usable(value.savedAt)?value:null}catch{return null}
 }
 private loadCatalog(force:boolean):Promise<void>{
  const kind=this.state.kind
  if(this.interruptedKey===`catalog:${kind}`){force=true;this.interruptedKey=null}
  const candidate=this.catalogs.get(kind),cached=candidate&&this.usable(candidate.time)?candidate:null
  if(candidate&&!cached){this.catalogs.delete(kind);if(this.state.catalog===candidate.value)this.update({catalog:null})}
  if(cached){this.update({catalog:cached.value,status:'ready',error:'',refreshing:false});if(!force&&this.fresh(cached.time))return Promise.resolve()}
  return this.request(async(id,revision)=>{
   if(!cached&&this.api.cached){
    const snapshot=await this.snapshot(kind,0,'');if(revision!==this.revision)return ()=>{}
    if(snapshot?.catalog&&validCatalog(snapshot.catalog,kind)){
     this.catalogs.set(kind,{time:snapshot.savedAt,value:snapshot.catalog});this.update({catalog:snapshot.catalog,status:'ready',refreshing:true})
     if(!force&&!this.forceRequested&&this.fresh(snapshot.savedAt))return ()=>{}
    }
   }
   const catalog=await this.api.catalog(kind,id)
   if(!validCatalog(catalog,kind))throw Error('QQ 音乐推荐返回异常数据，请重试')
   return ()=>{this.catalogs.set(kind,{time:this.clock(),value:catalog});this.update({catalog,status:'ready',error:''})}
  })
 }
 private loadCollection(force:boolean):Promise<void>{
  const entry=this.state.selected;if(!entry)return Promise.resolve()
  const key=discoveryKey(entry)
  if(this.interruptedKey===`collection:${key}`){force=true;this.interruptedKey=null}
  const candidate=this.collections.get(key),cached=candidate&&this.usable(candidate.time)?candidate:null
  if(candidate&&!cached){this.collections.delete(key);if(this.state.collection===candidate.value)this.update({collection:null})}
  if(cached){this.collections.delete(key);this.collections.set(key,cached);this.update({selected:cached.value.entry,collection:cached.value,status:'ready',error:'',refreshing:false});if(!force&&this.fresh(cached.time))return Promise.resolve()}
  return this.request(async(id,revision)=>{
   if(!cached&&this.api.cached){
    const snapshot=await this.snapshot(entry.kind,entry.id,entry.period);if(revision!==this.revision)return ()=>{}
    if(snapshot?.collection&&validCollection(snapshot.collection,entry)){
     this.rememberCollection(key,snapshot.collection,snapshot.savedAt);this.update({selected:snapshot.collection.entry,collection:snapshot.collection,status:'ready',refreshing:true})
     if(!force&&!this.forceRequested&&this.fresh(snapshot.savedAt))return ()=>{}
    }
   }
   const collection=await this.api.collection(entry.kind,entry.id,entry.period,id)
   if(!validCollection(collection,entry))throw Error('QQ 音乐推荐返回异常数据，请重试')
   return ()=>{this.rememberCollection(key,collection,this.clock());this.update({selected:collection.entry,collection,status:'ready',error:''})}
  })
 }
 private rememberCollection(key:string,value:DiscoveryCollection,time:number){
  for(const oldKey of this.collections.keys())if(oldKey.startsWith(`${value.entry.kind}:${value.entry.id}:`))this.collections.delete(oldKey)
  this.collections.set(key,{time,value});while(this.collections.size>8)this.collections.delete(this.collections.keys().next().value!)
 }
 private request(load:(id:string,revision:number)=>Promise<()=>void>):Promise<void>{
  this.cancel();const revision=this.revision
  this.update({status:this.hasData()?'ready':'loading',refreshing:this.hasData(),error:''})
  let id:string
  try{id=this.makeID();this.activeID=id}catch{this.forceRequested=false;this.update({status:this.hasData()?'ready':'error',refreshing:false,error:'无法创建推荐请求，请重试'});return Promise.resolve()}
  const task=(async()=>{
   try{
    const commit=await load(id,revision);if(revision!==this.revision)return;commit()
   }catch(cause){
    if(revision!==this.revision)return
    this.update({status:this.hasData()?'ready':'error',error:cause instanceof Error?cause.message:String(cause)||'无法读取推荐，请重试'})
   }finally{if(revision===this.revision){this.activeID=null;this.pending=null;this.forceRequested=false;this.update({refreshing:false})}}
  })()
  this.pending=task;return task
 }
}
