import { test, expect, type Page } from '@playwright/test'

async function mockBridge(page: Page) {
  await page.addInitScript(() => {
    const state = { settings: { downloadDirectory:'/Users/test/Downloads', downloadQuality:'highest', playbackQuality:'highest', volume:1 }, writes: [] as unknown[], reads:0, picks:[] as string[], sources:[] as {id:string;name:string;author:string;version:string;createdAt:string;checkStatus:string;checkError:string;qualities:string[];active:boolean}[], cancelImport:false, confirmDelete:true, checkFails:false, validationCalls:0, incompleteValidation:false }
    Object.assign(window, { testState:state, go:{ main:{ App:{
      GetSearchHistory: async () => [],
      RecordSearchHistory: async (query:string) => [query],
      RemoveSearchHistory: async () => [],
      ClearSearchHistory: async () => [],
      GetSources: async () => [...state.sources],
      ImportSource: async () => {
        if (state.cancelImport) return null
        const duplicate = state.sources.length > 0
        const source = duplicate ? state.sources[0] : { id:'test-source', name:'测试音源', author:'测试作者', version:'v1', createdAt:'2026-01-01T00:00:00Z', checkStatus:'unchecked', checkError:'', qualities:[] as string[], active:false }
        state.validationCalls++
        const validated = state.incompleteValidation ? source : { ...source, checkStatus:state.checkFails ? 'failed' : 'ready', checkError:state.checkFails ? '脚本未声明 QQ 音乐支持' : '', qualities:state.checkFails ? [] : ['flac','320k','128k'], active:state.checkFails ? false : source.active }
        state.sources = duplicate ? state.sources.map(entry => entry.id === source.id ? validated : entry) : [validated, ...state.sources]
        return { source:validated, duplicate, validationError:state.incompleteValidation ? '无法保存音源校验结果' : undefined }
      },
      DeleteSource: async (id:string) => {
        if (!state.confirmDelete) return false
        state.sources = state.sources.filter(source => source.id !== id)
        return true
      },
      CheckSource: async (id:string) => {
        state.validationCalls++
        state.sources = state.sources.map(source => source.id !== id ? source : { ...source, checkStatus:state.checkFails ? 'failed' : 'ready', checkError:state.checkFails ? '脚本未声明 QQ 音乐支持' : '', qualities:state.checkFails ? [] : ['flac','320k','128k'], active:state.checkFails ? false : source.active })
        return [...state.sources]
      },
      ActivateSource: async (id:string) => {
        if (!state.sources.some(source => source.id === id && source.checkStatus === 'ready')) throw new Error('音源尚未通过校验')
        state.sources = state.sources.map(source => ({ ...source, active:source.id === id }))
        return [...state.sources]
      },
      DisableSource: async () => {
        state.sources = state.sources.map(source => ({ ...source, active:false }))
        return [...state.sources]
      },
      GetFavorites: async () => [], AddFavorite:async()=>{}, RemoveFavorite:async()=>{},
      GetForYou: async () => ({songs:[],reason:'no_favorites',seedCount:0,skipped:0,warning:''}),
      CancelForYou: async () => true,
      GetDiscoveryCatalog: async () => ({kind:'chart',entries:[]}),
      GetDiscoveryCollection: async () => {throw Error('no fixture collection')},
      CancelDiscovery: async () => true,
      GetAppearance: async () => { if(Reflect.get(window,'appearanceReadFails'))throw Error('read failed');return localStorage.getItem('test-native-appearance') || 'dark' },
      SaveAppearance: async (mode:string) => { if(Reflect.get(window,'appearanceSaveFails'))throw Error('save failed');localStorage.setItem('test-native-appearance',mode) },
      GetDiscoveryPreferences: async () => ({groups:[]}),
      SaveDiscoveryPreferences: async () => {},
      GetSettings: async () => { state.reads++; return { ...state.settings } },
      SaveSettings: async (value: typeof state.settings) => { state.settings = { ...value }; state.writes.push(value) },
      ChooseDownloadDirectory: async (initial:string) => { state.picks.push(initial); return '/Users/test/Documents' },
    } } } })
  })
}

