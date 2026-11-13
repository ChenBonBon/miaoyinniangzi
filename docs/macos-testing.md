# macOS 验收

跨平台功能在 macOS 和 Windows 分别验收。Go/浏览器测试、原生探针和完整 `.app` 手动体验分开记录，不用其中一项代替其他项。

## 本轮结果

基于 `05848ee` 的推荐缓存/脱敏诊断和 Windows 残留回收改动，补充 macOS 验收；另修正搜索 E2E 对 macOS WebKit 键盘遍历的假设，未改变应用焦点行为。

当前环境：macOS 27.2、arm64。以下已执行通过：

- 全项目 `go test -race -count=1 -timeout=90s ./...` 和 `go vet ./...`。
- Svelte/TypeScript 检查零错误、零警告，124 条前端单元测试及生产前端构建。
- WebKit、Chromium 全量 E2E 各 95 条，包括推荐持久缓存、刷新失败保留内容、显式重试、无缓存错误、切页/取消及迟到响应保护。使用合成桥接，**不是实际 Wails 后端或真实 QQ 断网验收**。
- 独立临时 `.app` 的 Cocoa/Wails 生命周期探针，菜单退出和系统 terminate 两种模式均通过：关闭/⌘W 隐藏、最小化/Dock 恢复、WebView 不重载、后台上下文存活、隐藏窗口确认退出及真正退出。探针不使用播放器的个人数据，不能单独替代完整应用验收；用户后续已确认实际应用退出/恢复回归通过，见下文。
- 真实 libmpv 加载合成 FLAC，通过本地回环服务验证暂停、播放时钟、精确跳转和结束状态；预备依赖及最终 `.app/Contents/Frameworks` 中的实际库均通过。输出初始静音，不播放用户音乐。此自动探针未覆盖真实 MP3/听感；用户后续已确认完整应用 FLAC、320K/128K MP3 播放正常，见下文。
- 在当前 Mac 真实网络下单次执行匿名 QQ 元数据 opt-in：榜单目录/内容、新歌目录/内容均通过，不记录音乐标识，不解析、播放或下载媒体；仍不等于完整应用的断网/恢复验收。
- 完整 `build/bin/妙音娘子.app` 构建成功，自动打包 libmpv，并通过 `codesign --verify --deep --strict`；主程序架构 arm64，最低系统标记 13.0。当前签名为本地 ad-hoc，不是 Developer ID 签名或公证；不表示 macOS 13 实机通过。

推荐错误改为 toast 后，追加通过前端类型检查、124 条单测及 WebKit/Chromium 各 97 条 E2E；验证无缓存目录/榜单报错、缓存列表保留、无内嵌错误卡片/重试按钮、重复失败不叠加及顶部刷新恢复。桌面与紧凑窗口截图已检查；用户已在 `macos-toast-571d503` 完整应用确认本轮 toast 改动测试通过。

Windows worker 残留回收只在 Windows 编译，macOS 没有 AppContainer staging，因此该协议在 macOS 为“不适用”，不是“跨平台已通过”。Windows 新回收测试仍按 [Windows 验收](windows-testing.md) 执行。

已知限制：Seatbelt 的系统弃用警告仍存在，不通过关闭隔离来消除警告。

## 完整应用手动验收

用户已在 `build/bin/macos-toast-571d503/妙音娘子.app` 按另行提供的七项基础回归清单反馈测试通过：已有个人源手动校验，普通 FLAC 和 320K/128K MP3 声音正常，暂停/继续/前后跳转/切歌及浏览推荐不重启音频，新建下载与不覆盖已有文件，关闭/⌘W 隐藏和 Dock 恢复不打断播放，⌘Q 后队列/暂停位置恢复但不自动播放，以及下载中退出后重开不自动续传、手动继续正常完成。

该反馈记录为当前 macOS 用户环境的完整应用基础回归通过，不扩大为 macOS 13、其他设备、长时间压力或真实拔插/满盘/断电验收。以下保留推荐专项和基础回归操作；用户随后另行确认推荐专项测试通过，推荐重启缓存及联网恢复细项也记录为当前环境通过。

先用 ⌘Q 退出旧版本，再从仓库根目录打开本轮完整包：

```sh
open 'build/bin/macos-toast-571d503/妙音娘子.app'
```

不清数据库、不删除缓存或音源、不重新导入脚本。断网动作由用户自行决定，测试结束恢复原网络设置；不要提供歌曲名、脚本、密钥、URL 或完整个人日志。

1. **联网与重启缓存**：进入推荐，加载一个榜单，真正退出后重开。上次成功内容可恢复，歌曲不自动播放、任务不自动下载；已有音源不会被自动重新校验或启用。
2. **已有缓存的断网刷新**：断网后回到已成功加载的榜单，显式刷新。内容仍可浏览，toast 提示“更新未完成，已保留上次内容”；正文不显示错误卡片，toast 没有重试按钮。仅显示缓存不应解析或播放歌曲。
3. **无缓存的断网请求**：打开一个从未加载过的榜单。toast 提示读取失败，而不是“没有歌曲”；正文不显示错误卡片，toast 没有重试按钮。不能把另一个榜单的内容当作本榜单成功结果。
4. **恢复联网与重试**：恢复网络，通过顶部「刷新」显式重试，成功后旧 toast 清除并显示正确内容；重复点击不应造成并行重复刷新。返回目录/切换榜单时，旧响应不应覆盖当前页面。
5. **音源与播放回归**：使用已有且启用的个人音源，显式校验、播放、暂停/继续、跳转和切歌；覆盖普通 FLAC、320K/128K MP3。浏览推荐和切换页面不应重启当前音频。若要测试断网不影响浏览时的播放，选已完整缓存或已下载的曲目，不能要求尚未下载的媒体在断网后继续获取字节。
6. **下载及退出回归**：向现有正常目录新建一笔下载，成功文件可播放，不覆盖已有文件；关闭/⌘W 隐藏、Dock 恢复、⌘Q 真正退出，重开保留暂停位置和队列，不自动续传。不做格式化、满盘或真实拔盘测试。

