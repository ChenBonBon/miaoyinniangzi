import { toast } from 'svelte-sonner'
import { DiscoveryController, initialDiscoveryState, type DiscoveryAPI } from './discovery-controller'
type Bridge={GetDiscoveryCatalog:DiscoveryAPI['catalog'];GetDiscoveryCollection:DiscoveryAPI['collection'];CancelDiscovery:DiscoveryAPI['cancel'];GetDiscoverySnapshot?:NonNullable<DiscoveryAPI['cached']>}
function bridge():Bridge{
 const api=(window as Window & {go?:{main?:{App?:Partial<Bridge>}}}).go?.main?.App
 if(!api?.GetDiscoveryCatalog||!api.GetDiscoveryCollection||!api.CancelDiscovery)throw Error('请在桌面应用中读取 QQ 音乐推荐')
 return api as Bridge
}
export const discoveryState=$state(initialDiscoveryState())
export const discoveryMemory=$state({catalogScroll:{chart:0,new:0},collectionScroll:0,page:1,focusEntry:''})
export const discoveryController=new DiscoveryController({catalog:async(...args)=>bridge().GetDiscoveryCatalog(...args),collection:async(...args)=>bridge().GetDiscoveryCollection(...args),cached:async(...args)=>bridge().GetDiscoverySnapshot?.(...args)??{savedAt:0},cancel:async id=>bridge().CancelDiscovery(id)},state=>{
 const previousError=discoveryState.error
 Object.assign(discoveryState,state)
 if(state.error && state.error!==previousError){
  const message=state.status==='ready'?'更新未完成，已保留上次内容':'暂时无法读取推荐'
  if(state.status==='ready')toast.warning(message,{id:'discovery',description:state.error})
  else toast.error(message,{id:'discovery',description:state.error})
 }else if(previousError && !state.error)toast.dismiss('discovery')
})
if(import.meta.hot)import.meta.hot.dispose(()=>discoveryController.suspend())