const mainNav = (page: Page) => page.getByRole('navigation', { name:'主导航' })
const categories = (page: Page) => page.getByRole('navigation', { name:'设置分类' })
async function openSourceSettings(page: Page) {
  await mainNav(page).getByRole('button', { name:'设置', exact:true }).click()
  await categories(page).getByRole('link', { name:'音源', exact:true }).click()
}

test('appearance switches immediately, persists independently and follows live system changes',async({page})=>{
 await mockBridge(page);await page.emulateMedia({colorScheme:'light'});await page.goto('/')
 await expect(page.locator('html')).toHaveAttribute('data-theme','dark')
 await mainNav(page).getByRole('button',{name:'设置',exact:true}).click()
 await categories(page).getByRole('link',{name:'外观',exact:true}).click()
 const mode=page.getByLabel('主题',{exact:true})
 await mode.selectOption('light');await expect(page.locator('html')).toHaveAttribute('data-theme','light')
 await expect.poll(()=>page.evaluate(()=>localStorage.getItem('test-native-appearance'))).toBe('light')
 expect(await page.evaluate(()=>Reflect.get(window,'testState').writes)).toEqual([])
 const contrast=await page.locator('#appearance-mode-description').evaluate(element=>{
  const canvas=document.createElement('canvas');canvas.width=canvas.height=1;const context=canvas.getContext('2d')!
  function luminance(color:string){context.clearRect(0,0,1,1);context.fillStyle=color;context.fillRect(0,0,1,1);const rgb=[...context.getImageData(0,0,1,1).data].slice(0,3).map(value=>{const n=value/255;return n<=.04045?n/12.92:((n+.055)/1.055)**2.4});return rgb[0]!*.2126+rgb[1]!*.7152+rgb[2]!*.0722}
  const foreground=luminance(getComputedStyle(element).color),background=luminance(getComputedStyle(document.body).backgroundColor)
  return (Math.max(foreground,background)+.05)/(Math.min(foreground,background)+.05)
 })
 expect(contrast).toBeGreaterThanOrEqual(4.5)
 await page.screenshot({path:'test-results/appearance-light.png'})
 await mode.selectOption('system');await page.emulateMedia({colorScheme:'dark'})
 await expect(page.locator('html')).toHaveAttribute('data-theme','dark');await expect(mode).toHaveValue('system')
 await page.emulateMedia({colorScheme:'light'});await expect(page.locator('html')).toHaveAttribute('data-theme','light')
 await mode.selectOption('dark');await page.emulateMedia({colorScheme:'light'})
 await expect(page.locator('html')).toHaveAttribute('data-theme','dark')
 await expect.poll(()=>page.evaluate(()=>localStorage.getItem('test-native-appearance'))).toBe('dark')
 await page.screenshot({path:'test-results/appearance-dark.png'})
 await mode.selectOption('light');await expect.poll(()=>page.evaluate(()=>localStorage.getItem('test-native-appearance'))).toBe('light')
 await page.reload();await expect(page.locator('html')).toHaveAttribute('data-theme','light')
 await mainNav(page).getByRole('button',{name:'设置',exact:true}).click();await categories(page).getByRole('link',{name:'外观',exact:true}).click()
 await expect(page.getByLabel('主题',{exact:true})).toHaveValue('light')
 await page.setViewportSize({width:640,height:620});await page.screenshot({path:'test-results/appearance-light-compact.png'})
 expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(640)
})

