# 官方来源与独立实现说明

核查日期：2026-10-12，UTC+8。网页检索在本会话未返回可引用内容；技术结论以本地实际取得并校验 commit 的官方源码为依据。以下版本是本项目固定版本，不声称都是当天最新发行。

| 用途 | 官方仓库 | 固定提交 |
|---|---|---|
| 运行后端 | `https://github.com/amnezia-vpn/amneziawg-go`，标签 v3.1.20260828 | b5928efb6ca19f0153958460c3d141f04abc5c2e |
| WireGuard 互通测试对端 | `https://github.com/WireGuard/wireguard-go` | 2631ce99a06f27120d581611cf125d68bc6aa565，提交时间 2026-10-06 16:45:05 UTC |

可审计位置：

- Go `README.md` 的配置章节：AWG 3+ 头部保护、内容 padding、时序和签名包说明。
- Go `device/uapi.go`：实际支持字段、uint16 S、uint32 J、H 范围、bool 开关、peer UAPI、header 区间冲突与 nonce 长度检查。
- Go `device/noise-types.go`：uint32 闭区间的解析与选择；header cipher nonce 为 12。
- Go `device/send.go`：TUN 阻塞 Read 前获取 S4 的原始行为。
- Go `tun/tun_linux.go`：`/dev/net/tun` 打开路径。
- WireGuard 官方 `device` / `conn` / `tun/tuntest`：测试中另一端使用原版 WireGuard 实现。

本项目不使用官方命令行守护程序的默认 UAPI 目录，而是把官方 device/conn/tun 包链接到自己的 Android ELF 中。外部控制 socket 是本项目的 JSON 协议，只提供状态和停止，不暴露原始含密钥 UAPI。插件将配置校验后直接送入 `Device.IpcSet`。

## 两个最小上游补丁

1. `/dev/net/tun` 不存在时才回退 `/dev/tun`；权限拒绝时不回退，不创建节点、不改权限。
2. 首次 TUN Read 可能在初始 IpcSet 前捕获 S4=0。Read 返回后重新读取 S4，将 payload 移到新 offset，再交给加密队列。越界时丢弃该包。管理器禁止运行中热改配置，因此无需承诺任意时刻热改 padding 的兼容性。

回归检查确实复现了未修改源码的首包失败；带补丁三种模式均通过首次传输。脚本与日志见 `tools/check_regression.py` 和 `regression-unpatched.txt`。

## KANO 调用约定

本次从 U30 Air 上通过 `pm path` / `adb pull` 取得 KANO **4.1.5** 的公开 APK 代码，检查其 assets，未读取 App 私有配置、token、真实隧道密钥或备份。APK 仅留在工作临时目录，不随插件分发。

- `runShellWithRoot(command, timeout=10000)` -> `POST KANO_baseURL + '/root_shell'`，JSON `{command, timeout}`，headers 使用 `common_headers`。
- 请求响应 `{result, error}` 被宿主包装成 `{success, content}`。宿主 success 不代表 shell 的退出码，插件使用随机命令结束标记验证退出状态。
- 此设备页面 `KANO_baseURL` 为 `/api`；插件使用宿主变量，不硬编码服务器地址。
- `/upload_img` 使用 FormData 的 `file` 字段，响应 `url`。宿主自己的代码映射到 `/data/data/com.minikano.f50_sms/files/` 下。插件接受有/无起始斜杠的安全相对路径，并拒绝 traversal、双斜杠、shell 字符。
- `.functions-container` 是插入点，`collapseGen` 没有成为必需依赖。`createToast` 可用，但本版采用内置 aria-live 状态栏反馈。
- 未携认证信息的设备 `/api/root_shell` 返回 HTTP 401。没有尝试绕过登录，也没有取得已经登录的浏览器 token。

旧项目仅只读查看了宿主调用约定与说明；上一轮草稿仅查看 `docs/RESEARCH.md`、`resources/backend.json`。没有导入其管理实现、UI、测试或资源二进制。
