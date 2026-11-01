# QQ 音乐歌曲搜索

## 已实现

- 仅搜索 QQ 音乐单曲，可输入歌曲、歌手、专辑名称；不返回歌手或专辑分类结果。
- 展示歌曲、副标题、歌手、专辑、封面和时长，支持上一页、下一页。
- 每页 20 首，页码限制 1–100；是否还有下一页同时参考上游分页信息和返回总数。
- 支持关键词校验、加载骨架、空结果、错误重试和取消。
- 新查询取消旧请求，前后端均防止旧结果覆盖新结果。相同查询与页码正在加载时不重复提交。
- 搜索输入框固定在主内容区顶部，所有页面共用；侧栏没有搜索入口。聚焦只展开输入框下方的非模态提示，不切页、不改变当前内容滚动位置。点击搜索图标、回车、点击提示项或悬停提示后回车，才提交并进入搜索页面。
- 搜索页没有额外的页面标题、路径或音源状态；结果数量只展示「约 N 首」。
- 搜索状态在内存中跨页面保留。历史关键词本地保存为 tags，最近优先、去重、最多 20 条，重启保留；点击可重搜，支持逐条删除、清空。
- 有效关键词提交时记录历史，空输入和翻页不记录；上游失败也保留用户的检索意图。不自动搜索，也不需要启用音源。
- 结果行支持播放、下一首、收藏、单曲下载，以及可靠 MID 对应的[歌手/专辑详情](music-details.md)；可下载本页。保留独立「解析」验收按钮，取得当前音源地址但不播放；详见 [音源运行时](source-runtime.md#真实解析)。

## 后端

`internal/music` 定义统一 Song、Artist 和 SearchResult；`QQClient` 负责请求与转换，和音源脚本、Keychain、SQLite 完全分离。

当前使用 `https://u.y.qq.com/cgi-bin/musicu.fcg` 的 `music.search.SearchCgiService.DoSearchForQQMusicDesktop` 匿名桌面单曲搜索协议。请求只发送检索参数及匿名客户端兼容字段，`uin=0`；不发送 cookies、音源 API key 或用户账号。每次请求生成随机搜索会话 ID，不复制固定设备标识。

这不是稳定、承诺开放的服务端 API。平台可能调整参数、限流或限制匿名访问，应遵守 QQ 音乐服务条款；不绕过登录或访问限制，不自动重试或转到其他音乐平台。

早期探测中有接口返回 `code=0` 但 `meta.is_filter=-2` 且歌曲列表为空。适配器会把这种情况报告为请求受限，而非正常「没有歌曲」。

### 安全与资源

- 输入去除首尾空白，限制 100 个 Unicode 字符，拒绝控制字符和无效页码。
- 目标地址固定为 QQ 音乐，不接受前端提供网络地址，不跟随 HTTP 重定向。
- HTTP 客户端 8 秒超时，桌面任务 10 秒超时，连接/响应头单独限时。
- 响应解压后限制 2MB，响应头限制 64KB，限制返回条数、字段长度和歌手数量。
- 只解析需要的 JSON 字段，不输出原始响应、请求关键词、cookies 或网络错误 URL 到日志。
- 401/403、429、超时、网络失败、结构异常分别给出脱敏错误；只有正常成功的空列表才显示没有歌曲。
- 忽略上游高亮 HTML、跳转 URL、播放 URL；所有曲目 source 固定为 `tx`。
- 曲目、歌手和专辑 MID 限定为短字母数字串。封面只由安全 MID 构造到固定 QQ CDN 域名，前端懒加载并提供失败回退。
- 以 songmid 为主键去重；另保留 QQ 数字 songId（字符串，防止 JS 精度丢失）、mediaMid、歌手 MID 和专辑 MID，供后续详情与解析适配使用。
- `availableQualities` 只反映 QQ 元数据的普通格式声明，不代表音源一定能解析或用户拥有播放/下载授权。

### Wails 方法

- `SearchSongs(query, page, requestID)`：启动单曲查询，取消当前旧请求；服务器序号保护新任务，避免旧任务结束时误清理取消句柄。
- `CancelSongSearch(requestID)`：仅取消匹配的当前请求，不影响后来的查询。

请求 ID 仅用于本地生命周期管理，不作为身份凭据；它不直接传给 QQ 接口。

## 本地脱敏诊断

桌面端在首次有效请求后按需创建 `~/Library/Application Support/妙音娘子/logs/qq-metadata.jsonl`。搜索和输入提示共用日志，记录成功、失败与取消，便于区分首次请求和后续请求；不自动上传。

- 字段仅有 UTC 时间、固定操作名 `search`/`suggestions`、耗时毫秒、HTTP 状态、固定结果分类，以及数字业务码。
- 搜索保留外层 `code`、`request_code`、`data_code`、`meta_ret` 和 `filter`；提示保留 `code` 和 `subcode`。零值业务码也保留，没有收到或解析的字段不记录。
- 不记录搜索词、歌曲信息、请求/响应正文、请求 ID、URL、IP、Header、Cookie、账号、音源密钥或原始异常信息。搜索历史仍是独立的本地功能，不会复制进诊断日志。
- 单文件最多 64KiB，轮转保留一个 `.1` 备份；目录权限 0700，新建文件 0600。同一应用内并发写入串行化，拒绝已有日志符号链接；使用受目录限制的文件操作和打开前后文件身份检查。
- 写入属于 best effort，不改变搜索返回值，也不会因为目录不可写而阻止搜索。旧版本发生的错误无法补录。

HTTP 401/403、QQ 各层非零业务码、负数 `filter` 都可能对应同一条「暂时拒绝搜索」提示。诊断能区分这些分支，但仅凭错误码仍不应擅自断言是风控、网络出口或账号问题。

## 前端

- `SearchPage.svelte`：历史 tags 和查询状态展示，不再重复渲染输入框。
- `components/search/SearchForm.svelte`：主内容区顶部的常驻输入、校验、搜索与取消；shadcn Popover + Command 下拉提示，保持输入焦点，支持鼠标选择、悬停后回车、方向键、Esc、Tab 和外部点击。
- `search-suggestions.ts`：独立的防抖、缓存和取消状态，不复用完整搜索的取消通道。
- `internal/music/suggestions.go` / 根目录 `suggestions.go`：QQ 歌曲建议与桌面桥接。
- `components/search/SearchHistory.svelte`：shadcn Badge + Button 组成的可操作 tags。
- `search-history.ts` / `search-history-state.svelte.ts`：本地历史桥接、串行修改队列和跨页面状态；历史存储失败不阻塞音乐搜索。
- `components/search/SearchResults.svelte`：shadcn Table 与分页。
- `components/search/SongCover.svelte`：图片懒加载和错误回退。
- `search-controller.ts`：独立于组件的请求顺序、取消与重试逻辑，可用 Node 单测。
- `search-state.svelte.ts`：应用内存状态；页面卸载不丢失结果，开发热更新时取消旧请求；有效首屏查询触发独立的历史持久化，不在结果返回时重新写入，避免清空后又恢复。
- `music.ts`：类型与 Wails 桥接。浏览器无桥接时明确提示使用桌面端，不返回演示结果。

上游文本只作为普通文本渲染，不使用 HTML 注入。窄屏合并元信息，避免横向溢出；翻页回到内容顶部。顶部搜索栏和底部播放器保持固定，只有中间内容滚动。主页面仍按需加载，每次只渲染最多 20 首，无需提前引入虚拟列表。

历史保存在应用私有 SQLite，不使用浏览器 localStorage，也不做云同步。schema v4 添加 `search_history`，编辑及返回列表在同一事务内完成；前端串行处理读取、记录、删除和清空，避免慢读取或旧写入覆盖新操作。数据库启用 secure_delete，但清空不保证擦除系统快照或既有备份。

## 输入提示

建议来自固定 QQ 接口 `https://c.y.qq.com/splcloud/fcgi-bin/smartbox_new.fcg`，匿名 GET，仅传 `key`、`format=json`，不使用账号、Cookie 或音源密钥。只采用 `data.song.itemlist`，忽略歌手、专辑和视频分类；选择歌曲建议会按歌名进行完整搜索，不直接播放。

- 聚焦空框展示历史，输入后展示当前关键词、匹配历史和歌曲建议；关键词用高对比品牌色标记。
- 输入停顿 300ms 后请求，中文输入法合成期间不请求、不提交；确认候选后仍需另一次回车才搜索。
- 关闭或改词取消旧提示，revision 拒绝迟到结果；不能取消或覆盖已提交的完整搜索。提示失败时仍可回车搜索。
- 提示结果只在内存缓存，最多 10 个关键词、每项 60 秒。输入中的关键词不写入历史。
- 上游请求限时 3 秒、响应最多 128KiB、最多采用 8 首；参数校验与完整搜索一致，无重定向和自动重试。
- 提示通过 Portal 渲染，不受内容滚动区裁切。输入与列表使用 combobox/listbox 语义、aria-expanded、aria-controls 和 aria-activedescendant 关联。
- `SuggestSongs(query, requestID)` / `CancelSongSuggestions(requestID)` 管理独立的建议请求。

## 验证

```sh
go test -race ./...
go vet ./...
# 可选真实 QQ 请求，默认测试不访问网络：
MIAOYIN_LIVE_QQ_SEARCH=1 go test -v ./internal/music -run 'TestLiveQQ(Search|Suggestions)'
cd frontend
npm run check
npm test
npm run test:e2e
```

Go 测试覆盖请求参数与匿名字段、真实数据形状转换、空结果、受限结果、异常 JSON、响应大小、重定向、超时/取消、去重和精确 ID；诊断测试覆盖各层业务码、脱敏、并发写入、轮转、私有权限、符号链接拒绝和不可写目录下搜索正常完成。桌面服务测试覆盖新请求取消旧请求、过期取消、重复请求 ID 清理保护及无音源时搜索。

Node 测试覆盖前端输入、去重、错误重试、取消、迟到响应和非 QQ 数据拒绝。Playwright 使用 Wails mock 验证列表、分页、加载、空/错区别、状态保留、文本转义和窄屏布局。真实 API 冒烟测试与浏览器 mock 是两类不同验证，不能用 mock 证明上游可用；macOS 客户端仍需人工验收。