test('appearance read failures retain the first-paint cache without silently saving defaults',async({page})=>{
 await mockBridge(page)
 await page.addInitScript(()=>{Reflect.set(window,'appearanceReadFails',true);localStorage.setItem('miaoyin-appearance-v1','light')})
 await page.goto('/');await mainNav(page).getByRole('button',{name:'设置',exact:true}).click()
 await categories(page).getByRole('link',{name:'外观',exact:true}).click()
 await expect(page.locator('html')).toHaveAttribute('data-theme','light')
 await expect(page.getByLabel('主题',{exact:true})).toBeDisabled()
 expect(await page.evaluate(()=>localStorage.getItem('test-native-appearance'))).toBeNull()
 await page.evaluate(()=>Reflect.set(window,'appearanceReadFails',false))
 await page.getByRole('button',{name:'重试读取外观'}).click()
 await expect(page.getByLabel('主题',{exact:true})).toBeEnabled()
 await expect(page.locator('html')).toHaveAttribute('data-theme','dark')
 expect(await page.evaluate(()=>localStorage.getItem('test-native-appearance'))).toBeNull()
})

test('appearance save failure preserves preview and retry never overwrites other settings',async({page})=>{
 await mockBridge(page);await page.goto('/');await mainNav(page).getByRole('button',{name:'设置',exact:true}).click()
 await categories(page).getByRole('link',{name:'外观',exact:true}).click()
 await page.evaluate(()=>Reflect.set(window,'appearanceSaveFails',true))
 await page.getByLabel('主题',{exact:true}).selectOption('light')
 await expect(page.locator('html')).toHaveAttribute('data-theme','light')
 await expect(page.getByRole('button',{name:'重试保存外观'})).toBeVisible()
 expect(await page.evaluate(()=>Reflect.get(window,'testState').writes)).toEqual([])
 await page.evaluate(()=>Reflect.set(window,'appearanceSaveFails',false));await page.getByRole('button',{name:'重试保存外观'}).click()
 await expect.poll(()=>page.evaluate(()=>localStorage.getItem('test-native-appearance'))).toBe('light')
 await expect(page.getByRole('button',{name:'重试保存外观'})).toHaveCount(0)
})

test('settings is lazy and renders all sections continuously without a page header', async ({ page }) => {
  const requests:string[]=[]
  page.on('request', request => requests.push(request.url()))
  await mockBridge(page)
  await page.goto('/')
  await expect(page.getByRole('heading', { name:'把时间交给音乐。' })).toBeVisible()
  expect(requests.some(url=>url.includes('/SettingsPage-'))).toBe(false)
  expect(await page.evaluate(()=>Reflect.get(window,'testState').reads)).toBe(0)
  await expect(mainNav(page).getByRole('button')).toHaveCount(4)
  await expect(mainNav(page).getByRole('button', { name:'搜索', exact:true })).toHaveCount(0)
  await expect(mainNav(page).getByRole('button', { name:'音源设置' })).toHaveCount(0)
  await mainNav(page).getByRole('button', { name:'设置', exact:true }).click()
  await expect(categories(page).getByRole('link',{name:'外观',exact:true})).toHaveAttribute('aria-current','location')
  await expect(page.getByRole('heading',{name:'外观',exact:true})).toBeInViewport()
  await expect(page.getByLabel('播放音质', { exact:true })).toBeVisible()
  await expect(page.getByRole('heading', { name:'设置', exact:true })).toHaveCount(0)
  await expect(page.locator('main header').getByRole('search')).toBeVisible()
  await expect(page.locator('main header > span')).toHaveCount(0)
  await expect(page.getByRole('button', { name:'未启用音源' })).toHaveCount(0)
  await expect(page.getByRole('region', { name:'播放设置', exact:true })).toHaveCount(1)
  await expect(page.getByRole('region', { name:'下载设置', exact:true })).toHaveCount(1)
  await expect(page.getByRole('region', { name:'音源设置', exact:true })).toHaveCount(1)
  await expect(page.locator('#settings-source')).toHaveCSS('min-height', '0px')
  await expect(page.getByText('更改后自动保存', { exact:true })).toHaveCount(0)
  await expect(page.locator('#settings-playback [data-slot="slider-thumb"]')).toHaveAttribute('aria-valuenow', '1')
  await categories(page).getByRole('link', { name:'下载', exact:true }).click()
  await expect(page.getByLabel('下载目录', { exact:true })).toBeVisible()
  await expect(page.getByLabel('下载音质', { exact:true })).toBeVisible()
  await expect(page.getByLabel('播放音质', { exact:true })).toHaveCount(1)
  await categories(page).getByRole('link', { name:'音源', exact:true }).click()
  await expect(categories(page).getByRole('link', { name:'音源', exact:true })).toHaveAttribute('aria-current', 'location')
  await expect(page.getByRole('button', { name:'导入脚本' })).toBeVisible()
  await expect(page.getByText('音乐平台', { exact:true })).toHaveCount(0)
  await page.screenshot({ path:'test-results/settings-source.png' })
})

