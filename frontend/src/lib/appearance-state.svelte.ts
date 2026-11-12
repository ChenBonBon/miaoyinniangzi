import { AppearanceController,validAppearance,resolveAppearance,type Appearance,type AppearanceState } from './appearance-controller'

const initial=document.documentElement.dataset.appearance
export const appearanceState=$state<AppearanceState>({mode:validAppearance(initial)?initial:'dark',loaded:false,loading:false,status:'saved',error:''})
export const appearanceView=$state({resolved:resolveAppearance(appearanceState.mode,matchMedia('(prefers-color-scheme: dark)').matches)})
let nativeMode:Appearance|undefined
function apply(){
 const mode=appearanceState.mode,resolved=resolveAppearance(mode,matchMedia('(prefers-color-scheme: dark)').matches)
 appearanceView.resolved=resolved
 const root=document.documentElement
 root.dataset.appearance=mode;root.dataset.theme=resolved
 root.classList.toggle('dark',resolved==='dark');root.classList.toggle('light',resolved==='light');root.style.colorScheme=resolved
 // Windows uses Wails' runtime; macOS AppKit appearance is applied by the
 // backend on its main queue after preference load/save.
 const native=(window as Window&{runtime?:{WindowSetDarkTheme?:()=>void;WindowSetLightTheme?:()=>void;WindowSetSystemDefaultTheme?:()=>void}}).runtime
 if(mode!==nativeMode){
  const update=mode==='system'?native?.WindowSetSystemDefaultTheme:mode==='dark'?native?.WindowSetDarkTheme:native?.WindowSetLightTheme
  if(update)try{update.call(native);nativeMode=mode}catch{}
 }
}
type Bridge={GetAppearance():Promise<string>;SaveAppearance(mode:Appearance):Promise<void>}
function bridge(){const value=(window as Window&{go?:{main?:{App?:Partial<Bridge>}}}).go?.main?.App;if(!value?.GetAppearance||!value.SaveAppearance)throw Error('外观设置存储不可用');return value as Bridge}
export const appearanceController=new AppearanceController({load:()=>bridge().GetAppearance(),save:mode=>bridge().SaveAppearance(mode)},state=>{Object.assign(appearanceState,state);apply()},appearanceState.mode,mode=>{try{localStorage.setItem('miaoyin-appearance-v1',mode)}catch{}})
export function initializeAppearance(){
 const media=matchMedia('(prefers-color-scheme: dark)');const update=()=>apply()
 apply();media.addEventListener('change',update);void appearanceController.load()
 return ()=>media.removeEventListener('change',update)
}
