import { test, expect, type Page } from '@playwright/test'

async function openSearch(page: Page) {
  await page.addInitScript(() => {
    const state = { resolutionCalls:[] as {song:{id:string};id:string}[], resolutionCanceled:[] as string[], resolutionMode:'normal', resolutionPending:[] as {finish:()=>void}[], suggestionCalls:[] as {query:string;id:string}[], suggestionCanceled:[] as string[], suggestionMode:'normal', suggestionPending:[] as {id:string;finish:()=>void}[], calls:[] as {query:string;page:number;id:string}[], canceled:[] as string[], mode:'normal', unsafeTitle:'', history:JSON.parse(localStorage.getItem('test-search-history') || '[]') as string[], pending:[] as {id:string;finish:()=>void}[] }
    function result(query:string,page:number) {
      const song = { id:`Song${page}`, source:'tx', songId:'97773', mediaMid:'Media1', title:state.unsafeTitle || (page === 1 ? '晴天' : '稻香'), subtitle:'录音室版', artist:'周杰伦', artists:[{id:'Singer1',name:'周杰伦'}], album:'叶惠美', albumMid:'Album1', coverUrl:'', duration:269, availableQualities:['flac','320k','128k'] }
      return { query, page, pageSize:20, total:state.mode === 'empty' ? 0 : 21, hasMore:state.mode !== 'empty' && page === 1, songs:state.mode === 'empty' ? [] : [song] }
    }
    Object.assign(window, { searchTest:state, go:{main:{App:{
      GetSources:async () => [],
      ResolveSong:async (song:{id:string},id:string)=>{
        state.resolutionCalls.push({song,id})
        const result={songId:song.id,sourceId:'source-1',requestedQuality:'flac',quality:state.resolutionMode==='downgrade'?'320k':'flac',downgraded:state.resolutionMode==='downgrade',url:'http://cdn.example/media?private=signed'}
        if(state.resolutionMode==='error')throw Error('请先在设置中启用一个已校验的音源')
        if(state.resolutionMode==='pending')return new Promise(resolve=>state.resolutionPending.push({finish:()=>resolve(result)}))
        return result
      },
      CancelSongResolution:async(id:string)=>{state.resolutionCanceled.push(id);return true},
      SuggestSongs:async (query:string,id:string) => {
        state.suggestionCalls.push({query,id})
        const items=[{id:'Suggest1',source:'tx',title:query+'一生跟你走',artist:'张学友'},{id:'Suggest2',source:'tx',title:query+'说声爱你',artist:'白小白/朱鸽'}]
        if(state.suggestionMode==='error')throw Error('suggestion unavailable')
        if(state.suggestionMode==='pending')return new Promise(resolve=>state.suggestionPending.push({id,finish:()=>resolve(items)}))
        return items
      },
      CancelSongSuggestions:async (id:string)=>{state.suggestionCanceled.push(id);return true},
      GetSearchHistory:async () => [...state.history],
      RecordSearchHistory:async (query:string) => {
        state.history = [query,...state.history.filter(item => item !== query)].slice(0,20)
        localStorage.setItem('test-search-history',JSON.stringify(state.history))
        return [...state.history]
      },
      RemoveSearchHistory:async (query:string) => {
        state.history = state.history.filter(item => item !== query)
        localStorage.setItem('test-search-history',JSON.stringify(state.history))
        return [...state.history]
      },
      ClearSearchHistory:async () => {
        state.history = []
        localStorage.setItem('test-search-history','[]')
        return []
      },
      GetDiscoveryPreferences: async () => ({groups:[]}),
      SaveDiscoveryPreferences: async () => {},
      SearchSongs:async (query:string,page:number,id:string) => {
        state.calls.push({query,page,id})
        if (state.mode === 'error') throw new Error('QQ 音乐暂时拒绝搜索请求，请稍后重试')
        if (state.mode === 'pending') return new Promise(resolve => state.pending.push({id,finish:()=>resolve(result(query,page))}))
        return result(query,page)
      },
      CancelSongSearch:async (id:string) => { state.canceled.push(id); return true },
    }}} })
  })
  await page.goto('/')
  await page.getByLabel('歌曲、歌手或专辑').focus()
}
const form = (page: Page) => page.getByRole('search', { name:'QQ 音乐歌曲搜索' })
const submit = async (page: Page, query: string) => {
  await page.getByLabel('歌曲、歌手或专辑').fill(query)
  await form(page).getByRole('button', { name:'搜索', exact:true }).click()
}

