import { test, expect, type Page } from '@playwright/test'

async function setup(page: Page) {
  await page.addInitScript(() => {
    const audios: HTMLAudioElement[] = []
    const OriginalAudio = window.Audio
    window.Audio = class extends OriginalAudio {
      constructor(src?: string) {
        super(src)
        audios.push(this)
      }
    }
    Object.assign(window, { testAudios: audios })
    const songs = ['第一首', '第二首'].map((title, index) => ({
      id: 'Song' + index,
      source: 'tx',
      songId: '123',
      mediaMid: 'Media1',
      title,
      subtitle: '',
      artist: '测试歌手',
      artists: [{id:'Artist1',name:'测试歌手'}],
      album: '测试专辑',
      albumMid: 'Album1',
      coverUrl: '',
      duration: 6,
      availableQualities: ['flac', '320k', '128k']
    }))
    const downloadTasks = JSON.parse(localStorage.getItem('download-tasks') || '[]') as { id: string; batch: string; song: {id: string; title: string}; directory: string; requested: string; quality: string; codec: string; state: string; bytes: number; total: number; file: string; message: string; created: number }[]
    for (const task of downloadTasks) if (['queued','downloading'].includes(task.state)) task.state = 'paused'
    const saveDownloads = () => localStorage.setItem('download-tasks', JSON.stringify(downloadTasks))
    const state = {
      downloadTasks,
      downloadDirectory: '/Downloads',
      downloadQuality: 'highest',
      downloadRemoves: [] as [string, boolean][],
      downloadEnqueues: 0,
      downloadControls: [] as {ids:string[];action:string;all:boolean}[],
      layoutWrites: [] as unknown[],
      lifecycleEvents: {} as Record<string,(...args:any[])=>void>,
      quitAcks: [] as string[],
      creditRequests: [] as string[],
      testSongs: songs,
      artistTrackCount: 2,
      detailMode: 'normal',
      detailRequests: [] as string[],
      detailPending: [] as (()=>void)[],
      layoutMode: 'normal',
      discoveryMode: localStorage.getItem('discovery-fixture-mode') || 'normal',
      discoveryCalls: [] as string[],
      discoveryCancels: [] as string[],
      discoveryPending: [] as (() => void)[],
      prepares: [] as string[],
      canceled: [] as string[],
      mode: 'normal',
      favoriteMode: 'normal',
      favoriteWrites: [] as string[],
      favoritePending: [] as (() => void)[],
      codec: 'flac',
      lyricMode: 'normal',
      lyricCalls: [] as string[],
      session: '',
      native: { position: 0, duration: 6, playing: false, ended: false, volume: 1 },
      pending: [] as (() => void)[],
      saves: 0,
      neighborhoods: [] as string[][]
    }
    setInterval(() => {
      if (!state.native.playing) return
      state.native.position = Math.min(6, state.native.position + 0.05)
      if (state.native.position >= 6) {
        state.native.ended = true
        state.native.playing = false
      }
    }, 50)
    Object.assign(window, {
      playerTest: state,
      runtime:{EventsOn:(name:string,callback:(...args:any[])=>void)=>{state.lifecycleEvents[name]=callback;return()=>delete state.lifecycleEvents[name]}},
      go: {
        main: {
          App: {
            AcknowledgeQuit: async (id:string) => {state.quitAcks.push(id);return true},
            GetSongCredits: async (id:string) => {state.creditRequests.push(id);return {id,artists:[{id:'Artist1',name:'测试歌手'}],albumMid:'Album1'}},
            GetMusicDetail: async (kind:string,id:string,albumPage:number) => {
              state.detailRequests.push(`${kind}:${id}:${albumPage}`)
              if(state.detailMode==='error')throw Error('详情读取失败')
              const value={kind,id,title:kind==='artist'?(id==='Artist2'?'合唱歌手':'测试歌手'):'测试专辑',coverUrl:'',artists:kind==='album'?[{id:'Artist1',name:'测试歌手'}]:[],date:kind==='album'?'2026-01-01':'',albumPage,totalAlbums:kind==='artist'?21:0,hasMoreAlbums:kind==='artist'&&albumPage===1,songs:state.detailMode==='empty'?[]:kind==='artist'?(state.artistTrackCount===2?songs:Array.from({length:state.artistTrackCount},(_,index)=>({...songs[0],id:`HotTrack${index}`,title:`热门曲目${index}`}))):Array.from({length:120},(_,index)=>({...songs[0],id:`AlbumTrack${index===119?0:index}`,title:`专辑曲目${index}`})),albums:kind==='artist'?(albumPage===1?Array.from({length:20},(_,index)=>({id:`Album${index+1}`,title:index===0?'测试专辑':index===1?'第二张专辑':`专辑${index+1}`,artist:'测试歌手',coverUrl:'',date:'2026-01-01'})):[{id:'Album21',title:'后续专辑',artist:'测试歌手',coverUrl:'',date:'2024-01-01'}]):[]}
              if(state.detailMode==='pending')return new Promise(resolve=>state.detailPending.push(()=>resolve(value)))
              return value
            },
            CancelMusicDetail: async () => true,
            GetDiscoveryPreferences: async () => { if(localStorage.getItem('layout-read-error'))throw Error('read failed');return JSON.parse(localStorage.getItem('discovery-layout') || '{"groups":[]}') },
            SaveDiscoveryPreferences: async (value: unknown) => { if(state.layoutMode==='error')throw Error('save failed');state.layoutWrites.push(value);localStorage.setItem('discovery-layout',JSON.stringify(value)) },
            GetDiscoverySnapshot: async (kind:string,id:number,period:string) => JSON.parse(localStorage.getItem('discovery-cache-fixture') || '{}')[`${kind}:${id}:${period}`] || {savedAt:0},
            GetDiscoveryCatalog: async (kind: string) => {
              state.discoveryCalls.push('catalog:' + kind)
              if (state.discoveryMode === 'error') throw Error('QQ 音乐暂时无法提供推荐内容，请稍后重试')
              const entries = state.discoveryMode === 'empty' ? [] : kind === 'chart' ? Array.from({ length: 18 }, (_, index) => ({kind:'chart',id:index+1,title:`榜单${index+1}`,group:index<6?'巅峰榜':index<12?'地区榜':'全球榜',coverUrl:'',period:'2026_43',updated:'2026-10-29',total:120})) : [5,1,6,2,4,3].map((id,index)=>({kind:'new',id,title:['最新','内地','港台','欧美','韩国','日本'][index],group:'新歌',coverUrl:'',period:'',updated:'',total:0}))
              const result={kind,entries}
              if(state.discoveryMode==='pending')return new Promise(resolve=>state.discoveryPending.push(()=>resolve(result)))
              return result
            },
            GetDiscoveryCollection: async (kind: string,id: number,period: string) => {
              state.discoveryCalls.push(`collection:${kind}:${id}`)
              if(state.discoveryMode==='error')throw Error('QQ 音乐暂时无法提供推荐内容，请稍后重试')
              const result={entry:{kind,id,title:kind==='chart'?`榜单${id}`:'最新新歌',group:kind==='chart'?'巅峰榜':'新歌',coverUrl:'',period,updated:'',total:state.discoveryMode==='empty'?0:kind==='chart'?120:18},songs:state.discoveryMode==='empty'?[]:Array.from({length:kind==='chart'?120:18},(_,index)=>({...songs[0],id:`Discover${id}Song${index}`,title:`推荐歌曲${index}`}))}
              if(state.discoveryMode==='pending')return new Promise(resolve=>state.discoveryPending.push(()=>resolve(result)))
              return result
            },
            CancelDiscovery: async (id: string) => {state.discoveryCancels.push(id);return true},
            GetDownloadTasks: async () => downloadTasks.map(task => ({...task})),
            EnqueueDownloads: async (entries: {id: string; title: string}[]) => {
              state.downloadEnqueues++
              const batch = 'Batch' + state.downloadEnqueues
              const existing=entries.filter(song=>downloadTasks.some(task=>task.state==='success'&&task.song.id===song.id&&task.directory===state.downloadDirectory)).length
              const remaining=entries.filter((song,index)=>entries.findIndex(entry=>entry.id===song.id)===index&&!downloadTasks.some(task=>task.state==='success'&&task.song.id===song.id&&task.directory===state.downloadDirectory))
              const added = remaining.map((song, index) => ({id:'Download' + state.downloadEnqueues + '-' + index, batch, song, directory:state.downloadDirectory, requested:state.downloadQuality, quality:'', codec:'', state:'queued', bytes:0, total:0, file:'', message:'', created:Date.now()+index}))
              downloadTasks.unshift(...added); saveDownloads(); return {tasks:added,existing}
            },
            ControlDownloads: async (ids: string[], action: string, all: boolean) => {
              state.downloadControls.push({ids,action,all})
              for(const task of downloadTasks){
                if(!all&&!ids.includes(task.id))continue
                if(action==='start'&&['paused','failed','canceled'].includes(task.state)){task.state='queued';task.message=''}
                if(action==='pause'&&['queued','downloading'].includes(task.state)){task.state='paused';task.message=''}
                if(action==='cancel'&&['queued','downloading','paused','failed'].includes(task.state)){task.state='canceled';task.message='已取消，可手动继续'}
              }
              for(let i=downloadTasks.length-1;i>=0;i--)if(downloadTasks[i].state==='canceled')downloadTasks.splice(i,1)
              saveDownloads()
            },
            PauseDownload: async (id: string) => { const task=downloadTasks.find(task=>task.id===id)!;task.state='paused';saveDownloads() },
            ContinueDownload: async (id: string) => { const task=downloadTasks.find(task=>task.id===id)!;task.state='queued';task.message='正在准备下载';saveDownloads() },
            RemoveDownload: async (id: string, remove: boolean) => { state.downloadRemoves.push([id,remove]);const index=downloadTasks.findIndex(task=>task.id===id);if(index>=0)downloadTasks.splice(index,1);saveDownloads() },
            CancelDownloadBatch: async (batch: string) => { for(const task of downloadTasks)if(task.batch===batch&&['queued','downloading'].includes(task.state))task.state='canceled';saveDownloads() },
            RetryDownloadBatch: async (batch: string) => { for(const task of downloadTasks)if(task.batch===batch&&task.state==='failed')task.state='queued';saveDownloads() },
            GetFavorites: async () => {
              if (state.favoriteMode === 'read-error') throw Error('read failed')
              return JSON.parse(localStorage.getItem('favorite-songs') || '[]')
            },
            AddFavorite: async (song: { id: string }) => {
              state.favoriteWrites.push('add:' + song.id)
              if (state.favoriteMode === 'write-error') throw Error('write failed')
              if (state.favoriteMode === 'pending') await new Promise<void>(resolve => state.favoritePending.push(resolve))
              const favorites = JSON.parse(localStorage.getItem('favorite-songs') || '[]') as { id: string }[]
              if (!favorites.some(entry => entry.id === song.id)) favorites.unshift(song)
              localStorage.setItem('favorite-songs', JSON.stringify(favorites))
            },
            RemoveFavorite: async (id: string) => {
              state.favoriteWrites.push('remove:' + id)
              if (state.favoriteMode === 'write-error') throw Error('write failed')
              const favorites = JSON.parse(localStorage.getItem('favorite-songs') || '[]') as { id: string }[]
              localStorage.setItem('favorite-songs', JSON.stringify(favorites.filter(song => song.id !== id)))
            },
            GetPlayerSession: async () => ({
              playback: JSON.parse(
                localStorage.getItem('player-snapshot') || '{"queue":[],"index":-1,"position":0,"mode":"sequential"}'
              ),
              volume: 1
            }),
            SavePlayback: async (value: unknown) => {
              state.saves++
              localStorage.setItem('player-snapshot', JSON.stringify(value))
            },
            SearchSongs: async (query: string, page: number) => ({
              query,
              page,
              pageSize: 20,
              total: 2,
              hasMore: false,
              songs
            }),
            CancelSongSearch: async () => true,
            GetSearchHistory: async () => [],
            RecordSearchHistory: async () => [],
            RemoveSearchHistory: async () => [],
            ClearSearchHistory: async () => [],
            SuggestSongs: async () => [],
            CancelSongSuggestions: async () => true,
            PrepareSong: async (song: { id: string }, id: string) => {
              state.prepares.push(song.id)
              if (state.mode === 'error') throw Error('请先启用音源')
              const prepare = () => {
                state.session = id
                state.native = { position: 0, duration: 6, playing: false, ended: false, volume: 1 }
                return {
                  songId: song.id,
                  engine: 'libmpv',
                  nativeSession: id,
                  mediaUrl: '',
                  quality: state.codec === 'mp3' ? '320k' : 'flac',
                  codec: state.codec,
                  duration: 6,
                  seekable: true,
                  downgraded: false
                }
              }
              if (state.mode === 'pending')
                return new Promise((resolve) => state.pending.push(() => resolve(prepare())))
              return prepare()
            },
            PrefetchNeighbors: async (_id: string, _revision: number, current: { id: string }, previous: { id: string } | null, next: { id: string } | null) => {
              state.neighborhoods.push([current.id, previous?.id ?? '', next?.id ?? ''])
              return true
            },
            NativePlayback: async (id: string, action: string, value: number) => {
              if (id !== state.session) throw Error('播放会话已失效')
              if (action === 'play') {
                state.native.playing = true
                state.native.ended = false
              }
              if (action === 'pause') state.native.playing = false
              if (action === 'seek') {
                state.native.position = Math.min(value, 5.999)
                state.native.ended = false
              }
              if (action === 'volume') state.native.volume = value
              return { ...state.native, seekable: true, buffering: false }
            },
            CancelPlayback: async (id: string) => {
              state.canceled.push(id)
              if (state.session === id) {
                state.session = ''
                state.native.playing = false
              }
              return true
            },
            GetLyrics: async (songId: string) => {
              state.lyricCalls.push(songId)
              if (state.lyricMode === 'error') throw Error('upstream')
              if (state.lyricMode === 'empty') return { songId, lines: [], text: '' }
              if (state.lyricMode === 'long')
                return {
                  songId,
                  lines: Array.from({ length: 100 }, (_, index) => ({ time: index * 0.05, text: `歌词第${index}行` })),
                  text: ''
                }
              if (state.lyricMode === 'plain') return { songId, lines: [], text: '未同步的歌词\n第二行' }
              return {
                songId,
                lines: [
                  { time: 0, text: `${songId} 开始` },
                  { time: 2, text: '点击这一行' },
                  { time: 4, text: '<img src=x onerror=alert(1)>' }
                ],
                text: ''
              }
            },
            CancelLyrics: async () => true,
            GetSettings: async () => ({
              downloadDirectory: '/Downloads',
              downloadQuality: 'highest',
              playbackQuality: 'highest',
              volume: 1
            }),
            SaveSettings: async () => {}
          }
        }
      }
    })
  })
  await page.goto('/')
}
async function search(page: Page) {
  const input = page.getByLabel('歌曲、歌手或专辑')
  await input.fill('测试')
  await input.press('Enter')
  await expect(page.getByRole('table')).toBeVisible()
}
const footer = (page: Page) => page.getByRole('contentinfo', { name: '播放控制' })
const nativePosition = (page: Page) => page.evaluate(() => Reflect.get(window, 'playerTest').native.position)
const nativeVolume = (page: Page) => page.evaluate(() => Reflect.get(window, 'playerTest').native.volume)

