# 妙音娘子

macOS 本地音乐播放器，使用 Wails v2 + Svelte 5 + TypeScript + shadcn-svelte + Tailwind CSS v4。

## 界面开发

- 页面组织与按需加载见 [前端架构](docs/frontend.md)。设置分为播放、下载、音源三个二级分类。
- 基础组件位于 `frontend/src/lib/components/ui/`，由 shadcn-svelte CLI 生成，使用 Bits UI 提供交互。
- 使用 Tailwind 工具类布局，`frontend/src/app.css` 管理语义主题变量，不再使用全局按钮或输入框样式覆盖组件。
- 图标使用 `@lucide/svelte`，动效保留 Svelte transition 并适配减少动画偏好。
- 添加组件：在 `frontend` 目录运行 `npx shadcn-svelte@latest add <组件名>`。

## 当前状态

已建立桌面入口与推荐、搜索、下载、设置界面。QQ 音乐歌曲搜索已接通真实数据，入口在顶部搜索栏，支持列表、分页、取消、错误重试，以及本地持久化历史 tags，详见 [QQ 音乐搜索](docs/qq-search.md)。搜索结果现可显式解析当前音源，支持普通音质降级与取消，详见 [音源运行时](docs/source-runtime.md#真实解析)。点击歌曲已接通真实解析与播放，支持暂停、切歌、拖动、音量和恢复暂停状态，详见 [播放链路](docs/playback.md)。没有演示曲目。已接入 [下载任务](docs/downloads.md)，支持搜索/收藏的单曲与批量下载、安全去重和手动断点续传。

已实现 SQLite 本地持久化基础层与 Wails 读写绑定，支持设置、收藏和播放快照。设置页已接通读取、保存、恢复，以及原生下载目录选择；收藏已接入搜索、底栏、播放详情及左侧「喜欢」页面，详见 [收藏](docs/favorites.md)；播放快照已接入恢复流程。详见 [本地持久化](docs/persistence.md)。

已实现音源本地导入、AES-GCM 加密副本、Keychain 密钥保管、内容去重与确认删除，详见 [音源导入](docs/sources.md)。导入成功后自动在隔离进程中离线校验（含重复导入），可手动重新校验；不自动启用。已支持单选启用、禁用和重启恢复，详见 [音源运行器](docs/source-runtime.md)。

已支持 QQ 搜索、音源解析及统一 libmpv 的 FLAC/MP3 流式播放、队列与暂停恢复，详见 [播放](docs/playback.md)。已接入 [播放详情与逐行歌词](docs/lyrics.md) 和 [三槽播放缓存](docs/cache-plan.md)（当前完整后台缓冲、邻曲前缀与原生预热），以及 [播放队列面板](docs/queue.md)（点选、移除、清空与下一首播放）。[推荐榜单](docs/discovery.md) 已接通真实目录与完整曲目列表，并支持自定义分组/榜单的显隐和顺序；[歌手与专辑详情](docs/music-details.md) 已接通真实元数据、热门歌曲和完整专辑曲目。macOS 已支持[关闭隐藏、Dock 恢复及真正退出](docs/window-lifecycle.md)。用户提供的音源脚本未复制进项目。

## 前端预览

需要 Node.js 22.12+（或受 Vite 支持的更新版本）。

```sh
cd frontend
npm ci
npm run dev
```

## macOS 桌面开发

当前验证环境为 Go 1.27.3、Wails v2.17.0、macOS arm64；最低系统版本随 Go 工具链设为 macOS 13。需安装 Xcode Command Line Tools。

```sh
xcode-select --install
go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.0
# 将 $(go env GOPATH)/bin 加入 PATH
wails doctor
go mod tidy
# Python 3.11+；首次下载并校验约 108MiB 的上游开发依赖，不安装 IINA
python3 tools/prepare_libmpv_macos.py --arch arm64
MIAOYIN_MPV_LIBRARY="$PWD/build/deps/libmpv/Frameworks/libmpv.2.dylib" wails dev
```

构建：`CGO_CFLAGS='-mmacosx-version-min=13.0' CGO_LDFLAGS='-mmacosx-version-min=13.0' wails build`。生成的 go.sum 应纳入版本控制。

已升级 Wails 以解决旧版本绑定生成器与 Go 1.27 的兼容问题，并更新存在已知安全问题的传递依赖。不再使用 Go 1.25.3 临时工具链。

## Windows x64 开发验收

Windows 上保留 Bun，安装 Go 和 WebView2 后，在仓库根目录执行：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\tools\build_windows.ps1 -Tests
```

脚本使用冻结的 Bun 锁、固定 Wails CLI 和经过 SHA-256/PE 校验的 libmpv + Vulkan Loader，输出新的便携目录及 ZIP，不需要预装 Node/Python/7-Zip。详见 [Windows 构建与验收](docs/windows-testing.md)。**Windows x64 build 26200 的原生沙箱及完整音源运行时测试已由用户实测通过；完整便携包中的个人源播放、跳转、切歌、缓存和下载后文件播放也已通过。新环境仍须先运行 [沙箱原生测试](docs/windows-sandbox.md)，任何隔离步骤失败仍拒绝执行。当前 FAT32 U 盘的基础下载及合成恢复已通过；exFAT、真实拔插/满盘以及发行合规/签名/安装包尚未完成，该包不是公开发行版本。**

## 检查

```sh
cd frontend
npm run check
npm run build
```

产物为 `build/bin/妙音娘子.app`。构建后自动打包 libmpv 依赖并重新签名验证；没有准备依赖时构建会失败。原生目录选择、Keychain 授权及实际播放听感仍需在客户端验收。当前 libmpv 包是本地开发构建，公开发行前仍需完成 [第三方许可证与对应源码清单](docs/third-party/libmpv.md)。

脚本隔离使用 Seatbelt 的 `sandbox_init`，当前 SDK 会输出弃用警告；启用限制失败时拒绝执行，不会降级到未隔离模式。发行前需评估签名后的 XPC/App Sandbox 辅助服务，详见运行器文档。

## 接入边界

- QQ 音乐适配层已接通搜索，后续负责榜单、新歌和详情；不假设洛雪脚本提供这些能力。
- 曲目模型和解析请求仅允许 QQ 音乐 `tx`。
- 音源脚本可能包含密钥，只保存在应用私有数据目录，不纳入代码仓库。
- 当前脚本校验运行于隔离进程，禁止文件、系统命令、Wails 桥接及网络请求；用于导入后的自动校验、手动校验及启用前重校验。真实解析阶段已接入父进程受控网络代理，详见 [音源运行时](docs/source-runtime.md)。
- 禁用音源后不得继续解析新地址；下载需要校验响应、限制并发、支持取消并安全写入文件。
- 仅在获得必要授权、符合服务条款的前提下使用音乐及下载能力。