test('focus and typing keep the current page; hovering a song and Enter submits only that suggestion', async ({page})=>{
  await openSearch(page)
  const input=page.getByRole('combobox')
  await expect(page.getByRole('tablist',{name:'推荐类型'})).toHaveCount(0)
  await input.fill('只想')
  await expect(page.getByRole('heading',{name:'把时间交给音乐。'})).toBeVisible()
  const option=page.getByRole('option',{name:'只想说声爱你 · 白小白/朱鸽',exact:true})
  await expect(option).toBeVisible()
  expect(await page.evaluate(()=>Reflect.get(window,'searchTest').calls.length)).toBe(0)
  expect(await page.evaluate(()=>Reflect.get(window,'searchTest').history)).toEqual([])
  await expect(input).toBeFocused()
  await page.screenshot({path:'test-results/search-suggestions.png'})
  await option.hover()
  await expect(option).toHaveAttribute('aria-selected','true')
  await expect(input).toHaveAttribute('aria-activedescendant',(await option.getAttribute('id'))!)
  const contrast=await option.locator('mark').evaluate(mark=>{
    const canvas=document.createElement('canvas');canvas.width=canvas.height=1
    const context=canvas.getContext('2d')!
    function luminance(color:string){
      context.clearRect(0,0,1,1);context.fillStyle=color;context.fillRect(0,0,1,1)
      const rgb=[...context.getImageData(0,0,1,1).data].slice(0,3).map(value=>{const n=value/255;return n<=.04045?n/12.92:((n+.055)/1.055)**2.4})
      return rgb[0]!*.2126+rgb[1]!*.7152+rgb[2]!*.0722
    }
    return (luminance(getComputedStyle(mark).color)+.05)/(luminance(getComputedStyle(mark.closest('[role=option]')!).backgroundColor)+.05)
  })
  expect(contrast).toBeGreaterThanOrEqual(4.5)
  await input.press('Enter')
  await expect(page.getByRole('heading',{name:'「只想说声爱你」的搜索结果',exact:true})).toBeVisible()
  await expect(input).toHaveValue('只想说声爱你')
  await expect(page.getByRole('listbox',{name:'搜索提示'})).toHaveCount(0)
  expect(await page.evaluate(()=>Reflect.get(window,'searchTest').calls.map((call:{query:string})=>call.query))).toEqual(['只想说声爱你'])
})

test('clicking a suggestion and keyboard selection both navigate explicitly',async({page})=>{
  await openSearch(page)
  const input=page.getByRole('combobox')
  await input.fill('只想')
  await page.getByRole('option',{name:'只想一生跟你走 · 张学友',exact:true}).click()
  await expect(page.getByRole('heading',{name:'「只想一生跟你走」的搜索结果',exact:true})).toBeVisible()
  await page.getByRole('navigation',{name:'主导航'}).getByRole('button',{name:'推荐',exact:true}).click()
  await input.fill('爱')
  await expect(page.getByRole('option',{name:'爱一生跟你走 · 张学友',exact:true})).toBeVisible()
  await input.press('ArrowDown')
  await expect(page.getByRole('option',{name:'爱一生跟你走 · 张学友',exact:true})).toHaveAttribute('aria-selected','true')
  await input.press('ArrowUp')
  await expect(page.getByRole('option',{name:'搜索「爱」',exact:true})).toHaveAttribute('aria-selected','true')
  await input.press('ArrowDown')
  await input.press('Enter')
  await expect(page.getByRole('heading',{name:'「爱一生跟你走」的搜索结果',exact:true})).toBeVisible()
})

