export function registerWindowLifecycle(flush:()=>Promise<void>):()=>void {
 const host=window as Window&{runtime?:{EventsOn?:(name:string,callback:(id:string)=>void)=>()=>void};go?:{main?:{App?:{AcknowledgeQuit?:(id:string)=>Promise<boolean>}}}}
 const unsubscribe=host.runtime?.EventsOn?.('app:before-quit',id=>{
  if(typeof id!=='string'||!/^\d{1,20}$/.test(id))return
  void Promise.resolve().then(flush).catch(()=>{}).then(()=>host.go?.main?.App?.AcknowledgeQuit?.(id)).catch(()=>{})
 })
 return ()=>unsubscribe?.()
}
