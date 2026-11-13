import type {Song} from './music'
import {requestID} from './search-controller.ts'
export const forYouBatchSize=12
export interface ForYouResult{songs:Song[];reason:'ready'|'no_favorites'|'missing_song_ids'|'no_candidates';seedCount:number;skipped:number;warning:string}
export interface ForYouAPI{load(id:string):Promise<ForYouResult>;cancel(id:string):Promise<boolean>}
export interface ForYouState{batch:number;songs:Song[];status:'idle'|'loading'|'ready'|'error';refreshing:boolean;reason:ForYouResult['reason'];error:string}
export const initialForYouState=():ForYouState=>({batch:0,songs:[],status:'idle',refreshing:false,reason:'no_favorites',error:''})
const safe=(id:unknown)=>typeof id==='string'&&/^[a-zA-Z0-9]{1,64}$/.test(id)
function valid(value:ForYouResult){return value&&Array.isArray(value.songs)&&value.songs.length<=128&&['ready','no_favorites','missing_song_ids','no_candidates'].includes(value.reason)&&typeof value.warning==='string'&&(value.reason==='ready'||value.songs.length===0)&&new Set(value.songs.map(song=>song?.id)).size===value.songs.length&&value.songs.every(song=>song&&song.source==='tx'&&safe(song.id)&&typeof song.songId==='string'&&/^[1-9][0-9]{0,18}$/.test(song.songId)&&safe(song.albumMid)&&typeof song.title==='string'&&!!song.title&&typeof song.artist==='string'&&typeof song.coverUrl==='string'&&(song.coverUrl===''||/^https:\/\/y\.gtimg\.cn\/music\/photo_new\/T002R300x300M000[a-zA-Z0-9]{1,64}\.jpg$/.test(song.coverUrl)))}
export function pickForYou(pool:Song[],excluded:Set<string>,seen:Set<string>,previous:Set<string>,random=Math.random){
 const unique=new Map(pool.filter(song=>!excluded.has(song.id)&&!excluded.has(`numeric:${song.songId}`)).map(song=>[song.id,song]))
 const candidates=[...unique.values()]
 for(let i=candidates.length-1;i>0;i--){const j=Math.floor(random()*(i+1));[candidates[i],candidates[j]]=[candidates[j],candidates[i]]}
 const priority=(song:Song)=>!seen.has(song.id)?0:!previous.has(song.id)?1:2
 candidates.sort((a,b)=>priority(a)-priority(b))
 const chosen:Song[]=[],ids=new Set<string>(),artists=new Map<string,number>(),albums=new Map<string,number>()
 for(const [artistLimit,albumLimit] of [[2,1],[3,2],[Infinity,Infinity]]){
  for(const song of candidates){
   const artist=song.artists?.[0]?.id||song.artist
   if(ids.has(song.id)||(artists.get(artist)||0)>=artistLimit||(albums.get(song.albumMid)||0)>=albumLimit)continue
   chosen.push(song);ids.add(song.id);artists.set(artist,(artists.get(artist)||0)+1);albums.set(song.albumMid,(albums.get(song.albumMid)||0)+1)
   if(chosen.length===forYouBatchSize)return chosen
  }
 }
 return chosen
}
export class ForYouController{
 private state=initialForYouState();private pool:Song[]=[];private key:string|null=null
 private favorites=new Set<string>();private queue=new Set<string>();private seen=new Set<string>()
 private revision=0;private active:string|null=null;private pending:Promise<void>|null=null;private loadedAt=0
 private api:ForYouAPI;private publish:(state:ForYouState)=>void;private notice:(message:string,warning:boolean)=>void;private makeID:()=>string;private clock:()=>number
 constructor(api:ForYouAPI,publish:(state:ForYouState)=>void,notice:(message:string,warning:boolean)=>void=()=>{},makeID=requestID,clock=Date.now){this.api=api;this.publish=publish;this.notice=notice;this.makeID=makeID;this.clock=clock}
 private update(patch:Partial<ForYouState>){this.state={...this.state,...patch};this.publish(this.state)}
 setFavorites(songs:Song[]){
  const key=JSON.stringify(songs.map(song=>[song.id,song.songId]))
  if(key===this.key)return
  this.suspend();this.key=key;this.favorites=new Set(songs.flatMap(song=>song.songId?[song.id,`numeric:${song.songId}`]:[song.id]));this.pool=[];this.seen.clear();this.loadedAt=0;this.update(initialForYouState())
 }
 setQueue(songs:Song[]){this.queue=new Set(songs.flatMap(song=>song.songId?[song.id,`numeric:${song.songId}`]:[song.id]))}
 private batch(){
  const songs=pickForYou(this.pool,new Set([...this.favorites,...this.queue]),this.seen,new Set(this.state.songs.map(song=>song.id)))
  if(!songs.length&&this.state.songs.length&&this.pool.length)return false
  for(const song of songs)this.seen.add(song.id)
  while(this.seen.size>256)this.seen.delete(this.seen.values().next().value!)
  this.update({songs,batch:this.state.batch+1});return true
 }
 activate(force=false):Promise<void>{
  if(this.pending)return this.pending
  if(!force&&this.loadedAt&&this.clock()>=this.loadedAt&&this.clock()-this.loadedAt<3600000)return Promise.resolve()
  const revision=++this.revision;this.update({status:this.pool.length?'ready':'loading',refreshing:!!this.pool.length,error:''})
  let id:string
  try{id=this.makeID();this.active=id}catch{this.update({status:this.pool.length?'ready':'error',refreshing:false,error:'无法创建推荐请求，请重试'});this.notice(this.state.error,false);return Promise.resolve()}
  const task=Promise.resolve().then(async()=>{
   try{
    const result=await this.api.load(id);if(revision!==this.revision)return
    if(!valid(result))throw Error('QQ 音乐推荐返回异常数据，请重试')
    this.pool=result.songs;this.loadedAt=this.clock();this.update({reason:result.reason,status:'ready',error:result.warning});this.batch()
    if(result.warning)this.notice(result.warning,true)
   }catch(cause){if(revision!==this.revision)return;const error=cause instanceof Error?cause.message:String(cause)||'无法获取猜你喜欢，请重试';this.update({status:this.pool.length?'ready':'error',error});this.notice(this.pool.length?'更新未完成，已保留上次推荐':error,!!this.pool.length)}
   finally{if(revision===this.revision){this.active=null;this.pending=null;this.update({refreshing:false})}}
  });this.pending=task;return task
 }
 next(){if(this.pending)return;if(this.pool.length){if(!this.batch())this.notice('没有更多可用候选，已保留当前推荐',true)}else void this.activate(true)}
 suspend(){this.revision++;const id=this.active;this.active=null;this.pending=null;if(id)try{void this.api.cancel(id).catch(()=>{})}catch{};this.update({status:this.pool.length?'ready':'idle',refreshing:false})}
}