test('focusing and typing in search preserves settings and its scroll position',async({page})=>{
  await mockBridge(page);await page.goto('/')
  await openSourceSettings(page)
  await expect(categories(page).getByRole('link',{name:'音源',exact:true})).toHaveAttribute('aria-current','location')
  const scroll=page.locator('[data-content-scroll]')
  const before=await scroll.evaluate(element=>element.scrollTop)
  await page.getByRole('combobox',{name:'歌曲、歌手或专辑'}).fill('只想')
  await expect(page.getByRole('region',{name:'音源设置',exact:true})).toBeVisible()
  await expect(page.getByRole('listbox',{name:'搜索提示'})).toBeVisible()
  expect(await scroll.evaluate(element=>element.scrollTop)).toBe(before)
  await expect(categories(page).getByRole('link',{name:'音源',exact:true})).toHaveAttribute('aria-current','location')
  await page.getByRole('combobox',{name:'歌曲、歌手或专辑'}).press('Escape')
  await expect(page.getByRole('button',{name:'导入脚本',exact:true})).toBeVisible()
})

test('cross-category edits are saved together after leaving settings and restored on return', async ({ page }) => {
  await mockBridge(page)
  await page.goto('/')
  await mainNav(page).getByRole('button', { name:'设置', exact:true }).click()
  await page.getByLabel('播放音质', { exact:true }).selectOption('320k')
  await categories(page).getByRole('link', { name:'下载', exact:true }).click()
  await page.getByLabel('下载音质', { exact:true }).selectOption('flac')
  await page.getByRole('button', { name:'选择目录' }).click()
  await expect(page.getByLabel('下载目录', { exact:true })).toHaveValue('/Users/test/Documents')
  await page.getByRole('button', { name:'选择目录' }).click()
  expect(await page.evaluate(()=>Reflect.get(window,'testState').picks)).toEqual(['/Users/test/Downloads','/Users/test/Documents'])
  await page.getByLabel('歌曲、歌手或专辑').focus()
  await expect.poll(()=>page.evaluate(()=>Reflect.get(window,'testState').writes.length)).toBe(1)
  await expect(page.getByText('设置已保存', { exact:true })).toBeVisible()
  await mainNav(page).getByRole('button', { name:'设置', exact:true }).click()
  await expect(page.getByLabel('下载音质', { exact:true })).toHaveValue('flac')
  await categories(page).getByRole('link', { name:'播放', exact:true }).click()
  await expect(page.getByLabel('播放音质', { exact:true })).toHaveValue('320k')
  expect(await page.evaluate(()=>Reflect.get(window,'testState').reads)).toBe(1)
})

test('source settings remain accessible without recommendation shortcut', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('button', { name:'配置音源', exact:true })).toHaveCount(0)
  await mainNav(page).getByRole('button', { name:'设置', exact:true }).click()
  await categories(page).getByRole('link', { name:'音源', exact:true }).click()
  await expect(page.getByRole('heading', { name:'音源', exact:true })).toBeVisible()
  await expect(categories(page).getByRole('link', { name:'音源', exact:true })).toHaveAttribute('aria-current', 'location')
})

