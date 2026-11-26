# Windows 11 测试环境资产

## 环境概览

- 用途：Farm 在 Windows 上的部署与兼容性测试。
- 形态：macOS 宿主上的 VMware Fusion 虚拟机（非独立物理机）。
- 负责人：liuzhuo。

## 主机 / 虚机

| 角色 | 地址 | 说明 |
| --- | --- | --- |
| 宿主 Mac | 10.0.0.12 | MacBook Pro 2013，macOS 11.7.6，VMware Fusion 13.5.2，已配置免密 `ssh liuzhuo@10.0.0.12` |
| 测试虚机 | 10.0.0.119 | hostname `WIN11TEST`，桥接网络（与宿主同网段） |

- 虚机规格：4 vCPU / 4 GB 内存 / 60 GB 精简磁盘。
- 系统：Windows 11 企业版 **LTSC 2024 评估版**，build 26100（90 天评估授权）。
- 虚机文件：宿主机 `~/Virtual Machines.localized/Windows 11 Test.vmwarevm`。
- VMware Tools：已安装，VMware Fusion 可直接读写 guest 网络。

## 访问方式

| 用途 | 入口 | 凭据 |
| --- | --- | --- |
| 远程桌面 | `10.0.0.119:3389` | 用户 `test`，口令不入库（见「凭据存放」） |
| SSH | `ssh test@10.0.0.119` | 已写入宿主机 `~/.ssh/id_rsa.pub`，免密登录 |
| 宿主机 | `ssh liuzhuo@10.0.0.12` | 免密登录 |
| 本地控制台 | 宿主 Fusion 双击 `Windows 11 Test` | 已开启 `test` 自动登录 |

guest 内服务：`sshd`、`TermService`（远程桌面）、`VMTools` 均为自动启动。

## 凭据存放

- Windows 用户 `test` 的口令不写入仓库，由 liuzhuo 保管（密码管理器）。
- 修改口令：`ssh test@10.0.0.119 "net user test <新口令>"`，随后同步 `Winlogon` 的 `DefaultPassword`（自动登录用）。
- SSH 依赖宿主机 `~/.ssh/id_rsa`；吊销时删除 guest 的 `C:\ProgramData\ssh\administrators_authorized_keys` 对应行。

## 部署 Farm

```bash
# 在开发机交叉编译（前端产物需先构建）
cd Farm && pnpm --dir frontend build
GOOS=windows GOARCH=amd64 go build -o farm-windows-amd64.exe .

# 上传并运行
scp farm-windows-amd64.exe test@10.0.0.119:C:/farm/
ssh test@10.0.0.119 "C:\\farm\\farm-windows-amd64.exe"   # 默认监听 8080
```

- 设备控制依赖 Windows 版 `adb.exe`（platform-tools），需在 PATH 中。
- 访问地址：`http://10.0.0.119:8080`（构建参数默认 HTTP；生产才用 `-https`）。

## 已知限制

- 宿主 macOS 11 低于 Fusion 13.5 官方要求（macOS 12+）：**EFI 引导不可用**，虚机为传统 BIOS 启动；无 3D 加速，建议用 RDP 操作。
- 无 TPM 2.0 / Secure Boot（安装时 LabConfig 绕过），BitLocker 等能力不可用。
- 评估版 90 天授权，长期使用需正式授权。
- 防火墙三档当前均为关闭（调试期）；如需长期对局域网开放，应仅放行 22 / 3389。
- 虚机以 headless（`vmrun ... nogui`）运行期间，Fusion GUI 无法接管该虚机，会报「无法开启」；需先停止再用 GUI 打开。

## 变更提示

- 2026-11-22：新建 Windows 11 测试环境，登记宿主 `10.0.0.12`、虚机 `10.0.0.119`、RDP/SSH 连接方式与已知限制。
- 2026-11-22：部署桌面端 `Farm.exe`（v2026.11.21，官方 `farm-windows-gui-amd64.exe`）到桌面并完成一轮界面自测，证据与报告见 `docs/verification/evidence-2026-11-22-win11/`。
