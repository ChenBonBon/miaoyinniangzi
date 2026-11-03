# Windows 音源 worker 隔离

状态：实现已接入开发构建；**Windows 原生隔离测试及真实源验收仍待实机通过**。交叉编译不是隔离验证，不保证绝对防逃逸。macOS 继续使用原有 Seatbelt 路径。

## 边界

Windows 使用独立的 GUI 子系统（`-H=windowsgui`）`miaoyin-source-worker.exe`，只链接 Goja 和受控协议所需代码，不初始化 Wails、SQLite、DPAPI 或播放器。主进程解密脚本后经匿名管道传送，不把脚本、密钥、请求、歌词或音乐地址放进命令行、环境、临时文件和日志。

主程序与辅助 EXE 必须来自同次构建并保持相邻；缺少辅助程序直接失败，不回退到主进程或普通权限进程。macOS 保持同二进制辅助分支，不受这个布局影响。Windows 使用独立的运行器版本 `lx-inspect-win-lpac-afd-v2`，旧检查状态需重新校验，不自动启用音源。

## 创建顺序

1. 创建/取得当前用户专用的 `com.miaoyinniangzi.source.v1` AppContainer profile/SID；不安装系统服务，不提权，不修改网络豁免。拒绝该 SID 已存在的 loopback 豁免。
2. 只在这个专用 profile 的 package/AC 目录设置当前用户、SYSTEM 完全控制和该 SID 只读/执行的 DACL；拒绝未知目录布局或 reparse point，不更改全局 AppData、Packages 或其他应用目录权限。
3. 使用原生 handle-relative 原子创建私有随机 staging 子目录，ACL 在创建时生效。容器 SID 只读/执行，OWNER RIGHTS 限制隐式 WRITE_DAC；保留不共享删除的目录句柄。只复制公开的辅助 EXE，校验复制前后 SHA-256，固定只读且不共享写/删的文件句柄。正常结束删除副本。
4. 配置匿名 Job：单进程、2 秒用户 CPU 时间、256MiB 进程/Job 提交内存、kill-on-job-close、禁止 breakaway，完整 UI/剪贴板限制。Go 仍有 64MiB 软内存预算，父进程继续每 10ms 监测 RSS，192MiB 或连续无法监控则终止整个 Job。
5. `STARTUPINFOEX` 必须同时包含无 capabilities 的 AppContainer、LPAC opt-out、禁止子进程、创建时 Job list、标准输入/输出/NUL 共三个句柄的白名单。环境使用显式白名单：Go 资源预算、系统目录/盘、UTC，以及通过 OS Known Folders 获取的 USERPROFILE、LOCALAPPDATA、APPDATA、TEMP/TMP 路径，供 Windows lowbox/profile 初始化展开；另显式传入仅用于校验的 `MIAOYIN_SANDBOX_LOOPBACK_PORT`，其值由本次父进程监听器产生，不从用户环境继承。不复制父进程环境，不继承 HOME、PATH、USERNAME、凭据或开发调试变量；传入路径不授予相应文件访问权限，原生私有文件/注册表拒绝测试仍须通过。工作目录为系统目录。
6. `CREATE_SUSPENDED` 创建，父进程核对实际映射程序的 native image 名与固定文件一致、令牌确为预期 SID 的低完整性、未提权 LPAC、零 capabilities、且属于指定 Job，然后才恢复线程。任一 API、属性或复核失败，终止并回收，不尝试弱化配置重启。
7. worker 在读取第一字节脚本前再次核对令牌、Job 上限/UI 限制和禁止子进程策略。网络预检直接使用 AFD 的 IPv4 TCP endpoint/bind/connect，不再要求 Winsock 初始化或读取其 HKLM catalog；不增加 `registryRead` 或任何 capability。仅接受明确的内核 `STATUS_ACCESS_DENIED` / `STATUS_NETWORK_ACCESS_DENIED`，初始化失败、ABI 错误、超时、离线、连接被拒绝和连接成功均不算隔离通过。loopback 目标为父进程实际监听的随机端口。同步原生调用由父进程墙钟期限和 kill-on-close Job 约束；脚本在预检通过前不会读取。原有 Goja 栈、定时器、墙钟中断、有界 JSON/JSONL、主进程 HTTPS broker 和媒体地址验证保持不变。Job 是资源边界，不能拿它替代 AppContainer 文件/网络边界。

