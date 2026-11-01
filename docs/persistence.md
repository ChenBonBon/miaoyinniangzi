# 本地持久化

## 已实现

- `internal/storage`：SQLite（modernc.org/sqlite，纯 Go 驱动），单连接、5 秒锁等待、参数化查询。
- 事务式 schema 迁移，通过 `PRAGMA user_version` 管理版本；拒绝打开未来版本，不自动重置损坏数据。
- 设置：下载目录、下载音质、播放音质、音量。
- 收藏：仅允许 QQ 音乐曲目，以 ID 去重，按加入顺序倒序返回；重复收藏不更改原顺序。
- 播放快照：完整队列、当前下标、秒数进度、播放模式。没有自动播放标记。
- Wails `App` 提供读写方法；数据库在启动时打开，Wails 返回后关闭。初始化失败时不继续启动。

macOS 数据库位置：

```text
~/Library/Application Support/妙音娘子/miaoyin.sqlite3
```

新建数据目录权限为 `0700`，数据库权限为 `0600`。默认下载目录为 `~/Downloads`，不会在保存设置时自动创建下载目录。

## 数据约定

- `Song.id` 为 QQ 曲目稳定 ID，`source` 必须为 `tx`。
- 音质值：`highest`、`flac`、`320k`、`128k`。只是设置值，并不声明某首歌或当前音源一定支持它；音源接入后再做能力交集。
- 音量为 0–1，默认 1（100%）；已有用户保存值保持不变。
- 播放模式为 `sequential`、`shuffle`、`repeat-one`。
- 无选中曲目时下标为 -1，进度为 0；空列表返回数组而非 null。
- 播放快照整体原子写入，允许队列重复歌曲，最多 10000 项。
- 收藏删除仅更新数据库，不操作音频文件。

## Wails 方法

`GetSettings` / `SaveSettings`、`ChooseDownloadDirectory`、`GetPlayback` / `SavePlayback`、`GetFavorites` / `AddFavorite` / `RemoveFavorite`。

目录选择器使用 Wails 原生对话框，接收表单当前目录作为初始位置，不依赖自动保存是否已经完成。目录无效时回退至已保存的有效目录。取消返回空字符串，不改变草稿或已保存设置。

绑定在 `wails dev` 或 `wails build` 时生成到 `frontend/wailsjs`。设置界面通过 `frontend/src/lib/settings.ts` 调用 Wails 桥接；不依赖生成文件参与前端类型检查，因此纯浏览器预览仍可构建，但明确提示不能本地保存。

设置页启动时读取数据库。修改停止 1000ms 后自动保存，连续修改重新计时，首次读取不触发保存。保存成功后使用 shadcn-svelte Sonner 在右上角显示绿色 toast，失败显示红色 toast；取消目录选择或回到原值不触发多余保存。

自动保存控制器串行写入，保存期间仍可修改，后续修改独立防抖，避免旧请求覆盖新状态。失败时保留草稿，显示错误和重试入口，不无限自动重试。切换页面保留草稿和计时器；关闭应用前尚未触发或完成的保存目前不保证落盘，应等待保存成功提示。

目前音源尚未接入，音质列表只代表可保存偏好，接入后需按音源能力过滤。底部播放器音量和实际播放恢复尚未接入；收藏也尚无界面。

schema v2 已新增音源元信息与 AES-GCM 密文副本，v3 增加检查状态、普通音质和唯一启用项，详见 [音源导入](sources.md) 和 [运行器](source-runtime.md)。数据库不保存音源密钥或脚本明文。schema v4 添加本地搜索历史：有效关键词提交时记录，最近优先、去重，最多 20 条，支持逐条删除及清空；编辑与返回列表处于同一事务，开启 secure_delete。迁移保留现有设置、收藏与音源副本和启用状态。下载任务和本地文件映射将在对应功能开发时增量迁移。

## 验证

```sh
go test -race ./...
go vet ./...
CGO_CFLAGS='-mmacosx-version-min=13.0' CGO_LDFLAGS='-mmacosx-version-min=13.0' wails build -platform darwin/arm64
cd frontend
node --experimental-strip-types --test tests/*.test.mjs
npm run check
npm run build
```

桥接测试覆盖浏览器环境拒绝保存、桌面读写、传递当前目录、取消目录选择与后端错误传播。自动保存测试使用虚拟计时器覆盖 1000ms 边界、连续修改、还原原值、串行写入、写入中继续修改、失败重试及销毁取消。Go 测试覆盖目录选择的当前值优先及无效目录回退。原生目录对话框、toast 位置和实际窗口交互仍需在 macOS 人工验收。

测试使用临时目录，不接触真实用户数据。覆盖默认值、关闭再打开后的恢复、收藏去重与顺序、非法状态拒绝、并发收藏、取消请求、数据库权限、未来版本拒绝、损坏数据报错和迁移失败回滚。
