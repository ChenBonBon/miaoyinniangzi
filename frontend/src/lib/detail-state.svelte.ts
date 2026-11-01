import { DetailController, type DetailAPI, type DetailKind, type DetailState } from './detail-controller'
import { navigate } from './navigation.svelte'
export const detailSelection=$state({kind:'artist' as DetailKind,id:'',albumPage:1,songPage:1,hotExpanded:false,albumsExpanded:false})
export const detailState=$state<DetailState>({status:'idle',value:null,error:''})
function bridge(){const api=(window as Window&{go?:{main?:{App?:{GetMusicDetail:DetailAPI['get'];CancelMusicDetail:DetailAPI['cancel']}}}}).go?.main?.App;if(!api?.GetMusicDetail)throw Error('详情服务不可用');return api}
export const detailController=new DetailController({get:async(...args)=>bridge().GetMusicDetail(...args),cancel:async id=>bridge().CancelMusicDetail(id)},value=>Object.assign(detailState,value))
export function openMusicDetail(kind:DetailKind,id:string){
 if(!/^[a-zA-Z0-9]{1,64}$/.test(id))return
 navigate('音乐详情',()=>{Object.assign(detailSelection,{kind,id,albumPage:1,songPage:1,hotExpanded:false,albumsExpanded:false});void detailController.open(kind,id)})
}