LPAC 验证读取原生 `NtQueryInformationToken(TokenSecurityAttributes)`（class 39）中的系统 UInt64 属性 `WIN://NOALLAPPPKG=1`，确认 `ALL_APPLICATION_PACKAGES` opt-out。该方式与 NtCoreLib 的 `LowPrivilegeAppContainer` 判断一致；不再依赖 SDK 虽有枚举、但已在 Windows build 26200 实测返回 `STATUS_INVALID_INFO_CLASS` 的 class 46。查询失败、属性缺失/禁用、类型/版本/数量异常、重复标记、越界指针或非布尔值均不放行。仍同时要求 AppContainer、预期包 SID、低完整性、无提权、零 capabilities 和全部 Job/文件/网络约束。

需要支持上述属性的现代 Windows 10/11；老版本或企业策略不支持时拒绝执行。不要求管理员权限或开发者模式。本项目在 Windows build 26200 的 LPAC/Job 组合下已实测：默认 console 子系统在进入 Go 前以 `0xc0000142` 退出，GUI 子系统可进入 Go；辅助程序构建固定 GUI，stage 发现 console EXE 时提前拒绝并给出编译提示，不改令牌或 Job 来迁就 console 初始化。

## 原生验收

先在 Windows 仓库根目录运行（不设置真实源测试环境变量）：

```powershell
go test -count=1 -v -ldflags="-H=windowsgui" ./internal/sourceruntime
```

默认测试二进制需保持与最小 worker 一样不加载应用层初始化依赖。真实缓存/下载/原生音频测试已移到 `source_live` 显式编译标签下，避免即使运行时 skip 仍静态链接 SQLite/C 运行库；增加依赖回归检查。运行 Windows 原生探针时不带 `source_live`。

新增 Windows 专用测试不会因缺少隔离支持而跳过：

- 直接原生读取/写入用户私有测试文件被拒，容器不能修改 staging 文件夹或其 ACL；只使用合成假密钥，不访问真实用户密钥。
- 用户 HKCU 合成私有键不可读取/篡改；不把容器私有/虚拟化注册表等同于零注册表访问。
- 每次原生边界测试先在普通可信、无脚本子进程中运行同一 AFD 探针；必须连接父进程真实监听器，且父进程实际 accept，不能仅凭 API 返回值宣称成功。该对照子进程有独立 5 秒期限。对照失败则不继续宣称隔离通过。
- 受限进程对 TCP loopback 及固定公网 `1.1.1.1:443` 的相同探针必须得到明确的内核访问拒绝；其他结果均失败。公开连接只发起 TCP 握手，不发 HTTP、媒体请求或身份数据。Go `net`/Winsock 的初始化缓存不参与这个判断。Winsock 对照和最小环境/系统资源数字诊断仅保留在测试二进制，不再是生产启动依赖。
- AFD 为 Windows 私有 ABI；结构大小、EA 内容、指针宽度和 sockaddr 偏移有测试/运行时断言。普通进程正向对照和受限进程拒绝对照必须在支持的 Windows 实机上共同通过；不能凭交叉编译宣布可用。接口改变或返回未知错误时拒绝，不改能力/ACL 兜底。
- 子进程启动用原生 CreateProcess 尝试启动已知可读/执行的同一固定 helper，而不是依赖 Go os/exec 或系统 cmd 的加载权限；仅接受明确的 child-policy/访问拒绝/Job quota 错误，任何创建成功均失败并立即终止。读取/复制父进程句柄被拒；额外可继承事件句柄未泄漏：父进程在发送测试请求前只读查询低箱进程的 `ProcessHandleInformation` 快照（不要求低箱额外具备句柄枚举权限），搜索对应值与继承属性，不对本应无效的候选值调用 SetEvent/CloseHandle/GetHandleInformation；保留严格句柄检查，避免测试自己触发不可恢复的无效句柄异常。普通进程正向对照验证快照确能发现可继承事件、区分清除继承标志后的事件，并安全搜索任意无效值；stdin 管道不能 impersonate 成可授权访问的普通 broker 令牌。
- 超过 Job 提交内存的原生分配被拒、无限持有的内存任务及原生 CPU 死循环被回收；拒绝无容器/Job 的直接 worker 调用。
- 真实杀死独立 broker 后，worker 随 Job 自动退出，不依赖 Go defer 或前端退出事件。
- 离线检查、父进程伪造网络 RPC、取消、异常脱敏及输入输出上限在 Windows 真进程中运行，而不再跳过正向协议测试。