for (const codec of ['flac', 'mp3']) {
  test(`${codec} uses libmpv without HTML Audio; pause, seek and queue survive navigation`, async ({ page }) => {
    await setup(page)
    await page.evaluate((codec) => {
      Reflect.get(window, 'playerTest').codec = codec
    }, codec)
    await search(page)
    await page.getByRole('button', { name: '播放 第一首', exact: true }).click()
    await expect(footer(page).getByRole('button', { name: '暂停', exact: true })).toBeVisible()
    await expect(footer(page).getByText('第一首', { exact: true })).toBeVisible()
    await expect.poll(() => nativePosition(page)).toBeGreaterThan(0)
    const volume = footer(page).getByRole('slider', { name: '音量' })
    await volume.focus()
    await volume.press('Home')
    await expect.poll(() => nativeVolume(page)).toBe(0)
    await volume.press('End')
    await expect.poll(() => nativeVolume(page)).toBe(1)
    await footer(page).getByRole('button', { name: '暂停', exact: true }).click()
    const slider = footer(page).getByRole('slider', { name: '播放进度' })
    await slider.focus()
    await slider.press('ArrowRight')
    await expect(footer(page).getByRole('button', { name: '播放', exact: true })).toBeVisible()
    await footer(page).getByRole('button', { name: '播放', exact: true }).click()
    await expect(footer(page).getByRole('button', { name: '暂停', exact: true })).toBeVisible()
    await page.getByRole('navigation', { name: '主导航' }).getByRole('button', { name: '推荐', exact: true }).click()
    await expect(footer(page).getByRole('button', { name: '暂停', exact: true })).toBeVisible()
    await footer(page).getByRole('button', { name: '下一首', exact: true }).click()
    await expect(footer(page).getByText('第二首', { exact: true })).toBeVisible()
    await expect(footer(page).getByRole('button', { name: '暂停', exact: true })).toBeVisible()
    expect(await page.evaluate(() => Reflect.get(window, 'playerTest').prepares)).toEqual(['Song0', 'Song1'])
    expect(await page.evaluate(() => Reflect.get(window, 'testAudios').length)).toBe(0)
  })
}

test('progress hit area and drag agree with the confirmed libmpv bridge position', async ({ page }) => {
  await setup(page)
  await search(page)
  await page.getByRole('button', { name: '播放 第一首', exact: true }).click()
  await expect(footer(page).getByRole('button', { name: '暂停', exact: true })).toBeVisible()
  await footer(page).getByRole('button', { name: '暂停', exact: true }).click()
  const thumb = footer(page).getByRole('slider', { name: '播放进度' })
  const track = thumb.locator('xpath=ancestor::*[@data-slot="slider"]')
  const box = (await track.boundingBox())!
  expect(box.height).toBeGreaterThanOrEqual(24)
  await page.mouse.move(box.x + box.width * 0.6, box.y + 2)
  await expect(track).toHaveCSS('cursor', 'pointer')
  await page.mouse.down()
  await page.mouse.move(box.x + box.width * 0.75, box.y + 2, { steps: 5 })
  await page.mouse.up()
  await expect.poll(() => nativePosition(page)).toBeGreaterThan(4)
  await expect
    .poll(async () => Math.abs(Number(await thumb.getAttribute('aria-valuenow')) - (await nativePosition(page))))
    .toBeLessThan(0.15)
  await page.mouse.click(box.x + box.width * 0.25, box.y + box.height - 2)
  await expect.poll(() => nativePosition(page)).toBeLessThan(2)
  await expect
    .poll(async () => Math.abs(Number(await thumb.getAttribute('aria-valuenow')) - (await nativePosition(page))))
    .toBeLessThan(0.15)
})

test('far-right click and endpoint keyboard seek stay paused on the current song', async ({ page }) => {
  await setup(page)
  await search(page)
  await page.getByRole('button', { name: '播放 第一首', exact: true }).click()
  await expect(footer(page).getByRole('button', { name: '暂停', exact: true })).toBeVisible()
  await footer(page).getByRole('button', { name: '暂停', exact: true }).click()
  const thumb = footer(page).getByRole('slider', { name: '播放进度' })
  const box = (await thumb.locator('xpath=ancestor::*[@data-slot="slider"]').boundingBox())!
  await page.mouse.click(box.x + box.width - 1, box.y + box.height / 2)
  await expect.poll(() => nativePosition(page)).toBeGreaterThan(5)
  await thumb.focus()
  await thumb.press('End')
  await expect.poll(() => nativePosition(page)).toBeGreaterThan(5.9)
  await page.waitForTimeout(300)
  await expect(footer(page).getByText('第一首', { exact: true })).toBeVisible()
  await expect(footer(page).getByRole('button', { name: '播放', exact: true })).toBeVisible()
  expect(await page.evaluate(() => Reflect.get(window, 'playerTest').prepares)).toEqual(['Song0'])
})

test('mute restores volume and feedback stays percentage rather than save status', async ({ page }) => {
  await setup(page)
  await search(page)
  await page.getByRole('button', { name: '播放 第一首', exact: true }).click()
  await expect(footer(page).getByRole('button', { name: '暂停', exact: true })).toBeVisible()
  const volume = footer(page).getByRole('slider', { name: '音量' })
  await volume.focus()
  await volume.press('Home')
  for (let i = 0; i < 37; i++) await volume.press('ArrowRight')
  await expect(volume).toHaveAttribute('aria-valuetext', '37%')
  await expect(page.getByText('音量 37%', { exact: true })).toBeVisible()
  await footer(page).getByRole('button', { name: '静音', exact: true }).click()
  await expect(page.getByText('已静音', { exact: true })).toBeVisible()
  await expect.poll(() => nativeVolume(page)).toBe(0)
  await footer(page).getByRole('button', { name: '恢复音量', exact: true }).click()
  await expect(volume).toHaveAttribute('aria-valuetext', '37%')
  await expect.poll(() => nativeVolume(page)).toBe(0.37)
  await page.waitForTimeout(1100)
  await expect(page.getByText('设置已保存', { exact: true })).toHaveCount(0)
})

test('reload restores song and progress without autoplay or re-resolving', async ({ page }) => {
  await setup(page)
  await search(page)
  await page.getByRole('button', { name: '播放 第一首', exact: true }).click()
  await expect(footer(page).getByRole('button', { name: '暂停', exact: true })).toBeVisible()
  await footer(page).getByRole('button', { name: '暂停', exact: true }).click()
  await expect.poll(() => page.evaluate(() => Reflect.get(window, 'playerTest').saves)).toBeGreaterThan(1)
  await page.reload()
  await expect(footer(page).getByText('第一首', { exact: true })).toBeVisible()
  await expect(footer(page).getByRole('button', { name: '播放', exact: true })).toBeVisible()
  expect(await page.evaluate(() => Reflect.get(window, 'playerTest').prepares)).toEqual([])
})

test('canceled preparations and source errors never pretend to play', async ({ page }) => {
  await setup(page)
  await search(page)
  await page.evaluate(() => {
    Reflect.get(window, 'playerTest').mode = 'pending'
  })
  await page.getByRole('button', { name: '播放 第一首', exact: true }).click()
  await footer(page).getByRole('button', { name: '取消播放', exact: true }).click()
  await page.evaluate(() => Reflect.get(window, 'playerTest').pending[0]())
  await expect(footer(page).getByRole('button', { name: '播放', exact: true })).toBeVisible()
  await page.evaluate(() => {
    Reflect.get(window, 'playerTest').mode = 'error'
  })
  await page.getByRole('button', { name: '播放 第二首', exact: true }).click()
  await expect(footer(page).getByText('请先启用音源', { exact: true })).toBeVisible()
  await expect(footer(page).getByRole('button', { name: '暂停', exact: true })).toHaveCount(0)
})

async function details(page: Page) {
  await search(page)
  await page.getByRole('button', { name: '播放 第一首', exact: true }).click()
  await expect(footer(page).getByRole('button', { name: '暂停', exact: true })).toBeVisible()
  await footer(page).getByRole('button', { name: '暂停', exact: true }).click()
  await footer(page).getByRole('button', { name: '打开播放详情', exact: true }).click()
  await expect(page.getByRole('heading', { name: '第一首', exact: true })).toBeVisible()
}

test('details follows confirmed lyrics, seeks while paused, escapes text and returns without reparsing', async ({
  page
}) => {
  await setup(page)
  await details(page)
  const region = page.getByRole('region', { name: '歌词', exact: true })
  await expect(region.getByRole('button', { name: /Song0 开始/ })).toHaveAttribute('aria-current', 'true')
  await region.getByRole('button', { name: /点击这一行/ }).click()
  await expect.poll(() => nativePosition(page)).toBe(2)
  await expect(region.getByRole('button', { name: /点击这一行/ })).toHaveAttribute('aria-current', 'true')
  await expect(footer(page).getByRole('button', { name: '播放', exact: true })).toBeVisible()
  await expect(region.locator('img')).toHaveCount(0)
  await page.screenshot({ path: 'test-results/now-playing.png' })
  await page.getByRole('button', { name: '返回', exact: true }).click()
  await expect(page.getByRole('table')).toBeVisible()
  expect(await page.evaluate(() => Reflect.get(window, 'playerTest').prepares)).toEqual(['Song0'])
})

