# 音源网络安全拒绝诊断

2026-11-13：Clash Verge 开启系统代理/TUN 时，新旧应用均被网络安全策略拒绝；关闭代理可播放。DNS 覆写界面的截图不是实际生效配置，不能单凭截图断定原因。此轮仅加诊断，不修改用户 Clash、网络策略、DNS 回退或音源。

父进程为音源解析、播放/缓存和下载的网络客户端配置只读观察器。复用既有私有有界 `qq-metadata.jsonl`（64 KiB × 2），操作名为 `source_network`；这是观察事件，不是整次播放完成证据。

仅新增三个白名单分类字段：

- `stage`：request / lookup / fallback / address_check / media_url。
- `answers`：none / invalid / zoned / fake_v4 / fake_v6 / non_public / public_v4 / public_v6 / mixed。
- `reason`：success / begin / answer_group / lookup_failed / local_inspection_failed / local_interface / non_public_or_zone / invalid_url / request_shape / request_build / headers / redirect / scheme / credentials / fragment / port / host。

磁盘入口拒绝任意非白名单字符串，不记录域名、IP、URL、参数、请求/响应内容、头、密钥、源脚本、歌曲、私有路径或原始错误。不增加诊断网络请求。IPv6 合成范围只用于分类，不增加信任、不新增回退、不允许连接合成地址/私网。现有策略和拒绝结果保持不变。

测试覆盖 IPv4/IPv6/合成/混合分类、诊断字符串不含原始输入、未因 IPv6 分类启用回退、私网仍拒绝及任意字符串不能写盘。Go 全量 race/vet 和 Windows amd64 CGO=0 交叉编译/vet 通过；不等同于真实 Windows 执行或代理问题已解决。

诊断版 `922a522` 已由用户保持 Clash 配置复现。仅提取白名单字段观察到 lookup=mixed，随后 address_check=non_public_or_zone，尚无歌曲/域名/IP信息。无需上传完整日志或任何订阅、节点、音源脚本。

## 混合 DNS 回退兼容修复

同环境公开元数据域名对照观察到已知 IPv4 Fake-IP 与非公网 IPv6 混合；旧逻辑只有整个答案集全为 IPv4 Fake-IP 才会回退，因此混合结果落到正常公网检查并被拒绝。

修复将「存在已知 IPv4 Fake-IP 标记」视为重新查询的条件，而不是原地址可连接的证据：整组原始答案丢弃，使用原有固定 HTTPS DNS 权威重新查询，再对返回的**所有**地址执行既有公网/本机接口检查，连接仍固定到通过检查的 IP。未知私网 IPv6 不变成受信任地址；没有已知 IPv4 合成标记的普通公网/私网混合结果仍拒绝，不回退。无效/带 zone 的原结果不触发回退，可信查询失败也不连接原地址。未改 Clash、HTTPS/端口/头/重定向规则或 `PublicIP`。

混合 lookup 额外按单个答案输出分类（reason=answer_group），仍无原始地址或关联标识。测试验证原合成/私网/看似公网答案都不拨号、仅拨号独立验证的回退结果，回退混入私网/合成地址整组拒绝，以及本机接口仍拒绝。

显式 `MIAOYIN_LIVE_SOURCE_DNS=1` 同环境公共 DNS 测试已通过：mixed → fake_v4 + non_public → fallback public_v4 → 公网校验成功，地址数 1。只记录分类/计数，无用户源或媒体请求。Go 全量 race/vet 通过；真实个人源播放恢复需用户用新完整包确认，不能以 DNS 测试替代。
