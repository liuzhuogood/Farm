# Farm Companion

独立实现的只读 Android 无障碍控件树快通道。设计参考 [Pinky Companion](https://github.com/DIYgod/pinky/tree/ff473e2/android/companion) 的“设备端 AccessibilityService + ADB 转发本地 socket”架构，未复制其源码或协议。Farm 原有 UIAutomator dump 继续作为无助手、助手未启用或采集失败时的回退。

## 构建

1. 安装 JDK 8+、Android SDK Platform 35、Build Tools 35.0.0。
2. 运行 `./packages/android-companion/build.sh`。产物是同目录 `farm-companion.apk`；构建中间文件在忽略的 `build/` 中。
3. 本地开发签名首次生成在 `~/.local/state/farm/android-companion/dev-signing.jks`，不会写入仓库。同一份开发 APK 后续升级必须继续使用这份密钥。
4. 正式签名传 `FARM_COMPANION_KEYSTORE`、`FARM_COMPANION_KEY_ALIAS`、`FARM_COMPANION_STORE_PASSWORD`，按需提供 `FARM_COMPANION_KEY_PASSWORD`。密码只经 apksigner 的环境变量接口读取。`ANDROID_SDK_ROOT`、`FARM_ANDROID_PLATFORM`、`FARM_ANDROID_BUILD_TOOLS` 可指定 SDK 位置和已安装版本。
5. 构建同时运行纯 Java 的非法 Unicode/严格 XML 字节上限、旧协议拒绝与采集线程占满时 ping 独立应答验证；运行 `python3 packages/android-companion/check.py` 验证最终 APK 的签名、包名、SDK 和权限边界。

应用版本 `1.0.1`（versionCode `2`），包名 `app.farm.companion`，服务组件 `app.farm.companion/.FarmAccessibilityService`，最低 Android 8（API 26），目标 Android 15（API 35）。安装/升级由 Farm 主机端显式动作执行；本项目构建脚本不连接设备。手机桌面的说明页可打开无障碍设置。

## 通信

- 设备端 Android abstract Unix socket 名称：`farm_companion`。应用没有 INTERNET 权限，也不监听网络端口。
- 只接受 Unix peer UID 为 root（0）、ADB shell（2000）或应用自身的连接。主机经已授权 ADB 连接转发后访问。
- 一行 UTF-8 JSON 一个请求，最长 1024 字节；请求总读取超时及 socket 发送超时各 3 秒。
- 协议版本固定为整数 `2`，旧开发版协议 `1` 明确不兼容，需更新 APK；不按相似字段猜测或自动兼容。
- 验活请求：`{"version":2,"command":"ping"}`；成功只返回 `{"version":2,"command":"pong"}`。ping 直接在客户端处理线程响应，不进入采集队列、不检查锁屏、不读取窗口。ready 仅表示协议通信可用，不代表当前树一定可读。
- 快照请求：`{"version":2,"command":"snapshot"}`。
- 快照成功：`{"version":2,"xml":"<hierarchy rotation=\"0\">...</hierarchy>","width":1080,"height":2400,"captured_at_ms":1794550000000}`。
- 失败：`{"version":2,"error":"device_locked"}`；失败不带旧 XML 或猜测尺寸，旧版本请求返回 `unsupported_protocol_or_command`。
- 单连接允许连续请求，每次 snapshot 都重新读取当前树；ping 仅验活，不缓存页面内容。只读快照只返回默认显示器 0，宽高和 bounds 均是当前旋转后的实际显示坐标。

XML 使用 Farm 已有 UIAutomator node 属性。额外 `editable` 属性来自真实节点的 `isEditable()`。密码节点的 text 和 content-desc 均为空。不可见节点及完全在屏幕外的节点不返回。

## 容量与生命周期

最多 2 个客户端连接；采集工作线程最多 1 个、排队最多 1 个。采集总预算 2 秒，最大 5000 节点、64 层、2 MiB XML，超限明确报错。单个 Binder 调用无法被 Java 强制终止，超时会取消其 Future，后续遍历检测取消；工作线程仍被阻塞时会明确 busy，不增开无界线程。

停用/解绑/销毁服务会 shutdown 监听 socket、关闭已连接客户端、中断连接和采集线程并取消未启动任务。`onInterrupt` 只代表无障碍反馈被中断，不被误当成解绑。日志只记录耗时、节点数、字节数和错误原因，不打印控件文字。应用不提供触控/输入/通知读取能力，也不申请修改系统设置的权限；启停服务由主机端或手机系统设置管理。当前由用户显式启停服务，尚未实现按任务自动启停的 lease；启用期间服务持续可用。

本轮设备实际安装、权限授权、真机耗时及回退验证由主机集成验收记录；构建成功和 APK 自检不等同于真机行为通过。
