# 妙音娘子

## Register

product

## Users

在 macOS 本地听歌的用户，希望快速发现、搜索、播放和下载 QQ 音乐曲目。

## Product Purpose

Wails + Svelte + TypeScript 桌面播放器，界面使用 shadcn-svelte + Tailwind CSS。推荐采用 QQ 音乐榜单与新歌；搜索、播放、下载只使用 QQ 音乐。洛雪兼容脚本用于解析地址，需支持导入、启用和禁用。不提供注册、登录、歌单同步。

## Brand Personality

清爽、专注、灵动。借鉴 Spotify 的音乐优先布局和交互反馈，不复制品牌。

## Anti-references

不做社交信息流、广告入口、多平台聚合或复杂管理后台。

## Design Principles

- 音乐优先，配置退后。
- 应用内界面及确认、错误、加载等交互统一使用 shadcn-svelte，保持样式与动效一致；文件/目录选择、Keychain 授权等操作系统界面保留原生。
- 数据获取与音源解析分离，仅允许 tx 来源。
- 动画服务于导航、播放状态和操作反馈。
- 未接通功能明确展示状态，不伪造曲目、播放或下载成功。
- 本地脚本和密钥不进入仓库。

## Accessibility & Inclusion

默认支持键盘焦点、明确的控件名称、清晰文字对比度和减少动画偏好。