test('details changes with the queue and manual lyric scrolling can resume following', async ({ page }) => {
  await setup(page)
  await details(page)
  const region = page.getByRole('region', { name: '歌词', exact: true })
  await region.hover()
  await page.mouse.wheel(0, 160)
  await expect(page.getByRole('button', { name: '回到当前歌词', exact: true })).toBeVisible()
  await page.getByRole('button', { name: '回到当前歌词', exact: true }).click()
  await expect(page.getByRole('button', { name: '回到当前歌词', exact: true })).toHaveCount(0)
  await footer(page).getByRole('button', { name: '下一首', exact: true }).click()
  await expect(page.getByRole('heading', { name: '第二首', exact: true })).toBeVisible()
  await expect(region.getByRole('button', { name: /Song1 开始/ })).toBeVisible()
})

test('details fits compact windows and respects reduced motion', async ({ page }) => {
  await page.setViewportSize({ width: 720, height: 600 })
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await setup(page)
  await details(page)
  const region = page.getByRole('region', { name: '歌词', exact: true })
  await expect(region).toBeVisible()
  await expect(footer(page)).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await expect(region.getByRole('button', { name: /点击这一行/ })).toHaveCSS('transition-duration', '0s')
  await page.screenshot({ path: 'test-results/now-playing-compact.png' })
  await page.setViewportSize({ width: 1024, height: 600 })
  await expect(page.getByRole('heading', { name: '第一首', exact: true })).toBeVisible()
  const lyricsBox = (await region.boundingBox())!
  const footerBox = (await footer(page).boundingBox())!
  expect(lyricsBox.y + lyricsBox.height).toBeLessThanOrEqual(footerBox.y)
})

test('restored paused session allows progress and lyric selection without preparing or autoplay', async ({ page }) => {
  await setup(page)
  await details(page)
  await page.reload()
  const progress = footer(page).getByRole('slider', { name: '播放进度' })
  await expect(progress).toBeEnabled()
  await progress.focus()
  await progress.press('Home')
  await progress.press('ArrowRight')
  await expect.poll(async () => Number(await progress.getAttribute('aria-valuenow'))).toBeGreaterThan(0)
  const track = (await progress.locator('xpath=ancestor::*[@data-slot="slider"]').boundingBox())!
  await page.mouse.click(track.x + track.width * 0.5, track.y + track.height / 2)
  await expect.poll(async () => Number(await progress.getAttribute('aria-valuenow'))).toBeGreaterThan(2)
  await footer(page).getByRole('button', { name: '打开播放详情', exact: true }).click()
  await page
    .getByRole('region', { name: '歌词', exact: true })
    .getByRole('button', { name: /点击这一行/ })
    .click()
  await expect(progress).toHaveAttribute('aria-valuenow', '2')
  await expect(footer(page).getByRole('button', { name: '播放', exact: true })).toBeVisible()
  expect(await page.evaluate(() => Reflect.get(window, 'playerTest').prepares)).toEqual([])
  await footer(page).getByRole('button', { name: '播放', exact: true }).click()
  await expect.poll(() => nativePosition(page)).toBeGreaterThanOrEqual(2)
})

test('long lyrics stay scrollable through playback ticks and follow the active line', async ({ page }) => {
  await setup(page)
  await page.evaluate(() => {
    Reflect.get(window, 'playerTest').lyricMode = 'long'
  })
  await details(page)
  const region = page.getByRole('region', { name: '歌词', exact: true })
  expect(await region.evaluate((element) => element.scrollHeight > element.clientHeight + 1000)).toBe(true)
  await region.hover()
  await page.mouse.wheel(0, 900)
  await expect.poll(() => region.evaluate((element) => element.scrollTop)).toBeGreaterThan(300)
  const before = await region.evaluate((element) => element.scrollTop)
  await page.waitForTimeout(450)
  expect(Math.abs((await region.evaluate((element) => element.scrollTop)) - before)).toBeLessThan(5)
  await footer(page).getByRole('button', { name: '播放', exact: true }).click()
  await page.waitForTimeout(450)
  expect(Math.abs((await region.evaluate((element) => element.scrollTop)) - before)).toBeLessThan(5)
  await region.hover()
  await page.mouse.wheel(0, 400)
  await expect.poll(() => region.evaluate((element) => element.scrollTop)).toBeGreaterThan(before + 200)
  await footer(page).getByRole('button', { name: '暂停', exact: true }).click()
  await page.evaluate(() => {
    Reflect.get(window, 'playerTest').native.position = 4
  })
  await expect(region.locator('[aria-current="true"]')).toHaveAttribute('data-lyric-index', '80')
  await page.getByRole('button', { name: '回到当前歌词', exact: true }).click()
  await expect
    .poll(async () => {
      const pane = (await region.boundingBox())!
      const active = (await region.locator('[aria-current="true"]').boundingBox())!
      return Math.abs(active.y + active.height / 2 - pane.y - pane.height / 2)
    })
    .toBeLessThan(10)
})

test('long lyrics automatically follow forward and backward without manual intervention', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'no-preference' })
  await setup(page)
  await page.evaluate(() => {
    Reflect.get(window, 'playerTest').lyricMode = 'long'
  })
  await details(page)
  const region = page.getByRole('region', { name: '歌词', exact: true })
  for (const [position, index] of [
    [4, '80'],
    [0.5, '10']
  ] as const) {
    await page.evaluate((position) => {
      Reflect.get(window, 'playerTest').native.position = position
    }, position)
    await expect(region.locator('[aria-current="true"]')).toHaveAttribute('data-lyric-index', index)
    await expect
      .poll(async () => {
        const pane = (await region.boundingBox())!
        const active = (await region.locator('[aria-current="true"]').boundingBox())!
        return Math.abs(active.y + active.height / 2 - pane.y - pane.height / 2)
      })
      .toBeLessThan(10)
  }
})

test('previous switches after three seconds and both queue ends wrap', async ({ page }) => {
  await setup(page)
  await search(page)
  await page.getByRole('button', { name: '播放 第一首', exact: true }).click()
  await expect(footer(page).getByRole('button', { name: '暂停', exact: true })).toBeVisible()
  await page.evaluate(() => {
    Reflect.get(window, 'playerTest').native.position = 4
  })
  await expect.poll(() => nativePosition(page)).toBeGreaterThan(3)
  await footer(page).getByRole('button', { name: '上一首', exact: true }).click()
  await expect(footer(page).getByText('第二首', { exact: true })).toBeVisible()
  await expect(footer(page).getByRole('button', { name: '下一首', exact: true })).toBeEnabled()
  await footer(page).getByRole('button', { name: '下一首', exact: true }).click()
  await expect(footer(page).getByText('第一首', { exact: true })).toBeVisible()
})

test('shuffle previous traverses history and is unavailable without history', async ({ page }) => {
  await setup(page)
  await search(page)
  await page.getByRole('button', { name: '播放 第一首', exact: true }).click()
  await expect(footer(page).getByRole('button', { name: '暂停', exact: true })).toBeVisible()
  await footer(page).getByRole('button', { name: '播放模式：顺序播放', exact: true }).click()
  await expect(footer(page).getByRole('button', { name: '上一首', exact: true })).toBeDisabled()
  await footer(page).getByRole('button', { name: '下一首', exact: true }).click()
  await expect(footer(page).getByText('第二首', { exact: true })).toBeVisible()
  await footer(page).getByRole('button', { name: '上一首', exact: true }).click()
  await expect(footer(page).getByText('第一首', { exact: true })).toBeVisible()
})

test('real quit signal flushes confirmed playback progress before acknowledging the host', async ({page}) => {
  await setup(page);await search(page)
  await page.getByRole('button',{name:'播放 第一首',exact:true}).click()
  await expect(footer(page).getByRole('button',{name:'暂停',exact:true})).toBeVisible()
  await page.evaluate(()=>Reflect.get(window,'playerTest').native.position=3)
  await expect.poll(()=>footer(page).getByRole('slider',{name:'播放进度'}).getAttribute('aria-valuenow')).toMatch(/^3/)
  const count=await page.evaluate(()=>Reflect.get(window,'playerTest').prepares.length)
  await page.evaluate(()=>Reflect.get(window,'playerTest').lifecycleEvents['app:before-quit']('9'))
  await expect.poll(()=>page.evaluate(()=>Reflect.get(window,'playerTest').quitAcks)).toEqual(['9'])
  const snapshot=await page.evaluate(()=>JSON.parse(localStorage.getItem('player-snapshot')!))
  expect(snapshot.position).toBeGreaterThanOrEqual(3);expect(snapshot.index).toBe(0);expect(snapshot.queue.length).toBe(2)
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').prepares.length)).toBe(count)
  await expect(footer(page).getByRole('button',{name:'播放',exact:true})).toBeVisible()
})

test('restored legacy artist names resolve only on click and persist credits without playback', async ({page}) => {
  await setup(page)
  await page.addInitScript(()=>{if(!localStorage.getItem('legacy-seeded')){localStorage.setItem('player-snapshot',JSON.stringify({queue:[{id:'Song0',source:'tx',songId:'123',mediaMid:'Media1',title:'旧歌曲',artist:'测试歌手',album:'测试专辑',coverUrl:'',duration:6}],index:0,position:2,mode:'sequential'}));localStorage.setItem('legacy-seeded','1')}})
  await page.reload()
  const artist=footer(page).getByRole('button',{name:'查看歌手 测试歌手',exact:true})
  await expect(artist).toBeVisible()
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').creditRequests)).toEqual([])
  await artist.click()
  await expect(page.locator('[data-music-detail="artist"] h1')).toHaveText('测试歌手')
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').creditRequests)).toEqual(['Song0'])
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').prepares)).toEqual([])
  await expect.poll(()=>page.evaluate(()=>JSON.parse(localStorage.getItem('player-snapshot')!).queue[0].artists?.[0]?.id)).toBe('Artist1')
  expect(await page.evaluate(()=>JSON.parse(localStorage.getItem('player-snapshot')!).position)).toBe(2)
  await page.reload();await expect(artist).toBeVisible();await artist.click()
  await expect(page.locator('[data-music-detail="artist"] h1')).toHaveText('测试歌手')
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').creditRequests)).toEqual([])
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').prepares)).toEqual([])
})

test('artist previews show twenty songs and ten albums with album credits and view-more controls', async ({page}) => {
  await setup(page);await search(page)
  await page.evaluate(()=>Reflect.get(window,'playerTest').artistTrackCount=45)
  await page.getByRole('button',{name:'查看歌手 测试歌手',exact:true}).first().click()
  const hot=page.getByRole('region',{name:'热门歌曲',exact:true}),albums=page.getByRole('region',{name:'歌手专辑',exact:true})
  await expect(hot.getByRole('table').locator('tbody tr')).toHaveCount(20)
  await expect(hot.getByRole('columnheader',{name:'专辑',exact:true})).toBeVisible()
  await expect(hot.getByRole('columnheader',{name:'歌手',exact:true})).toHaveCount(0)
  await expect(albums.getByRole('button',{name:/查看专辑 /})).toHaveCount(10)
  await expect(page.getByRole('button',{name:'查看专辑',exact:true})).toHaveCount(0)
  await hot.getByRole('button',{name:'查看更多',exact:true}).click()
  await expect(hot.getByRole('table').locator('tbody tr')).toHaveCount(45)
  await albums.getByRole('button',{name:'查看更多',exact:true}).click()
  await expect(albums.getByRole('button',{name:/查看专辑 /})).toHaveCount(20)
  await expect(page.getByRole('navigation',{name:'歌手专辑分页'})).toBeVisible()
  await hot.getByRole('button',{name:'查看专辑 测试专辑',exact:true}).first().click()
  await page.getByRole('navigation',{name:'浏览导航'}).getByRole('button',{name:'后退',exact:true}).click()
  await expect(hot.getByRole('table').locator('tbody tr')).toHaveCount(45)
  await expect(albums.getByRole('button',{name:/查看专辑 /})).toHaveCount(20)
  await page.setViewportSize({width:640,height:620})
  await hot.getByRole('button',{name:'查看专辑 测试专辑',exact:true}).first().scrollIntoViewIfNeeded()
  await expect(hot.getByRole('button',{name:'查看专辑 测试专辑',exact:true}).first()).toBeVisible()
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').prepares)).toEqual([])
})

