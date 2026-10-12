# 固定版本的配置契约

权威校验位于 `internal/config`；前端不会用正则“通过”替代设备端校验。固定实现为官方 amneziawg-go `b5928efb6ca19f0153958460c3d141f04abc5c2e`。以下“本插件限制”用于约束资源消耗，不是官方协议上限。

| 字段 | 官方实现/含义 | 本插件接受范围 |
|---|---|---|
| PrivateKey / PublicKey / PresharedKey | 32 字节密钥 | 规范 Base64 32 字节，拒绝全零；PSK 可省略 |
| Address | TUN 地址，由管理器应用 | IPv4/IPv6 CIDR，拒绝 mapped IPv6 与组播 |
| AllowedIPs | 对端路由前缀 | 规范网络 CIDR；不同 peer 不允许完全重复前缀 |
| ListenPort | uint16 | 0–65535；0 为自动选择 |
| Endpoint | UDP 对端 | 域名/IP + 1–65535 端口；IPv6 使用方括号 |
| Jc | 握手前 junk 包数，Go uint32 | 0–64，本插件限制 |
| Jmin / Jmax | junk 包长，Go uint32 | 0–1400，Jmin ≤ Jmax，本插件限制 |
| S1 / S2 / S3 / S4 | 握手请求/响应/cookie/传输 padding，UAPI uint16 | 0–1024，本插件限制 |
| H1 / H2 / H3 / H4 | 对应消息类型，uint32 闭区间 | 1–4294967295，四个有效区间不得相交；缺省值分别 1/2/3/4 |
| I1–I5 | 握手前依次发送签名包 | 连续 `<b 0x…>`、`<r n>`、`<rc n>`、`<rd n>`、`<t>`；长度 1–1400，本插件限制 |
| HeaderProtectionKey | AWG 3+ 头部保护密钥 | Base64 32 字节且非零；S1–S4 均 ≥12 |
| ContentPaddingAddition | AWG 3+ 内容 padding 闭区间 | 0–1024，本插件限制 |
| RekeyAfterTime | 秒区间 | 1–86400，本插件限制 |
| RekeyTimeout | 秒区间 | 1–86400，本插件限制 |
| RejectAfterTime | 秒区间 | 1–86400，本插件限制 |
| KeepaliveTimeout | 秒区间 | 1–86400，本插件限制 |
| MaxHandshakeAttempts | 次数区间 | 1–86400，本插件限制 |
| PersistentKeepalive | peer 保活秒数 | WireGuard 单值 0–65535；AWG 单值或闭区间 0–86400 |
| RandomTrailers / DisableCookies | Go UAPI 布尔开关 | `true` / `false` |

S1–S4、H1–H4、HeaderProtectionKey 等服务端相关参数需按服务端配置匹配。Junk 和签名包参数是客户端行为配置。ContentPaddingAddition、时序、随机 trailer 的合法值并不保证所有网络条件下表现合理；当前上限也不保证 UDP 永不发生分片。

未设置头部保护时，S 字段可以为 0。省略字段由官方后端使用其默认值；不把 `(off)` 传给当前 Go `UintRange.FromString`，因为固定源码实际不接受该字符串。WireGuard 模式使用新的设备实例，不会从上一次 AWG 实例继承混淆状态。

不接受 `AdvancedSecurity`：官方 tools 字段集与固定 Go peer UAPI 不是同一集合。也不接受脚本钩子、任意 shell、系统 DNS 命令、任意表号或 FwMark。

示例配置不是工作凭据，所有密钥为占位文本。测试密钥通过 Go `crypto/ecdh` + `crypto/rand` 每次生成，不读取用户配置。

## v0.1.4 自动代理字段

配置 JSON 新增 `networkMode`（gateway/device/lan/tunnel/sources）、`lanInterface`（默认 br0）、`ipv6Mode`（block/nat/routed）。缺失 networkMode 的历史配置保持旧行为：空 sources 为仅隧道，非空为来源策略路由。自动模式不得同时填写 sources。

自动模式需要 IPv4 Address 与 AllowedIPs；DNS 按同族 AllowedIPs 校验。block 模式忽略 IPv6 DNS 并阻止所选范围的外网 IPv6。nat 模式需要 IPv6 Address、AllowedIPs 和内核 NAT66；routed 仅用于 lan 模式，使用 IPv4 DNS 并要求服务端配置 LAN IPv6 回程路由。隧道 UDP socket 使用保留标记，避免被本机代理再次接管。
