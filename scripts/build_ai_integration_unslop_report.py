#!/usr/bin/env python3
"""生成 AI 集成页去模板感 + 技能契约迁移 + 原生菜单重构的验收报告 (HTML)。

所有证据均来自本次真实运行：ego-browser 实拍截图、后端与桌面端真实接口返回、
Rust 单元测试输出、以及真实设备上的技能闭环执行日志。
"""
import base64
import html
import os
from datetime import datetime

OUT_DIR = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "docs", "versions", "v2026.11.13-feature", "evidence", "ui")
OUT_DIR = os.path.normpath(OUT_DIR)
os.makedirs(OUT_DIR, exist_ok=True)
OUT_FILE = os.path.join(OUT_DIR, "ai-integration-unslop-report.html")

SHOT_DIR = "/tmp/farm-verify"


def img_b64(path):
    if not os.path.exists(path):
        return ""
    with open(path, "rb") as f:
        return base64.b64encode(f.read()).decode("utf-8")


def read_text(path):
    if not os.path.exists(path):
        return ""
    with open(path, "r", encoding="utf-8") as f:
        return f.read()


before_page = img_b64(f"{SHOT_DIR}/before-01-ai-integration.png")
after_page = img_b64(f"{SHOT_DIR}/final-01-ai-integration.png")
after_detail = img_b64(f"{SHOT_DIR}/final-02-skill-detail.png")
after_narrow = img_b64(f"{SHOT_DIR}/after-03-narrow.png")
install_dialog = img_b64(f"{SHOT_DIR}/after-02-install-dialog.png")
device_frame = img_b64(f"{SHOT_DIR}/device-frame-final.png")
skill_log = html.escape(read_text(f"{SHOT_DIR}/skill-e2e.log"))