test('artist and album details use real metadata actions without autoplay and restore browsing history', async ({page}) => {
  await setup(page);await search(page)
  const table=page.getByRole('table',{name:'搜索结果'})
  await table.getByRole('button',{name:'查看歌手 测试歌手',exact:true}).first().click()
  await expect(page.locator('[data-music-detail="artist"] h1')).toHaveText('测试歌手')
  await expect(page.locator('[data-music-detail]').getByRole('button',{name:'返回',exact:true})).toHaveCount(0)
  await expect(page.getByRole('heading',{name:'热门歌曲',exact:true})).toBeVisible()
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').prepares)).toEqual([])
  await expect(page.getByRole('button',{name:'查看专辑',exact:true})).toHaveCount(0)
  const albums=page.getByRole('region',{name:'歌手专辑',exact:true})
  await albums.getByRole('button',{name:'查看专辑 测试专辑',exact:true}).click()
  await expect(page.locator('[data-music-detail="album"] h1')).toHaveText('测试专辑')
  await expect(page.locator('[data-music-detail]').getByRole('button',{name:'返回',exact:true})).toHaveCount(0)
  await expect(page.getByText('2026-01-01 · 120 首',{exact:true})).toBeVisible()
  await page.getByRole('navigation',{name:'详情曲目分页'}).getByRole('button',{name:'下一页',exact:true}).click()
  await expect(page.getByRole('button',{name:'播放 专辑曲目50',exact:true})).toBeVisible()
  await page.getByRole('button',{name:'收藏 专辑曲目50',exact:true}).click()
  await page.getByRole('button',{name:'下一首播放 专辑曲目50',exact:true}).click()
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').prepares)).toEqual([])
  await page.getByRole('button',{name:'下载全部',exact:true}).click()
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').downloadTasks.length)).toBe(119)
  await page.getByRole('button',{name:'播放全部',exact:true}).click()
  await expect.poll(()=>page.evaluate(()=>Reflect.get(window,'playerTest').prepares.at(-1))).toBe('AlbumTrack0')
  expect(await page.evaluate(()=>JSON.parse(localStorage.getItem('player-snapshot')!).queue.length)).toBe(120)
  const nav=page.getByRole('navigation',{name:'浏览导航'})
  await nav.getByRole('button',{name:'后退',exact:true}).click()
  await expect(page.getByRole('button',{name:'播放 专辑曲目0',exact:true})).toBeVisible()
  await nav.getByRole('button',{name:'后退',exact:true}).click()
  await expect(page.locator('[data-music-detail="artist"] h1')).toHaveText('测试歌手')
  await nav.getByRole('button',{name:'前进',exact:true}).click()
  await nav.getByRole('button',{name:'前进',exact:true}).click()
  await expect(page.getByRole('button',{name:'播放 专辑曲目50',exact:true})).toBeVisible()
  await page.screenshot({path:'test-results/album-detail.png'})
})

test('artist album pagination, refresh, error retry and compact multiple-artist choice work', async ({page}) => {
  await setup(page);await search(page)
  await page.getByRole('button',{name:'查看歌手 测试歌手',exact:true}).first().click()
  await page.getByRole('region',{name:'歌手专辑',exact:true}).getByRole('button',{name:'查看更多',exact:true}).click()
  await page.getByRole('navigation',{name:'歌手专辑分页'}).getByRole('button',{name:'下一页',exact:true}).click()
  await expect(page.getByRole('button',{name:'查看专辑 后续专辑',exact:true})).toBeVisible()
  const nav=page.getByRole('navigation',{name:'浏览导航'})
  const calls=await page.evaluate(()=>Reflect.get(window,'playerTest').detailRequests.length)
  await nav.getByRole('button',{name:'刷新',exact:true}).click()
  await expect.poll(()=>page.evaluate(()=>Reflect.get(window,'playerTest').detailRequests.length)).toBe(calls+1)
  await page.evaluate(()=>Reflect.get(window,'playerTest').detailMode='error')
  await nav.getByRole('button',{name:'刷新',exact:true}).click()
  await expect(page.getByText('暂时无法读取详情',{exact:true})).toBeVisible()
  await page.evaluate(()=>Reflect.get(window,'playerTest').detailMode='normal')
  await page.getByRole('button',{name:'重试详情',exact:true}).click()
  await expect(page.locator('[data-music-detail="artist"] h1')).toHaveText('测试歌手')
  await page.screenshot({path:'test-results/artist-detail.png'})
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').prepares)).toEqual([])
  await page.setViewportSize({width:640,height:620});await page.emulateMedia({reducedMotion:'reduce'})
  await page.evaluate(()=>{const song=Reflect.get(window,'playerTest').testSongs[0];song.artist='测试歌手 / 合唱歌手';song.artists=[{id:'Artist1',name:'测试歌手'},{id:'Artist2',name:'合唱歌手'}]})
  await search(page)
  await page.getByRole('button',{name:'选择歌手 测试歌手 / 合唱歌手',exact:true}).first().click()
  await page.getByRole('button',{name:'合唱歌手',exact:true}).click()
  await expect(page.locator('[data-music-detail="artist"] h1')).toHaveText('合唱歌手')
  await search(page)
  await page.evaluate(()=>{const state=Reflect.get(window,'playerTest');state.detailMode='empty'})
  await page.getByRole('button',{name:'查看专辑 测试专辑',exact:true}).first().click()
  await expect(page.getByText('暂无曲目',{exact:true})).toBeVisible()
  await expect(page.getByRole('button',{name:'播放全部',exact:true})).toBeDisabled()
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
  await page.screenshot({path:'test-results/album-detail-compact.png'})
})

test('late detail replies cannot undo navigation or replace another entity', async ({page}) => {
  await setup(page);await search(page)
  await page.evaluate(()=>Reflect.get(window,'playerTest').detailMode='pending')
  await page.getByRole('button',{name:'查看歌手 测试歌手',exact:true}).first().click()
  await expect(page.getByRole('status',{name:'正在读取音乐详情'})).toBeVisible()
  await page.getByRole('navigation',{name:'浏览导航'}).getByRole('button',{name:'后退',exact:true}).click()
  await page.evaluate(()=>Reflect.get(window,'playerTest').detailMode='normal')
  await page.getByRole('button',{name:'查看专辑 测试专辑',exact:true}).first().click()
  await page.evaluate(()=>Reflect.get(window,'playerTest').detailPending.splice(0).forEach((finish:()=>void)=>finish()))
  await expect(page.locator('[data-music-detail="album"] h1')).toHaveText('测试专辑')
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').prepares)).toEqual([])
})

test('persisted recommendations remain usable after reload and failed catalog or collection refresh', async ({page}) => {
  await setup(page)
  await page.evaluate(() => {
    const entry={kind:'chart',id:1,title:'缓存榜单',group:'缓存分组',coverUrl:'',period:'2026_43',updated:'',total:1}
    const song={...Reflect.get(window,'playerTest').testSongs[0],id:'CachedSong',title:'缓存曲目'}
    localStorage.setItem('discovery-cache-fixture',JSON.stringify({'chart:0:':{savedAt:Date.now()-600000,catalog:{kind:'chart',entries:[entry]}},'chart:1:2026_43':{savedAt:Date.now()-600000,collection:{entry,songs:[song]}}}))
    localStorage.setItem('discovery-fixture-mode','error')
  })
  await page.reload()
  await expect(page.getByRole('button',{name:'查看 缓存榜单',exact:true})).toBeVisible()
  await expect(page.getByText('更新未完成，已保留上次内容')).toBeVisible()
  await page.getByRole('button',{name:'查看 缓存榜单',exact:true}).click()
  await expect(page.getByRole('table')).toContainText('缓存曲目')
  await expect(page.getByText('更新未完成，已保留上次内容')).toBeVisible()
  await expect(page.getByRole('button',{name:'播放全部',exact:true})).toBeEnabled()
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').prepares)).toEqual([])
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').downloadEnqueues)).toBe(0)
  await page.screenshot({path:'test-results/discovery-cache-refresh-error.png'})
  await page.setViewportSize({width:640,height:620})
  expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(640)
  await page.screenshot({path:'test-results/discovery-cache-refresh-error-compact.png'})
  await page.evaluate(()=>Reflect.get(window,'playerTest').discoveryMode='normal')
  await page.getByRole('button',{name:'重试更新',exact:true}).click()
  await expect(page.getByRole('table')).toContainText('推荐歌曲0')
  await expect(page.getByText('更新未完成，已保留上次内容')).toHaveCount(0)
})

test('fresh persisted recommendation catalog avoids an automatic QQ request but explicit refresh requests once', async ({page}) => {
  await setup(page)
  await page.evaluate(()=>localStorage.setItem('discovery-cache-fixture',JSON.stringify({'chart:0:':{savedAt:Date.now(),catalog:{kind:'chart',entries:[{kind:'chart',id:1,title:'缓存榜单',group:'缓存分组',coverUrl:'',period:'2026_43',updated:'',total:1}]}}})))
  await page.reload()
  await expect(page.getByRole('button',{name:'查看 缓存榜单',exact:true})).toBeVisible()
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').discoveryCalls)).toEqual([])
  await page.getByRole('button',{name:'刷新',exact:true}).click()
  await expect(page.getByRole('button',{name:'查看 榜单1',exact:true})).toBeVisible()
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').discoveryCalls)).toEqual(['catalog:chart'])
})

test('recommendation adjustments persist group and chart visibility and both sorting levels', async ({page}) => {
  await setup(page)
  await page.getByRole('button',{name:'调整',exact:true}).click()
  const panel=page.getByRole('dialog',{name:'调整推荐',exact:true})
  await panel.getByRole('button',{name:'显示榜单 榜单1',exact:true}).click()
  await panel.getByRole('button',{name:'显示分组 巅峰榜',exact:true}).click()
  await panel.getByRole('button',{name:'上移分组 地区榜',exact:true}).click()
  await panel.getByRole('button',{name:'上移榜单 榜单8',exact:true}).click()
  await expect.poll(()=>page.evaluate(()=>JSON.parse(localStorage.getItem('discovery-layout')||'{}').groups?.[0]?.charts?.[0]?.id)).toBe(8)
  await page.screenshot({path:'test-results/discovery-adjustments.png'})
  await page.keyboard.press('Escape')
  await expect(page.getByRole('button',{name:'调整',exact:true})).toBeFocused()
  await expect(page.locator('[data-discovery-group]').first()).toHaveAttribute('aria-label','地区榜')
  await expect(page.getByRole('button',{name:'查看 榜单1',exact:true})).toHaveCount(0)
  await page.reload()
  await expect(page.locator('[data-discovery-group]').first()).toHaveAttribute('aria-label','地区榜')
  await page.getByRole('button',{name:'调整',exact:true}).click()
  await expect(panel.getByRole('button',{name:'显示榜单 榜单1',exact:true})).toHaveAttribute('aria-pressed','false')
  await panel.getByRole('button',{name:'显示分组 巅峰榜',exact:true}).click()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('button',{name:'查看 榜单2',exact:true})).toBeVisible()
  await expect(page.getByRole('button',{name:'查看 榜单1',exact:true})).toHaveCount(0)
  await page.getByRole('button',{name:'调整',exact:true}).click()
  await panel.getByRole('button',{name:'恢复默认',exact:true}).click()
  await page.getByRole('alertdialog').getByRole('button',{name:'恢复默认',exact:true}).click()
  await expect.poll(()=>page.evaluate(()=>JSON.parse(localStorage.getItem('discovery-layout')||'{}').groups?.length)).toBe(0)
  await page.keyboard.press('Escape')
  await expect(page.locator('[data-discovery-group]').first()).toHaveAttribute('aria-label','巅峰榜')
  await expect(page.getByRole('button',{name:'查看 榜单1',exact:true})).toBeVisible()
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').prepares)).toEqual([])
})

