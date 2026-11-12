export type Appearance = 'dark' | 'light' | 'system'
export const validAppearance=(value:unknown):value is Appearance=>value==='dark'||value==='light'||value==='system'
export const resolveAppearance=(mode:Appearance,dark:boolean):'dark'|'light'=>mode==='system'?(dark?'dark':'light'):mode
export interface AppearanceAPI{load():Promise<string>;save(mode:Appearance):Promise<void>}
export interface AppearanceState{mode:Appearance;loaded:boolean;loading:boolean;status:'saved'|'saving'|'error';error:string}
export class AppearanceController{
 state:AppearanceState
 private confirmed:Appearance|null
 private reading:Promise<void>|null=null
 private writing=false
 private pending=false
 private api:AppearanceAPI
 private publish:(state:AppearanceState)=>void
 private remember:(mode:Appearance)=>void
 constructor(api:AppearanceAPI,publish:(state:AppearanceState)=>void,initial:Appearance='dark',remember:(mode:Appearance)=>void=()=>{}){
  this.api=api;this.publish=publish;this.remember=remember;this.confirmed=initial;this.state={mode:initial,loaded:false,loading:false,status:'saved',error:''}
 }
 private update(patch:Partial<AppearanceState>){this.state={...this.state,...patch};this.publish(this.state)}
 load():Promise<void>{
  if(this.state.loaded)return Promise.resolve()
  if(this.reading)return this.reading
  this.update({loading:true,error:''})
  this.reading=Promise.resolve().then(async()=>{
   try{const mode=await this.api.load();if(!validAppearance(mode))throw Error('invalid appearance');this.confirmed=mode;this.remember(mode);this.update({mode,loaded:true})}
   catch{this.update({error:'无法读取外观设置，请重试'})}
   finally{this.reading=null;this.update({loading:false})}
  });return this.reading
 }
 select(mode:unknown){
  if(!validAppearance(mode)||!this.state.loaded)return
  if(mode===this.state.mode && this.state.status!=='error')return
  this.pending=true;this.update({mode,error:'',status:'saving'});void this.flush()
 }
 retry(){if(!this.state.loaded){void this.load();return}this.pending=true;this.update({status:'saving',error:''});void this.flush()}
 private async flush(){
  if(this.writing)return
  this.writing=true
  try{while(this.pending){
   this.pending=false;const target=this.state.mode
   if(target!==this.confirmed){
    try{await this.api.save(target);this.confirmed=target;this.remember(target)}
    catch{this.confirmed=null;if(this.pending)continue;this.update({status:'error',error:'外观设置未保存，请重试'});return}
   }
   if(!this.pending)this.update({status:'saved',error:''})
  }}finally{this.writing=false}
 }
}
