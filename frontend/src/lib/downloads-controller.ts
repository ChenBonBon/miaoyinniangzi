import type { Song } from './music'
export type DownloadStatus = 'queued' | 'downloading' | 'success' | 'failed' | 'skipped' | 'canceled' | 'paused'
export interface DownloadTask { id: string; batch: string; song: Song; directory: string; requested: string; quality: string; codec: string; state: DownloadStatus; bytes: number; total: number; file: string; message: string; created: number }
export interface EnqueueResult { tasks: DownloadTask[]; existing: number }
export type DownloadAction = 'start' | 'pause' | 'cancel'
export function canControlDownload(task: DownloadTask, action: DownloadAction) {
 if(action==='start')return ['paused','failed'].includes(task.state)
 if(action==='pause')return ['queued','downloading'].includes(task.state)
 return ['queued','downloading','paused','failed'].includes(task.state)
}
export interface DownloadsAPI {
 list(): Promise<DownloadTask[]>
 enqueue(songs: Song[]): Promise<EnqueueResult>
 pause(id: string): Promise<void>
 resume(id: string): Promise<void>
 remove(id: string, deleteFile: boolean): Promise<void>
 cancelBatch(batch: string): Promise<void>
 retryBatch(batch: string): Promise<void>
 control(ids: string[], action: DownloadAction, all: boolean): Promise<void>
}
export interface DownloadsState { tasks: DownloadTask[]; status: 'idle' | 'loading' | 'ready' | 'error'; pending: string[]; error: string; speeds: Record<string, number> }
export const initialDownloadsState = (): DownloadsState => ({ tasks: [], status: 'idle', pending: [], error: '', speeds: {} })
export const downloadLabels: Record<DownloadStatus, string> = { queued: '等待中', downloading: '下载中', success: '下载成功', failed: '下载失败', skipped: '已跳过', canceled: '已取消', paused: '已暂停' }
export class DownloadsController {
 private api: DownloadsAPI
 private publish: (state: DownloadsState) => void
 private notice: (message: string, error: boolean) => void
 private state = initialDownloadsState()
 private reading: Promise<void> | null = null
 private samples = new Map<string, { bytes: number; time: number }>()
 private clock: () => number
 constructor(api: DownloadsAPI, publish: (state: DownloadsState) => void, notice: (message: string, error: boolean) => void = () => {}, clock = () => performance.now()) { this.api=api; this.publish=publish; this.notice=notice; this.clock=clock }
 private update(patch: Partial<DownloadsState>) { this.state={...this.state,...patch};this.publish(this.state) }
 refresh(afterPending = false): Promise<void> {
  if (this.reading) return afterPending ? this.reading.then(() => this.refresh()) : this.reading
  if (this.state.status === 'idle') this.update({status:'loading'})
  this.reading=this.read().finally(()=>this.reading=null)
  return this.reading
 }
 private async read() {
  try {
   const tasks=await this.api.list()
   if (!Array.isArray(tasks) || tasks.some(task=>!task || !task.id || !task.song || !Object.hasOwn(downloadLabels, task.state) || !Number.isFinite(task.bytes) || task.bytes<0 || !Number.isFinite(task.total) || task.total<0)) throw Error('invalid download state')
   const now=this.clock(), samples=new Map<string,{bytes:number;time:number}>(), speeds:Record<string,number>={}
   for(const task of tasks){
    if(task.state!=='downloading')continue
    const previous=this.samples.get(task.id)
    if(previous && task.bytes>=previous.bytes && now>previous.time){
     if(now-previous.time>=500){speeds[task.id]=(task.bytes-previous.bytes)*1000/(now-previous.time);samples.set(task.id,{bytes:task.bytes,time:now})}
     else{samples.set(task.id,previous);if(this.state.speeds[task.id]!==undefined)speeds[task.id]=this.state.speeds[task.id]}
    }else samples.set(task.id,{bytes:task.bytes,time:now})
   }
   this.samples=samples
   this.update({tasks:tasks.filter(task=>task.state!=='canceled'),status:'ready',error:'',speeds})
  } catch { this.samples.clear();this.update({status:'error',error:'无法读取下载记录，请重试',speeds:{}}) }
 }
 async enqueue(songs: Song[]): Promise<boolean> {
  try {
   const result=await this.api.enqueue(songs);await this.refresh(true)
   const messages: string[]=[]
   if(result.tasks.length)messages.push(`已加入 ${result.tasks.length} 首到下载队列`)
   if(result.existing)messages.push(result.existing===1?'下载目录中已有该歌曲，无需重复下载':`下载目录中已有 ${result.existing} 首歌曲，无需重复下载`)
   this.notice(messages.join('；')||'已有对应下载任务',false);return true
  }
  catch(cause) { this.notice(cause instanceof Error ? cause.message : String(cause),true);return false }
 }
 async action(key: string, operation: () => Promise<void>): Promise<boolean> {
  if(this.state.pending.includes(key))return false
  this.update({pending:[...this.state.pending,key]})
  try { await operation();await this.refresh(true);return true }
  catch(cause) { this.notice(cause instanceof Error ? cause.message : String(cause),true);return false }
  finally { this.update({pending:this.state.pending.filter(id=>id!==key)}) }
 }
 pause(id:string){return this.action(id,()=>this.api.pause(id))}
 resume(id:string){return this.action(id,()=>this.api.resume(id))}
 remove(id:string,deleteFile=false){return this.action(id,()=>this.api.remove(id,deleteFile))}
 cancelBatch(batch:string){return this.action(batch,()=>this.api.cancelBatch(batch))}
 retryBatch(batch:string){return this.action(batch,()=>this.api.retryBatch(batch))}
 control(ids: string[], action: DownloadAction, all=false){
  if(!all&&!ids.length)return Promise.resolve(false)
  return this.action('download-control',()=>this.api.control([...new Set(ids)],action,all))
 }
}