test('recommendation adjustments support scoped dragging and keyboard sorting', async ({page}) => {
  await setup(page);await page.getByRole('button',{name:'调整',exact:true}).click()
  const panel=page.getByRole('dialog',{name:'调整推荐',exact:true})
  await expect(panel.getByRole('heading',{name:'调整推荐',exact:true})).toBeFocused()
  await expect(panel.locator('.sort-controls').first()).toHaveCSS('opacity','0')
  await panel.getByRole('button',{name:'拖动榜单 榜单2',exact:true}).dragTo(panel.locator('[data-discovery-adjust-chart="4"]'))
  await expect.poll(()=>panel.locator('[data-discovery-adjust-group="name:巅峰榜"] [data-discovery-adjust-chart]').evaluateAll(rows=>rows.map(row=>row.getAttribute('data-discovery-adjust-chart')))).toEqual(['1','3','4','2','5','6'])
  await expect(panel.getByRole('checkbox')).toHaveCount(0)
  const before=await panel.locator('[data-discovery-adjust-chart]').evaluateAll(rows=>rows.map(row=>row.getAttribute('data-discovery-adjust-chart')))
  await panel.getByRole('button',{name:'拖动榜单 榜单1',exact:true}).dragTo(panel.locator('[data-discovery-adjust-chart="7"]'))
  expect(await panel.locator('[data-discovery-adjust-chart]').evaluateAll(rows=>rows.map(row=>row.getAttribute('data-discovery-adjust-chart')))).toEqual(before)
  await panel.locator('[data-discovery-adjust-group="name:巅峰榜"] h3').hover()
  await panel.getByRole('button',{name:'拖动分组 巅峰榜',exact:true}).dragTo(panel.locator('[data-discovery-adjust-group="name:地区榜"] .editing-row').first())
  await expect(panel.locator('[data-discovery-adjust-group]').first()).toHaveAttribute('data-discovery-adjust-group','name:地区榜')
  const move=panel.getByRole('button',{name:'下移分组 地区榜',exact:true});await move.focus()
  await expect(panel.locator('[data-discovery-adjust-group="name:地区榜"] .sort-controls').first()).toHaveCSS('opacity','1')
  await page.keyboard.press('Enter')
  await expect(panel.locator('[data-discovery-adjust-group]').first()).toHaveAttribute('data-discovery-adjust-group','name:巅峰榜')
})

test('all hidden recommendations remain adjustable and failed saves retain the unsaved draft', async ({page}) => {
  await page.setViewportSize({width:640,height:620});await page.emulateMedia({reducedMotion:'reduce'});await setup(page)
  await page.getByRole('button',{name:'调整',exact:true}).click()
  const panel=page.getByRole('dialog',{name:'调整推荐',exact:true})
  for(const group of ['巅峰榜','地区榜','全球榜'])await panel.getByRole('button',{name:`显示分组 ${group}`,exact:true}).click()
  await page.keyboard.press('Escape')
  await expect(page.getByText('还没有显示的榜单',{exact:true})).toBeVisible()
  await page.getByRole('button',{name:'调整',exact:true}).click()
  await page.evaluate(()=>Reflect.get(window,'playerTest').layoutMode='error')
  await panel.getByRole('button',{name:'显示分组 巅峰榜',exact:true}).click()
  await expect(panel.getByText('尚未保存',{exact:true})).toBeVisible()
  await expect(panel.getByRole('button',{name:'显示分组 巅峰榜',exact:true})).toHaveAttribute('aria-pressed','true')
  await page.evaluate(()=>Reflect.get(window,'playerTest').layoutMode='normal')
  await panel.getByRole('button',{name:'重试保存布局',exact:true}).click()
  await expect(panel.getByText('尚未保存',{exact:true})).toHaveCount(0)
  await expect(panel.getByText('已自动保存',{exact:true})).toHaveCount(0)
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
  await page.screenshot({path:'test-results/discovery-adjustments-compact.png'})
})

test('recommendation layout read failures can retry without overwriting persisted choices', async ({page}) => {
  await setup(page)
  await page.evaluate(()=>{localStorage.setItem('layout-read-error','1');localStorage.setItem('discovery-layout',JSON.stringify({groups:[{key:'name:巅峰榜',hidden:true,charts:[]}]}))})
  await page.reload()
  await expect(page.getByText('无法读取推荐布局，请重试',{exact:true})).toBeVisible()
  await expect(page.getByRole('button',{name:'调整',exact:true})).toBeDisabled()
  await page.evaluate(()=>localStorage.removeItem('layout-read-error'))
  await page.getByRole('button',{name:'重试读取布局',exact:true}).click()
  await expect(page.getByRole('button',{name:'查看 榜单1',exact:true})).toHaveCount(0)
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').layoutWrites.length)).toBe(0)
})

test('toolbar history tracks visits, ignores search drafts and truncates forward history', async ({page}) => {
  await setup(page)
  const nav=page.getByRole('navigation',{name:'浏览导航'}), sidebar=page.getByRole('navigation',{name:'主导航'})
  await expect(nav.getByRole('button',{name:'后退',exact:true})).toBeDisabled()
  await expect(nav.getByRole('button',{name:'前进',exact:true})).toBeDisabled()
  await page.getByLabel('歌曲、歌手或专辑').fill('仅输入')
  await expect(nav.getByRole('button',{name:'后退',exact:true})).toBeDisabled()
  await sidebar.getByRole('button',{name:'喜欢',exact:true}).click()
  await expect(page.getByText('还没有收藏音乐',{exact:true})).toBeVisible()
  await sidebar.getByRole('button',{name:'下载',exact:true}).click()
  await expect(page.getByRole('heading',{name:'下载',exact:true})).toBeVisible()
  await nav.getByRole('button',{name:'后退',exact:true}).click()
  await expect(page.getByText('还没有收藏音乐',{exact:true})).toBeVisible()
  await nav.getByRole('button',{name:'前进',exact:true}).click()
  await expect(page.getByRole('heading',{name:'下载',exact:true})).toBeVisible()
  await nav.getByRole('button',{name:'后退',exact:true}).click()
  await sidebar.getByRole('button',{name:'设置',exact:true}).click()
  await expect(nav.getByRole('button',{name:'前进',exact:true})).toBeDisabled()
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').prepares)).toEqual([])
})

test('toolbar restores chart pagination and catalog scroll without playback', async ({page}) => {
  await setup(page)
  const nav=page.getByRole('navigation',{name:'浏览导航'}),entry=page.getByRole('button',{name:'查看 榜单18',exact:true})
  await entry.scrollIntoViewIfNeeded()
  const scroll=await page.locator('[data-content-scroll]').evaluate(element=>element.scrollTop)
  await entry.click()
  await page.getByRole('navigation',{name:'推荐曲目分页'}).getByRole('button',{name:'下一页',exact:true}).click()
  await expect(page.getByRole('button',{name:'播放 推荐歌曲50',exact:true})).toBeVisible()
  await nav.getByRole('button',{name:'后退',exact:true}).click()
  await expect(page.getByRole('button',{name:'播放 推荐歌曲0',exact:true})).toBeVisible()
  await nav.getByRole('button',{name:'后退',exact:true}).click()
  await expect(entry).toBeVisible()
  await expect.poll(()=>page.locator('[data-content-scroll]').evaluate(element=>element.scrollTop)).toBeGreaterThanOrEqual(scroll-5)
  await nav.getByRole('button',{name:'前进',exact:true}).click()
  await nav.getByRole('button',{name:'前进',exact:true}).click()
  await expect(page.getByRole('button',{name:'播放 推荐歌曲50',exact:true})).toBeVisible()
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').prepares)).toEqual([])
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').discoveryCalls.filter((call:string)=>call.startsWith('collection:')).length)).toBe(1)
})

test('toolbar can leave a loading collection and late replies do not reset restored scroll', async ({page}) => {
  await setup(page)
  const entry=page.getByRole('button',{name:'查看 榜单18',exact:true}),nav=page.getByRole('navigation',{name:'浏览导航'})
  await entry.scrollIntoViewIfNeeded()
  const before=await page.locator('[data-content-scroll]').evaluate(element=>element.scrollTop)
  await page.evaluate(()=>Reflect.get(window,'playerTest').discoveryMode='pending')
  await entry.click()
  await expect(page.getByRole('status',{name:'正在读取 QQ 音乐推荐'})).toBeVisible()
  await nav.getByRole('button',{name:'后退',exact:true}).click()
  await expect(entry).toBeVisible()
  await nav.getByRole('button',{name:'前进',exact:true}).click()
  await expect(page.getByRole('status',{name:'正在读取 QQ 音乐推荐'})).toBeVisible()
  await expect(nav.getByRole('button',{name:'后退',exact:true})).toBeEnabled()
  await nav.getByRole('button',{name:'后退',exact:true}).click()
  await expect(entry).toBeVisible()
  await page.evaluate(()=>Reflect.get(window,'playerTest').discoveryPending.splice(0).forEach((finish:()=>void)=>finish()))
  await expect.poll(()=>page.locator('[data-content-scroll]').evaluate(element=>element.scrollTop)).toBeGreaterThanOrEqual(before-5)
  await expect(page.getByRole('table',{name:'推荐曲目'})).toHaveCount(0)
})

test('toolbar restores submitted searches and refreshes metadata without restarting music', async ({page}) => {
  await setup(page);await search(page)
  await page.getByRole('button',{name:'播放 第一首',exact:true}).click()
  await expect(footer(page).getByRole('button',{name:'暂停',exact:true})).toBeVisible()
  const nav=page.getByRole('navigation',{name:'浏览导航'})
  await page.getByLabel('歌曲、歌手或专辑').fill('第二次搜索');await page.getByLabel('歌曲、歌手或专辑').press('Enter')
  await expect(page.getByRole('heading',{name:'「第二次搜索」的搜索结果',exact:true})).toBeVisible()
  await nav.getByRole('button',{name:'后退',exact:true}).click()
  await expect(page.getByRole('heading',{name:'「测试」的搜索结果',exact:true})).toBeVisible()
  await expect(page.getByLabel('歌曲、歌手或专辑')).toHaveValue('测试')
  await nav.getByRole('button',{name:'前进',exact:true}).click()
  await expect(page.getByLabel('歌曲、歌手或专辑')).toHaveValue('第二次搜索')
  const prepares=await page.evaluate(()=>Reflect.get(window,'playerTest').prepares.slice())
  await nav.getByRole('button',{name:'刷新',exact:true}).click()
  await footer(page).getByRole('button',{name:'打开播放详情',exact:true}).click()
  await expect(page.getByRole('region',{name:'歌词',exact:true})).toBeVisible()
  const count=await page.evaluate(()=>Reflect.get(window,'playerTest').lyricCalls.length)
  await nav.getByRole('button',{name:'刷新',exact:true}).click()
  await expect.poll(()=>page.evaluate(()=>Reflect.get(window,'playerTest').lyricCalls.length)).toBe(count+1)
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').prepares)).toEqual(prepares)
  await expect(footer(page).getByRole('button',{name:'暂停',exact:true})).toBeVisible()
  await page.setViewportSize({width:640,height:620})
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
  await page.screenshot({path:'test-results/navigation-toolbar-compact.png'})
})

test('discovery directly shows chart entries without category tabs or source shortcut', async ({page}) => {
  await setup(page)
  await expect(page.getByRole('button',{name:'查看 榜单1',exact:true})).toBeVisible()
  await expect(page.getByRole('table',{name:'推荐曲目'})).toHaveCount(0)
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').prepares)).toEqual([])
  await expect(page.getByRole('tablist',{name:'推荐类型'})).toHaveCount(0)
  await expect(page.getByRole('button',{name:'配置音源',exact:true})).toHaveCount(0)
  await page.getByRole('button',{name:'查看 榜单1',exact:true}).click()
  await expect(page.getByRole('heading',{name:'榜单1',exact:true})).toBeVisible()
  await expect(page.getByRole('table',{name:'推荐曲目'}).locator('tbody tr')).toHaveCount(50)
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').prepares)).toEqual([])
})

test('discovery pagination plays and batch downloads the entire collection, not only the visible page', async ({page}) => {
  await setup(page)
  await page.getByRole('button',{name:'查看 榜单1',exact:true}).click()
  const table=page.getByRole('table',{name:'推荐曲目'})
  await expect(table.locator('tbody tr')).toHaveCount(50)
  await page.getByRole('navigation',{name:'推荐曲目分页'}).getByRole('button',{name:'下一页',exact:true}).click()
  await expect(table.getByRole('button',{name:'播放 推荐歌曲50',exact:true})).toBeVisible()
  await table.getByRole('button',{name:'收藏 推荐歌曲50',exact:true}).click()
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').prepares)).toEqual([])
  await page.getByRole('button',{name:'下载全部',exact:true}).click()
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').downloadTasks.length)).toBe(120)
  await page.getByRole('button',{name:'播放全部',exact:true}).click()
  await expect.poll(()=>page.evaluate(()=>Reflect.get(window,'playerTest').prepares)).toEqual(['Discover1Song0'])
  expect(await page.evaluate(()=>JSON.parse(localStorage.getItem('player-snapshot')!).queue.length)).toBe(120)
  await footer(page).getByRole('button',{name:'打开播放详情',exact:true}).click()
  await page.locator('[data-now-playing]').getByRole('button',{name:'返回',exact:true}).click()
  await expect(table.getByRole('button',{name:'播放 推荐歌曲50',exact:true})).toBeVisible()
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').discoveryCalls.filter((call:string)=>call.startsWith('collection:')).length)).toBe(1)
  await page.screenshot({path:'test-results/discovery-collection.png'})
})

