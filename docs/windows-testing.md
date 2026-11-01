# Windows x64 构建与验收

当前目标是开发验收便携包，不是公开发行安装包。基础构建与界面已由用户在 Windows 验证。新增 AppContainer/LPAC + Job 音源 worker 已接入开发构建，**原生隔离和真实源全链路仍待实机通过**；先按 `WINDOWS-SANDBOX.md`（源码中为 [Windows 沙箱](windows-sandbox.md)）运行原生测试，再导入个人音源。隔离初始化失败始终拒绝执行，不降级。

## 1. 准备环境

使用 Windows 10/11 x64。保留已有 Bun，升级至 1.4.0 或更新版本：

```powershell
bun upgrade
```

安装 Windows amd64 的 Go 工具链（至少 1.26）：

```powershell
winget install --id GoLang.Go --exact
```

或从 https://go.dev/dl/ 安装 Windows amd64 MSI。安装后重新打开 PowerShell：

```powershell
go version
bun --version
```

项目 `go.mod` 指定 `toolchain go1.27.3`，脚本仅为当前进程启用 `GOTOOLCHAIN=auto`，必要时 Go 会从正常工具链渠道下载要求的版本。保持 Go 校验开启，不以关闭 GOSUMDB 绕过网络或校验错误。Windows ARM64/32 位 Go、Bun 不适用于此脚本。

安装或确认 WebView2 Evergreen Runtime：

```powershell
winget install --id Microsoft.EdgeWebView2Runtime --exact
```

也可使用微软官方下载：https://developer.microsoft.com/microsoft-edge/webview2/ 。Edge 浏览器本身不等于已安装 WebView2 Runtime。构建脚本检查当前用户及机器的 EdgeUpdate 注册表。

不要求预装 Node.js、Python、7-Zip、MinGW、Visual Studio 或全局 Wails CLI。Wails CLI 固定通过 Go 运行 v2.17.0；Go 构建关闭 CGO，播放器通过 purego/Windows 原生 DLL 调用。首次构建需要访问 Go 模块、Bun 包源及下述依赖官方站点。

## 2. 复制项目

复制完整项目源码，包含 `go.mod` / `go.sum`、`frontend/bun.lock`、`tools/`、`docs/` 和 `build/appicon.png`。**不复制 Mac 的 `node_modules`、已构建 `.app`、本地数据库、Keychain 数据或用户音源脚本。** Windows 会重新安装对应平台的前端原生包。

在仓库根目录执行：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\tools\build_windows.ps1 -Tests
```

这是本次进程的脚本策略，不修改系统 ExecutionPolicy。脚本只修改并在结束后恢复自身进程的 Go 构建环境，不安装全局 Wails、不修改 PATH 或系统 DLL。

它会：

1. 检查 Windows x64 Go/Bun、WebView2。
2. 校验并准备固定 libmpv、7zr 提取工具和 Vulkan Loader；不执行未经哈希验证的提取器。
3. 检查两个 DLL 的 SHA-256、PE x64 类型、必要导出及普通/延迟导入依赖闭包；拒绝缺失的非系统依赖。
4. 在 Windows 原生调用 libmpv client API 进行 Warmup；此检查不打开设备、不解析媒体、不播放音频。
5. 使用 Bun 冻结锁安装、类型/单元测试及前端构建，再运行固定 Wails 版本构建 EXE。跳过 Wails 内建 npm 前端步骤，不需要 Node。
6. 编译独立的 `miaoyin-source-worker.exe`，使用该实际辅助程序验证离线检查和伪造网络 broker 协议，不访问真实服务；失败立即停止。
7. 输出新的 `build/bin/windows-x64-时间-随机后缀/` 及同名 ZIP，包含主 EXE、辅助 EXE、**两个 DLL**、依赖锁、哈希清单、版权文本和验收说明。两个 EXE 都做 PE x64/系统依赖检查。每次输出新目录，不清除旧验收包或 Mac 构建。

只构建、不运行可加 `-AllowMissingWebView2`，但启动前仍须安装 Runtime。需要 Wails 环境报告时可运行 `go run github.com/wailsapp/wails/v2/cmd/wails@v2.17.0 doctor`；其 Node/npm 提示不等于本项目 Bun 构建流程必须使用 Node。

## 3. 运行与反馈

解压完整 ZIP，运行目录中的 `miaoyinniangzi.exe`。**不要只复制 EXE，也不要把 DLL 放进 System32。** 同目录应有 `miaoyin-source-worker.exe`、`libmpv-2.dll` 和 `vulkan-1.dll`。应用目录可以整体移动；下载目录仍按设置决定。

优先检查：

- 是否正常显示窗口；从别的工作目录启动仍能找到相邻 DLL。
- 推荐、调整布局、搜索建议及显式搜索、歌手/专辑详情；平台限制应显示错误，不能冒充空结果。
- 收藏、设置和暂停队列重启恢复，且不自动播放/下载。
- `-Tests` 下 Windows DPAPI、文件 ACL/重解析点、SQLite、原子落盘等测试结果；编译成功不能替代运行成功。
- Windows 普通关闭应真正退出，不套用 macOS 的关闭隐藏行为。

新增沙箱先通过 Windows 原生探针与辅助 EXE 冒烟，再验收个人源的离线检查、激活、解析及真实播放/缓存/下载。原生 libmpv Warmup 或交叉编译成功不代表隔离、可听播放、切歌或输出设备已经验收；不得删除安全检查来“修好”测试。

有错误请提供：Windows 版本、`go version` / `bun --version`、失败步骤、完整的非敏感报错、DLL 是否齐全。不要提供音源脚本、密钥、签名音乐地址、数据库或含识别性音乐信息的日志。

## Windows 测试的平台边界

私有数据库、缓存和日志在 Windows 使用当前用户与 SYSTEM 的 DACL，测试直接检查 ACL，不以 `os.FileMode` 的 POSIX 权限位判断 Windows 隐私。普通用户缺少符号链接权限时，只跳过相应子测试；可选启用 Windows 开发者模式后重跑，无需以管理员身份运行应用。打开的 `os.Root` 在 Windows 上可由系统禁止目录重命名，测试把这种明确的 sharing violation 作为更强的目录固定保护验证，其他错误仍然失败。

macOS 和 Windows 均运行工作进程正向协议测试；Windows 额外运行原生越权探针和无容器直接调用必须拒绝的测试，不绕过沙箱。下载发布针对 NTFS 复用已删除文件名可能产生的创建时间隧道效应，通过已验证的原文件句柄恢复创建时间，再完整复核卷、文件 ID 与创建时间；不忽略身份字段，也不按哈希推断未知文件归属。若重命名后、身份时间恢复前中断，恢复仍保守拒绝不匹配目标，不覆盖文件。

## 4. 依赖与构建限制

依赖固定在 `tools/windows-dependencies.json`，不自动追踪 latest。7zr 官方地址为固定文件名，因此另固定其 26.04 字节 SHA-256，站点更新后校验会失败，不能静默执行新版。缓存校验失败应删除报错对应的 `build/deps/windows-x64/` 文件后重新下载，不跳过哈希检查。

可单独检查：

```powershell
go run ./tools/windows_build verify
go run ./tools/windows_build native
```

本地环境只做了跨平台编译/PE 和哈希核验时，不算 Windows 实机验收。公开分发前还须完成 Windows 沙箱原生验收、实际音频/下载测试、应用许可证、全部第三方许可与 GPL 对应源码、签名及安装包。详见 `THIRD-PARTY-WINDOWS.md`。
