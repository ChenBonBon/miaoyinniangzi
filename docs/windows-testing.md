# Windows x64 构建与验收

当前目标是开发验收便携包，不是公开发行安装包。**Windows 音源沙箱尚未实现，音源脚本执行保持安全阻断；不能据此宣布真实音源解析、播放、下载已可用。** 现阶段可验证构建、DLL 加载、界面、QQ 元数据、收藏/队列持久化和 Windows DPAPI 等基础能力。

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
6. 输出新的 `build/bin/windows-x64-时间-随机后缀/` 及同名 ZIP，包含 EXE、**两个 DLL**、依赖锁、哈希清单、版权文本及本说明。每次输出新目录，不清除旧验收包或 Mac 构建。

只构建、不运行可加 `-AllowMissingWebView2`，但启动前仍须安装 Runtime。需要 Wails 环境报告时可运行 `go run github.com/wailsapp/wails/v2/cmd/wails@v2.17.0 doctor`；其 Node/npm 提示不等于本项目 Bun 构建流程必须使用 Node。

## 3. 运行与反馈

解压完整 ZIP，运行目录中的 `miaoyinniangzi.exe`。**不要只复制 EXE，也不要把 DLL 放进 System32。** 同目录应有 `libmpv-2.dll` 和 `vulkan-1.dll`。应用目录可以整体移动；下载目录仍按设置决定。

优先检查：

- 是否正常显示窗口；从别的工作目录启动仍能找到相邻 DLL。
- 推荐、调整布局、搜索建议及显式搜索、歌手/专辑详情；平台限制应显示错误，不能冒充空结果。
- 收藏、设置和暂停队列重启恢复，且不自动播放/下载。
- `-Tests` 下 Windows DPAPI、文件 ACL/重解析点、SQLite、原子落盘等测试结果；编译成功不能替代运行成功。
- Windows 普通关闭应真正退出，不套用 macOS 的关闭隐藏行为。

暂不把以下失败判为完整功能回归：音源导入后的执行检查、音源激活/解析，以及依赖该解析的真实音乐播放和下载。原因是 Windows 源工作进程沙箱尚待开发，`sandbox_other.go` 会明确拒绝，不能通过移除沙箱检查来“修好”。原生 libmpv Warmup 成功也不代表可听播放、切歌、缓存或输出设备已经验收。

有错误请提供：Windows 版本、`go version` / `bun --version`、失败步骤、完整的非敏感报错、DLL 是否齐全。不要提供音源脚本、密钥、签名音乐地址、数据库或含识别性音乐信息的日志。

## 4. 依赖与构建限制

依赖固定在 `tools/windows-dependencies.json`，不自动追踪 latest。7zr 官方地址为固定文件名，因此另固定其 26.04 字节 SHA-256，站点更新后校验会失败，不能静默执行新版。缓存校验失败应删除报错对应的 `build/deps/windows-x64/` 文件后重新下载，不跳过哈希检查。

可单独检查：

```powershell
go run ./tools/windows_build verify
go run ./tools/windows_build native
```

本地环境只做了跨平台编译/PE 和哈希核验时，不算 Windows 实机验收。公开分发前还须补齐 Windows 沙箱、实际音频/下载测试、应用许可证、全部第三方许可与 GPL 对应源码、签名及安装包。详见 `THIRD-PARTY-WINDOWS.md`。