test('discovery back restores catalog scroll and entry focus without refetching', async ({page}) => {
  await setup(page)
  const entry=page.getByRole('button',{name:'查看 榜单18',exact:true})
  await entry.scrollIntoViewIfNeeded()
  const before=await page.locator('[data-content-scroll]').evaluate(element=>element.scrollTop)
  expect(before).toBeGreaterThan(0)
  await entry.click()
  await expect(page.getByRole('table',{name:'推荐曲目'})).toBeVisible()
  await page.getByRole('button',{name:'返回推荐',exact:true}).click()
  await expect(entry).toBeFocused()
  await expect.poll(()=>page.locator('[data-content-scroll]').evaluate(element=>element.scrollTop)).toBeGreaterThanOrEqual(before-5)
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').discoveryCalls.filter((call:string)=>call==='catalog:chart').length)).toBe(1)
})

test('discovery errors are not empty catalogs and stale responses cannot undo navigation', async ({page}) => {
  await setup(page)
  await expect(page.getByRole('button',{name:'查看 榜单1',exact:true})).toBeVisible()
  await page.evaluate(()=>Reflect.get(window,'playerTest').discoveryMode='error')
  await page.getByRole('button',{name:'刷新',exact:true}).click()
  await expect(page.getByText('更新未完成，已保留上次内容',{exact:true})).toBeVisible()
  await expect(page.getByRole('button',{name:'查看 榜单1',exact:true})).toBeVisible()
  await expect(page.getByText('暂时没有推荐列表',{exact:true})).toHaveCount(0)
  await page.evaluate(()=>Reflect.get(window,'playerTest').discoveryMode='empty')
  await page.getByRole('button',{name:'重试更新',exact:true}).click()
  await expect(page.getByText('暂时没有推荐列表',{exact:true})).toBeVisible()
  await page.evaluate(()=>Reflect.get(window,'playerTest').discoveryMode='pending')
  await page.getByRole('button',{name:'刷新',exact:true}).click()
  await page.getByRole('navigation',{name:'主导航'}).getByRole('button',{name:'喜欢',exact:true}).click()
  await page.evaluate(()=>Reflect.get(window,'playerTest').discoveryMode='normal')
  await page.getByRole('navigation',{name:'主导航'}).getByRole('button',{name:'推荐',exact:true}).click()
  await page.getByRole('button',{name:'查看 榜单1',exact:true}).click()
  await page.evaluate(()=>Reflect.get(window,'playerTest').discoveryPending.splice(0).forEach((finish:()=>void)=>finish()))
  await expect(page.getByRole('heading',{name:'榜单1',exact:true})).toBeVisible()
  await expect(page.getByRole('table',{name:'推荐曲目'})).toBeVisible()
})

test('discovery fits compact reduced-motion windows and has keyboard entry activation', async ({page}) => {
  await setup(page)
  await page.setViewportSize({width:640,height:620})
  await page.emulateMedia({reducedMotion:'reduce'})
  const entry=page.getByRole('button',{name:'查看 榜单1',exact:true})
  await entry.focus();await page.keyboard.press('Enter')
  await expect(page.getByRole('heading',{name:'榜单1',exact:true})).toBeVisible()
  await expect(page.getByRole('table',{name:'推荐曲目'})).toBeVisible()
  expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(640)
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').prepares)).toEqual([])
  await page.getByRole('button',{name:'返回推荐',exact:true}).click()
  await page.screenshot({path:'test-results/discovery-compact.png'})
})

async function downloadsPage(page: Page) { await page.getByRole('navigation', {name:'主导航'}).getByRole('button',{name:'下载',exact:true}).click() }

test('existing download only shows a toast and does not add history', async ({page}) => {
  await setup(page);await search(page)
  const table=page.getByRole('table',{name:'QQ 音乐搜索结果'})
  await table.getByRole('button',{name:'下载 第一首',exact:true}).click()
  await page.evaluate(()=>{const task=Reflect.get(window,'playerTest').downloadTasks[0];task.state='success';task.quality='320k';task.total=1024;task.bytes=1024;task.file='song.mp3'})
  await table.getByRole('button',{name:'下载 第一首',exact:true}).click()
  await expect(page.getByText('下载目录中已有该歌曲，无需重复下载',{exact:true})).toBeVisible()
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').downloadTasks.length)).toBe(1)
  await downloadsPage(page)
  await expect(page.getByRole('table',{name:'下载任务'}).locator('tbody tr')).toHaveCount(1)
  await expect(page.getByText(/最近一批/)).toHaveCount(0)
  await expect(page.getByRole('progressbar',{name:'最近一批整体进度'})).toHaveCount(0)
})

test('resume preparation retains progress and uses an icon-only continue action', async ({page}) => {
  await setup(page);await search(page)
  await page.getByRole('table',{name:'QQ 音乐搜索结果'}).getByRole('button',{name:'下载 第一首',exact:true}).click()
  await page.evaluate(()=>{const task=Reflect.get(window,'playerTest').downloadTasks[0];task.state='paused';task.quality='320k';task.total=1024;task.bytes=512;task.message='等待手动继续'})
  await downloadsPage(page)
  await expect(page.getByText('等待手动继续',{exact:true})).toHaveCount(0)
  await page.getByRole('tab',{name:'下载中',exact:true}).click()
  const resume=page.getByRole('button',{name:'继续下载 第一首',exact:true})
  await expect(resume).toHaveText('')
  await expect(page.getByRole('progressbar',{name:'第一首 下载进度'})).toHaveAttribute('value','50')
  await resume.click()
  await expect(page.getByText('准备中',{exact:true})).toBeVisible()
  await expect(page.getByRole('progressbar',{name:'第一首 下载进度'})).toHaveAttribute('value','50')
  await page.setViewportSize({width:640,height:620})
  expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(640)
  await page.screenshot({path:'test-results/downloads-resume-compact.png'})
})

test('download list hides entity restart notices without hiding progress', async ({page}) => {
  await setup(page)
  await page.evaluate(()=>Reflect.get(window,'playerTest').downloadTasks.push({id:'RestartTask',batch:'Batch1',song:{id:'Song1',source:'tx',title:'重新下载',artist:'歌手'},directory:'/Downloads',requested:'flac',quality:'flac',codec:'flac',state:'downloading',bytes:100,total:1000,file:'',message:'无法确认同一媒体实体，已从头下载',created:1}))
  await downloadsPage(page)
  await expect(page.getByText('无法确认同一媒体实体，已从头下载',{exact:true})).toHaveCount(0)
  await expect(page.getByRole('progressbar',{name:'重新下载 下载进度',exact:true})).toBeVisible()
  await expect(page.getByRole('button',{name:'暂停下载 重新下载',exact:true})).toBeVisible()
})

test('download selection actions are conditional and all controls operate across tasks', async ({page}) => {
  await setup(page);await search(page)
  await page.getByRole('button',{name:'下载本页',exact:true}).click()
  await downloadsPage(page)
  const selection=page.getByRole('group',{name:'所选下载任务操作'})
  await expect(selection).toHaveCount(0)
  await expect(page.getByRole('button',{name:'取消本批',exact:true})).toHaveCount(0)
  await expect(page.getByRole('button',{name:'全部开始',exact:true})).toBeDisabled()
  await page.getByRole('checkbox',{name:'选择下载任务 第一首',exact:true}).check()
  await expect(selection.getByRole('button',{name:'开始',exact:true})).toBeVisible()
  await expect(selection.getByRole('button',{name:'暂停',exact:true})).toBeEnabled()
  await expect(page.getByRole('checkbox',{name:'全选本页下载任务',exact:true})).toHaveAttribute('aria-checked','mixed')
  await selection.getByRole('button',{name:'暂停',exact:true}).click()
  await expect(selection).toHaveCount(0)
  await expect(page.getByRole('button',{name:'继续下载 第一首',exact:true})).toBeVisible()
  await expect(page.getByRole('button',{name:'暂停下载 第二首',exact:true})).toBeVisible()
  await page.getByRole('button',{name:'全部暂停',exact:true}).click()
  await expect(page.getByRole('button',{name:/继续下载/})).toHaveCount(2)
  await page.getByRole('button',{name:'全部开始',exact:true}).click()
  await expect(page.getByRole('button',{name:/暂停下载/})).toHaveCount(2)
  await page.getByRole('checkbox',{name:'全选本页下载任务',exact:true}).check()
  await page.screenshot({path:'test-results/download-controls.png'})
  await selection.getByRole('button',{name:'取消',exact:true}).click()
  await expect(page.getByRole('table',{name:'下载任务'})).toHaveCount(0)
  await expect(page.getByText('还没有下载',{exact:true})).toBeVisible()
  await page.reload();await downloadsPage(page)
  await expect(page.getByRole('table',{name:'下载任务'})).toHaveCount(0)
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').downloadRemoves)).toEqual([])
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').prepares)).toEqual([])
  await search(page);await page.getByRole('button',{name:'下载本页',exact:true}).click();await downloadsPage(page)
  await page.getByRole('checkbox',{name:'选择下载任务 第一首',exact:true}).check()
  await page.getByRole('tab',{name:'下载中',exact:true}).click()
  await expect(selection).toHaveCount(0)
})

test('download select-all is page scoped and compact keyboard selection clears on pagination', async ({page}) => {
  await page.setViewportSize({width:820,height:640});await page.emulateMedia({reducedMotion:'reduce'});await setup(page)
  await page.evaluate(()=>Reflect.get(window,'playerTest').downloadTasks.push(...Array.from({length:51},(_,index)=>({id:`Task${index}`,batch:'Batch1',song:{id:`Song${index}`,source:'tx',title:`任务${index}`,artist:'歌手'},directory:'/Downloads',requested:'flac',quality:'flac',codec:'flac',state:'paused',bytes:100,total:1000,file:'',message:'',created:index}))))
  await downloadsPage(page)
  const checkbox=page.getByRole('checkbox',{name:'全选本页下载任务',exact:true})
  await checkbox.focus();await page.keyboard.press('Space')
  const selection=page.getByRole('group',{name:'所选下载任务操作'})
  await expect(selection).toContainText('已选 50 项')
  await selection.getByRole('button',{name:'开始',exact:true}).click()
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').downloadControls.at(-1).ids.length)).toBe(50)
  await checkbox.check();await page.getByRole('button',{name:'下一页',exact:true}).click()
  await expect(selection).toHaveCount(0)
  await expect(page.getByRole('table',{name:'下载任务'}).locator('tbody tr')).toHaveCount(1)
  await expect(page.getByRole('button',{name:'继续下载 任务50',exact:true})).toBeVisible()
  await page.getByRole('checkbox',{name:'选择下载任务 任务50',exact:true}).check()
  await expect(selection).toContainText('已选 1 项')
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
  await page.screenshot({path:'test-results/download-controls-compact.png'})
  const nav=page.getByRole('navigation',{name:'浏览导航'})
  await nav.getByRole('button',{name:'后退',exact:true}).click()
  await expect(page.getByRole('table',{name:'下载任务'}).locator('tbody tr')).toHaveCount(50)
  await nav.getByRole('button',{name:'前进',exact:true}).click()
  await expect(page.getByRole('button',{name:'继续下载 任务50',exact:true})).toBeVisible()
  await expect(selection).toHaveCount(0)
})

test('single download snapshots settings, pauses and resumes without starting playback', async ({page}) => {
  await setup(page);await search(page)
  await page.evaluate(()=>{Reflect.get(window,'playerTest').downloadDirectory='/Old';Reflect.get(window,'playerTest').downloadQuality='320k'})
  await page.getByRole('table').getByRole('button',{name:'下载 第一首',exact:true}).click()
  await page.evaluate(()=>{Reflect.get(window,'playerTest').downloadDirectory='/New';Reflect.get(window,'playerTest').downloadQuality='flac'})
  await page.getByRole('table').getByRole('button',{name:'下载 第二首',exact:true}).click()
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').prepares)).toEqual([])
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').downloadTasks.map((task:{directory:string;requested:string})=>[task.directory,task.requested]))).toEqual([['/New','flac'],['/Old','320k']])
  await downloadsPage(page)
  await page.getByRole('button',{name:'暂停下载 第一首',exact:true}).click()
  await expect(page.getByRole('button',{name:'继续下载 第一首',exact:true})).toBeVisible()
  await page.getByRole('button',{name:'继续下载 第一首',exact:true}).click()
  await expect(page.getByRole('button',{name:'暂停下载 第一首',exact:true})).toBeVisible()
  await page.reload();await downloadsPage(page)
  await expect(page.getByRole('button',{name:'继续下载 第一首',exact:true})).toBeVisible()
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').downloadEnqueues)).toBe(0)
})