test('Escape, outside click, and Tab close suggestions without navigation or losing draft',async({page,browserName})=>{
  await openSearch(page)
  const input=page.getByRole('combobox')
  const list=page.getByRole('listbox',{name:'搜索提示'})
  await input.fill('只想');await expect(list).toBeVisible()
  await input.press('Escape');await expect(list).toHaveCount(0)
  await expect(input).toBeFocused();await expect(input).toHaveValue('只想')
  await input.click();await expect(list).toBeVisible()
  await page.getByRole('heading',{name:'把时间交给音乐。'}).click();await expect(list).toHaveCount(0)
  await input.click();await expect(list).toBeVisible()
  await input.press('Tab');await expect(list).toHaveCount(0)
  await expect(input).toHaveValue('只想');await expect(input).not.toBeFocused()
  const searchButton=form(page).getByRole('button',{name:'搜索',exact:true})
  if(browserName==='webkit' && process.platform==='darwin' && !await searchButton.evaluate(element=>element===document.activeElement)){
    // macOS WebKit's default Tab cycle skips buttons. Option-Tab includes
    // all controls; respect full keyboard access when already enabled.
    await input.focus();await expect(list).toBeVisible()
    await input.press('Alt+Tab');await expect(list).toHaveCount(0)
  }
  await expect(searchButton).toBeFocused()
  await expect(page.getByRole('heading',{name:'把时间交给音乐。'})).toBeVisible()
  expect(await page.evaluate(()=>Reflect.get(window,'searchTest').calls.length)).toBe(0)
})

test('IME confirmation does not submit and suggestion failure never blocks explicit search',async({page})=>{
  await openSearch(page)
  const input=page.getByRole('combobox')
  await page.evaluate(()=>{Reflect.get(window,'searchTest').suggestionMode='error'})
  await input.dispatchEvent('compositionstart')
  await input.fill('只想')
  await page.waitForTimeout(350)
  expect(await page.evaluate(()=>Reflect.get(window,'searchTest').suggestionCalls.length)).toBe(0)
  expect(await page.evaluate(()=>Reflect.get(window,'searchTest').calls.length)).toBe(0)
  await input.evaluate(element=>{
    element.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',keyCode:229,isComposing:true,bubbles:true}))
    element.dispatchEvent(new CompositionEvent('compositionend',{bubbles:true}))
    element.closest('form')!.requestSubmit()
  })
  expect(await page.evaluate(()=>Reflect.get(window,'searchTest').calls.length)).toBe(0)
  await expect(page.getByText('暂时无法获取歌曲建议，仍可回车搜索',{exact:true})).toBeVisible()
  await expect(page.getByRole('heading',{name:'把时间交给音乐。'})).toBeVisible()
  await input.press('Enter')
  await expect(page.getByRole('heading',{name:'「只想」的搜索结果',exact:true})).toBeVisible()
})

test('closing typeahead does not cancel an already submitted search',async({page})=>{
  await openSearch(page)
  await page.evaluate(()=>{const state=Reflect.get(window,'searchTest');state.mode='pending';state.suggestionMode='pending'})
  await submit(page,'周杰伦')
  const input=page.getByRole('combobox')
  await input.fill('只想')
  await expect.poll(()=>page.evaluate(()=>Reflect.get(window,'searchTest').suggestionCalls.length)).toBe(1)
  await input.press('Escape')
  await expect.poll(()=>page.evaluate(()=>Reflect.get(window,'searchTest').suggestionCanceled.length)).toBe(1)
  expect(await page.evaluate(()=>Reflect.get(window,'searchTest').canceled)).toEqual([])
  await page.evaluate(()=>Reflect.get(window,'searchTest').pending[0].finish())
  await expect(page.getByRole('heading',{name:'「周杰伦」的搜索结果',exact:true})).toBeVisible()
  await expect(input).toHaveValue('只想')
  await page.evaluate(()=>Reflect.get(window,'searchTest').suggestionPending[0].finish())
  await expect(page.getByRole('listbox',{name:'搜索提示'})).toHaveCount(0)
})

