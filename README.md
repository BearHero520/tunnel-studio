# Tunnel Studio

KANO 的 WireGuard / AmneziaWG 双协议插件，**v0.1.5 联调预览版**。独立实现的管理页、Go 后端、网络事务与测试；旧 WireGuard 项目仅作为宿主行为参考，不修改旧仓库。

- [下载单文件插件](https://github.com/BearHero520/tunnel-studio/releases/download/v0.1.5/tunnel-studio.js)
- [完整源码包](https://github.com/BearHero520/tunnel-studio/releases/download/v0.1.5/tunnel-studio-0.1.5.zip)
- [Raw JS](https://raw.githubusercontent.com/BearHero520/tunnel-studio/v0.1.5/dist/tunnel-studio.js)
- [发行说明与校验文件](https://github.com/BearHero520/tunnel-studio/releases/tag/v0.1.5)

## 从 v0.1.3 / v0.1.4 更新

1. 在旧插件刷新状态，停止 Tunnel Studio 隧道。
2. 在 KANO 插件管理删除旧 Tunnel Studio 网页插件项，导入新版，保存并刷新。
3. 确认标题 v0.1.5，点击“安装 / 更新”。设备配置会保留。
4. 页面会自动载入设备已保存配置，核对代理范围；按需选择“本机 + 热点”。U30 Air 热点接口为 br0；其他设备需核对。
5. 检查配置的 AllowedIPs 和 DNS。默认阻止所选范围的外网 IPv6，避免在 IPv4 代理期间绕过隧道；需要 IPv6 时选择合适模式。
6. 保存并确认流量接管设置，再启动。核对右侧**运行范围**、握手与实际网页访问；左侧未保存的选择不是运行状态。

旧配置的空来源列表仍解释为“仅隧道”，不会在升级时未经选择就改变网络。必须读取配置、选择范围并保存。不要同时保留多个版本的网页插件。

## 流量模式

| 模式 | 行为 |
|---|---|
| 本机 + 热点 | 接管本机 OUTPUT 与指定热点接口进入的流量 |
| 仅本机 | 不改热点转发 |
| 仅热点 | 接管指定接口的转发流量，不接管本机普通流量 |
| 仅隧道 | 只建立 kts0，不代理上网 |
| 自定义来源路由 | 兼容早期显式 CIDR 模式，不自动添加 NAT/DNS/放行规则 |

自动代理只匹配 AllowedIPs；非默认 AllowedIPs 表示分流，不是全局代理。自动模式设置 IPv4 MASQUERADE、所需转发与已配置 DNS 的 TCP/UDP 53 重定向。DNS 地址须被同族 AllowedIPs 覆盖；未设置可用 DNS 时不重定向，不偷偷使用公共 DNS。按隧道 MTU 限制新建 TCP 连接的 MSS，减少路由重定向后的大包问题。已建立连接可能需要应用重连。

IPv6 的“阻止外网”保留局域网、链路本地和隧道传输；“NAT66”需要内核 IPv6 NAT；“热点路由”只用于热点模式，要求服务器 Peer AllowedIPs、回程路由及出口支持 LAN IPv6 前缀，不支持自动接管本机 IPv6。没有 IPv6 Address/AllowedIPs 时不能启用 IPv6 代理。

## 网络资源与恢复

- 独立接口 kts0；目录 /data/adb/kano-tunnel-studio，私有权限。
- 专属路由表 18460；自动模式 priority 9000，传统来源模式 18460。占用时拒绝启动。
- 自动代理标记 0x40000000/0x40000000；隧道 UDP 传输使用 0x20000000，保留 Android 低位网络选择标记。
- KTS_ROUTE、KTS_DNS、KTS_SNAT、KTS_FWD、KTS_BLOCK、KTS_MSS 均检查冲突后创建。只按本次事务记录撤销单条规则，不全局 flush。
- 先建路由和内部链，再接入流量；回滚按逆序先移除入口。系统转发设置记录旧值，停止时仅在值仍等于本插件设置时恢复。启用 IPv6 转发前保留 RA。
- 同次启动若留下未完成事务，拒绝自动接管。先核对 runtime/routes.json、接口归属和只读诊断，不要删除其他服务规则。超时或外部修改导致清理不能确定时保留日志。
- 回滚到不支持自动代理的旧后端会被拒绝，避免静默退回仅隧道。

使用现有 KANO 特权通道；不需要另外获取 ADB root。完全无特权设备的 Android VPN 服务未在此插件实现。不读取现有 wg0 配置或接管其他接口。

## 后端与验证

固定官方 amneziawg-go v3.1.20260828，Android ARM64 离线内嵌资源；标准模式通过独立官方 WireGuard Go 实现验证。保留第三方 MIT 许可。固定源码与两个补丁见 docs/SOURCES.md、resources/manifest.json。

Go 配置/网络计划/事务测试、浏览器导入/安装模拟/双栏及明暗布局测试通过。Android x86_64 模拟器执行过真实 ip/iptables/ip6tables 规则与清理测试，入口使用未挂接测试链，不影响真实流量；不是完整公网测试。

用户提供的 v0.1.3 U30 Air 诊断证实安装、kts0、执行身份及一次握手，但当时来源为空，未接管流量。**用户在 v0.1.4 上反馈部分网站无法访问，以及运行一段时间后断网。v0.1.5 只改进配置加载和诊断，尚未修复或定位这些联网故障；吞吐、自启及长时间稳定性仍未验证。** 不把握手、计数或模拟测试等同于代理可用。

## 构建与测试

需要 Python、Node 和 Go 1.25 工具链。固定源码由 tools/prepare.py 验证后准备；构建不包含用户配置。

```sh
python tools/build.py
npm ci
npx playwright install chromium
npm test
npm run test:ui
npm run test:layout
npm run test:import
go test ./... -count=1
go vet ./...
python tools/pack.py
```

HTML 导入回归需要 Git 历史中的 v0.1.0 标签（git fetch origin tag v0.1.0）。源码 zip 不包含 .git。Android 冒烟测试只在专用测试环境显式设置 KTS_POLICY_SMOKE=1 时执行，不能在实际用户设备随意启用。

历史 v0.1.0 导入恢复见 docs/RELEASE-v0.1.1.md；v0.1.1 容器裁剪修复见 docs/RELEASE-v0.1.2.md。
