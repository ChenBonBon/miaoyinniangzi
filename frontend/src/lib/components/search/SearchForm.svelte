<script lang="ts">
  import { onDestroy, tick } from 'svelte'
  import { Search, LoaderCircle, CornerDownLeft } from '@lucide/svelte'
  import { Button } from '$lib/components/ui/button'
  import { Input } from '$lib/components/ui/input'
  import * as Field from '$lib/components/ui/field'
  import * as Popover from '$lib/components/ui/popover'
  import * as Command from '$lib/components/ui/command'
  import KeywordMatch from './KeywordMatch.svelte'
  import { searchForm, searchState, searchController } from '$lib/search-state.svelte'
  import { historyController as searchHistoryController, historyState as searchHistoryState } from '$lib/search-history-state.svelte'
  import { normalizeSearchQuery } from '$lib/search-controller'
  import { SuggestionsController, suggestionsAPI, initialSuggestionsState } from '$lib/search-suggestions'
  import { navigate } from '$lib/navigation.svelte'

  let formElement: HTMLFormElement
  let inputElement = $state<HTMLInputElement|null>(null)
  let contentElement = $state<HTMLDivElement|null>(null)
  let open = $state(false)
  let composing = $state(false)
  let imeSubmit = false
  let active = $state('')
  let suggestions = $state(initialSuggestionsState())
  const suggestionController = new SuggestionsController(suggestionsAPI, state => suggestions=state)
  const query = $derived(searchForm.query.trim())
  const histories = $derived(searchHistoryState.items.filter(item => item !== query && item.toLowerCase().includes(query.toLowerCase())).slice(0,5))
  const songs = $derived(suggestions.query===query ? suggestions.items : [])
  const choices = $derived([
    ...(query ? [{id:'keyword',query,label:`搜索「${query}」`,kind:'keyword',artist:''}] : []),
    ...histories.map(term => ({id:`history:${term}`,query:term,label:term,kind:'history',artist:''})),
    ...songs.map(song => ({id:`song:${song.id}`,query:song.title,label:song.title,kind:'song',artist:song.artist})),
  ])
  const activeIndex = $derived(choices.findIndex(choice => choice.id===active))

  $effect(() => {
    if(open && !composing) suggestionController.schedule(searchForm.query)
    else suggestionController.close()
  })
  onDestroy(() => suggestionController.close())

  function showSuggestions(){open=true;void searchHistoryController.load()}
  function submit(input: string){
    const normalized=normalizeSearchQuery(input)
    if(normalized===null){void searchController.search(input);return}
    open=false
    suggestionController.close()
    navigate('搜索', () => { searchForm.query=normalized; void searchController.search(normalized) })
    const content=formElement.closest('main')?.querySelector<HTMLElement>('[data-content-scroll]')
    content?.scrollTo({top:0,behavior:'instant'})
  }
  function chooseActive(){submit(open && activeIndex>=0 ? choices[activeIndex]!.query : searchForm.query)}
  function onKeydown(event: KeyboardEvent){
    if(event.isComposing || composing || event.keyCode===229){
      // Preserve the IME's native confirmation, but ignore any implicit form
      // submission it produces in this event turn (including Safari keyCode 229).
      if(event.key==='Enter'){imeSubmit=true;setTimeout(()=>imeSubmit=false,0)}
      return
    }
    imeSubmit=false
    if(event.key==='Escape'){
      if(open){event.preventDefault();event.stopPropagation();open=false}
      return
    }
    if(event.key==='Enter'){event.preventDefault();chooseActive();return}
    if(event.key==='ArrowDown' || event.key==='ArrowUp'){
      event.preventDefault()
      showSuggestions()
      if(!choices.length)return
      const index=activeIndex<0 ? (event.key==='ArrowDown'?0:choices.length-1) : (activeIndex+(event.key==='ArrowDown'?1:-1)+choices.length)%choices.length
      active=choices[index]!.id
      void tick().then(() => document.getElementById(`search-suggestion-${index}`)?.scrollIntoView({block:'nearest'}))
    }
  }
</script>