test('toolbar refresh reads saved settings but never discards pending edits', async ({page}) => {
  await mockBridge(page);await page.goto('/')
  await mainNav(page).getByRole('button',{name:'设置',exact:true}).click()
  const input=page.getByLabel('播放音质',{exact:true}),refresh=page.getByRole('navigation',{name:'浏览导航'}).getByRole('button',{name:'刷新',exact:true})
  await expect(input).toHaveValue('highest')
  await refresh.click()
  await expect.poll(()=>page.evaluate(()=>Reflect.get(window,'testState').reads)).toBe(2)
  await input.selectOption('flac')
  await refresh.click()
  await expect(page.getByText('请等待设置保存完成后再刷新',{exact:true})).toBeVisible()
  await expect(input).toHaveValue('flac')
  expect(await page.evaluate(()=>Reflect.get(window,'testState').reads)).toBe(2)
})

test('scrolling updates anchor highlight and keeps the menu visible', async ({ page }) => {
  await page.setViewportSize({ width:1180, height:600 })
  await mockBridge(page)
  await page.goto('/')
  await mainNav(page).getByRole('button', { name:'设置', exact:true }).click()
  await expect(page.getByLabel('播放音质', { exact:true })).toBeVisible()
  await page.locator('#settings-download').evaluate(element => element.scrollIntoView({ block:'start' }))
  await expect(categories(page).getByRole('link', { name:'下载', exact:true })).toHaveAttribute('aria-current', 'location')
  await page.locator('[data-content-scroll]').evaluate(element => element.scrollTop = element.scrollHeight)
  await expect(categories(page).getByRole('link', { name:'音源', exact:true })).toHaveAttribute('aria-current', 'location')
  const bounds = await categories(page).boundingBox()
  expect(bounds!.y).toBeGreaterThanOrEqual(0)
  expect(bounds!.y).toBeLessThan(160)
  await categories(page).getByRole('link', { name:'播放', exact:true }).click()
  await expect(categories(page).getByRole('link', { name:'播放', exact:true })).toHaveAttribute('aria-current', 'location')
})

test('settings anchors also work in the compact fixed layout', async ({ page }) => {
  await page.setViewportSize({ width:600, height:800 })
  await mockBridge(page)
  await page.goto('/')
  await mainNav(page).getByRole('button', { name:'设置', exact:true }).click()
  await expect(page.getByLabel('播放音质', { exact:true })).toBeVisible()
  await categories(page).getByRole('link', { name:'音源', exact:true }).click()
  await expect(categories(page).getByRole('link', { name:'音源', exact:true })).toHaveAttribute('aria-current', 'location')
  await page.locator('[data-content-scroll]').evaluate(element => element.scrollTo(0, 0))
  await expect(categories(page).getByRole('link', { name:'外观', exact:true })).toHaveAttribute('aria-current', 'location')
})

test('setting rows place descriptions left and controls right, then stack on narrow windows', async ({ page }) => {
  await mockBridge(page)
  await page.goto('/')
  await mainNav(page).getByRole('button', { name:'设置', exact:true }).click()
  const label = page.locator('label[for="playback-quality"]')
  const control = page.getByLabel('播放音质', { exact:true })
  await expect(control).toBeVisible()
  const left = await label.boundingBox()
  const right = await control.boundingBox()
  expect(right!.x).toBeGreaterThan(left!.x + left!.width)
  const track = page.locator('#settings-playback [data-slot="slider-track"]')
  await expect(track).toHaveCSS('height', '4px')
  await expect(page.locator('#settings-playback [data-slot="separator"]')).toHaveCSS('height', '1px')
  await page.screenshot({ path:'test-results/settings-rows.png' })
  await page.setViewportSize({ width:600, height:800 })
  const narrowLabel = await label.boundingBox()
  const narrowControl = await control.boundingBox()
  expect(narrowControl!.y).toBeGreaterThan(narrowLabel!.y + narrowLabel!.height)
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
})