用户已用上一版完整应用确认第 2、3 项的失败/缓存保留语义正常，随后在 `macos-toast-571d503` 完整应用确认无重试按钮的 toast 改动测试通过。该推荐反馈记录为第 2、3 项通过。随后用户按七项清单确认个人源校验、播放、下载、窗口生命周期、暂停队列恢复及手动续传基础回归通过；用户随后另行确认推荐专项测试通过，第 1、4 项也记录为当前环境通过。记录反馈只需场景编号和通过/失败、固定错误提示，不需要音乐标识。

## 复现自动检查

```sh
export DEVELOPER_DIR=/Library/Developer/CommandLineTools
export PATH="/usr/local/go/bin:$HOME/go/bin:$PATH"
export CGO_CFLAGS='-mmacosx-version-min=13.0'
export CGO_LDFLAGS='-mmacosx-version-min=13.0'
go test -race -count=1 -timeout=90s ./...
go vet ./...
(cd frontend && bun run check && bun run test && bun run build)
(cd frontend && MIAOYIN_E2E_BROWSER=webkit bunx playwright test)
(cd frontend && bunx playwright test)
python3 tools/test_window_lifecycle_macos.py
MIAOYIN_MPV_SMOKE=1 \
MIAOYIN_MPV_LIBRARY="$PWD/build/deps/libmpv/Frameworks/libmpv.2.dylib" \
go test -count=1 -timeout=45s -v ./internal/mpvaudio -run '^TestLibmpvLoopbackPlaybackAndSeeking$'
# 可选：真实匿名 QQ 元数据，无自动重试，不输出曲目信息
MIAOYIN_LIVE_QQ_DISCOVERY=1 go test -count=1 -timeout=45s -v ./internal/music -run '^TestLiveQQDiscovery$'
wails build -s -m -nosyncgomod
codesign --verify --deep --strict 'build/bin/妙音娘子.app'
# 再用最终包中的实际依赖复验
MIAOYIN_MPV_SMOKE=1 \
MIAOYIN_MPV_LIBRARY="$PWD/build/bin/妙音娘子.app/Contents/Frameworks/libmpv.2.dylib" \
go test -count=1 -timeout=45s -v ./internal/mpvaudio -run '^TestLibmpvLoopbackPlaybackAndSeeking$'
```

需事先准备固定的 libmpv 依赖，详见 [README](../README.md#macos-桌面开发)。Playwright 使用生产预览，必须先构建前端；上述真实音频测试有明确 opt-in，采用合成静音起播，不应改成用户歌曲或关闭任何沙箱/网络限制。构建前先退出旧 `.app`，按需保留旧开发产物，不使用清空整个 `build/bin` 的选项。

macOS 默认 WebKit 的 Tab 周期可能跳过按钮，Option-Tab 可遍历全部控件。E2E 同时保留 Tab 关闭建议且不丢草稿/不提交的断言，并在需要时用 Option-Tab 验证搜索按钮可达；不修改系统全键盘控制偏好、不强制组件抢焦点。

## 基于收藏的真实猜你喜欢（2026-11-13，待用户验收）

实现提交 `d52ad36` / `74d778f` / `6514a57`，规则与缓存边界见 [推荐](discovery.md)。未读取用户收藏做接口探测，未清空任何现有记录，未调用用户音源或解析/播放/下载真实媒体。

- Go 全量 race、vet 通过；Windows amd64 CGO=0 全量编译/交叉 vet 通过，只证明可编译，不代表在 Windows 执行或完整包验收。
- 类型检查零错误/警告，132 条前端单测；Chromium / WebKit 各 103 条 E2E。覆盖无收藏不填假歌曲、12 首三列/四列、缩放不换批、本地换批不增请求、真实播放/专辑控制器桥接、后退保留批次、迟到推荐不改变榜单页面、未点击不解析媒体及既有回归。截图检查为合成桥接/封面，不是用户推荐质量或实际音源播放证据。
- 两次独立显式匿名公共种子测试：`TestLiveQQSimilar` 的真实 QQ 读取/规范化得到 12 首；`TestLiveForYouWithIsolatedPublicSeed` 通过隔离临时收藏库和真实 QQ 客户端构建 12 首候选。只输出计数，未输出歌曲/种子信息；无登录、用户收藏、媒体地址或源调用。
- macOS arm64 独立完整包 `build/bin/macos-foryou-real-6514a57/妙音娘子.app` 已构建并严格验证本地 ad-hoc 签名，保留旧包不覆盖；打包了原有固定 libmpv 依赖，构建使用最低 13.0 参数。没有将构建/签名成功当作 macOS 13 或当前用户完整应用通过。
- 待用户在独立确认清空收藏记录后，重新收藏 5–7 首进行推荐有效性、换批、真实专辑和个人源显式播放验收。只允许清空收藏记录，不动音乐文件、源、队列、下载或设置；当前尚未执行。Windows 真实接入完整包验收继续延后。