若启动失败，原生测试只输出固定阶段、`win32=数字`、Windows 内核 build/架构及父进程是否在 Job 中；令牌失败时还输出 elevation、完整性 RID、AppContainer/LPAC 标志、capability 数量与预期 SID 是否匹配。只输出数字/布尔状态，不输出 SID 正文、源脚本、路径或密钥。进程无正常结果时还保留 `before_entry` / `probe_entry` / `sandbox_ready` / `request_read` 及文件、网络、子进程、句柄、暂存目录、注册表、内存、impersonation 等测试体固定阶段、父侧只读句柄审计结果、分类后的管道读写错误和十六进制退出码；不输出原始 stderr；测试体可恢复 panic 只返回固定失败阶段，不输出 panic 值、堆栈或异常文本。网络预检失败保留目标类别（loopback/public）、固定阶段（open/bind/connect/setup）、是否连接成功和数字 NTSTATUS，不把超时/连接被拒绝当作访问拒绝。请保留这些诊断，不以移除 LPAC/Job 来排查。

通过后运行完整构建：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\tools\build_windows.ps1 -Tests
```

脚本额外编译最小辅助 EXE，并使用**该实际辅助 EXE**做离线声明和伪造网络 broker 协议冒烟，不访问真实服务、不读取用户源。失败即不生成便携包。两个 EXE 都做 PE x64/依赖检查并写入包内哈希清单。

之后才在实际应用中导入个人源，检查/激活，再验收真实播放、缓存和下载；任何一步尚未执行都不标记通过。

## 限制与恢复

AppContainer/LPAC 不等于完全没有系统资源访问；系统 DLL、必要的系统只读资源及容器范围的注册表行为仍可能存在。Goja 不提供文件、进程或直连网络 API，OS 隔离是额外边界。系统管理员、被修改的 OS 安全策略、恶意内核/驱动及操作系统崩溃转储不属于应用可独立保证的威胁边界。GOTRACEBACK=none、stderr=NUL、固定错误协议不代表能阻止管理员启用系统级内存取证。

复制仅含公开 EXE。父进程崩溃可能留下 staging 目录，最多接受 16 个该前缀目录/至多 64 个 profile 根条目，超限拒绝，不擅自删除另一个运行实例的文件。开发阶段如遇 `staging directory limit`，先完全退出应用与测试进程，再检查这个专用 profile 下遗留的 `miaoyin-worker-*`；不要删除应用数据库、DPAPI 文件或音源副本。更完善的残留回收仍需后续实现与测试。

待 Windows 实机确认：profile 布局/ACL、LPAC 加载 Go 程序、令牌/Job 自检、企业网络策略、非管理员账户、并发 worker、抗病毒软件影响、实际源兼容性及音频/下载全链路。不得通过删除 `sandboxWorker` 检查、授予网络 capabilities、增加 loopback 豁免、放宽文件 ACL 或禁用 Job 来通过测试。

## API 依据

- AFD transport-independent ABI 布局参考 [phnt `ntafd.h`](https://github.com/winsiderss/systeminformer/blob/master/phnt/include/ntafd.h) 的 `AFD_OPEN_PACKET`、`AFD_BIND_INFO_TL` 和 `AFD_CONNECT_JOIN_INFO_TL`；使用原生 `NtCreateFile` / `NtDeviceIoControlFile`，不是复制第三方 socket 实现。

- [CreateAppContainerProfile](https://learn.microsoft.com/windows/win32/api/userenv/nf-userenv-createappcontainerprofile)、[GetAppContainerFolderPath](https://learn.microsoft.com/windows/win32/api/userenv/nf-userenv-getappcontainerfolderpath)
- [UpdateProcThreadAttribute](https://learn.microsoft.com/windows/win32/api/processthreadsapi/nf-processthreadsapi-updateprocthreadattribute)，LPAC opt-out/child/job 属性数值对应 Microsoft WinSDK `WinBase.h`。
- [NtQueryInformationToken](https://learn.microsoft.com/windows-hardware/drivers/ddi/ntifs/nf-ntifs-ntqueryinformationtoken)，原生属性结构核对 [phnt `ntseapi.h`](https://github.com/winsiderss/systeminformer/blob/master/phnt/include/ntseapi.h)；有效 LPAC 属性判断参考 [NtCoreLib `NtToken.LowPrivilegeAppContainer`](https://github.com/googleprojectzero/sandbox-attacksurface-analysis-tools/blob/main/NtCoreLib/NtToken.cs)。
- [Job Objects](https://learn.microsoft.com/windows/win32/procthread/job-objects)、[NetworkIsolationGetAppContainerConfig](https://learn.microsoft.com/windows/win32/api/networkisolation/nf-networkisolation-networkisolationgetappcontainerconfig)