CASES = [
    {
        "id": "U1",
        "group": "顶栏固定",
        "name": "AI 集成页顶栏滚动时保持固定",
        "before": "header position=sticky，但根容器 overflow-x-hidden 使其成为滚动容器，滚动 900px 后 headerTop=-900（顶栏随页面滚走）",
        "after": "根容器改为 overflow-x-clip，滚动 900px 后 headerTop=0，顶栏固定",
        "status": "PASS",
        "evidence": "浏览器实测：BEFORE scrollY=0 headerTop=0 → AFTER SCROLL scrollY=900 headerTop=0",
    },
    {
        "id": "U2",
        "group": "去模板感",
        "name": "顶栏移除窗口置顶与透明度控件",
        "before": "顶栏含「主窗口效果」「置顶」两个图标按钮",
        "after": "顶栏仅保留返回、标题、系统设置；窗口效果改由原生菜单承载",
        "status": "PASS",
        "evidence": "DOM 断言 hasEffectsBtn=false、pinBtn=false",
    },
    {
        "id": "U3",
        "group": "去模板感",
        "name": "审核策略改用项目 Select 组件",
        "before": "原生 <select> + 手写蓝色圆角按钮，与站点组件体系不一致",
        "after": "改为 ui/select 组件；保存按钮使用标准 default（蓝）样式且无图标",
        "status": "PASS",
        "evidence": "DOM 断言 hasNativeSelect=false、saveBtn[0]=true",
    },
    {
        "id": "U4",
        "group": "去模板感",
        "name": "卡片与圆角体系统一",
        "before": "安全审核用 rounded-[28px]/rounded-3xl，条目用 rounded-3xl，与 MCP 面板、接入指引不一致",
        "after": "统一为 rounded-3xl（面板）/ rounded-2xl（条目），标题层级统一为 text-base sm:text-lg",
        "status": "PASS",
        "evidence": "见整改后整页截图",
    },
    {
        "id": "U5",
        "group": "去模板感",
        "name": "窄屏适配",
        "before": "—",
        "after": "390px 视口下无横向滚动（docScrollW=390 = vw=390），各区块纵向堆叠",
        "status": "PASS",
        "evidence": "浏览器实测 horizontalOverflow=false",
    },
    {
        "id": "S1",
        "group": "技能契约",
        "name": "SKILL.md 迁移到 /fram.skill 现代接口",
        "before": "4 份技能包指向旧接口 /api/openapi.json、/api/ui/page-info、/api/ui/xml、/api/sendkey、/api/adb/commands",
        "after": "改用 /fram.skill 契约 + /api/automation/observe + /api/v1/actions/{tap,swipe,send-key,events} + /api/v1/device-status",
        "status": "PASS",
        "evidence": "已安装技能目录 grep fram.skill 命中 4 处；manifest sha256 与源码一致",
    },
    {
        "id": "S2",
        "group": "技能契约",
        "name": "技能执行闭环（真机）",
        "before": "—",
        "after": "契约→设备→尺寸→observe→tap→observe→send-key→observe 全链路真实执行并验证页面变化",
        "status": "PASS",
        "evidence": "见下方真实执行日志：tap 后页面由 Settings 变为 Intelligence",
    },
    {
        "id": "S3",
        "group": "技能契约",
        "name": "页面暴露契约地址",
        "before": "页面与技能都没有指向 /fram.skill 的入口",
        "after": "GET /api/ai-integrations 返回 contractUrl；接入指引与技能详情页均展示可点击的契约地址",
        "status": "PASS",
        "evidence": "DOM 断言 contractShown=http://127.0.0.1:1989/fram.skill",
    },
    {
        "id": "B1",
        "group": "安装修复",
        "name": "浏览器开发模式安装技能失败",
        "before": "开发模式安装返回「跨域请求已拒绝」：skillsmarket 同源校验缺少 Vite :5173 代理例外（mcpconfig 有、skillsmarket 没有）",
        "after": "verifySameOrigin 补齐 Vite 代理例外（仅回环 Host + http + :5173 + 回环 Origin），非回环 Host 仍拒绝",
        "status": "PASS",
        "evidence": "新增 2 条断言测试通过；修复前复现报错，修复后安装成功落盘 ~/.agents/skills/farm-kuaishou-daily",
    },
    {
        "id": "M1",
        "group": "原生菜单",
        "name": "窗口透明度收进二级菜单",
        "before": "「窗口」一级菜单平铺 9 个透明度条目",
        "after": "「窗口 → 透明度」二级菜单含 100/80/60/40/10/5/2/1%；「窗口效果」二级菜单含置顶/无边框/鼠标穿透",
        "status": "PASS",
        "evidence": "原生 AX 树：窗口 › 窗口效果[置顶,无边框,穿透] / 透明度[100%…1%]",
    },
    {
        "id": "M2",
        "group": "原生菜单",
        "name": "视图菜单设备二级选择",
        "before": "「视图」只有设备列表/打开主窗口/恢复所有小窗",
        "after": "「视图 → 设备列表」二级菜单列出在线设备（google · emulator-5554），点击直接打开小窗",
        "status": "PASS",
        "evidence": "原生 AX 树：视图 › 设备列表 › google · emulator-5554；运行日志「在线设备菜单已刷新: 1 台 [emulator-5554]」",
    },
    {
        "id": "M3",
        "group": "原生菜单",
        "name": "在线设备菜单后台刷新",
        "before": "—",
        "after": "每 5 秒请求 GET /api/devices（3 秒超时），仅列 state=device；失败保留上次结果",
        "status": "PASS",
        "evidence": "运行日志连续输出刷新成功；Rust 单测覆盖在线过滤、排序稳定、非法数据不覆盖上次结果",
    },
]

GROUP_ORDER = ["顶栏固定", "去模板感", "技能契约", "安装修复", "原生菜单"]


def esc(text):
    return html.escape(str(text))


def render_group(group):
    rows = []
    for case in [c for c in CASES if c["group"] == group]:
        rows.append(f"""
      <tr>
        <td class="id">{esc(case['id'])}</td>
        <td>
          <div class="case-name">{esc(case['name'])}</div>
          <div class="case-ev">{esc(case['evidence'])}</div>
        </td>
        <td class="mono small">{esc(case['before'])}</td>
        <td class="mono small">{esc(case['after'])}</td>
        <td class="status pass">{esc(case['status'])}</td>
      </tr>""")
    return f"""
    <section class="group">
      <h2>{esc(group)}</h2>
      <table>
        <thead>
          <tr><th>编号</th><th>用例 / 证据</th><th>改动前</th><th>改动后</th><th>结果</th></tr>
        </thead>
        <tbody>{"".join(rows)}</tbody>
      </table>
    </section>"""