test('download tabs filter exact states and file deletion requires explicit confirmation', async ({page}) => {
  await setup(page)
  await page.addInitScript(()=>localStorage.setItem('download-tasks',JSON.stringify(['queued','downloading','success','failed','skipped','canceled','paused'].map((state,index)=>({id:`Task${index}`,batch:'Batch1',song:{id:`Song${index}`,source:'tx',title:`任务${index}`,artist:'歌手'},directory:'/Downloads',requested:'flac',quality:'flac',codec:'flac',state,bytes:100,total:1000,file:state==='success'?'music.flac':'',message:'',created:index})))))
  await page.reload()
  // Simulate a running backend after startup; queued/active restoration itself is paused.
  await page.evaluate(()=>{const tasks=Reflect.get(window,'playerTest').downloadTasks;tasks.splice(0,tasks.length,...JSON.parse(localStorage.getItem('download-tasks')!));tasks[1].state='downloading';tasks[0].state='queued';tasks[0].bytes=0;tasks[0].total=0})
  await downloadsPage(page)
  await expect(page.getByRole('table',{name:'下载任务'}).locator('tbody tr')).toHaveCount(6)
  await page.getByRole('tab',{name:'下载中',exact:true}).click()
  await expect(page.getByRole('table',{name:'下载任务'}).locator('tbody tr')).toHaveCount(2)
  await expect(page.getByRole('button',{name:'继续下载 任务6',exact:true})).toBeVisible()
  const speed=page.locator('[data-download-id="Task1"] [data-download-speed]')
  await expect(speed).toBeVisible()
  await expect(page.locator('[data-download-id="Task6"] [data-download-speed]')).toHaveCount(0)
  await page.waitForTimeout(600)
  await page.evaluate(()=>{Reflect.get(window,'playerTest').downloadTasks[1].bytes+=512})
  await page.getByRole('button',{name:'刷新下载记录',exact:true}).click()
  await expect(speed).toHaveText(/\/s$/)
  expect(await speed.evaluate(element=>element.previousElementSibling?.hasAttribute('data-download-size'))).toBe(true)
  const sizeBox=await page.locator('[data-download-id="Task1"] [data-download-size]').boundingBox()
  const speedBox=await speed.boundingBox()
  expect(speedBox!.x).toBeGreaterThan(sizeBox!.x+sizeBox!.width)
  expect(Math.abs(speedBox!.y-sizeBox!.y)).toBeLessThan(3)
  await page.getByRole('tab',{name:'下载成功',exact:true}).click()
  await page.getByRole('button',{name:'删除本地文件 任务2',exact:true}).click()
  await page.getByRole('alertdialog').getByRole('button',{name:'取消',exact:true}).click()
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').downloadRemoves)).toEqual([])
  await page.getByRole('button',{name:'删除本地文件 任务2',exact:true}).click()
  await page.getByRole('alertdialog').getByRole('button',{name:'删除文件',exact:true}).click()
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').downloadRemoves)).toEqual([['Task2',true]])
  await page.getByRole('tab',{name:'下载失败',exact:true}).click()
  await page.getByRole('button',{name:'重试下载 任务3',exact:true}).click()
  await expect(page.getByText('暂无对应任务',{exact:true})).toBeVisible()
  await page.getByRole('tab',{name:'全部',exact:true}).click()
  await page.getByRole('button',{name:'移除下载记录 任务4',exact:true}).click()
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').downloadRemoves.at(-1))).toEqual(['Task4',false])
  await page.screenshot({path:'test-results/downloads-desktop.png'})
})

test('favorites batch download queues the collection without playing and can all be paused', async ({page}) => {
  await setup(page);await search(page)
  const table=page.getByRole('table',{name:'QQ 音乐搜索结果'})
  await table.getByRole('button',{name:'收藏 第一首',exact:true}).click()
  await table.getByRole('button',{name:'收藏 第二首',exact:true}).click()
  await favoritesPage(page)
  await page.getByRole('button',{name:'下载全部',exact:true}).click()
  await downloadsPage(page)
  await expect(page.getByRole('table',{name:'下载任务'}).locator('tbody tr')).toHaveCount(2)
  await page.getByRole('button',{name:'全部暂停',exact:true}).click()
  await expect(page.getByRole('button',{name:/继续下载/})).toHaveCount(2)
  expect(await page.evaluate(()=>Reflect.get(window,'playerTest').prepares)).toEqual([])
})

async function favoritesPage(page: Page) {
  await page.getByRole('navigation', { name: '主导航' }).getByRole('button', { name: '喜欢', exact: true }).click()
}

test('favorites has a dedicated sidebar destination and details return preserves it', async ({ page }) => {
  await setup(page)
  const nav = page.getByRole('navigation', { name: '主导航' })
  await expect(nav.getByRole('button')).toHaveCount(4)
  await expect(page.getByRole('tab', { name: '喜欢的音乐', exact: true })).toHaveCount(0)
  await favoritesPage(page)
  await expect(nav.getByRole('button', { name: '喜欢', exact: true })).toHaveAttribute('aria-current', 'page')
  await expect(page.getByRole('heading', { name: /喜欢的音乐/ })).toBeVisible()
  await expect(page.getByText('把时间交给音乐。', { exact: true })).toHaveCount(0)
  await nav.getByRole('button', { name: '推荐', exact: true }).click()
  await expect(page.getByRole('tab', { name: '喜欢的音乐', exact: true })).toHaveCount(0)
  await search(page)
  const table = page.getByRole('table', { name: 'QQ 音乐搜索结果' })
  await table.getByRole('button', { name: '收藏 第一首', exact: true }).click()
  await table.getByRole('button', { name: '播放 第一首', exact: true }).click()
  await favoritesPage(page)
  await footer(page).getByRole('button', { name: '打开播放详情', exact: true }).click()
  await page.locator('[data-now-playing]').getByRole('button', { name: '返回', exact: true }).click()
  await expect(nav.getByRole('button', { name: '喜欢', exact: true })).toHaveAttribute('aria-current', 'page')
  await expect(page.getByRole('table', { name: '喜欢的音乐列表' })).toBeVisible()
  expect(await page.evaluate(() => Reflect.get(window, 'playerTest').prepares)).toEqual(['Song0'])
})

test('favorite buttons share confirmed state across search, player and details without starting audio', async ({ page }) => {
  await setup(page)
  await search(page)
  const searchTable = page.getByRole('table', { name: 'QQ 音乐搜索结果' })
  await expect(searchTable.getByRole('button', { name: '收藏 第一首', exact: true }).locator('svg')).toHaveAttribute('fill', 'none')
  await searchTable.getByRole('button', { name: '收藏 第一首', exact: true }).click()
  const liked = searchTable.getByRole('button', { name: '取消收藏 第一首', exact: true })
  await expect(liked).toHaveAttribute('aria-pressed', 'true')
  await expect(liked.locator('svg')).toHaveAttribute('fill', 'currentColor')
  await expect(liked.locator('..').getByRole('button', { name: '下一首播放 第一首', exact: true })).toBeVisible()
  expect(await liked.evaluate(button => {
    const previous = button.previousElementSibling
    return previous?.getAttribute('aria-label') === '下一首播放 第一首'
  })).toBe(true)
  expect(await liked.locator('svg').evaluate(svg => {
    const canvas = document.createElement('canvas'), context = canvas.getContext('2d')!
    context.fillStyle = getComputedStyle(svg).color; context.fillRect(0, 0, 1, 1)
    const [r,g,b] = context.getImageData(0,0,1,1).data
    return r > g && r > b
  })).toBe(true)
  expect(await page.evaluate(() => Reflect.get(window, 'playerTest').prepares)).toEqual([])
  await searchTable.getByRole('button', { name: '播放 第一首', exact: true }).click()
  await expect(footer(page).getByRole('button', { name: '取消收藏 第一首', exact: true })).toHaveAttribute('aria-pressed', 'true')
  await expect(footer(page).getByRole('button', { name: '取消收藏 第一首', exact: true }).locator('..').getByRole('button', { name: '查看 第一首 的播放详情', exact: true })).toBeVisible()
  await footer(page).getByRole('button', { name: '打开播放详情', exact: true }).click()
  await expect(page.locator('[data-now-playing]').getByRole('button', { name: '取消收藏 第一首', exact: true }).locator('..').getByRole('heading', { name: '第一首', exact: true })).toBeVisible()
  await page.locator('[data-now-playing]').getByRole('button', { name: '取消收藏 第一首', exact: true }).click()
  await expect(footer(page).getByRole('button', { name: '收藏 第一首', exact: true })).toHaveAttribute('aria-pressed', 'false')
  await expect(footer(page).getByRole('button', { name: '暂停', exact: true })).toBeVisible()
  expect(await page.evaluate(() => Reflect.get(window, 'playerTest').prepares)).toEqual(['Song0'])
})

test('favorites list is newest first, persists, plays the collection and removal preserves current queue', async ({ page }) => {
  await setup(page)
  await search(page)
  const table = page.getByRole('table', { name: 'QQ 音乐搜索结果' })
  await table.getByRole('button', { name: '收藏 第一首', exact: true }).click()
  await table.getByRole('button', { name: '收藏 第二首', exact: true }).click()
  await favoritesPage(page)
  const list = page.getByRole('table', { name: '喜欢的音乐列表' })
  await expect(list.getByRole('button', { name: /^播放收藏/ })).toHaveText(['第二首', '第一首'])
  await page.reload()
  await favoritesPage(page)
  await expect(list.getByRole('button', { name: /^播放收藏/ })).toHaveText(['第二首', '第一首'])
  await page.getByRole('button', { name: '播放全部', exact: true }).click()
  await expect(footer(page).getByRole('button', { name: '暂停', exact: true })).toBeVisible()
  await list.getByRole('button', { name: '取消收藏 第二首', exact: true }).click()
  await expect(list.getByRole('button', { name: /^播放收藏/ })).toHaveText(['第一首'])
  await expect(footer(page).getByRole('button', { name: '查看 第二首 的播放详情', exact: true })).toBeVisible()
  await expect(footer(page).getByRole('button', { name: '暂停', exact: true })).toBeVisible()
  await footer(page).getByRole('button', { name: '播放队列', exact: true }).click()
  await expect(queuePanel(page).getByRole('listitem')).toHaveCount(2)
  await page.screenshot({ path: 'test-results/favorites-desktop.png' })
})

test('pending favorite writes disable the same song across views without blocking playback', async ({ page }) => {
  await setup(page)
  await search(page)
  await page.evaluate(() => Reflect.get(window, 'playerTest').favoriteMode = 'pending')
  const table = page.getByRole('table', { name: 'QQ 音乐搜索结果' })
  await table.getByRole('button', { name: '收藏 第一首', exact: true }).click()
  await expect(table.getByRole('button', { name: '收藏 第一首', exact: true })).toBeDisabled()
  await table.getByRole('button', { name: '播放 第一首', exact: true }).click()
  await expect(footer(page).getByRole('button', { name: '收藏 第一首', exact: true })).toBeDisabled()
  await expect(footer(page).getByRole('button', { name: '暂停', exact: true })).toBeVisible()
  await page.evaluate(() => Reflect.get(window, 'playerTest').favoritePending.splice(0).forEach((finish: () => void) => finish()))
  await expect(footer(page).getByRole('button', { name: '取消收藏 第一首', exact: true })).toBeEnabled()
  expect(await page.evaluate(() => Reflect.get(window, 'playerTest').favoriteWrites)).toEqual(['add:Song0'])
})

test('favorites empty state is distinct from a read failure', async ({ page }) => {
  await setup(page)
  await favoritesPage(page)
  await expect(page.getByText('还没有收藏音乐', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: '播放全部', exact: true })).toBeDisabled()
  await page.evaluate(() => Reflect.get(window, 'playerTest').favoriteMode = 'read-error')
  await page.getByRole('button', { name: '刷新收藏', exact: true }).click()
  await expect(page.getByText('无法读取收藏', { exact: true })).toBeVisible()
  await expect(page.getByText('还没有收藏音乐', { exact: true })).toHaveCount(0)
})

