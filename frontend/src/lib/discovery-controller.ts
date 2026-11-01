import type { Song } from './music'
import { requestID } from './search-controller.ts'
export type DiscoveryKind = 'chart' | 'new'
export interface DiscoveryEntry { kind: DiscoveryKind; id: number; title: string; group: string; groupId?: number; coverUrl: string; period: string; updated: string; total: number }
export interface DiscoveryCatalog { kind: DiscoveryKind; entries: DiscoveryEntry[] }
export interface DiscoveryCollection { entry: DiscoveryEntry; songs: Song[] }
export interface DiscoveryAPI {
 catalog(kind: DiscoveryKind, requestID: string): Promise<DiscoveryCatalog>
 collection(kind: DiscoveryKind, id: number, period: string, requestID: string): Promise<DiscoveryCollection>
 cancel(requestID: string): Promise<boolean>
}
export interface DiscoveryState { kind: DiscoveryKind; catalog: DiscoveryCatalog | null; selected: DiscoveryEntry | null; collection: DiscoveryCollection | null; status: 'idle' | 'loading' | 'ready' | 'error'; error: string }
export const initialDiscoveryState = (): DiscoveryState => ({ kind:'chart',catalog:null,selected:null,collection:null,status:'idle',error:'' })
export const discoveryKey = (entry: DiscoveryEntry) => `${entry.kind}:${entry.id}:${entry.period}`
const validEntry = (entry: DiscoveryEntry,kind:DiscoveryKind) => entry && entry.kind===kind && Number.isInteger(entry.id) && entry.id>0 && entry.id<=1000000 && typeof entry.title==='string' && !!entry.title && typeof entry.period==='string' && /^[a-zA-Z0-9_-]{0,32}$/.test(entry.period)
export class DiscoveryController {
 private api: DiscoveryAPI
 private publish: (state: DiscoveryState) => void
 private makeID: () => string
 private clock: () => number
 private state=initialDiscoveryState()
 private revision=0
 private activeID: string|null=null
 private catalogs=new Map<DiscoveryKind,{time:number;value:DiscoveryCatalog}>()
 private collections=new Map<string,{time:number;value:DiscoveryCollection}>()
 constructor(api:DiscoveryAPI,publish:(state:DiscoveryState)=>void,makeID=requestID,clock=Date.now){this.api=api;this.publish=publish;this.makeID=makeID;this.clock=clock}
 private update(patch:Partial<DiscoveryState>){this.state={...this.state,...patch};this.publish(this.state)}
 private cancel(){
  this.revision++
  const id=this.activeID;this.activeID=null
  if(id){try{void this.api.cancel(id).catch(()=>{})}catch{}}
 }
 activate(){if(this.state.status==='ready')return Promise.resolve();return this.refresh()}
 async selectKind(kind:DiscoveryKind){
  if(kind===this.state.kind&&!this.state.selected&&(this.state.status==='ready'||this.state.status==='loading'))return
  this.cancel();this.update({kind,catalog:null,selected:null,collection:null,status:'idle',error:''})
  await this.loadCatalog(false)
 }
 async open(entry:DiscoveryEntry){
  if(!validEntry(entry,this.state.kind))return
  if(this.state.selected&&discoveryKey(this.state.selected)===discoveryKey(entry)&&this.state.status==='loading')return
  this.cancel();this.update({selected:entry,collection:null,status:'idle',error:''})
  await this.loadCollection(false)
 }
 async back(){
  this.cancel();this.update({selected:null,collection:null,status:'idle',error:''})
  await this.loadCatalog(false)
 }
 refresh(){return this.state.selected?this.loadCollection(true):this.loadCatalog(true)}
 suspend(){this.cancel();if(this.state.status==='loading')this.update({status:'idle',error:''})}
 private async loadCatalog(force:boolean){
  const kind=this.state.kind,cached=this.catalogs.get(kind)
  if(!force&&cached&&this.clock()-cached.time<300000){this.update({catalog:cached.value,status:'ready',error:''});return}
  await this.request(async id=>{
   const catalog=await this.api.catalog(kind,id)
   if(!catalog||catalog.kind!==kind||!Array.isArray(catalog.entries)||catalog.entries.length>128||catalog.entries.some(entry=>!validEntry(entry,kind))||new Set(catalog.entries.map(entry=>entry.id)).size!==catalog.entries.length)throw Error('QQ 音乐推荐返回异常数据，请重试')
   return ()=>{this.catalogs.set(kind,{time:this.clock(),value:catalog});this.update({catalog,status:'ready'})}
  })
 }
 private async loadCollection(force:boolean){
  const entry=this.state.selected;if(!entry)return
  const key=discoveryKey(entry),cached=this.collections.get(key)
  if(!force&&cached&&this.clock()-cached.time<300000){this.collections.delete(key);this.collections.set(key,cached);this.update({selected:cached.value.entry,collection:cached.value,status:'ready',error:''});return}
  await this.request(async id=>{
   const collection=await this.api.collection(entry.kind,entry.id,entry.period,id)
   if(!collection||!validEntry(collection.entry,entry.kind)||collection.entry.id!==entry.id||entry.period!==''&&collection.entry.period!==entry.period||!Array.isArray(collection.songs)||collection.songs.length>1000||collection.songs.some(song=>!song||song.source!=='tx'||typeof song.id!=='string'||!song.id||typeof song.title!=='string'||!song.title)||new Set(collection.songs.map(song=>song.id)).size!==collection.songs.length)throw Error('QQ 音乐推荐返回异常数据，请重试')
   return ()=>{
    for(const cachedKey of this.collections.keys())if(cachedKey.startsWith(`${entry.kind}:${entry.id}:`))this.collections.delete(cachedKey)
    this.collections.set(key,{time:this.clock(),value:collection})
    while(this.collections.size>8)this.collections.delete(this.collections.keys().next().value!)
    this.update({selected:collection.entry,collection,status:'ready'})
   }
  })
 }
 private async request(load:(id:string)=>Promise<()=>void>){
  this.cancel();const revision=this.revision
  this.update({status:'loading',error:''})
  try{
   const id=this.makeID();this.activeID=id
   const commit=await load(id);if(revision!==this.revision)return;commit()
  }catch(cause){if(revision!==this.revision)return;this.update({status:'error',error:cause instanceof Error?cause.message:String(cause)||'无法读取推荐，请重试'})}
  finally{if(revision===this.revision)this.activeID=null}
 }
}
