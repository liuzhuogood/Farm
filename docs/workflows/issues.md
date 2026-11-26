# 问题登记（issues）

把可复现的问题登记到 GitHub Issues，**必须带截图与日志**。测试失败、验收不通过、CI 失败、用户反馈都走这里；纯需求用 `enhancement`，不走本流程。

## 1. 什么时候登记

- [test.md](test.md) 任一验收项不通过。
- beta 或正式环境里用户反馈的缺陷。
- Actions 失败且重跑仍失败（runner 偶发失败先重跑，不重复开 issue）。
- 同一问题已有 open issue：**不新建**，改成评论追加新证据（见第 3 节）。

## 2. 硬规则：仓库是公开的

本仓库 `liuzhuogood/Farm` 是 public，与 [环境资产约定](../environments/README.md) 一致：**不把内网信息写到外部服务**。

禁止出现在 issue 标题、正文、截图、日志里的内容：

| 禁止 | 例子 |
| --- | --- |
| 服务器 IP / 域名 / SSH 账号 | `10.0.0.6`、`liuzhuo@10.0.0.6`、`farm.s5.5slive.com` |
| 设备 serial | `10.0.0.104:5555` |
| 本地绝对路径 | `/Users/liuzhuo/code/Farm` |
| 凭据 | token、access key、minisign 私钥、`.env` 内容 |

日志只贴**判定所需的最小片段**（默认 ≤ 60 行），贴之前必须过一遍脱敏：

```bash
sed -E 's/([0-9]{1,3}\.){3}[0-9]{1,3}(:[0-9]+)?/<ip>/g; s#/Users/[^ /]+#/Users/<user>#g' /tmp/raw.log | tail -60 > /tmp/issue-log.txt
```

## 3. 先查重

```bash
gh issue list --state open --search "关键现象词" --limit 10
```

命中同一问题就追加评论，避免每次复测刷一条：

```bash
gh issue comment <编号> --body-file /tmp/comment.md
```

评论里写清：复测版本（beta.N / 正式号）、时间、新的截图与日志、结论（仍复现 / 已变化）。

## 4. 证据一：截图

界面类问题**必须**有截图；交互类问题贴「期望 vs 实际」两张。

上传到 OSS，目录固定 `test/`：

```bash
OSS_SKILL_DIR=/Users/liuzhuo/.pi/agent/skills/lz-act/modules/lz-oss
python3 "$OSS_SKILL_DIR/scripts/oss.py" put ./shot.png test/v2026.11.25-player-webrtc.png
python3 "$OSS_SKILL_DIR/scripts/oss.py" stat test/v2026.11.25-player-webrtc.png   # 先确认对象存在
python3 "$OSS_SKILL_DIR/scripts/oss.py" url test/v2026.11.25-player-webrtc.png    # 取正文引用地址
```

正文里用 `![实际画面](<url>)` 引用。截图前先裁掉或遮挡内网地址、serial、账号等区域。

## 5. 证据二：日志

| 来源 | 取法 |
| --- | --- |
| 后端服务 | `./scripts/services.sh logs backend`（文件在 `.cache/services/backend.log`） |
| 前端开发服务 | `./scripts/services.sh logs frontend` |
| 设备链路（scrcpy） | 后端日志里 `[scrcpy:<serial>]` 前缀的行，贴前把 serial 脱敏 |
| WebRTC 通道 | 后端日志里 `[webrtc]`、`[rtc]` 前缀的行 |
| Actions | `gh run view <run-id> --log-failed`，只贴失败步骤的尾部 |

日志片段要能支撑结论：贴「报错行 + 之前 10~20 行上下文」，不要整文件。脱敏后用代码块包起来。

## 6. 标题与标签

- 标题格式：`<范围>：<现象>`，写现象不写猜测。
  - `投屏：WebRTC 通道在 5% 丢包下反复重启编码，画面持续卡顿`
  - `桌面：Windows 小窗恢复后不再置顶`
- 标签：`bug` 必选；测试验收不通过再加 `test-failure`（首次使用前创建一次）：

```bash
gh label create test-failure --description "测试工作流验收不通过" --color FBCA04
```

## 7. 正文模板

把正文写到文件再提交，避免命令行引号转义出错：

```bash
cat > /tmp/issue.md <<'EOF'
## 现象
（一句话说清看到了什么，附截图）

## 期望
（按 tasks.md 的验收口径应该怎样）

## 复现步骤
1.
2.
3.
（注明是必现还是偶现；偶现写触发频率）

## 环境
- 版本 / 通道：
- 客户端：浏览器（名称+版本）/ 桌面端（系统+架构）/ 手机端
- 设备：型号 + Android 版本（**不写 serial**）

## 证据
- 截图：
- 日志：
- Actions / 提交：

## 影响
- 阻塞项：test.md 的哪一条 / tasks.md 的哪一项
- 首次出现：
EOF

gh issue create --title "投屏：WebRTC 通道在 5% 丢包下反复重启编码" \
  --body-file /tmp/issue.md --label bug --label test-failure
```

## 8. 关闭与回填

- 修复后在同一 issue 里贴修复提交与复测证据（截图 + 日志），**人工确认后关闭**，不自动关。
- 把 issue 链接回填到 `docs/versions/<版本>/tasks.md` 的对应验收项，作为该版本的问题记录。
- 复测通过但问题仍存在争议时，保留 open 并在评论里写清差异，不靠关单掩盖。

## 9. 与测试工作流的关系

[test.md](test.md) 第 4 步「不通过时退回开发」按本节执行：登记或追评 issue → 链接回填 tasks.md → 修复后重发 `beta.N+1` 复测，不复用旧 beta 结论。