test('autosave success uses green toast and failure uses red toast', async ({ page }) => {
  await mockBridge(page)
  await page.goto('/')
  await mainNav(page).getByRole('button', { name:'设置', exact:true }).click()
  await page.getByLabel('播放音质', { exact:true }).selectOption('320k')
  const success = page.locator('[data-sonner-toast][data-type="success"]')
  await expect(success).toBeVisible()
  await expect(success).toHaveAttribute('data-rich-colors', 'true')
  const green = await success.evaluate(element => getComputedStyle(element).backgroundColor.match(/\d+/g)!.map(Number))
  expect(green[1]).toBeGreaterThan(green[0])
  expect(green[1]).toBeGreaterThan(green[2])
  await page.evaluate(() => { Reflect.get(window, 'go').main.App.SaveSettings = async () => { throw new Error('disk full') } })
  await page.getByLabel('播放音质', { exact:true }).selectOption('128k')
  const error = page.locator('[data-sonner-toast][data-type="error"]')
  await expect(error).toBeVisible()
  await expect(error).toHaveAttribute('data-rich-colors', 'true')
  const red = await error.evaluate(element => getComputedStyle(element).backgroundColor.match(/\d+/g)!.map(Number))
  expect(red[0]).toBeGreaterThan(red[1])
  expect(red[0]).toBeGreaterThan(red[2])
})

test('source import lists metadata, prevents duplicate rows, supports cancellation and deletion', async ({ page }) => {
  await mockBridge(page)
  await page.goto('/')
  await openSourceSettings(page)
  const importButton = page.getByRole('button', { name:'导入脚本', exact:true })
  await expect(importButton).toBeEnabled()
  await page.evaluate(() => { Reflect.get(window,'testState').cancelImport = true })
  await importButton.click()
  await expect(page.getByText('测试音源', { exact:true })).toHaveCount(0)
  await page.evaluate(() => { Reflect.get(window,'testState').cancelImport = false })
  await importButton.click()
  await expect(page.getByText('测试音源', { exact:true })).toHaveCount(1)
  await expect(page.getByText('校验通过', { exact:true })).toBeVisible()
  expect(await page.evaluate(() => Reflect.get(window,'testState').validationCalls)).toBe(1)
  await expect(page.getByRole('switch', { name:'启用音源', exact:true })).toHaveCount(0)
  await expect(page.getByRole('radio', { name:'启用音源 测试音源', exact:true })).toBeEnabled()
  await expect(page.getByRole('radio', { name:'启用音源 测试音源', exact:true })).not.toBeChecked()
  await importButton.click()
  await expect(page.getByText('音源已存在，校验通过', { exact:true })).toBeVisible()
  expect(await page.evaluate(() => Reflect.get(window,'testState').validationCalls)).toBe(2)
  await expect(page.getByRole('button', { name:'删除音源 测试音源', exact:true })).toHaveCount(1)
  const deleteButton = page.getByRole('button', { name:'删除音源 测试音源', exact:true })
  await deleteButton.click()
  const dialog = page.getByRole('alertdialog')
  await expect(dialog).toBeVisible()
  await dialog.getByRole('button', { name:'取消', exact:true }).click()
  await expect(dialog).toHaveCount(0)
  await expect(page.getByText('测试音源', { exact:true })).toHaveCount(1)
  await expect(deleteButton).toBeFocused()
  await deleteButton.click()
  await page.evaluate(() => { Reflect.get(window,'testState').confirmDelete = false })
  await dialog.getByRole('button', { name:'删除音源', exact:true }).click()
  await expect(dialog.getByText('删除失败', { exact:true })).toBeVisible()
  await expect(page.getByText('测试音源', { exact:true })).toHaveCount(1)
  await page.evaluate(() => { Reflect.get(window,'testState').confirmDelete = true })
  await dialog.getByRole('button', { name:'删除音源', exact:true }).click()
  await expect(dialog).toHaveCount(0)
  await expect(page.getByText('测试音源', { exact:true })).toHaveCount(0)
  await expect(importButton).toBeFocused()
})