test('favorite save failures retain membership and read errors can recover without clearing data', async ({ page }) => {
  await setup(page)
  await search(page)
  const table = page.getByRole('table', { name: 'QQ 音乐搜索结果' })
  await page.evaluate(() => Reflect.get(window, 'playerTest').favoriteMode = 'write-error')
  await table.getByRole('button', { name: '收藏 第一首', exact: true }).click()
  await expect(page.getByText('收藏失败，请重试', { exact: true })).toBeVisible()
  await expect(table.getByRole('button', { name: '收藏 第一首', exact: true })).toHaveAttribute('aria-pressed', 'false')
  await page.evaluate(() => Reflect.get(window, 'playerTest').favoriteMode = 'normal')
  await table.getByRole('button', { name: '收藏 第一首', exact: true }).click()
  await favoritesPage(page)
  await page.evaluate(() => Reflect.get(window, 'playerTest').favoriteMode = 'read-error')
  await page.getByRole('button', { name: '刷新收藏', exact: true }).click()
  await expect(page.getByText('无法读取收藏', { exact: true })).toBeVisible()
  expect(await page.evaluate(() => JSON.parse(localStorage.getItem('favorite-songs')!).length)).toBe(1)
  await page.evaluate(() => Reflect.get(window, 'playerTest').favoriteMode = 'normal')
  await page.getByRole('button', { name: '重试读取收藏', exact: true }).click()
  await expect(page.getByRole('table', { name: '喜欢的音乐列表' }).getByRole('button', { name: '播放收藏 第一首', exact: true })).toBeVisible()
})

test('large favorite collections paginate and compact layout stays inside the viewport', async ({ page }) => {
  await setup(page)
  await page.addInitScript(() => localStorage.setItem('favorite-songs', JSON.stringify(Array.from({ length: 120 }, (_, index) => ({ id: `Favorite${index}`, source: 'tx', title: `收藏歌曲${index}`, artist: '收藏歌手', album: '', coverUrl: '', duration: 60.125 })))))
  await page.reload()
  await page.setViewportSize({ width: 640, height: 620 })
  await favoritesPage(page)
  const list = page.getByRole('table', { name: '喜欢的音乐列表' })
  await expect(list.getByRole('button', { name: /^播放收藏/ })).toHaveCount(50)
  await page.getByRole('navigation', { name: '收藏分页' }).getByRole('button', { name: '下一页', exact: true }).click()
  await expect(list.getByRole('button', { name: /^播放收藏/ }).first()).toHaveText('收藏歌曲50')
  await page.getByRole('button', { name: '播放全部', exact: true }).click()
  await expect.poll(() => page.evaluate(() => Reflect.get(window, 'playerTest').prepares)).toEqual(['Favorite0'])
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(640)
  await page.screenshot({ path: 'test-results/favorites-compact.png' })
})

async function seedQueue(page: Page, count: number, index = 0) {
  await page.addInitScript(({ count, index }) => {
    const queue = Array.from({ length: count }, (_, i) => ({ id: `Queue${i}`, source: 'tx', title: `队列歌曲${i}`, artist: '队列歌手', album: '', coverUrl: '', duration: 60, songId: '123', mediaMid: 'Media1' }))
    localStorage.setItem('player-snapshot', JSON.stringify({ queue, index, position: 0, mode: 'sequential' }))
  }, { count, index })
  await page.reload()
  await expect(footer(page).getByRole('button', { name: '播放', exact: true })).toBeEnabled()
}
const queuePanel = (page: Page) => page.getByRole('complementary', { name: /播放队列/ })

test('queue opens empty and Escape restores focus without playing', async ({ page }) => {
  await setup(page)
  const trigger = footer(page).getByRole('button', { name: '播放队列', exact: true })
  await trigger.click()
  await expect(queuePanel(page).getByText('队列为空', { exact: true })).toBeVisible()
  await expect(trigger).toHaveAttribute('aria-expanded', 'true')
  await page.keyboard.press('Escape')
  await expect(queuePanel(page)).toHaveCount(0)
  await expect(trigger).toBeFocused()
  expect(await page.evaluate(() => Reflect.get(window, 'playerTest').prepares)).toEqual([])
})

test('queue removes other songs without interrupting audio and clear requires confirmation', async ({ page }) => {
  await setup(page)
  await search(page)
  await page.getByRole('button', { name: '播放 第一首', exact: true }).click()
  await expect(footer(page).getByRole('button', { name: '暂停', exact: true })).toBeVisible()
  await footer(page).getByRole('button', { name: '播放队列', exact: true }).click()
  await queuePanel(page).getByRole('button', { name: '从队列移除 第二首', exact: true }).click()
  await expect(queuePanel(page).getByRole('listitem')).toHaveCount(1)
  expect(await page.evaluate(() => Reflect.get(window, 'playerTest').prepares)).toEqual(['Song0'])
  await expect.poll(() => nativePosition(page)).toBeGreaterThan(0)
  await queuePanel(page).getByRole('button', { name: '清空', exact: true }).click()
  await page.getByRole('alertdialog').getByRole('button', { name: '取消', exact: true }).click()
  await expect(footer(page).getByRole('button', { name: '暂停', exact: true })).toBeVisible()
  await queuePanel(page).getByRole('button', { name: '清空', exact: true }).click()
  await page.getByRole('alertdialog').getByRole('button', { name: '清空队列', exact: true }).click()
  await expect(queuePanel(page).getByText('队列为空', { exact: true })).toBeVisible()
  await expect(footer(page).getByRole('button', { name: '播放', exact: true })).toBeDisabled()
  await expect.poll(() => page.evaluate(() => JSON.parse(localStorage.getItem('player-snapshot')!).queue.length)).toBe(0)
})

test('search next action queues without starting playback and survives reload', async ({ page }) => {
  await setup(page)
  await search(page)
  await page.getByRole('table').getByRole('button', { name: '下一首播放 第二首', exact: true }).click()
  await page.getByRole('table').getByRole('button', { name: '下一首播放 第一首', exact: true }).click()
  expect(await page.evaluate(() => Reflect.get(window, 'playerTest').prepares)).toEqual([])
  await page.reload()
  await footer(page).getByRole('button', { name: '播放队列', exact: true }).click()
  await expect(queuePanel(page).getByRole('listitem')).toHaveCount(2)
  await footer(page).getByRole('button', { name: '播放', exact: true }).click()
  await expect(footer(page).getByRole('button', { name: '暂停', exact: true })).toBeVisible()
  await footer(page).getByRole('button', { name: '下一首', exact: true }).click()
  await expect.poll(() => page.evaluate(() => Reflect.get(window, 'playerTest').prepares)).toEqual(['Song1', 'Song0'])
})

test('queue selecting and reordering preserves the list and refreshes cache neighbors', async ({ page }) => {
  await setup(page)
  await seedQueue(page, 3)
  await footer(page).getByRole('button', { name: '播放', exact: true }).click()
  await expect(footer(page).getByRole('button', { name: '暂停', exact: true })).toBeVisible()
  await footer(page).getByRole('button', { name: '播放队列', exact: true }).click()
  await queuePanel(page).getByRole('button', { name: '下一首播放 队列歌曲2', exact: true }).click()
  await expect.poll(() => page.evaluate(() => Reflect.get(window, 'playerTest').neighborhoods.at(-1))).toEqual(['Queue0', 'Queue1', 'Queue2'])
  expect(await page.evaluate(() => Reflect.get(window, 'playerTest').prepares)).toEqual(['Queue0'])
  await footer(page).getByRole('button', { name: '下一首', exact: true }).click()
  await expect.poll(() => page.evaluate(() => Reflect.get(window, 'playerTest').prepares)).toEqual(['Queue0', 'Queue2'])
  await queuePanel(page).getByRole('button', { name: '播放队列中的 队列歌曲1', exact: true }).click()
  await expect.poll(() => page.evaluate(() => Reflect.get(window, 'playerTest').prepares)).toEqual(['Queue0', 'Queue2', 'Queue1'])
  await expect(queuePanel(page).getByRole('listitem')).toHaveCount(3)
  await page.screenshot({ path: 'test-results/queue-desktop.png' })
})

test('removing playing current continues next and Escape dismisses only clear confirmation', async ({ page }) => {
  await setup(page)
  await seedQueue(page, 3)
  await footer(page).getByRole('button', { name: '播放', exact: true }).click()
  await expect(footer(page).getByRole('button', { name: '暂停', exact: true })).toBeVisible()
  await footer(page).getByRole('button', { name: '播放队列', exact: true }).click()
  await queuePanel(page).getByRole('button', { name: '从队列移除 队列歌曲0', exact: true }).click()
  await expect.poll(() => page.evaluate(() => Reflect.get(window, 'playerTest').prepares)).toEqual(['Queue0', 'Queue1'])
  await expect(footer(page).getByRole('button', { name: '暂停', exact: true })).toBeVisible()
  await queuePanel(page).getByRole('button', { name: '清空', exact: true }).click()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('alertdialog')).toHaveCount(0)
  await expect(queuePanel(page)).toBeVisible()
  await expect(queuePanel(page).getByRole('listitem')).toHaveCount(2)
})

test('removing paused current selects next without resolving or autoplay', async ({ page }) => {
  await setup(page)
  await seedQueue(page, 3, 2)
  await footer(page).getByRole('button', { name: '播放队列', exact: true }).click()
  await queuePanel(page).getByRole('button', { name: '从队列移除 队列歌曲2', exact: true }).click()
  await expect(footer(page).getByRole('button', { name: '查看 队列歌曲0 的播放详情', exact: true })).toBeVisible()
  expect(await page.evaluate(() => Reflect.get(window, 'playerTest').prepares)).toEqual([])
  await expect(footer(page).getByRole('button', { name: '播放', exact: true })).toBeEnabled()
})

test('long queue renders a bounded window and supports keyboard navigation at compact size', async ({ page }) => {
  await setup(page)
  await seedQueue(page, 10000, 9999)
  await page.setViewportSize({ width: 640, height: 620 })
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await footer(page).getByRole('button', { name: '播放队列', exact: true }).click()
  const last = queuePanel(page).getByRole('button', { name: '播放队列中的 队列歌曲9999', exact: true })
  await expect(last).toBeVisible()
  expect(await queuePanel(page).getByRole('listitem').count()).toBeLessThan(40)
  await last.focus()
  await page.keyboard.press('Home')
  const first = queuePanel(page).getByRole('button', { name: '播放队列中的 队列歌曲0', exact: true })
  await expect(first).toBeFocused()
  await page.keyboard.press('End')
  await expect(last).toBeFocused()
  const box = await queuePanel(page).boundingBox()
  expect(box!.x + box!.width).toBeLessThanOrEqual(640)
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(640)
  expect(await page.evaluate(() => Reflect.get(window, 'playerTest').prepares)).toEqual([])
  await page.screenshot({ path: 'test-results/queue-compact.png' })
  await footer(page).getByRole('button', { name: '查看 队列歌曲9999 的播放详情', exact: true }).click()
  await expect(queuePanel(page)).toHaveCount(0)
  await expect(page.getByRole('heading', { name: '队列歌曲9999', exact: true })).toBeVisible()
})

for (const mode of ['empty', 'plain', 'error']) {
  test(`details ${mode} state is distinct and errors can retry`, async ({ page }) => {
    await setup(page)
    await page.evaluate((mode) => (Reflect.get(window, 'playerTest').lyricMode = mode), mode)
    await details(page)
    if (mode === 'empty') await expect(page.getByText('暂无歌词', { exact: true })).toBeVisible()
    if (mode === 'plain') {
      await expect(page.getByText('歌词暂无时间轴', { exact: true })).toBeVisible()
      await expect(page.getByRole('region', { name: '歌词', exact: true }).getByRole('button')).toHaveCount(0)
    }
    if (mode === 'error') {
      await expect(page.getByText('歌词暂时无法加载', { exact: true })).toBeVisible()
      await page.evaluate(() => (Reflect.get(window, 'playerTest').lyricMode = 'normal'))
      await page.getByRole('button', { name: '重试歌词', exact: true }).click()
      await expect(
        page.getByRole('region', { name: '歌词', exact: true }).getByRole('button', { name: /Song0 开始/ })
      ).toBeVisible()
    }
  })
}