test('typeahead stays anchored and inside narrow viewports',async({page})=>{
  await page.setViewportSize({width:600,height:700})
  await openSearch(page)
  await page.getByRole('combobox').fill('只想')
  const list=page.getByRole('listbox',{name:'搜索提示'})
  await expect(list.getByRole('option')).toHaveCount(3)
  const inputBox=await page.getByRole('combobox').boundingBox(),listBox=await list.boundingBox()
  expect(listBox!.y).toBeGreaterThan(inputBox!.y+inputBox!.height)
  expect(listBox!.x).toBeGreaterThanOrEqual(0)
  expect(listBox!.x+listBox!.width).toBeLessThanOrEqual(600)
  await page.screenshot({path:'test-results/search-suggestions-compact.png'})
})

test('explicit resolution reports quality and downgrade without fetching or playing media',async({page})=>{
  const mediaRequests:string[]=[];page.on('request',request=>{if(request.url().includes('cdn.example'))mediaRequests.push(request.url())})
  await openSearch(page);await submit(page,'周杰伦')
  await page.getByRole('button',{name:'解析 晴天',exact:true}).click()
  await expect(page.getByText('解析成功 · FLAC',{exact:true})).toBeVisible()
  await expect(page.getByRole('button',{name:'播放',exact:true})).toBeDisabled()
  await page.evaluate(()=>{Reflect.get(window,'searchTest').resolutionMode='downgrade'})
  await page.getByRole('button',{name:'解析 晴天',exact:true}).click()
  await expect(page.getByText('解析成功，已降至 320K',{exact:true})).toBeVisible()
  expect(mediaRequests).toEqual([])
  await expect(page.getByText('http://cdn.example/media?private=signed',{exact:true})).toHaveCount(0)
})

test('resolution can be canceled and missing-source errors do not affect search',async({page})=>{
  await openSearch(page);await submit(page,'周杰伦')
  await page.evaluate(()=>{Reflect.get(window,'searchTest').resolutionMode='pending'})
  await page.getByRole('button',{name:'解析 晴天',exact:true}).click()
  await page.getByRole('button',{name:'取消解析 晴天',exact:true}).click()
  await page.evaluate(()=>Reflect.get(window,'searchTest').resolutionPending[0].finish())
  await expect(page.getByText('解析成功 · FLAC',{exact:true})).toHaveCount(0)
  expect(await page.evaluate(()=>Reflect.get(window,'searchTest').resolutionCanceled.length)).toBe(1)
  await page.evaluate(()=>{Reflect.get(window,'searchTest').resolutionMode='error'})
  await page.getByRole('button',{name:'解析 晴天',exact:true}).click()
  await expect(page.getByText('请先在设置中启用一个已校验的音源',{exact:true})).toBeVisible()
  await expect(page.getByRole('table',{name:'QQ 音乐搜索结果'})).toBeVisible()
})

test('real search UI lists song metadata and paginates without an enabled source', async ({ page }) => {
  await openSearch(page)
  await submit(page, ' 周杰伦 ')
  const table = page.getByRole('table', { name:'QQ 音乐搜索结果' })
  await expect(table.getByText('晴天', { exact:true })).toBeVisible()
  await expect(table.getByRole('button',{name:'查看歌手 周杰伦',exact:true})).toBeVisible()
  await expect(table.getByRole('button',{name:'查看专辑 叶惠美',exact:true})).toBeVisible()
  await expect(table.getByText('4:29', { exact:true })).toBeVisible()
  await expect(page.getByText('约 21 首', { exact:true })).toBeVisible()
  await expect(page.getByRole('navigation', { name:'主导航' }).getByRole('button', { name:'搜索', exact:true })).toHaveCount(0)
  await expect(page.getByRole('heading', { name:'找到想听的那一首。', exact:true })).toHaveCount(0)
  await expect(page.getByRole('button', { name:'未启用音源', exact:true })).toHaveCount(0)
  await expect(page.getByRole('button', { name:'上一页', exact:true })).toBeDisabled()
  await page.getByRole('button', { name:'下一页', exact:true }).click()
  await expect(table.getByText('稻香', { exact:true })).toBeVisible()
  await expect(page.getByText('第 2 页', { exact:true })).toBeVisible()
  await expect(page.getByRole('button', { name:'下一页', exact:true })).toBeDisabled()
  expect(await page.evaluate(() => Reflect.get(window,'searchTest').calls.map((call:{query:string;page:number}) => [call.query,call.page]))).toEqual([['周杰伦',1],['周杰伦',2]])
  await page.screenshot({ path:'test-results/search-results.png' })
})

