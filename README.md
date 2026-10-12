# Tunnel Studio — KANO 双协议插件

从零实现的 WireGuard / AmneziaWG 管理控制台，版本 **0.1.2（联调预览版）**。管理页、设备管理程序、资源所有权、构建和测试均为独立新实现。没有复制旧 WireGuard 插件或上一轮草稿的 UI、管理脚本或测试。本项目独立发布，不修改旧 WireGuard 仓库。

## 下载与使用

- [下载 v0.1.2 单文件插件](https://github.com/BearHero520/tunnel-studio/releases/download/v0.1.2/tunnel-studio.js)
- [下载完整源码与构建包](https://github.com/BearHero520/tunnel-studio/releases/download/v0.1.2/tunnel-studio-0.1.2.zip)
- [浏览器直接读取 JS](https://raw.githubusercontent.com/BearHero520/tunnel-studio/v0.1.2/dist/tunnel-studio.js)
- [发行说明与 SHA-256 校验文件](https://github.com/BearHero520/tunnel-studio/releases/tag/v0.1.2)

- **导入 KANO 的单文件：`dist/tunnel-studio.js`**，约 2.3 MB，内置 gzip 压缩的 Android ARM64 可执行程序。安装时不从互联网下载后端。
- `docs/desktop.png` / `docs/phone.png` 是本实现的界面截图，数据来自明确标记的 UI 测试宿主，不是真机代理状态。
- 源码、固定来源、补丁、测试与许可均在本目录。`resources/manifest.json` 和 `dist/SHA256SUMS` 记录实际摘要。

1. 在已登录的 KANO 管理页导入单文件 JS。浏览器需支持 Shadow DOM 和 `DecompressionStream('gzip')`，建议新版 Chrome / Edge。
2. 打开“只读诊断”，点击“检查 KANO 执行通道”。需要该通道返回 `uid=0(root)`。**普通 ADB 无 root 不影响 KANO 自己已获授权的 root 通道。** 已有 `wg0` 可以继续运行，本插件不会读取它的密钥或接管它。
3. 在“版本与恢复”安装内置资源；安装后不会启动连接。
4. 在“协议与配置”明确选择 WireGuard 或 AmneziaWG，导入自己的配置，校验并保存。示例仅含占位密钥，不能直接连接。
5. 来源网段留空时只创建独立隧道。若要代理 LAN 流量，按需填写来源 CIDR，确认系统转发与服务端回程路由后启动。**不能仅凭“隧道已启动”认定代理已经连通**；需核对握手、流量与实际访问。
6. 可启用独立 Magisk `service.d` 自启；不修改系统其他自启脚本。

本版尚未取得可用于 U30 Air 管理操作的 KANO 已认证会话，因此**实际安装、系统 TUN 启停、来源路由代理、自启及 BE88U 握手尚未在真机完成联调**。可以交付和审计，但不称为已经替换现有代理的正式版本。详细证据见 `docs/TESTED.md`。

## 双协议如何实现

显式配置类型为 `wireguard` 或 `amneziawg`，每次运行一个本插件隧道。底层采用固定官方 AmneziaWG Go 实现的独立用户态程序。WireGuard 模式不允许混淆、头部保护等 AWG 扩展，使用标准兼容配置；已经与**另一份官方 WireGuard Go 实现**完成真实加密互通，而非拿两个模拟协议实现互相验证。

这不等于普通 WireGuard 内核接口支持 AmneziaWG，也不等于原版 `wg` 工具能管理 AWG。插件不采用未知来源的 `wg`、APK 或 JNI `.so`，也不要求刷内核。用户态后端的性能和功耗不能直接等同于现有 WireGuard 内核接口，吞吐仍需真机测试。

用户已确认华硕 BE88U 官改固件的 AmneziaWG 服务端已适配且与官方行为一致，本版以此为对端前提；连接配置与真实握手仍待联调。

## 配置与流量范围

支持 `[Interface]` / `[Peer]` INI；配置及本地备份 JSON 另含 `protocol`、`sources`。不执行 `PreUp`、`PostUp` 等脚本，也不接受 `Table`、`FwMark` 等未实现配置字段。未知字段、重复字段、无效密钥和版本不适配参数直接报错，不会悄悄忽略。没有私造的官方协议版本字段。

AmneziaWG 支持 Jc / Jmin / Jmax、S1–S4、H1–H4 单值或区间、I1–I5、HeaderProtectionKey、ContentPaddingAddition、五项时序区间、PersistentKeepalive 区间、RandomTrailers 和 DisableCookies。`AdvancedSecurity` 不属于此固定 Go peer UAPI，明确拒绝。完整边界见 `docs/CONFIG.md`。

- 默认 MTU 1280，可设 1280–1420。
- DNS 字段只接受 IP 并保留参考，**不会设置 Android 全局 DNS或添加 DNS DNAT**。浏览器、系统的 DNS 和 DNS 泄漏需要在部署环境另行验证。
- 域名 Endpoint 在启动时用设备解析器解析一次。纯 Go 在 Android 上可能无法取得正常系统 DNS；失败时停止启动，不使用硬编码公共 DNS。可使用经过核实的 IP Endpoint；本版不自动跟随域名地址变更。
- 来源网段留空：不添加策略路由。配置的 AllowedIPs 仍控制隧道内部对端选择。
- 填写来源网段：使用专属表与优先级 18460，添加 AllowedIPs 路由与来源规则；Endpoint 主机地址和来源 LAN 网段使用 throw 路由回到现有路由策略。不会刷新系统规则或默认路由。
- IPv6 使用路由方案，不要求 NAT66。服务端需有回程路由，设备需有合适转发和防火墙策略。本版不修改转发开关、iptables、nftables、DNS、MSS 或 Android netd 规则；因此不承诺自动覆盖 Android 本机所有应用或与其他 VPN 的所有组合。

## 所有权与变更边界

| 资源 | 本插件使用 |
|---|---|
| 宿主 DOM | `#kano-tunnel-studio` + Shadow DOM |
| 设备目录 | `/data/adb/kano-tunnel-studio`，0700 |
| 所有权 | `owner.json`，固定 namespace + 随机安装 ID |
| 配置、快照 | JSON，0600，独立备份目录 |
| 接口 | `kts0`；存在时拒绝接管 |
| 控制通道 | `runtime/control.sock`，0600，只支持脱敏状态与停止 |
| 资源版本 | 内容 SHA-256 标识的只读使用目录 + 原子版本指针 |
| 自启 | `/data/adb/service.d/kano-tunnel-studio.sh`，含安装所有权标记 |
| 路由 | 表/优先级 18460；冲突时拒绝启动 |

配置保存、恢复、升级和回滚仅在本插件隧道停止时允许。保存和恢复前自动生成旧配置快照。资源回滚校验旧二进制摘要，并使用旧程序重新校验当前配置后才切换指针。升级不自动重启连接。目录、文件及可执行程序拒绝非预期符号链接；设备执行前检查资源 SHA-256。

SHA-256 用于完整性与版本定位，**不是作者数字签名**；请从可信渠道取得 JS。本版不自动更新、不定期下载、不发送遥测。页面只保存布局偏好，不把配置放进 localStorage。

配置经 KANO 的受保护上传接口临时传递到设备，校验后删除临时上传文件，UI 也在 finally 中尝试清理。KANO 本身及其 root 通道属于信任边界，不能在不可信宿主中安全处理私钥。导出 JSON 包含密钥，会要求用户确认。

## 构建与测试

需要 Git、Python 3、Go 1.25.0、Node.js 及 npm。首次构建会拉取固定提交和 Go 依赖；最终插件运行、安装均不需要这些构建工具或联网下载。

```text
python tools/prepare.py
go mod download
go test ./... -count=1 -timeout=50s
go vet ./...
npm ci
npm test
python tools/build.py
npx playwright install chromium
npm run test:ui
python tools/check_regression.py
```

交叉编译由 build.py 固定 `GOOS=android GOARCH=arm64 CGO_ENABLED=0 GOTOOLCHAIN=go1.25.0`，启用 trimpath，去除构建 ID。源码补丁在 `resources/upstream.patch`，脚本会验证修改上下文与固定 commit。来源见 `docs/SOURCES.md`，日志与实际范围见 `docs/TESTED.md`。

## 中断恢复

正常停止按已记录的完整参数逐条撤销本插件路由，不 flush 全局表。SIGKILL 或异常断电可能留下路由日志；同一次内核启动中再次启动会拒绝自动清理，避免误删被其他程序复用的规则。内核重启后只清除本安装、上一次 boot ID 的旧日志，并仍检查路由冲突。

同次启动异常时，应先核对 `owner.json`、`runtime/routes.json`、`ip -4/-6 rule show`、`ip -4/-6 route show table 18460` 和接口 alias。确认只属于本次插件事务后按日志中的**单条逆命令**恢复，再移除该日志。不要使用 `ip rule flush`、全表 flush 或删除 `wg0`。本版不提供未经核对的一键强制清理按钮。

未实现自动卸载按钮，避免误删含密钥的备份。需要退场时先关闭自启、停止本插件隧道、导出所需配置，并按所有权记录人工移除本插件文件。

## v0.1.0 导入故障恢复

v0.1.0 缺少 KANO 必需的 script 封装，请改用 v0.1.2。先在插件管理删除故障项并保存；无法操作时参阅 [恢复步骤](docs/RELEASE-v0.1.1.md)，仅移除哈希匹配的旧插件。不要直接叠加导入或清空全部插件。

## v0.1.1 界面不可见

界面曾误挂到宿主折叠容器，受固定高度裁剪。v0.1.2 将管理界面移到功能列表下方的独立区域，折叠功能列表也不会隐藏它。删除旧插件项后导入新版并保存、刷新即可，无需卸载设备后端。
