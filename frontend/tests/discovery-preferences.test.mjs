import { test } from 'node:test'
import assert from 'node:assert/strict'
import { DiscoveryPreferencesController, discoveryLayout, materializePreferences, validDiscoveryPreferences, reorder } from '../src/lib/discovery-preferences.ts'
const entry=(id,group,groupId)=>({id,group,groupId,title:`Chart${id}`,kind:'chart',coverUrl:'',period:'',updated:'',total:1})
const entries=[entry(1,'A',0),entry(2,'A',0),entry(3,'B',1)]
const saved={groups:[{key:'qq:1',hidden:true,charts:[{id:3,hidden:false}]},{key:'qq:0',hidden:false,charts:[{id:2,hidden:true},{id:1,hidden:false}]}]}
function fixture(api={}){let state;const writes=[];const controller=new DiscoveryPreferencesController({get:async()=>({groups:[]}),save:async value=>writes.push(value),...api},value=>state=value);return{controller,writes,get state(){return state}}}
const settle=()=>new Promise(resolve=>setTimeout(resolve,0))
test('layout controls group and chart order independently from visibility',()=>{
 const layout=discoveryLayout(entries,saved)
 assert.deepEqual(layout.map(group=>[group.key,group.hidden]),[['qq:1',true],['qq:0',false]])
 assert.deepEqual(layout[1].charts.map(chart=>[chart.entry.id,chart.hidden]),[[2,true],[1,false]])
 assert.equal(saved.groups[0].hidden,true)
})
test('new charts and groups append, absent preferences survive and group renames keep identity',()=>{
 const next=[entry(1,'Renamed',0),entry(2,'Renamed',0),entry(4,'Renamed',0),entry(5,'C',2)]
 const layout=discoveryLayout(next,saved)
 assert.deepEqual(layout.map(group=>group.key),['qq:0','qq:2'])
 assert.deepEqual(layout[0].charts.map(chart=>chart.entry.id),[2,1,4])
 const merged=materializePreferences(next,saved)
 assert.equal(merged.groups[0].key,'qq:1');assert.equal(merged.groups[0].hidden,true)
 assert.deepEqual(merged.groups[1].charts.map(chart=>chart.id),[2,1,4])
 assert.equal(merged.groups.at(-1).key,'qq:2')
})
test('hiding a group preserves its child visibility and ordering',()=>{
 const copy=materializePreferences(entries,saved),group=copy.groups.find(group=>group.key==='qq:0')
 group.hidden=true;group.hidden=false
 assert.deepEqual(group.charts,[{id:2,hidden:true},{id:1,hidden:false}])
 assert.deepEqual(reorder([1,2,3],0,2),[2,3,1]);assert.deepEqual(reorder([1,2],0,-1),[1,2])
})
test('layout validates bounds and duplicate identities',()=>{
 assert.equal(validDiscoveryPreferences(saved),true)
 for(const value of [null,{groups:null},{groups:[{key:'qq:01',hidden:false,charts:[]}]},{groups:[{key:'qq:0',hidden:false,charts:[{id:1,hidden:false},{id:1,hidden:true}]}]}])assert.equal(validDiscoveryPreferences(value),false)
})
test('controller reads once and serializes rapid writes so older responses cannot win',async()=>{
 let release,reads=0
 const f=fixture({get:async()=>{reads++;return{groups:[]}},save:async value=>{f.writes.push(value);if(f.writes.length===1)await new Promise(resolve=>release=resolve)}})
 await Promise.all([f.controller.load(),f.controller.load()]);assert.equal(reads,1)
 f.controller.change(saved);f.controller.change({groups:[]});assert.equal(f.writes.length,1);assert.equal(f.state.saving,true)
 release();await settle();assert.equal(f.writes.length,2);assert.deepEqual(f.writes[1],{groups:[]});assert.equal(f.state.saving,false)
 await f.controller.load();assert.equal(reads,1)
})
test('failed writes keep the draft, disclose unsaved state and retry the latest value',async()=>{
 let fail=true;const f=fixture({save:async()=>{if(fail)throw Error('private')}});await f.controller.load()
 f.controller.change(saved);await settle();assert.deepEqual(f.state.value,saved);assert.match(f.state.error,/尚未保存/)
 fail=false;await f.controller.retry();assert.equal(f.state.error,'');assert.equal(f.state.saving,false)
})
test('failed reads cannot overwrite stored preferences with defaults',async()=>{
 let fail=true;const f=fixture({get:async()=>{if(fail)throw Error('private');return saved}})
 await f.controller.load();assert.equal(f.state.status,'error');f.controller.change({groups:[]});assert.equal(f.writes.length,0)
 fail=false;await f.controller.retry();assert.deepEqual(f.state.value,saved)
})
