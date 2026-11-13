import {toast} from 'svelte-sonner'
import {ForYouController,initialForYouState,type ForYouAPI} from './for-you-controller'
export const forYouState=$state(initialForYouState())
type Bridge={GetForYou:ForYouAPI['load'];CancelForYou:ForYouAPI['cancel']}
function bridge(){const value=(window as Window&{go?:{main?:{App?:Partial<Bridge>}}}).go?.main?.App;if(!value?.GetForYou||!value.CancelForYou)throw Error('请在桌面应用中读取猜你喜欢');return value as Bridge}
export const forYouController=new ForYouController({load:id=>bridge().GetForYou(id),cancel:id=>bridge().CancelForYou(id)},state=>Object.assign(forYouState,state),(message,warning)=>{if(warning)toast.warning(message,{id:'for-you'});else toast.error(message,{id:'for-you'})})
