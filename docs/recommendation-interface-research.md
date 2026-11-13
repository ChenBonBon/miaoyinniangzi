# QQ 推荐候选接口调查

调查日期：2026-11-13。仅记录接口证据与实测，不构成开发计划或接入承诺；应用代码、静态「猜你喜欢」及用户数据均未改动。

## 条件与范围

- 当前 Mac / 当前网络下，直接请求 `https://u.y.qq.com/cgi-bin/musicu.fcg`，不经过第三方代理 API。
- 使用公开测试曲目标识，不读取本地收藏、数据库、个人音源、凭据或个人日志。
- 使用应用当前匿名协议：`comm={ct:24,cv:0,uin:0,format:"json"}`，JSON POST，现有元数据 User-Agent/Referer；无登录 cookie、密钥、签名或账号，不尝试切换成其他客户端平台规避限制。
- 临时探针设置 8 秒网络 socket 超时、响应读取上限 2 MiB，拒绝跳转，无自动重试；这不是应用 HTTP 客户端总预算的替代实现。不同协议/种子及字段核对为有限的人工调查请求。只在内存解析返回音乐数据，输出固定探针名、数字状态、计数和布尔值，不保留原响应、返回的歌曲名/音乐标识/封面或地址。
- 同协议榜单目录作为阳性对照，HTTP 200、顶层/请求业务码均 0，返回 4 个分组，排除“所有元数据请求均失败”的情况。

## 已确认结果

| 候选 | 模块与方法 | 当前匿名实测 |
| --- | --- | --- |
| 相似歌曲（较新） | `music.recommend.TrackRelationServer.GetSimilarSongs` | 两个公开种子分别返回 12、10 首主候选；均 HTTP 200，顶层/请求/内部业务码 0 |
| 雷达推荐 | `music.recommend.TrackRelationServer.GetRadarSong` | 空 `FavSongs`、以及单个公开测试种子的调用均可返回 10 个主曲目条目；尚不能证明其按照所给种子产生个性化结果 |
| 歌曲标签 | `music.recommend.TrackRelationServer.GetSongLabels` | 一个公开种子返回 10 个标签条目，顶层/请求业务码 0；这是辅助信息，不是歌曲候选列表 |
| 相似歌手 | `music.SimilarSingerSvr.GetSimilarSingerList` | 一个公开歌手返回 5 条歌手记录，顶层/请求/内部业务码 0；不是直接歌曲候选 |
| 旧版相似歌曲 | `rcmusic.similarSongRadioServer.get_simsongs` | 两个种子的预期 `songInfoList` 均为空，虽然业务码为 0，不能据此认定拿到了候选 |
| 电台 / 名为猜你喜欢的接口 | `mb_track_radio_svr.get_radio_track`、`music.radioProxy.MbTrackRadioSvr.get_radio_track` | 两者当前匿名调用的请求业务码均为 1000，没有验证为当前协议下的可用曲目来源；不把该错误码直接解释成确定的登录原因 |

另查到 `GetRelatedPlaylist` 相关歌单接口，实测外层成功且有继续获取标志，但没有完成歌单摘要/歌曲内容链路核对，不列为“已确认可用的歌曲来源”。本次未确认另一个独立、可靠的匿名单曲电台接口；不能把普通分类电台测试当作单曲电台验证。公开旧实现的「今日私享」路径明确涉及登录态；本次没有请求个人日推或获取任何账号凭据。

### 相似歌曲：响应兼容性是关键发现

接收的是数字 **songid**，不是字符串 SongMID；返回曲目包含可用的 MID 和数字 ID。

本次响应的 `vecSongNew` 为 `null`，实际主候选位于 **`vecSong[*].track`**。仅按公开 Python 库的 `vecSongNew` 映射读取，会把有结果误判为空。

按主曲目精确计数，而不是递归搜索全部 `mid/name`（嵌套歌手、专辑也有这些字段）：

| 属性 | 种子 A | 种子 B |
| --- | ---: | ---: |
| 主候选数 / 唯一曲目 MID 数 | 12 / 12 | 10 / 10 |
| 含歌曲名、数字曲目 ID | 12 | 10 |
| 含歌手数组、专辑 MID、正数时长 | 12 | 10 |
| 含普通 FLAC / 320K / 128K 至少一种非零文件大小字段 | 12 | 10 |
| 候选中包含种子本身 | 否 | 否 |

两组主候选 MID 的交集为 0，共 22 个不同候选，说明这两个样本的结果不是同一份固定名单。它不等于已经验证推荐质量，也不证明所有种子有结果、每次返回固定数量或长期稳定。普通音质大小字段仅是元数据，未解析媒体地址，不能据此声称歌曲一定可播放。

雷达结果的主曲目路径为 `VecSongs[*].Track`；没有验证 `FavSongs` 的因果作用或本地收藏适配，不能因为接口名字含“推荐”就视为已经按用户收藏个性化。

## 公开证据

- [L-1124/QQMusicApi：歌曲接口](https://github.com/L-1124/QQMusicApi/blob/c046788dd1f11daacc74da487cbbf1bcd8ac6f33/qqmusic_api/modules/song.py)：相似歌曲、标签、相关歌单模块/方法及参数。
- [同版本响应模型](https://github.com/L-1124/QQMusicApi/blob/c046788dd1f11daacc74da487cbbf1bcd8ac6f33/qqmusic_api/models/song.py)：公开模型的 `vecSongNew` 路径，与本次实际 `vecSong` 响应存在差异。
- [推荐接口](https://github.com/L-1124/QQMusicApi/blob/c046788dd1f11daacc74da487cbbf1bcd8ac6f33/qqmusic_api/modules/recommend.py)：雷达与猜你喜欢方法；其文档对猜你喜欢注明客户端平台/凭据差异，本次没有切换平台继续试探。
- [相似歌手](https://github.com/L-1124/QQMusicApi/blob/c046788dd1f11daacc74da487cbbf1bcd8ac6f33/qqmusic_api/modules/singer.py)。
- [jsososo/QQMusicApi 旧版歌曲实现](https://github.com/jsososo/QQMusicApi/blob/13b08afd3180cc74d76fff208956b77a560abd22/routes/song.js)及[电台实现](https://github.com/jsososo/QQMusicApi/blob/13b08afd3180cc74d76fff208956b77a560abd22/routes/radio.js)：用来交叉核对旧方法，不把旧开源代码的存在当成当前可用性证明。

这些是非官方匿名接口的单机点状实测，不是平台授权、官方兼容承诺、推荐效果评估或 macOS/Windows 完整应用接入验收。本轮结论：**较新的种子相似歌曲接口已经确认可以匿名获得真实候选；其他接口的边界按上表分别记录，未进行应用接入或其他开发安排。**
