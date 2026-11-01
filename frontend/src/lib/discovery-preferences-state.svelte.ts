import { DiscoveryPreferencesController, initialPreferencesState, type PreferencesAPI } from './discovery-preferences'
function bridge(){
 const api=(window as Window & {go?:{main?:{App?:{GetDiscoveryPreferences:PreferencesAPI['get'];SaveDiscoveryPreferences:PreferencesAPI['save']}}}}).go?.main?.App
 if(!api?.GetDiscoveryPreferences||!api?.SaveDiscoveryPreferences)throw Error('推荐布局存储不可用')
 return api
}
export const discoveryPreferencesState=$state(initialPreferencesState())
export const discoveryPreferencesController=new DiscoveryPreferencesController({get:async()=>bridge().GetDiscoveryPreferences(),save:async value=>bridge().SaveDiscoveryPreferences(value)},state=>Object.assign(discoveryPreferencesState,state))
