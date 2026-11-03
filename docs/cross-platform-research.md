# 跨平台播放与音源隔离调研

状态：后续已将 FLAC/MP3 统一接入 libmpv，真实源三档与本地打包进入 macOS 验收；当前实现见 [播放](playback.md)。本页保留初轮调研的实验边界。macOS 保留 Keychain；Windows 当前用户 DPAPI 已实现；后续 Windows x64 build 26200 的原生音源运行时及完整便携包基础播放/缓存/下载链路已由用户实测通过，当前验收范围见 [Windows 记录](windows-testing.md)。

## 1. 系统密钥存储

**后续决定：保留 macOS Keychain，Windows 使用当前用户 DPAPI；不采用下述本地明文密钥提案，也不执行 Keychain 迁移。实现与验证边界见 [音源存储](sources.md#windows-dpapi)。以下保留为早期方案讨论。**

调研初始阶段 `internal/sources/keychain_darwin.go` 使用 macOS Keychain，其他平台的实现只返回不支持；后续已添加 Windows DPAPI 实现。

建议采用跨平台的本地 32 字节密钥文件，继续使用现有 AES-256-GCM 副本格式，不引入 Windows Credential Manager 或 DPAPI。此选择需要用户确认安全取舍：

- macOS：数据目录 0700、密钥文件 0600，拒绝符号链接、异常权限和错误长度。
- Windows：用当前用户 SID 与 SYSTEM 的受保护 DACL，禁用宽泛继承；`os.Chmod(0600)` 在 Windows 上不能替代 ACL 设置与验证。
- 密钥不进入数据库、日志、设置导出、Wails 返回值或音源 worker。
- 可以减少单独导出 SQLite 文件时泄露脚本的风险，但**复制整个数据目录即可同时取得密文和密钥**。同用户下的其他进程也更容易读取密钥。这不是 Keychain 的等价安全替代。

安全迁移顺序：

1. 检查现有副本和本地密钥。已有密文却缺失密钥时，不能静默生成一个新密钥覆盖旧数据的访问路径。
2. 对现有 macOS 数据显式读取旧 Keychain 密钥，验证现有副本能解密。迁移可能仍触发系统授权；不能承诺迁移时完全不访问 Keychain。
3. 原子写入并同步私有密钥文件，重新读取并验证。复用原密钥，因此不必改写音源密文或重置音源、搜索历史、播放记录。
4. 验证成功后再退役旧 Keychain 项；失败时保留旧项与全部副本。之后正常使用不再访问系统密钥存储。
5. Windows 新安装直接生成私有本地密钥。缺失/损坏时明确报错，不自动重置或删除音源。

若不希望迁移时再授权，也可设计“用户显式重新导入原始脚本”的迁移路径，但不能直接删掉旧密文或假定用户仍持有原文件。

## 2. libmpv 验证

### 环境与边界

- macOS arm64，libmpv 0.41.0 / FFmpeg 9.0.1。
- 临时只读挂载官方 IINA v1.5.0 DMG，使用其 libmpv C API，不安装 IINA、不修改应用依赖。
- DMG SHA-256：`8fad50479b10e09645053457cec46bd53d054ced2242cfe68f74586735a20dc7`，与该 GitHub release 的 digest 相符。
- 合成 40 秒、44.1kHz 单声道 chirp：瞬时频率 `220 + 50*t` Hz，编码为 FLAC、CBR MP3 320K、CBR MP3 128K。测试素材不包含歌曲、音源脚本、密钥或签名地址。
- IINA 的 FFmpeg 没有 libmp3lame 编码器，因此 MP3 测试素材由临时解包的官方 Homebrew LAME bottle 生成。**缺编码器不代表缺 MP3 解码器**。未执行 `brew install`。
- 使用本地 HTTP Range 服务，解复用缓存限制为 128KiB，观察到了三种格式的非零 Range 请求。

探针：`tools/probe_libmpv.py`，仅使用 Python 标准库。需自行提供可信 libmpv 及其可发现的动态依赖；若 libmpv 不含 MP3 编码器，额外传入 `--lame`。本次 macOS 借库使用 `DYLD_FALLBACK_LIBRARY_PATH` 指向 IINA 的 Frameworks 及临时 mpg123 的 lib 目录，再执行以下命令；不要用 `DYLD_LIBRARY_PATH` 全面覆盖系统 Swift 库。

```sh
python3 tools/probe_libmpv.py \
  --library /absolute/path/to/libmpv \
  --lame /absolute/path/to/lame

# 可选：使用真实输出设备，但始终静音；macOS 用 CoreAudio，Windows 用 WASAPI。
python3 tools/probe_libmpv.py \
  --library /absolute/path/to/libmpv \
  --lame /absolute/path/to/lame \
  --device-smoke
```

### 已完成

| 项目 | FLAC | MP3 320K | MP3 128K |
| --- | --- | --- | --- |
| HTTP 读取及非零 Range | 通过 | 通过 | 通过 |
| 2s、中点、98%、99.9% 起始定位的实际 PCM 内容 | 通过 | 通过 | 通过 |
| 暂停时末端跳转、再向前跳转 | 通过 | 通过 | 通过 |
| 播放中跳转、音量、停止 | 通过 | 通过 | 通过 |
| CoreAudio 真实设备静音运行上述控制 | 通过 | 通过 | 通过 |

PCM 使用 `ao=pcm` 导出并通过已知 chirp 的过零点估算实际内容时间，取中央区间避免截断边缘和有损编码预热扰动。验收阈值为 25ms。本次样本的最大估算误差分别为 FLAC 0.061ms、320K 0.031ms、128K 0.034ms；这些值**不是扬声器输出延迟或通用定位精度保证**。

必须区分两种验证：

- PCM 内容验证使用 `start` 定位后解码导出。
- 运行时 `seek absolute+exact` 另用 timed null AO 和静音 CoreAudio 检查时钟、暂停状态与控制。

不能把两者合称为“已经捕获并证明真实输出设备在运行时 seek 后的音频”。PCM 文件 AO 是 untimed 输出，在暂停/EOF 附近的缓冲行为不适合模拟真实设备；早期这样测试会出现空导出，最终探针明确拆分两种验证，而不是隐藏重试。

### 接入时需要处理

- 显式使用 `absolute+exact`。测试采用 `hr-seek=yes` 和 `hr-seek-demuxer-offset=0.25`：先从目标前方解复用，再解码丢弃到目标，降低 MP3 预热/bit reservoir 在短末段的影响。没有此预读时，128K 极短末段的测试信号曾无法可靠测量。
- 等待 `FILE_LOADED` 后的首次 `PLAYBACK_RESTART`，再执行运行时跳转，避免输出设备尚未建立的竞态。
- `keep-open=always` 防止末端 EOF 自动卸载文件，否则暂停末端后再 seek 可能失败。应用需观察 EOF 并自行实现队列、单曲循环和暂停意图，不依赖 libmpv 默认播放列表策略。
- 保留 Go 的受控网络读取和不透明 loopback URL，不能把源签名地址直接交给 libmpv，也不能让 libmpv 绕过公网地址校验。
- 生产应禁用用户配置、脚本、ytdl、外部文件自动加载与不必要解复用器；只读已经探测为 FLAC/MP3 的媒体。研究探针不是安全播放器后端。
- IINA 全功能包依赖较多；早期通过 `DYLD_LIBRARY_PATH` 全覆盖借库时出现 Swift 兼容库重复类警告，改用 `DYLD_FALLBACK_LIBRARY_PATH` 后静音设备测试通过且不再出现该警告。不应将该包或临时环境变量原样当作产品依赖：需要可复现的音频专用构建、正确的 dylib/DLL 搜索路径和签名。
- libmpv 默认通常是 GPL 构建。闭源发行若选择 LGPL 路径，需核对 `-Dgpl=false`、FFmpeg 与所有链接依赖的实际许可证，并履行分发义务，不能只看“动态链接”就认定合规。

### 初轮调研时尚未完成

后续进展：统一后端、真实两首歌曲三档流式跳转、本地动态库打包现已接入，见 [播放记录](playback.md#验证)。Windows x64 当前实机基础链路已有用户验收记录；完整发行许可证/源码包、其他系统、边缘网络与设备场景仍待后续。下列清单为初轮历史状态。

- 两首真实歌曲通过“现有 Go 代理 → libmpv”链路的用户听感验收。
- VBR MP3、缺失 Xing/索引、断网/慢网/过期、快速连续切歌、设备切换、休眠恢复、长时间资源释放。
- Windows x64/arm64 的真实 DLL 打包、WASAPI、WebView2/应用联调与跳转验证。
- 初轮探针未接入应用；后续已接入，不能再把本句当作当前运行状态。

结论：具备统一 FLAC/MP3 引擎的可行性，有实测基础；尚不是跨平台产品验收完成。

## 3. Windows 的 Seatbelt 替代方案

后续实现已接入独立 Go helper、AppContainer/LPAC、创建时 Job、句柄白名单和原生越权探针；Windows x64 build 26200 的原生边界及完整运行时测试已由用户实测通过，不推广为所有系统/策略均已验收。当前细节与限制见 [Windows 沙箱](windows-sandbox.md)，下文保留初轮调研方案。

Windows 没有同名 Seatbelt。建议仅把不可信音源 worker 放进 **AppContainer + Job Object**，主程序作为受控 broker，复用现有 JSONL、VM 限制及网络策略。不是把整个 Wails 主程序直接放进 AppContainer。

建议启动序列：

1. 创建/取得专用 AppContainer profile/SID。评估 LPAC 是否适用，减少普通 AppContainer 经 `ALL APPLICATION PACKAGES` 可读的资源；不能假定所有系统版本的 LPAC 配置相同。
2. 只对 worker 可执行文件及必要运行时依赖授予容器 SID 的读取/执行权限。不给应用数据、SQLite、本地密钥和用户文件访问权；不能给整个用户目录或应用数据目录增加容器权限。
3. 通过 `STARTUPINFOEX` + `PROC_THREAD_ATTRIBUTE_SECURITY_CAPABILITIES` 在创建时施加容器 token，不授予网络 capabilities；不能先启动普通权限 worker 再尝试降权。
4. 通过显式 handle list 仅继承所需 stdin/stdout 端点，避免泄露主进程、文件、密钥或 job handles。继续使用有帧大小和时限的现有协议。
5. 创建暂停进程，加入已配置的 Job Object 后才恢复执行；或使用经过验证的创建时 job list。设置 kill-on-job-close、单进程、CPU 时间和提交内存上限，禁止 breakaway，结合 child-process policy 禁止子进程。Windows 提交内存限制与 macOS RSS 限制不是同一指标，需实机校准 Go worker 的预算。
6. worker 启动后验证自己确实处于 AppContainer、预期 job/token 中，然后才接收脚本。任一步失败都拒绝运行，不降级为普通进程。
7. worker 仍不持有 Goja 文件/进程/直接网络 API；源 HTTP RPC 只由父进程校验并发起。AppContainer 是第二道边界，不是替代 VM 限制。

边界：

- **Job Object 是资源与生命周期控制，不是文件或网络沙箱。**
- 低完整性 token 主要限制写入，不能独立保证用户文件不可读取。
- AppContainer 通常有自己的隔离存储及可读系统资源，不等价于当前 Seatbelt 的逐项 deny-default。需要检查存储、注册表、设备、现有 loopback 豁免与对象 ACL，不能宣传“零文件访问”。
- 优先评估独立最小 `source-worker.exe`，避免把 Wails/WebView/native audio 的初始化和依赖带进容器。

Windows 真机验收至少覆盖：读取密钥/数据库/桌面文件被拒；写入容器外被拒；直接访问公网/局域网/loopback 被拒；启动子进程被拒；继承多余句柄被拒；内存/CPU/超时后被回收；父进程崩溃 worker 被清理；正常脚本与父进程代发 HTTP 仍可用。调研时环境只有 macOS，上述为初轮待验证清单；后续已完成的当前 Windows x64 实机测试范围与未测项见 [Windows 沙箱](windows-sandbox.md) 和 [Windows 验收](windows-testing.md)，不把此历史清单整体标为通过。

## 参考

- [mpv libmpv client API](https://github.com/mpv-player/mpv/blob/master/include/mpv/client.h)
- [mpv manual](https://mpv.io/manual/stable/)
- [mpv licensing](https://github.com/mpv-player/mpv/blob/master/Copyright)
- [IINA v1.5.0 release](https://github.com/iina/iina/releases/tag/v1.5.0)
- [AppContainer isolation](https://learn.microsoft.com/en-us/windows/win32/secauthz/appcontainer-isolation)
- [AppContainer for legacy apps](https://learn.microsoft.com/en-us/windows/win32/secauthz/appcontainer-for-legacy-applications-)
- [UpdateProcThreadAttribute](https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-updateprocthreadattribute)
- [Job Objects](https://learn.microsoft.com/en-us/windows/win32/procthread/job-objects)