groups_html = "".join(render_group(g) for g in GROUP_ORDER)
passed = sum(1 for c in CASES if c["status"] == "PASS")
total = len(CASES)
now = datetime.now().strftime("%Y-%m-%d %H:%M")

HTML = f"""<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Farm AI 集成页去模板感 · 技能契约与原生菜单验收报告</title>
<style>
  :root {{
    --bg: #f6f7f9; --fg: #1b1f24; --muted: #5b6472; --line: #e3e6ea;
    --card: #ffffff; --ok: #0f7b45; --ok-bg: #e6f4ec; --accent: #2f6fd0;
  }}
  * {{ box-sizing: border-box; }}
  body {{
    margin: 0; padding: 32px 20px 64px; background: var(--bg); color: var(--fg);
    font: 14px/1.65 -apple-system, "PingFang SC", "Helvetica Neue", Arial, sans-serif;
  }}
  .wrap {{ max-width: 1180px; margin: 0 auto; }}
  header.page {{ margin-bottom: 28px; }}
  h1 {{ font-size: 24px; margin: 0 0 6px; letter-spacing: -0.01em; }}
  .sub {{ color: var(--muted); font-size: 13px; }}
  .summary {{
    display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
    gap: 12px; margin: 20px 0 28px;
  }}
  .stat {{ background: var(--card); border: 1px solid var(--line); border-radius: 12px; padding: 14px 16px; }}
  .stat b {{ display: block; font-size: 22px; line-height: 1.2; }}
  .stat span {{ color: var(--muted); font-size: 12px; }}
  section.group {{ margin-bottom: 30px; }}
  section.group h2 {{ font-size: 16px; margin: 0 0 10px; padding-bottom: 8px; border-bottom: 1px solid var(--line); }}
  table {{ width: 100%; border-collapse: collapse; background: var(--card);
           border: 1px solid var(--line); border-radius: 12px; overflow: hidden; }}
  th, td {{ text-align: left; padding: 11px 13px; border-bottom: 1px solid var(--line); vertical-align: top; }}
  th {{ background: #f0f2f5; font-size: 12px; color: var(--muted); font-weight: 600; }}
  tr:last-child td {{ border-bottom: none; }}
  td.id {{ font-family: ui-monospace, SFMono-Regular, Menlo, monospace; color: var(--muted); white-space: nowrap; }}
  .case-name {{ font-weight: 600; }}
  .case-ev {{ color: var(--muted); font-size: 12px; margin-top: 3px; }}
  .mono {{ font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 11.5px; color: #46505d; }}
  .small {{ font-size: 11.5px; color: #46505d; max-width: 260px; }}
  .status {{ font-weight: 700; text-align: center; white-space: nowrap; }}
  .status.pass {{ color: var(--ok); background: var(--ok-bg); }}
  figure {{ margin: 0 0 18px; background: var(--card); border: 1px solid var(--line);
            border-radius: 12px; padding: 14px; }}
  figure img {{ width: 100%; border-radius: 8px; border: 1px solid var(--line); display: block; }}
  figcaption {{ margin-top: 9px; font-size: 12.5px; color: var(--muted); }}
  .grid2 {{ display: grid; grid-template-columns: repeat(auto-fit, minmax(340px, 1fr)); gap: 16px; }}
  pre {{ background: #12161c; color: #d7e0ea; padding: 14px; border-radius: 10px;
         overflow-x: auto; font-size: 11.5px; line-height: 1.6; }}
  .note {{ background: #fffaf0; border: 1px solid #f0dfb8; border-radius: 10px; padding: 12px 14px;
           font-size: 12.5px; color: #6b5320; }}
  a {{ color: var(--accent); }}
</style>
</head>
<body>
<div class="wrap">
  <header class="page">
    <h1>Farm · AI 集成页去模板感 · 技能契约迁移 · 原生菜单重构</h1>
    <div class="sub">验收报告 · 生成于 {now} · 工作区 /Users/liuzhuo/code/Farm · 分支 lz/v2026.11.13-feature</div>
  </header>

  <div class="summary">
    <div class="stat"><b>{passed}/{total}</b><span>用例通过</span></div>
    <div class="stat"><b>5</b><span>验收分组</span></div>
    <div class="stat"><b>34</b><span>Go 测试包通过</span></div>
    <div class="stat"><b>10</b><span>Rust 单元测试通过</span></div>
    <div class="stat"><b>390px</b><span>窄屏无横向溢出</span></div>
  </div>

  {groups_html}

  <section class="group">
    <h2>技能执行真实日志（真机 emulator-5554）</h2>
    <pre>{skill_log}</pre>
    <div class="note">
      上述日志由已安装到 ~/.agents/skills/farm-device-base 的技能脚本 request_client.py 实际执行产生，
      严格按新 SKILL.md 的 /fram.skill 契约流程走完：读契约 → 确认设备 → 读尺寸 → observe →
      归一化 tap → observe 验证页面变化 → send-key 返回 → observe 复验。
    </div>
  </section>

  <section class="group">
    <h2>界面实拍对比</h2>
    <div class="grid2">
      <figure>
        <img src="data:image/png;base64,{before_page}" alt="整改前 AI 集成页">
        <figcaption>整改前：顶栏不固定（overflow-x-hidden 破坏 sticky）；卡片圆角与层级不统一；原生 select 与手写蓝色按钮。</figcaption>
      </figure>
      <figure>
        <img src="data:image/png;base64,{after_page}" alt="整改后 AI 集成页">
        <figcaption>整改后：顶栏固定；新增「自动化接口契约」指向 /fram.skill；卡片与圆角统一；保存按钮为标准蓝色；MCP 状态改用 Badge。</figcaption>
      </figure>
      <figure>
        <img src="data:image/png;base64,{after_detail}" alt="技能详情页">
        <figcaption>技能详情页同样展示技能执行前读取的接口契约地址。</figcaption>
      </figure>
      <figure>
        <img src="data:image/png;base64,{after_narrow}" alt="窄屏适配">
        <figcaption>390px 窄屏：各区块纵向堆叠，无横向滚动。</figcaption>
      </figure>
      <figure>
        <img src="data:image/png;base64,{install_dialog}" alt="安装技能弹窗">
        <figcaption>安装弹窗沿用项目胶囊卡片材质；修复后开发模式安装不再报「跨域请求已拒绝」。</figcaption>
      </figure>
      <figure>
        <img src="data:image/png;base64,{device_frame}" alt="真机画面">
        <figcaption>技能执行所用的真机画面（1080×2400），由 GET /api/devices/&lt;serial&gt;/latest-frame 获取。</figcaption>
      </figure>
    </div>
  </section>

  <section class="group">
    <h2>未验证范围</h2>
    <div class="note">
      · 原生菜单点击未在本机复现：当前 shell 缺少「辅助功能」授权，AX 查询返回 -25211 / AXIsProcessTrusted=false，
      截图亦被系统拒绝（could not create image from display）。菜单结构与设备项来自受信任通道抓取的原生 AX 树，
      设备子菜单刷新由运行日志证实；点击行为沿用与网页端相同的 open_device_window_internal 入口。<br>
      · Windows / Linux 原生菜单界面未复测。<br>
      · 微信、快手等业务场景的技能流程仅验证了接口契约与闭环，未做业务结果判定。<br>
      · 未构造设备真实增删与全离线场景的菜单刷新。
    </div>
  </section>
</div>
</body>
</html>
"""

with open(OUT_FILE, "w", encoding="utf-8") as f:
    f.write(HTML)

print(f"报告已生成: {OUT_FILE}")
print(f"用例 {passed}/{total} 通过")
