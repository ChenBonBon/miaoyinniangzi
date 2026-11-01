# 真实播放

## 使用

搜索结果点击歌曲名称、封面附近或行内空白区域，替换队列为当前结果列表并播放该曲。名称是可键盘操作的 shadcn Button。「解析」仍是独立操作，不播放，不因冒泡重复发起播放。

底栏提供播放/暂停、解析/缓冲阶段取消、上一首/下一首、进度拖动、音量及顺序/随机/单曲循环。进度条至少 24px 点击区，保留悬停指针与滑块强调；拖动预览通过捕获阶段建立。正式进度与持久化以跳转回执更新，期间不让旧进度覆盖目标。音量图标静音/恢复会话中上次非零音量，滑到零也保留恢复值；显示百分比或「已静音」，仅音量保存不显示「设置已保存」。切页不中断播放。

顺序模式按队列首尾循环，最后一首自然结束后进入第一首；手动上一首/下一首同样首尾相接，取消超过三秒先回曲首的规则。单曲循环仅影响自然结束，手动切歌仍按队列切换。随机模式预选一次下一首且不连续选择当前曲，上一首回到实际播放历史；后退后再前进复用历史，不重新抽签。随机历史仅保留本次会话最多 1000 条，重启/换队列/切换模式重置，没有历史时上一首禁用。底栏封面/歌名可进入 [播放详情与逐行歌词](lyrics.md)，进出页面不影响播放。[队列面板](queue.md) 支持点选、移除、清空及下一首播放，并与三槽缓存同步；[收藏](favorites.md) 已接入，取消收藏不影响当前播放；[下载任务](downloads.md) 已接入，使用独立解析生命周期，不取消播放或邻曲准备。

## 统一 libmpv 路径

FLAC、MP3 320K、MP3 128K 现在均由 `internal/mpvaudio` 的 libmpv 0.41.0 client API 播放，不再按音质分配 WebKit/AVAudioEngine 后端。前端没有 HTMLAudioElement 播放实现。历史 AVFoundation/WebKit 测试保留作对照，不属于当前应用播放路径。

`PrepareSong(song, requestID, previous, next)` 读取播放音质，经当前启用音源解析；主进程读取最多 4096 字节探测实际容器、FLAC 时长、大小及 Range。未知容器拒绝播放，档位不作为实际编码或 MP3 码率证明。降档与请求 FLAC 却实际得到 MP3 的提示保持不变。

随后安装不透明 loopback capability，由 libmpv 读取。[三槽缓存](cache-plan.md) 让当前曲在初始数据就绪后开始播放，同时后台完整缓存；邻居保留覆盖五秒的完整帧，并提前准备暂停、静音的原生实例。切歌复用已就绪实例，旧当前项降为前缀；整曲缓存完成后定位/循环不再读取上游。缺少强 ETag/Range 等条件时明确降为流式播放，不创建下载记录。整个前台准备阶段预算 45 秒，可取消；不等待整首下载完成。libmpv 请求超时配置 8 秒，seek 等待确认最多 15 秒；错误不自动换音源。不承诺物理输出零延迟。

准备结果为 `engine: libmpv`、限定当前请求的 `nativeSession`、实际容器/时长等元信息；`mediaUrl` 为空。源签名 URL、代理 token、本地库路径均不返回给网页、不持久化。

### 播放控制

- `NativePlayback` 只接受当前会话的 play/pause/seek/volume/state；网络等待不持有应用锁，切歌、取消与源失效仍可关闭会话。
- Go 串行化同一播放器操作。初始加载需等首次 `PLAYBACK_RESTART` 建立输出，不把 `FILE_LOADED` 当作准备结束。
- `seek absolute+exact`，`hr-seek-demuxer-offset=0.25` 先从目标前方解复用再解码到目标。按顺序等待本次 `SEEK` 后的 `PLAYBACK_RESTART`，不能拿旧 restart 事件当新确认。
- `keep-open=always` 保留末端暂停后的定位能力。队列、EOF、循环与用户暂停意图由应用管理；暂停末端定位不会自动切歌。
- 前端约 150ms 读取位置、时长、可定位、缓冲及结束状态。缓冲不冒充用户暂停；过期 play/pause/seek 回执不能覆盖新操作。
- 恢复歌曲时先排队定位，再解除暂停，避免短暂播放曲首。Native 实际状态不是物理设备输出音频的捕获证明，仍需客户端听感验收。
- 关闭立即撤销 Go 侧引用并提交异步 stop，后台 `mpv_terminate_destroy` 回收库线程；动态库代码保留至进程退出，避免关闭竞态和卸载后回调。

## 网络与库安全