<form bind:this={formElement} class="min-w-0 flex-1 max-w-3xl" role="search" aria-label="QQ 音乐歌曲搜索" onsubmit={(event)=>{event.preventDefault();if(!composing && !imeSubmit)submit(searchForm.query)}}>
  <Popover.Root bind:open>
    <Field.Group>
      <Field.Field data-invalid={!!searchState.fieldError}>
        <Field.Label for="search" class="sr-only">歌曲、歌手或专辑</Field.Label>
        <div class="flex items-center gap-3">
          <Input id="search" bind:ref={inputElement} bind:value={searchForm.query} role="combobox" aria-autocomplete="list" aria-haspopup="listbox" aria-expanded={open} aria-controls={open?'search-suggestions':undefined} aria-activedescendant={open && activeIndex>=0 ? `search-suggestion-${activeIndex}`:undefined}
            onfocus={showSuggestions} onclick={showSuggestions} onkeydown={onKeydown}
            oninput={()=>{searchController.clearFieldError();open=true;active='keyword'}}
            onblur={(event)=>{if(!(event.relatedTarget instanceof Node) || !contentElement?.contains(event.relatedTarget))open=false}}
            oncompositionstart={()=>{composing=true}} oncompositionend={()=>{composing=false;open=true}}
            placeholder="搜索歌曲、歌手或专辑" maxlength={100} autocomplete="off" aria-invalid={!!searchState.fieldError} aria-describedby={searchState.fieldError?'search-field-error':undefined} class="h-11 min-w-0 flex-1" />
          <Button type="submit" size="icon" class="size-11" aria-label="搜索" disabled={searchState.status==='loading' && query===searchState.query}>
            {#if searchState.status==='loading'}<LoaderCircle data-icon="inline-start" class="animate-spin" />{:else}<Search data-icon="inline-start" />{/if}
          </Button>
          {#if searchState.status==='loading'}<Button variant="ghost" onclick={()=>searchController.cancel()}>取消</Button>{/if}
        </div>
        {#if searchState.fieldError}<Field.Error id="search-field-error">{searchState.fieldError}</Field.Error>{/if}
      </Field.Field>
    </Field.Group>
    <Popover.Content bind:ref={contentElement} customAnchor={inputElement} align="start" sideOffset={6} trapFocus={false} onOpenAutoFocus={event=>event.preventDefault()} onCloseAutoFocus={event=>event.preventDefault()} class="w-[var(--bits-popover-anchor-width)] p-0">
      <Command.Root bind:value={active} shouldFilter={false} label="搜索提示" class="h-auto">
        <Command.List id="search-suggestions" aria-label="搜索提示" class="max-h-80">
          {#each ['keyword','history','song'] as kind}
            {#if choices.some(choice=>choice.kind===kind)}
              <Command.Group heading={kind==='history'?'历史搜索':kind==='song'?'歌曲':undefined}>
                {#each choices as choice,index (choice.id)}
                  {#if choice.kind===kind}
                    <Command.Item id={`search-suggestion-${index}`} value={choice.id} onSelect={()=>submit(choice.query)} onpointerdown={event=>event.preventDefault()}>
                      {#if kind==='keyword'}<Search />{/if}
                      <span class="min-w-0 flex-1 truncate"><KeywordMatch text={choice.label} {query} />{#if choice.artist}<span class="text-muted-foreground">{' · '}{choice.artist}</span>{/if}</span>
                      {#if kind==='keyword'}<Command.Shortcut><CornerDownLeft /></Command.Shortcut>{/if}
                    </Command.Item>
                  {/if}
                {/each}
              </Command.Group>
            {/if}
          {/each}
          {#if !choices.length}<Command.Empty>输入关键词开始搜索</Command.Empty>{/if}
          {#if query && suggestions.query===query && suggestions.status==='loading'}
            <div class="text-muted-foreground flex items-center gap-2 px-3 py-2 text-xs" role="status"><LoaderCircle class="size-3 animate-spin" />正在获取歌曲建议</div>
          {:else if query && suggestions.query===query && suggestions.status==='error'}
            <p class="text-muted-foreground px-3 py-2 text-xs" role="status">暂时无法获取歌曲建议，仍可回车搜索</p>
          {/if}
        </Command.List>
      </Command.Root>
    </Popover.Content>
  </Popover.Root>
</form>