test('automatic validation failure preserves the imported copy and can be retried manually', async ({ page }) => {
  await mockBridge(page)
  await page.goto('/')
  await openSourceSettings(page)
  await page.evaluate(() => { Reflect.get(window,'testState').checkFails = true })
  await page.getByRole('button', { name:'导入脚本', exact:true }).click()
  await expect(page.getByText('校验未通过', { exact:true })).toBeVisible()
  await expect(page.getByText('测试音源', { exact:true })).toHaveCount(1)
  await expect(page.getByRole('radio', { name:'启用音源 测试音源', exact:true })).toBeDisabled()
  await page.evaluate(() => { Reflect.get(window,'testState').checkFails = false })
  await page.getByRole('button', { name:'校验音源 测试音源', exact:true }).click()
  await expect(page.getByText('校验通过', { exact:true })).toBeVisible()
  await expect(page.getByRole('radio', { name:'启用音源 测试音源', exact:true })).toBeEnabled()
  await expect(page.getByRole('radio', { name:'启用音源 测试音源', exact:true })).not.toBeChecked()
})

test('an interrupted validation is not misreported as an import failure', async ({ page }) => {
  await mockBridge(page)
  await page.goto('/')
  await openSourceSettings(page)
  await page.evaluate(() => { Reflect.get(window,'testState').incompleteValidation = true })
  await page.getByRole('button', { name:'导入脚本', exact:true }).click()
  await expect(page.getByText('测试音源', { exact:true })).toHaveCount(1)
  await expect(page.getByText('音源已保存，但校验未完成：无法保存音源校验结果', { exact:true })).toBeVisible()
  await expect(page.getByText('音源导入失败', { exact:true })).toHaveCount(0)
  await expect(page.getByRole('radio', { name:'启用音源 测试音源', exact:true })).toBeDisabled()
})

test('pending import validation survives leaving the settings page', async ({ page }) => {
  await mockBridge(page)
  await page.goto('/')
  await openSourceSettings(page)
  await page.evaluate(() => {
    const api = Reflect.get(window,'go').main.App
    const original = api.ImportSource
    api.ImportSource = () => new Promise(resolve => { Reflect.set(window,'finishImport', () => resolve(original())) })
  })
  await page.getByRole('button', { name:'导入脚本', exact:true }).click()
  await expect(page.getByRole('button', { name:'导入并校验中…', exact:true })).toBeDisabled()
  await page.getByLabel('歌曲、歌手或专辑').focus()
  await page.evaluate(() => Reflect.get(window,'finishImport')())
  await expect(page.getByText('音源已导入并通过校验', { exact:true })).toBeVisible()
  await openSourceSettings(page)
  await expect(page.getByText('校验通过', { exact:true })).toBeVisible()
  expect(await page.evaluate(() => Reflect.get(window,'testState').validationCalls)).toBe(1)
})

test('source checks gate single selection, switching, disabling and failed rechecks', async ({ page }) => {
  await mockBridge(page)
  await page.goto('/')
  await expect(mainNav(page).getByRole('button', { name:'设置', exact:true })).toBeVisible()
  await page.evaluate(() => {
    Reflect.get(window,'testState').sources = ['甲','乙'].map(id => ({ id, name:`音源${id}`, author:'测试', version:'v1', createdAt:'2026-01-01', checkStatus:'unchecked', checkError:'', qualities:[], active:false }))
  })
  await openSourceSettings(page)
  const first = page.getByRole('radio', { name:'启用音源 音源甲', exact:true })
  const second = page.getByRole('radio', { name:'启用音源 音源乙', exact:true })
  await expect(first).toBeDisabled()
  await page.getByRole('button', { name:'校验音源 音源甲', exact:true }).click()
  await expect(first).toBeEnabled()
  await first.click()
  await expect(first).toBeChecked()
  await expect(page.getByText('已启用', { exact:true })).toHaveCount(1)
  await page.getByRole('button', { name:'校验音源 音源乙', exact:true }).click()
  await second.click()
  await expect(second).toBeChecked()
  await expect(first).not.toBeChecked()
  await mainNav(page).getByRole('button', { name:'推荐', exact:true }).click()
  await expect(page.getByRole('button', { name:'音源乙', exact:true })).toHaveCount(0)
  await openSourceSettings(page)
  await page.evaluate(() => { Reflect.get(window,'testState').checkFails = true })
  await page.getByRole('button', { name:'校验音源 音源乙', exact:true }).click()
  await expect(second).toBeDisabled()
  await expect(second).not.toBeChecked()
  await expect(first).not.toBeChecked()
  await expect(page.getByText('脚本未声明 QQ 音乐支持', { exact:true })).toBeVisible()
  await page.evaluate(() => { Reflect.get(window,'testState').checkFails = false })
  await page.getByRole('button', { name:'校验音源 音源乙', exact:true }).click()
  await second.click()
  await page.getByRole('button', { name:'禁用', exact:true }).click()
  await expect(second).not.toBeChecked()
  await expect(page.getByText('已启用', { exact:true })).toHaveCount(0)
})

