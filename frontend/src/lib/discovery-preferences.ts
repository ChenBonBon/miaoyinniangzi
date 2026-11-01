import type { DiscoveryEntry } from './discovery-controller'
export interface ChartPreference { id: number; hidden: boolean }
export interface GroupPreference { key: string; hidden: boolean; charts: ChartPreference[] }
export interface DiscoveryPreferences { groups: GroupPreference[] }
export interface LayoutGroup { key: string; title: string; hidden: boolean; charts: { entry: DiscoveryEntry; hidden: boolean }[] }
export const defaultDiscoveryPreferences = (): DiscoveryPreferences => ({ groups: [] })
export const discoveryGroupKey = (entry: DiscoveryEntry) => Number.isInteger(entry.groupId) && entry.groupId! >= 0 && entry.groupId! <= 1000000 ? `qq:${entry.groupId}` : `name:${entry.group || '榜单'}`
export function validDiscoveryPreferences(value: DiscoveryPreferences): boolean {
 if (!value || !Array.isArray(value.groups) || value.groups.length>32) return false
 const keys=new Set<string>(); let total=0
 return value.groups.every(group=>{
  if(!group||typeof group.key!=='string'||keys.has(group.key)||typeof group.hidden!=='boolean'||!Array.isArray(group.charts))return false
  if(!/^qq:(0|[1-9]\d{0,6})$/.test(group.key)&&!/^name:[^\p{Cc}]{1,128}$/u.test(group.key))return false
  if(group.key.startsWith('qq:')&&Number(group.key.slice(3))>1000000)return false
  keys.add(group.key);const ids=new Set<number>()
  return group.charts.every(chart=>{total++;if(total>1024||!chart||!Number.isInteger(chart.id)||chart.id<=0||chart.id>1000000||typeof chart.hidden!=='boolean'||ids.has(chart.id))return false;ids.add(chart.id);return true})
 })
}
export function discoveryLayout(entries: DiscoveryEntry[], preferences: DiscoveryPreferences): LayoutGroup[] {
 const groups=new Map<string,LayoutGroup>()
 for(const entry of entries){const key=discoveryGroupKey(entry);if(!groups.has(key))groups.set(key,{key,title:entry.group||'榜单',hidden:false,charts:[]});groups.get(key)!.charts.push({entry,hidden:false})}
 const ordered: LayoutGroup[]=[]
 for(const saved of preferences.groups){
  const group=groups.get(saved.key);if(!group)continue
  group.hidden=saved.hidden
  const charts=new Map(group.charts.map(chart=>[chart.entry.id,chart])),sorted:LayoutGroup['charts']=[]
  for(const preference of saved.charts){const chart=charts.get(preference.id);if(chart){chart.hidden=preference.hidden;sorted.push(chart);charts.delete(preference.id)}}
  group.charts=[...sorted,...charts.values()];ordered.push(group);groups.delete(saved.key)
 }
 return [...ordered,...groups.values()]
}
// Materialize current defaults without throwing away temporarily absent QQ IDs.
export function materializePreferences(entries: DiscoveryEntry[], preferences: DiscoveryPreferences): DiscoveryPreferences {
 const result:DiscoveryPreferences=JSON.parse(JSON.stringify(preferences))
 for(const group of discoveryLayout(entries,preferences)){
  let saved=result.groups.find(item=>item.key===group.key)
  if(!saved){saved={key:group.key,hidden:false,charts:[]};result.groups.push(saved)}
  for(const chart of group.charts)if(!saved.charts.some(item=>item.id===chart.entry.id))saved.charts.push({id:chart.entry.id,hidden:false})
 }
 return result
}
export function reorder<T>(items:T[],from:number,to:number):T[]{
 if(from<0||to<0||from>=items.length||to>=items.length||from===to)return items
 const result=[...items], [item]=result.splice(from,1);result.splice(to,0,item);return result
}
export interface PreferencesAPI { get():Promise<DiscoveryPreferences>; save(value:DiscoveryPreferences):Promise<void> }
export interface PreferencesState { value:DiscoveryPreferences; status:'idle'|'loading'|'ready'|'error'; saving:boolean; error:string }
export const initialPreferencesState=():PreferencesState=>({value:defaultDiscoveryPreferences(),status:'idle',saving:false,error:''})
export class DiscoveryPreferencesController {
 private state=initialPreferencesState()
 private reading:Promise<void>|null=null
 private writing=false
 private revision=0
 private savedRevision=0
 private api:PreferencesAPI
 private publish:(state:PreferencesState)=>void
 constructor(api:PreferencesAPI,publish:(state:PreferencesState)=>void){this.api=api;this.publish=publish}
 private update(patch:Partial<PreferencesState>){this.state={...this.state,...patch};this.publish(this.state)}
 load():Promise<void>{
  if(this.state.status==='ready')return Promise.resolve()
  if(this.reading)return this.reading
  this.update({status:'loading',error:''})
  this.reading=this.api.get().then(value=>{if(!validDiscoveryPreferences(value))throw Error('invalid layout');this.update({value,status:'ready',error:''})}).catch(()=>this.update({status:'error',error:'无法读取推荐布局，请重试'})).finally(()=>this.reading=null)
  return this.reading
 }
 change(value:DiscoveryPreferences){
  if(this.state.status!=='ready')return
  if(!validDiscoveryPreferences(value)){this.update({error:'推荐布局超出限制，请恢复默认后重试'});return}
  this.revision++;this.update({value:JSON.parse(JSON.stringify(value)),error:''});void this.drain()
 }
 retry(){if(this.state.status==='error')return this.load();this.update({error:''});return this.drain()}
 private async drain(){
  if(this.writing||this.savedRevision===this.revision)return
  this.writing=true;this.update({saving:true})
  try{
   while(this.savedRevision!==this.revision){const version=this.revision,value=JSON.parse(JSON.stringify(this.state.value));await this.api.save(value);this.savedRevision=version}
  }catch{this.update({error:'推荐布局保存失败，调整尚未保存，请重试'})}
  finally{this.writing=false;this.update({saving:false})}
 }
}
