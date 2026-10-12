# Tunnel Studio v0.1.0 · 联调预览版

全新 KANO 双协议管理插件，显式支持 WireGuard / AmneziaWG 配置类型。单文件 JS 内置 Android ARM64 用户态后端，安装时无需联网下载资源。

- 全新桌面/手机界面，配置校验、启停、开机自启、快照恢复、资源升级回滚与只读诊断。
- 独立接口、受保护目录、控制通道和资源所有权，不接管已有 `wg0`。
- 固定官方 AmneziaWG Go 源码；WireGuard 模式已与官方 WireGuard Go 实现完成加密互通。
- 修复非零 S4 初次配置下首包丢失问题，原始源码失败与补丁通过均有回归证据。
- U30 Air 上完成真实本机 UDP + 内存 TUN 的三种模式、双向 IPv4/IPv6 加密测试。源码、许可、固定构建流程与测试随包提供。

**此版本仍是预览版。** KANO 已登录页面中的完整安装、实际系统 TUN / 来源策略路由、自启、BE88U 公网握手和长期吞吐尚未完成真机联调。内存 TUN 协议测试不能替代实际代理测试。DNS 仅保存供参考，不接管 Android 全局 DNS。

下载 `tunnel-studio.js` 导入 KANO；`tunnel-studio-0.1.0.zip` 为完整源码与构建包。下载后可用 `SHA256SUMS` 检查完整性。
