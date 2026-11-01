# Windows 原生依赖（开发验收包）

本文件不是完整发行合规声明。当前包只用于源码持有者的本地开发和验收，不能当作已准备完毕的公开发行包。

## libmpv

- 上游构建：shinchiro/mpv-winbuild-cmake，release `20261021`，`mpv-dev-x86_64-20261021-git-e471496c56.7z`。
- mpv commit：`e471496c56`，client API `2.6`；采用非 x86-64-v3 构建，不额外要求 v3 CPU。
- 构建工作流记录：https://github.com/shinchiro/mpv-winbuild-cmake/actions/runs/39518569492
- 构建配方：https://github.com/shinchiro/mpv-winbuild-cmake
- 源码：https://github.com/mpv-player/mpv/tree/e471496c56
- Archive SHA-256：`efc719eff3dc5954b4a5efc800632808bcec01cc9034940dd14e5eb7fe9ea633`
- libmpv-2.dll SHA-256：`04e4d27dc385dc18ddd93fbe69c75e635b864f3c6ec53a9122df54c2f7842f4a`

mpv 上游 Copyright 说明默认 GPLv2-or-later，链接的 FFmpeg 等构建选项可能进一步影响最终许可；不能因为仅使用音频就推断依赖不受 GPL 或其他许可约束。仓库保留该 commit 的 Copyright 和 GPL 文本，随开发包复制。**上游 Windows 构建中所有静态链接组件的许可清单、准确配置及对应源码仍需在公开分发前完整归档；仅附许可证或上游链接不等于履行对应源码义务。**

## Vulkan Loader

libmpv 的静态导入表包含 `vulkan-1.dll`，它不是每台 Windows 都自带的系统库。因此使用官方 LunarG Runtime 组件 ZIP 中的 x64 loader，应用本地放置，不安装驱动、不修改 System32、不附带 SDK、调试符号或 Vulkan 工具。

- 版本：`1.4.363.0`。
- 官方组件包：`VulkanRT-X64-1.4.363.0-Components.zip`。
- Archive SHA-256：`a25a927aa8b9f0371048f1861cf88ac3b9bc9b1fb332c42d897c8ab32695769a`
- x64 vulkan-1.dll SHA-256：`e1fcfc9489beefa6a6d9d11d6c7517f6a2b7f16e0d3fb8f4103bee0210f314a3`
- 随包保留原始 `VulkanRT-License.txt`；以该文件列明的 Apache/MIT 及相关条款为准。
- 源码：https://github.com/KhronosGroup/Vulkan-Loader/tree/vulkan-sdk-1.4.363.0

## 构建期提取工具

仅构建缓存使用官方 7-Zip reduced console `7zr.exe` 26.04，SHA-256 `256feca8e274e5da655e2a284fabafd9f554365eb164862089dacd4e8276d282`。执行前必须校验。来源 https://www.7-zip.org/ ，许可说明 https://www.7-zip.org/license.txt 。不把该 EXE 或提取器安装到系统，也不包含在应用 ZIP 中。

## 安全与未完成项

两个原生 DLL 都固定字节校验、x64 类型和导入闭包；未知非系统 DLL 直接阻止打包。运行时 libmpv 仍采用绝对应用相邻路径与 `LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR | LOAD_LIBRARY_SEARCH_SYSTEM32`，不以 PATH 或工作目录搜索替代。系统 API-set 和 Windows 自带 DLL 不从非官方站点补拷贝。

此清单不涵盖全部 Go/前端依赖许可，也不是应用许可证。Windows 音源沙箱实现已接入开发构建，但其原生安全验收、公开发行材料、签名/安装包及完整 Windows 实机验收仍未完成。不得移除沙箱限制来绕过源脚本执行阻断。