test('empty input stays local, empty results differ from upstream errors, and retry works', async ({ page }) => {
  await openSearch(page)
  await form(page).getByRole('button', { name:'搜索', exact:true }).click()
  await expect(page.getByText('请输入 1–100 个字符的歌曲、歌手或专辑名称', { exact:true })).toBeVisible()
  expect(await page.evaluate(() => Reflect.get(window,'searchTest').calls.length)).toBe(0)
  await page.evaluate(() => { Reflect.get(window,'searchTest').mode = 'empty' })
  await submit(page, '不存在的歌')
  await expect(page.getByText('没有找到相关歌曲', { exact:true })).toBeVisible()
  await page.evaluate(() => { Reflect.get(window,'searchTest').mode = 'error' })
  await submit(page, '周杰伦')
  await expect(page.getByText('QQ 音乐暂时拒绝搜索请求，请稍后重试', { exact:true })).toBeVisible()
  await expect(page.getByText('没有找到相关歌曲', { exact:true })).toHaveCount(0)
  await page.evaluate(() => { Reflect.get(window,'searchTest').mode = 'normal' })
  await page.getByRole('button', { name:'重试搜索', exact:true }).click()
  await expect(page.getByRole('table').getByText('晴天', { exact:true })).toBeVisible()
})

test('new submissions supersede pending searches and reject late results', async ({ page }) => {
  await openSearch(page)
  await page.evaluate(() => { Reflect.get(window,'searchTest').mode = 'pending' })
  await submit(page, '旧歌')
  await expect(page.getByRole('status', { name:'正在搜索 QQ 音乐' })).toBeVisible()
  await submit(page, '新歌')
  await page.evaluate(() => Reflect.get(window,'searchTest').pending[1].finish())
  await expect(page.getByRole('heading', { name:'「新歌」的搜索结果' })).toBeVisible()
  await page.evaluate(() => Reflect.get(window,'searchTest').pending[0].finish())
  await expect(page.getByRole('heading', { name:'「新歌」的搜索结果' })).toBeVisible()
  await expect(page.getByRole('heading', { name:'「旧歌」的搜索结果' })).toHaveCount(0)
  expect(await page.evaluate(() => Reflect.get(window,'searchTest').canceled.length)).toBe(1)
})

test('cancellation stops the loading state even if transport completion is late', async ({ page }) => {
  await openSearch(page)
  await page.evaluate(() => { Reflect.get(window,'searchTest').mode = 'pending' })
  await submit(page, '周杰伦')
  await form(page).getByRole('button', { name:'取消', exact:true }).click()
  await expect(page.getByText('搜索已取消', { exact:true })).toBeVisible()
  await page.evaluate(() => Reflect.get(window,'searchTest').pending[0].finish())
  await expect(page.getByRole('table')).toHaveCount(0)
  await expect(page.getByText('搜索已取消', { exact:true })).toBeVisible()
})

test('focus preserves the current page and an explicit resubmission refreshes results', async ({ page }) => {
  await openSearch(page)
  await submit(page, '周杰伦')
  await expect(page.getByRole('table')).toBeVisible()
  const navigation = page.getByRole('navigation', { name:'主导航' })
  await navigation.getByRole('button', { name:'推荐', exact:true }).click()
  await page.getByLabel('歌曲、歌手或专辑').focus()
  await expect(page.getByLabel('歌曲、歌手或专辑')).toHaveValue('周杰伦')
  await expect(page.getByRole('heading', { name:'把时间交给音乐。' })).toBeVisible()
  expect(await page.evaluate(() => Reflect.get(window,'searchTest').calls.length)).toBe(1)
  await page.getByLabel('歌曲、歌手或专辑').press('Enter')
  await expect(page.getByRole('table').getByText('晴天', { exact:true })).toBeVisible()
  expect(await page.evaluate(() => Reflect.get(window,'searchTest').calls.length)).toBe(2)
})

