import type { Artist } from './music'
export interface SongCredits { id:string;artists:Artist[];albumMid:string }
const requests=new Map<string,Promise<SongCredits>>()
export async function loadSongCredits(id:string):Promise<SongCredits>{
 if(!/^[a-zA-Z0-9]{1,64}$/.test(id))throw Error('歌曲标识无效')
 const cached=requests.get(id);if(cached)return cached
 const request=(async()=>{
  const api=(window as Window&{go?:{main?:{App?:{GetSongCredits(id:string):Promise<SongCredits>}}}}).go?.main?.App
  if(!api?.GetSongCredits)throw Error('歌曲信息服务不可用')
  const result=await api.GetSongCredits(id)
  if(result?.id!==id||!Array.isArray(result.artists)||result.artists.length>32||result.artists.some(artist=>!artist||typeof artist.name!=='string'||typeof artist.id!=='string'||artist.id!==''&&!/^[a-zA-Z0-9]{1,64}$/.test(artist.id))||typeof result.albumMid!=='string'||result.albumMid!==''&&!/^[a-zA-Z0-9]{1,64}$/.test(result.albumMid))throw Error('歌曲信息返回异常，请重试')
  return result
 })()
 requests.set(id,request)
 try {const result=await request;while(requests.size>32)requests.delete(requests.keys().next().value!);return result}
 catch(error){requests.delete(id);throw error}
}