- libmpv 只接受显式 127.0.0.1 HTTP 地址，由主进程生成，不能直接拿源 URL 联网；只启用探测所得的 FLAC/MP3 解复用格式，并将 FFmpeg 输入协议限制为 HTTP/TCP。
- 禁用用户配置、外部脚本、自动 profiles、ytdl、外部音频/字幕自动加载及引用跟随；不启用视频输出或 libmpv 自身的磁盘缓存；私有三槽磁盘缓存由 Go 单独管理。流和解复用 AVOptions 显式清空 `http_proxy`，避免把 loopback capability 交给环境代理；设置不可达环境代理的设备 smoke 已通过。
- MPV 解复用前/后向缓存上限分别 8MiB/2MiB；这不是整个播放器进程的内存硬上限。
- 主进程 `OpenMedia` 只支持 GET/HEAD、单段 Range；媒体允许公网 HTTP/80 或 HTTPS/443，逐连接固定校验过的公网 IP，沿用 Fake-IP DNS 兼容及私网/特殊网段/本机地址拒绝规则。
- 媒体最多三次重定向，逐跳校验；不转发源 API 的鉴权头、Cookie 或环境代理凭据。
- loopback 只绑定 127.0.0.1，严格 Host/随机路径、禁止 query，无任意目标 URL 参数；令牌 256 位、最长六小时，取消/切源/关闭即撤销并中断流。失败准备也撤销其令牌，迟到结果不能清除新会话。
- 单个代理最多四个同时读取；缓存层共用最多四个上游槽，其中预取最多占三个。单代理请求最长 20 分钟、最多 512MiB、32KiB 流式复制。探测拒绝 HTML/JSON/未知格式及异常长度/Content-Range。
- libmpv 在主进程内解析媒体，不等同于音源 worker 的操作系统沙箱；禁用协议/格式是缩小攻击面，不是解码器绝对安全保证。
- 动态库默认只从 `.app/Contents/Frameworks` 或 Windows 可执行文件旁的固定文件名加载，不搜索当前目录。Windows 依赖 DLL 限于库目录与 System32。开发/测试可显式指定绝对 `MIAOYIN_MPV_LIBRARY`，该变量不是音源可控输入。

## 构建与许可证

本地 macOS 集成使用校验 SHA-256 的 IINA v1.5.0 官方发行包中的 libmpv 0.41.0/FFmpeg 9.0.1。只复制所需依赖，裁剪 arm64，修正相对链接并重新签名；约 46MiB、44 个动态库。不安装 IINA、不要求用户安装 Homebrew。

```sh
python3 tools/prepare_libmpv_macos.py --arch arm64
# 然后执行正常 wails build；postBuildHooks 复制依赖并重新签名、严格验证。
```

准备脚本需 Python 3.11+ 和 Xcode 命令行工具。产物位于忽略的 `build/deps`；首次下载约 108MiB。开发热更新需显式设置：

```sh
MIAOYIN_MPV_LIBRARY="$PWD/build/deps/libmpv/Frameworks/libmpv.2.dylib" wails dev
```

这是 GPL 功能构建，不是 LGPL-only 发行。当前产物仅供本地开发验收，**不是可公开分发的正式 release**。完整应用许可、实际依赖声明及对应源码/构建材料仍待整理；通用上游链接不能替代 GPL 合规。来源说明见 [libmpv notice](third-party/libmpv.md)，随本地 `.app` 放入 Resources/third-party。

Windows FFI 已支持编译，DLL 获取/打包及真机测试按用户决定推迟到全部开发完成后统一进行。

## 状态与持久化

`player-controller.ts` 管理业务，`native-audio.ts` 适配 Wails libmpv 命令，`PlayerBar.svelte` 渲染控件。音量默认 100%，底栏与设置共享草稿，经既有 1000ms 自动保存。

点击列表替换队列并重置随机历史；上一首始终切歌，不依当前已播放时间改变含义。队列、索引、模式、进度以串行最新快照写 SQLite，选择/暂停/跳转/模式切换提交，播放中约五秒保存；关闭尽力保存。

启动只恢复暂停的歌曲记录和音量，不解析、不创建播放器实例、不联网播放。已恢复有效时长时允许在未播放状态选定并保存进度，明确点击播放才解析并在解除暂停前定位；未知时长不伪造进度范围。解析成功后的实际时长会保留到队列快照，供下次启动使用。持久化及播放/邻曲参数的 Song.duration 都使用 float64，与 libmpv/TypeScript 的小数秒一致，避免 Wails 在进入保存、重播或预取方法前因整数解码拒绝请求；旧 JSON 的整数秒仍可读取，无需 schema 升级。只存歌曲元数据，不存 URL、token、密钥或 playing 标记。

## 启动后台预热

窗口启动时并行预加载 libmpv 动态库/绑定，以及当前启用音源的本地副本；启动回调不等待。前者不创建播放器、输出设备或媒体会话；后者只读取系统密钥并解密，不执行脚本，不请求歌曲地址或读取歌曲，不消耗音源请求额度。

系统密钥用于解密后即尽力清除，不额外长期缓存主密钥；只将当前启用且校验通过的不可变脚本副本保留在主进程内存。解析复制独立快照，用完清除，worker 仍逐次隔离创建。切换、禁用、删除、重新检查当前音源及关闭时撤销并清理缓存；源变更后后台准备新的当前项。授权/解密采用单一进行中的任务，等待时释放管理锁；过期结果不能重新发布。

