# 音源网络安全拒绝诊断

2026-11-13：Clash Verge 开启系统代理/TUN 时，新旧应用均被网络安全策略拒绝；关闭代理可播放。DNS 覆写界面的截图不是实际生效配置，不能单凭截图断定原因。此轮仅加诊断，不修改用户 Clash、网络策略、DNS 回退或音源。

父进程为音源解析、播放/缓存和下载的网络客户端配置只读观察器。复用既有私有有界 `qq-metadata.jsonl`（64 KiB × 2），操作名为 `source_network`；这是观察事件，不是整次播放完成证据。

仅新增三个白名单分类字段：

- `stage`：request / lookup / fallback / address_check / media_url。
- `answers`：none / invalid / zoned / fake_v4 / fake_v6 / non_public / public_v4 / public_v6 / mixed。
- `reason`：success / begin / lookup_failed / local_inspection_failed / local_interface / non_public_or_zone / invalid_url / request_shape / request_build / headers / redirect / scheme / credentials / fragment / port / host。

磁盘入口拒绝任意非白名单字符串，不记录域名、IP、URL、参数、请求/响应内容、头、密钥、源脚本、歌曲、私有路径或原始错误。不增加诊断网络请求。IPv6 合成范围只用于分类，不增加信任、不新增回退、不允许连接合成地址/私网。现有策略和拒绝结果保持不变。

测试覆盖 IPv4/IPv6/合成/混合分类、诊断字符串不含原始输入、未因 IPv6 分类启用回退、私网仍拒绝及任意字符串不能写盘。Go 全量 race/vet 和 Windows amd64 CGO=0 交叉编译/vet 通过；不等同于真实 Windows 执行或代理问题已解决。

待用户保持 Clash 设置不变，在独立诊断完整包中显式播放一首复现后，检查上述固定分类再决定兼容修复。无需上传完整日志或任何订阅、节点、音源脚本。此次尚未据此断言根因或修复播放。
