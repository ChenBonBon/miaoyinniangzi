import { toast } from 'svelte-sonner'
import { DownloadsController, initialDownloadsState, type DownloadsAPI } from './downloads-controller'
type Bridge={GetDownloadTasks:DownloadsAPI['list'];EnqueueDownloads:DownloadsAPI['enqueue'];PauseDownload:DownloadsAPI['pause'];ContinueDownload:DownloadsAPI['resume'];RemoveDownload:DownloadsAPI['remove'];CancelDownloadBatch:DownloadsAPI['cancelBatch'];RetryDownloadBatch:DownloadsAPI['retryBatch'];ControlDownloads:DownloadsAPI['control']}
function bridge():Bridge{
 const api=(window as Window & {go?:{main?:{App?:Partial<Bridge>}}}).go?.main?.App
 if(!api?.GetDownloadTasks || !api.EnqueueDownloads)throw Error('请在桌面应用中管理下载')
 return api as Bridge
}
export const downloadsState=$state(initialDownloadsState())
let initialized=false
let unsubscribe:(()=>void)|undefined
export function initializeDownloads(){
 if(initialized)return
 initialized=true
 const host=window as Window & {runtime?:{EventsOn?:(name:string,callback:()=>void)=>()=>void}}
 unsubscribe=host.runtime?.EventsOn?.('download:duplicate',()=>{
  toast('下载目录中已有该歌曲，无需重复下载',{id:'download-duplicate'})
  void downloadsController.refresh(true)
 })
}
if(import.meta.hot)import.meta.hot.dispose(()=>unsubscribe?.())
export const downloadsController=new DownloadsController({list:async()=>bridge().GetDownloadTasks(),enqueue:async songs=>bridge().EnqueueDownloads(songs),pause:async id=>bridge().PauseDownload(id),resume:async id=>bridge().ContinueDownload(id),remove:async(id,remove)=>bridge().RemoveDownload(id,remove),cancelBatch:async batch=>bridge().CancelDownloadBatch(batch),retryBatch:async batch=>bridge().RetryDownloadBatch(batch),control:async(ids,action,all)=>bridge().ControlDownloads(ids,action,all)},state=>Object.assign(downloadsState,state),(message,error)=>{if(error)toast.error(message,{id:'downloads'});else toast.success(message,{id:'downloads'})})
