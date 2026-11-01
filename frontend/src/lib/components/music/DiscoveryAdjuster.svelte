<script lang="ts">
  import { ArrowUp, ArrowDown, GripVertical, Eye, EyeOff } from '@lucide/svelte'
  import { Button } from '$lib/components/ui/button'
  import CollectionCover from './CollectionCover.svelte'
  import { cn } from '$lib/utils'
  import * as Sheet from '$lib/components/ui/sheet'
  import * as Alert from '$lib/components/ui/alert'
  import * as AlertDialog from '$lib/components/ui/alert-dialog'
  import { discoveryPreferencesState as preferences, discoveryPreferencesController as controller } from '$lib/discovery-preferences-state.svelte'
  import { discoveryLayout, materializePreferences, reorder, defaultDiscoveryPreferences, type DiscoveryPreferences } from '$lib/discovery-preferences'
  import type { DiscoveryEntry } from '$lib/discovery-controller'
  let { entries, open = $bindable(false) }: { entries: DiscoveryEntry[]; open?: boolean } = $props()
  let resetting = $state(false)
  let drag = $state<{group:string;id?:number}|null>(null)
  let announcement = $state('')
  const groups = $derived(discoveryLayout(entries, preferences.value))
  function edit(change:(value:DiscoveryPreferences)=>void){const next=materializePreferences(entries,preferences.value);change(next);controller.change(next)}
  function groupVisibility(key:string,visible:boolean){edit(value=>{value.groups.find(group=>group.key===key)!.hidden=!visible})}
  function chartVisibility(key:string,id:number,visible:boolean){edit(value=>{value.groups.find(group=>group.key===key)!.charts.find(chart=>chart.id===id)!.hidden=!visible})}
  function moveGroup(key:string,target:string){
    if(key===target)return
    const title=groups.find(group=>group.key===key)?.title,position=groups.findIndex(group=>group.key===target)+1
    edit(value=>{value.groups=reorder(value.groups,value.groups.findIndex(group=>group.key===key),value.groups.findIndex(group=>group.key===target))})
    announcement=`${title} 已移至第 ${position} 个分组`
  }
  function moveChart(key:string,id:number,target:number){
    if(id===target)return
    const group=groups.find(group=>group.key===key)!
    const title=group.charts.find(chart=>chart.entry.id===id)?.entry.title,position=group.charts.findIndex(chart=>chart.entry.id===target)+1
    edit(value=>{const group=value.groups.find(group=>group.key===key)!;group.charts=reorder(group.charts,group.charts.findIndex(chart=>chart.id===id),group.charts.findIndex(chart=>chart.id===target))})
    announcement=`${title} 已移至组内第 ${position} 位`
  }
  function start(event:DragEvent,group:string,id?:number){drag={group,id};if(event.dataTransfer){event.dataTransfer.effectAllowed='move';event.dataTransfer.setData('text/plain','miaoyin-discovery-layout')}}
  function allow(event:DragEvent,group:string,chart=false){if(drag&&(chart?drag.id!==undefined&&drag.group===group:drag.id===undefined))event.preventDefault()}
  function drop(event:DragEvent,group:string,id?:number){
    if(!drag)return
    if(id!==undefined){if(drag.id===undefined||drag.group!==group)return;event.preventDefault();event.stopPropagation();moveChart(group,drag.id,id)}
    else {if(drag.id!==undefined)return;event.preventDefault();moveGroup(drag.group,group)}
    drag=null
  }
</script>

