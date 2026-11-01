# macOS 窗口生命周期

仅 macOS 启用关闭即隐藏，不改变 Windows 的关闭行为。

- 红色关闭按钮和「窗口 / 关闭窗口」（⌘W）隐藏应用，不退出、不卸载 WebView，不调用播放器 stop，不取消下载、缓存、歌词或元数据请求。
- 点击 Dock 图标/系统重开事件重新激活原来的窗口；最小化后的重开也恢复窗口。优先采用 Cocoa/Wails 原生行为，不替换 NSApplicationDelegate，不引入额外进程或第二个 WebView。
- 「窗口」菜单提供显示主窗口（⌘0）、关闭窗口（⌘W）、最小化（⌘M），作为显式恢复入口。保留原生应用和编辑菜单。
- 原生 Quit（⌘Q）、Dock 退出或系统退出请求才结束应用，不被改成隐藏。

## 真正退出

macOS `OnBeforeClose` 仅用于真正退出；Wails 的 `HideWindowOnClose` 在 NSWindowDelegate 层先处理普通关闭，因此隐藏不触发退出握手。

1. 后端发出一次 `app:before-quit`，附短请求编号。并发退出请求不重复开始。
2. 前端停止音频，保留当前选择/顺序/进度，并等待最新 SavePlayback 队列完成；随后 `AcknowledgeQuit`。
3. 后端最多等待两秒，前端等待持久化最多 1.5 秒。未响应或保存失败不会永久锁住退出，异常情况沿用最近成功的播放检查点，不声称所有故障下都能保存最新状态。
4. Wails 结束后仍按原有顺序关闭下载管理器、播放/缓存、音源、诊断和 SQLite。下载保留已同步断点，重启保持暂停，用户手动继续，不自动联网续传或播放。

编号必须匹配在途退出且只能确认一次；不含歌曲、路径、URL、账号或密钥。关闭/显示窗口不调用该确认接口。

## 验证

Go race/vet 测试覆盖平台范围、退出确认/超时/重复请求/取消上下文；前端覆盖等待保存后确认、非法事件、异常不阻塞退出、暂停队列保留和实际 UI 中的最新播放进度同步。

原生 Cocoa/Wails smoke：

```sh
export PATH="/usr/local/go/bin:$HOME/go/bin:$PATH"
python3 tools/test_window_lifecycle_macos.py
```

此测试构建独立临时 `.app`、独立 bundle ID，不读用户播放器数据库、音源脚本或 Keychain，也不结束已有播放器进程。真实调用原生窗口关闭、最小化、Launch Services 重开、⌘W 对应菜单、⌘Q 对应菜单与 NSApplication terminate；验证隐藏时应用上下文/后台计时仍存活、窗口恢复、WebView 不重载、隐藏 WebView 可回应退出、真正退出后进程结束。已通过两种退出入口。

该 smoke 验证原生生命周期，不冒充实际音乐可听性或大文件下载测试。最终 `.app` 验收仍应播放音乐并下载任务后：关闭窗口、等待、点击 Dock 检查两者未中断，再 ⌘Q 重开确认队列/进度及手动续传状态。