test('activation is not shown as selected until the backend succeeds', async ({ page }) => {
  await mockBridge(page)
  await page.goto('/')
  await expect(mainNav(page).getByRole('button', { name:'设置', exact:true })).toBeVisible()
  await page.evaluate(() => {
    const state = Reflect.get(window,'testState')
    state.sources = [{ id:'ready', name:'待启用', author:'', version:'', createdAt:'2026', checkStatus:'ready', checkError:'', qualities:['128k'], active:false }]
    const api = Reflect.get(window,'go').main.App
    const original = api.ActivateSource
    api.ActivateSource = (id:string) => new Promise(resolve => { Reflect.set(window,'finishActivation', () => resolve(original(id))) })
  })
  await openSourceSettings(page)
  const radio = page.getByRole('radio', { name:'启用音源 待启用', exact:true })
  await radio.click()
  await expect(radio).not.toBeChecked()
  await expect(radio).toBeDisabled()
  await page.evaluate(() => Reflect.get(window,'finishActivation')())
  await expect(radio).toBeChecked()
  await expect(radio).toBeEnabled()
})

test('active source qualities filter options without overwriting an unsupported saved preference', async ({ page }) => {
  await mockBridge(page)
  await page.goto('/')
  await expect(mainNav(page).getByRole('button', { name:'设置', exact:true })).toBeVisible()
  await page.evaluate(() => {
    Reflect.get(window,'testState').sources = [{ id:'limited', name:'标准音质', author:'', version:'', createdAt:'2026', checkStatus:'ready', checkError:'', qualities:['128k'], active:false }]
  })
  await mainNav(page).getByRole('button', { name:'设置', exact:true }).click()
  await page.getByLabel('播放音质', { exact:true }).selectOption('flac')
  await categories(page).getByRole('link', { name:'音源', exact:true }).click()
  await page.getByRole('radio', { name:'启用音源 标准音质', exact:true }).click()
  await categories(page).getByRole('link', { name:'播放', exact:true }).click()
  const select = page.getByLabel('播放音质', { exact:true })
  await expect(select).toHaveValue('flac')
  await expect(select.getByRole('option', { name:'无损 FLAC（当前音源不支持）', exact:true })).toBeDisabled()
  await expect(select.getByRole('option', { name:'320K', exact:true })).toHaveCount(0)
  await expect(select.getByRole('option', { name:'128K', exact:true })).toHaveCount(1)
})

test('search input survives lazy page unmounts', async ({ page }) => {
  await page.goto('/')
  await page.getByLabel('歌曲、歌手或专辑').focus()
  await page.getByLabel('歌曲、歌手或专辑').fill('周杰伦')
  await mainNav(page).getByRole('button', { name:'推荐', exact:true }).click()
  await page.getByLabel('歌曲、歌手或专辑').focus()
  await expect(page.getByLabel('歌曲、歌手或专辑')).toHaveValue('周杰伦')
})