test('history tags are unique, persisted, clickable and individually removable', async ({ page }) => {
  await openSearch(page)
  await submit(page,'周杰伦')
  const history = page.getByRole('region', { name:'历史搜索', exact:true })
  await expect(history.getByRole('button', { name:'搜索历史 周杰伦', exact:true })).toBeVisible()
  await submit(page,' 林俊杰 ')
  await submit(page,'周杰伦')
  await expect(history.getByRole('button', { name:/^搜索历史 / })).toHaveCount(2)
  await page.reload()
  await page.getByLabel('歌曲、歌手或专辑').focus()
  await expect(page.getByRole('heading', { name:'把时间交给音乐。' })).toBeVisible()
  await page.getByRole('option', { name:'林俊杰', exact:true }).click()
  await expect(history.getByRole('button', { name:'搜索历史 林俊杰', exact:true })).toBeVisible()
  await expect(page.getByLabel('歌曲、歌手或专辑')).toHaveValue('林俊杰')
  await expect(page.getByRole('heading', { name:'「林俊杰」的搜索结果', exact:true })).toBeVisible()
  await history.getByRole('button', { name:'删除历史搜索 周杰伦', exact:true }).click()
  await expect(history.getByRole('button', { name:'搜索历史 周杰伦', exact:true })).toHaveCount(0)
  await history.getByRole('button', { name:'清空历史', exact:true }).click()
  await expect(history).toHaveCount(0)
  await page.reload()
  await page.getByLabel('歌曲、歌手或专辑').focus()
  await expect(page.getByRole('heading', { name:'歌曲搜索', exact:true })).toHaveCount(0)
  await expect(page.getByRole('listbox', { name:'搜索提示' }).getByRole('option')).toHaveCount(0)
  await expect(page.getByRole('button', { name:/^搜索历史 / })).toHaveCount(0)
})

test('clearing tags while a search is pending does not restore them on completion', async ({ page }) => {
  await openSearch(page)
  await page.evaluate(() => { Reflect.get(window,'searchTest').mode = 'pending' })
  await submit(page,'周杰伦')
  const history = page.getByRole('region', { name:'历史搜索', exact:true })
  await expect(history.getByRole('button', { name:'搜索历史 周杰伦', exact:true })).toBeVisible()
  await history.getByRole('button', { name:'清空历史', exact:true }).click()
  await page.evaluate(() => Reflect.get(window,'searchTest').pending[0].finish())
  await expect(page.getByRole('table', { name:'QQ 音乐搜索结果' })).toBeVisible()
  await expect(history).toHaveCount(0)
})

test('history persistence errors do not block music results', async ({ page }) => {
  await openSearch(page)
  await page.evaluate(() => { Reflect.get(window,'go').main.App.RecordSearchHistory = async () => { throw Error('无法保存搜索历史') } })
  await submit(page,'周杰伦')
  await expect(page.getByRole('table', { name:'QQ 音乐搜索结果' })).toBeVisible()
  await expect(page.getByText('无法保存搜索历史', { exact:true })).toBeVisible()
  await expect(page.getByRole('button', { name:'搜索历史 周杰伦', exact:true })).toHaveCount(0)
})

test('upstream text is escaped and narrow windows do not overflow', async ({ page }) => {
  await page.setViewportSize({ width:600,height:800 })
  await openSearch(page)
  const unsafe = '<img src=x onerror="window.searchInjected=true">'
  await page.evaluate(value => { Reflect.get(window,'searchTest').unsafeTitle = value }, unsafe)
  await submit(page, '歌曲')
  await expect(page.getByRole('table').getByText(unsafe, { exact:true })).toBeVisible()
  await expect(page.locator('img[src="x"]')).toHaveCount(0)
  expect(await page.evaluate(() => Reflect.get(window,'searchInjected'))).toBeUndefined()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
})
