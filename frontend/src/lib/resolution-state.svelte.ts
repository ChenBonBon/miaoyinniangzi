import { toast } from 'svelte-sonner'
import { ResolutionController, initialResolutionState, type ResolutionAPI, type Resolution } from './resolution-controller'
import type { Song } from './music'

type Bridge={ResolveSong(song:Song,id:string):Promise<Resolution>;CancelSongResolution(id:string):Promise<boolean>}
function bridge():Bridge{
 const host=window as Window & {go?:{main?:{App?:Bridge}}}
 if(!host.go?.main?.App?.ResolveSong)throw Error('请在 macOS 桌面应用中解析音源')
 return host.go.main.App
}
const api:ResolutionAPI={resolve:(song,id)=>bridge().ResolveSong(song,id),cancel:id=>bridge().CancelSongResolution(id)}
export const resolutionState=$state(initialResolutionState())
export const resolutionController=new ResolutionController(api,state=>Object.assign(resolutionState,state),state=>{
 if(state.status==='error')toast.error(state.error,{id:'source-resolution'})
 else if(state.status==='success')toast.success(state.downgraded?`解析成功，已降至 ${state.quality.toUpperCase()}`:`解析成功 · ${state.quality.toUpperCase()}`,{id:'source-resolution'})
})
if(import.meta.hot)import.meta.hot.dispose(()=>resolutionController.cancel())