<Sheet.Root bind:open onOpenChange={() => drag=null}>
  <Sheet.Content class="gap-0 data-[side=right]:w-full data-[side=right]:sm:max-w-lg" onOpenAutoFocus={event => { event.preventDefault(); document.getElementById('discovery-adjust-title')?.focus({preventScroll:true}) }} onCloseAutoFocus={event => { event.preventDefault(); document.getElementById('discovery-adjust')?.focus({preventScroll:true}) }}>
    <Sheet.Header class="pr-12"><Sheet.Title id="discovery-adjust-title" tabindex={-1}>调整推荐</Sheet.Title><Sheet.Description class="sr-only">拖动封面或分组手柄排序，也可聚焦后使用上下箭头。眼睛按钮切换显示与隐藏。</Sheet.Description></Sheet.Header>
    <div class="min-h-0 flex-1 overflow-y-auto px-4 pb-4">
      {#if preferences.error}<Alert.Root variant="destructive" class="mb-4"><Alert.Title>{preferences.error}</Alert.Title><Button variant="outline" size="sm" class="mt-2" onclick={() => controller.retry()}>重试保存布局</Button></Alert.Root>{/if}
      <div class="flex flex-col gap-8">
        {#each groups as group, index (group.key)}
          <section role="group" aria-label={`调整分组 ${group.title}`} data-discovery-adjust-group={group.key} ondragover={event=>allow(event,group.key)} ondrop={event=>drop(event,group.key)}>
            <div class="editing-row mb-2 flex min-w-0 items-center gap-1 py-2">
              <h3 class={cn('min-w-0 flex-1 truncate font-semibold',group.hidden && 'text-muted-foreground')}>{group.title}</h3>
              <div class="sort-controls flex items-center gap-1">
                <Button variant="ghost" size="icon-sm" draggable="true" aria-label={`拖动分组 ${group.title}`} title="拖动分组排序" ondragstart={event=>start(event,group.key)} ondragend={()=>drag=null}><GripVertical /></Button>
                <Button variant="ghost" size="icon-sm" aria-label={`上移分组 ${group.title}`} title="上移分组" disabled={index===0} onclick={()=>moveGroup(group.key,groups[index-1].key)}><ArrowUp /></Button><Button variant="ghost" size="icon-sm" aria-label={`下移分组 ${group.title}`} title="下移分组" disabled={index===groups.length-1} onclick={()=>moveGroup(group.key,groups[index+1].key)}><ArrowDown /></Button>
              </div>
              <Button variant="ghost" size="icon-sm" aria-label={`显示分组 ${group.title}`} aria-pressed={!group.hidden} title={group.hidden ? '显示分组' : '隐藏分组'} onclick={()=>groupVisibility(group.key,group.hidden)}>{#if group.hidden}<EyeOff />{:else}<Eye />{/if}</Button>
            </div>
            <div class="flex flex-col gap-1">
              {#each group.charts as chart, chartIndex (chart.entry.id)}
                <div class="editing-row flex min-w-0 items-center gap-3 py-2" role="group" aria-label={`调整榜单 ${chart.entry.title}`} data-discovery-adjust-chart={chart.entry.id} ondragover={event=>allow(event,group.key,true)} ondrop={event=>drop(event,group.key,chart.entry.id)}>
                  <Button variant="ghost" class={cn('h-auto shrink-0 cursor-grab p-0 active:cursor-grabbing', (group.hidden || chart.hidden) && 'opacity-50 grayscale')} draggable="true" aria-label={`拖动榜单 ${chart.entry.title}`} title="拖动封面排序" ondragstart={event=>start(event,group.key,chart.entry.id)} ondragend={()=>drag=null}><CollectionCover src={chart.entry.coverUrl} compact /></Button>
                  <span class={cn('min-w-0 flex-1 truncate text-sm', (group.hidden || chart.hidden) && 'text-muted-foreground')} title={chart.entry.title}>{chart.entry.title}</span>
                  <div class="sort-controls flex items-center gap-1">
                    <Button variant="ghost" size="icon-sm" aria-label={`上移榜单 ${chart.entry.title}`} title="上移榜单" disabled={chartIndex===0} onclick={()=>moveChart(group.key,chart.entry.id,group.charts[chartIndex-1].entry.id)}><ArrowUp /></Button><Button variant="ghost" size="icon-sm" aria-label={`下移榜单 ${chart.entry.title}`} title="下移榜单" disabled={chartIndex===group.charts.length-1} onclick={()=>moveChart(group.key,chart.entry.id,group.charts[chartIndex+1].entry.id)}><ArrowDown /></Button>
                  </div>
                  <Button variant="ghost" size="icon-sm" aria-label={`显示榜单 ${chart.entry.title}`} aria-pressed={!chart.hidden} title={chart.hidden ? '显示榜单' : '隐藏榜单'} onclick={()=>chartVisibility(group.key,chart.entry.id,chart.hidden)}>{#if chart.hidden}<EyeOff />{:else}<Eye />{/if}</Button>
                </div>
              {/each}
            </div>
          </section>
        {/each}
      </div>
    </div>
    <Sheet.Footer class="flex-row items-center justify-between"><Button variant="ghost" size="sm" onclick={()=>resetting=true}>恢复默认</Button>{#if preferences.saving || preferences.error}<span role="status" class="text-xs text-muted-foreground">{preferences.saving ? '正在保存…' : '尚未保存'}</span>{/if}</Sheet.Footer>
    <span class="sr-only" aria-live="polite">{announcement}</span>
  </Sheet.Content>
</Sheet.Root>
<AlertDialog.Root bind:open={resetting}><AlertDialog.Content><AlertDialog.Header><AlertDialog.Title>恢复默认推荐布局？</AlertDialog.Title><AlertDialog.Description>将恢复 QQ 的分组顺序，重新显示所有榜单。</AlertDialog.Description></AlertDialog.Header><AlertDialog.Footer><AlertDialog.Cancel>取消</AlertDialog.Cancel><AlertDialog.Action onclick={()=>controller.change(defaultDiscoveryPreferences())}>恢复默认</AlertDialog.Action></AlertDialog.Footer></AlertDialog.Content></AlertDialog.Root>

<style>
  .sort-controls { opacity: 0; transition: opacity 120ms ease-out; }
  .editing-row:hover > .sort-controls, .editing-row:focus-within > .sort-controls { opacity: 1; }
  @media (hover: none) { .sort-controls { opacity: 1; } }
  @media (prefers-reduced-motion: reduce) { .sort-controls { transition: none; } }
</style>