Keychain 首次预读仍可能在启动时授权。原生授权调用无法强制中断，用户禁用/切源仍可立即撤销缓存和唤醒等待者；授权晚到的结果会清除。若点击播放早于预热完成，仍可能等待剩余准备时间。服务端解析和每次 worker 启动不被这项预热消除，不保证首次等待完全消失。

## 本地播放耗时诊断

新增 `logs/playback.jsonl`（macOS：`~/Library/Application Support/妙音娘子/logs/playback.jsonl`），单文件最多 64KiB，仅保留一个 `.1` 备份。目录/文件按 0700/0600 设置；拒绝日志文件符号链接与非普通文件。异步单写入者、32 条有界队列；慢盘或不可写时丢弃诊断，不让日志阻塞播放，不上报、不弹 toast。关闭仅尽力等待 250ms，异常退出可能丢失最后少量诊断。

预热记为 `warmup`（序号 0），阶段为 `mpv_library` 或 `source_prepare`，无启用音源时不读取密钥。每次准备一条 `prepare`，当前会话第一次解除暂停再记录一条 `first_unpause`；普通暂停/继续不重复记录。仅包含 UTC 时间、本次启动内数字序号、固定操作/结果、白名单容器/普通音质、耗时与 playing/buffering 布尔值。**不含歌曲名、歌手、搜索词、歌曲/音源 ID、请求 ID、脚本、密钥、地址、路径或原始错误。**数字序号与时间用于关联同一次操作，重启后序号可重新开始。

`stages` 使用固定名称、毫秒值：

- `setup`：参数检查、旧会话关闭、代理准备、应用锁等待。
- `settings`：读取播放设置。
- `cache_prepare` / `cache_reuse`：建立/使用缓存项到原生实例可交接。命中项仍在准备时也可能等待，不代表即时出声。
- 以下 `resolve` 与 `probe` 仅在本次新建项时附加，命中预取项不重复计入背景耗时。
- `resolve`：音源解析总耗时，包含可能的降档尝试。内部另记录 `source_snapshot`（选中项快照）、`source_script_hit`（已准备脚本的内存复制）或 `source_script_prepare`（本地读取/解密或等待进行中的预热）、`source_request`（每档的隔离 worker 与服务请求）。这些是 `resolve` 的子阶段，不能与父阶段重复相加。
- `probe`：读取格式探测头及检查 Range，不是下载整曲。
- 历史直接流式路径曾细分 `source_check`、`lease`、`mpv_library`、`mpv_initialize`、`mpv_load_ready`、`commit`；当前三槽应用路径将原生准备合并计入缓存阶段。启动动态库预热仍单独记录 `mpv_library`。
- `unpause_command`：第一次解除暂停的命令回执。

`duration_ms` 是该操作后端耗时；`since_prepare_ms` 是从后端准备入口到记录点的时间。两条之间的空隙可能包括 Wails 跨桥调用、音量设置、恢复定位和调度。`first_unpause` **不代表捕获了扬声器首个音频样本**，不能当作严格的点击到出声延迟。失败/取消保留已尝试阶段，错误仅分类为 success/failed/canceled/timeout。当前不据此改动任何缓存或播放参数。

## 验证

- 常规 Go race/vet、漏洞扫描、前端类型/单元测试和 93 条 Playwright 用例通过；浏览器测试只验证桥接/交互，不再声称浏览器音频解码代表 libmpv。
- `internal/mpvaudio` 假客户端测试：输入限制、取消、失败清理、精确 seek 事件顺序、末端暂停/EOF/销毁。真实 smoke 在 6 秒合成 FLAC、320K、128K 上通过静音 CoreAudio 的暂停末端/播放中跳转、自然 EOF 与单曲循环控制。
- 显式 `MIAOYIN_MPV_SMOKE=1`、`MIAOYIN_MPV_LIBRARY` 可运行设备 smoke；额外 `MIAOYIN_MPV_MP3_TEST_DIR` 指向含 `320k.mp3`、`128k.mp3` 合成素材的目录。
- `MIAOYIN_LIVE_MPV_SEEK=1` + `MIAOYIN_SOURCE_TEST_PATH` + `MIAOYIN_MPV_LIBRARY` 运行 `TestProvidedLibmpvSeekLive`：真实《偷心》《可惜没如果》三个普通音质经父进程代理，中点、98%、99.9%、向后及播放中跳转确认均已分别通过。期间曾出现 QQ 搜索及音源解析暂时失败，显式单项重跑通过；没有增加生产自动重试/换源。会消耗真实源额度/流量，默认不运行，不输出敏感数据。
- `tools/probe_libmpv.py` 用已知 chirp 验证指定起点解码后的实际 PCM，并另测运行时状态/设备。二者分开，不把它们误称为“运行时 seek 后扬声器输出已被采集验证”。细节见 [跨平台调研](cross-platform-research.md)。

- 三槽合成/真实音源测试、并发及容量验证见 [缓存说明](cache-plan.md)。

最终仍需使用真实 `.app` 检查三档音质的播放、缓存切歌与听感定位；Windows 真机留待后续。
