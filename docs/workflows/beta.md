# beta 发布工作流（release-beta）

用途：把 main 的当前代码打成 beta 预发布并部署 beta 服务实例，供测试工作流验收；不影响正式通道。

1. 确认在 main 分支：版本分支先合并回 main 并推送。beta 只从 main 出包，不在版本分支上打 tag。
2. 确认 `VERSION` 是本次目标正式号（如 `v2026.11.20`）。beta 号由它派生：`v2026.11.20-beta.N`，N 从 1 递增；tag 与 `VERSION` 不同是允许的。
   - 本版约定：开版本时就把 `VERSION` 进位到目标正式号并提交，不要等到正式发布才改；否则 beta 会继承上一个已发布版本的号。
3. 打 beta tag 并推送：`git tag v2026.11.20-beta.1 && git push origin v2026.11.20-beta.1`。
   - 同一目标版本可多次发，每次 N 递增；改了代码就重发，不复用旧 beta。
4. 跟踪 Actions：`release.yml`、`macos-dmg.yml` 全绿。beta 跳过 Docker job，不推 `latest` 镜像，也不发布 PyPI。
5. 核验 Release：标记为预发布（`prerelease: true`），9 个程序与 `desktop-latest.json` 齐全，签名与正式包同密钥；`releases/latest` 仍指向上一正式版，正式通道看不到 beta。
6. 部署 beta 服务实例：`bash scripts/deploy/deploy-beta.sh`（8081、`farm-beta.service`、`/opt/farm-beta`、独立数据目录），确认 8080 与 `farm.service` 未受影响。
7. 交接测试：把 beta 版本号、Release 链接、8081 地址写进 `docs/versions/<版本>/tasks.md`，进入 [test.md](test.md)。

## 版本号约定

| 形态 | 编号 | 说明 |
| --- | --- | --- |
| 正式首版 | `v2026.11.20` | 日期段递增 |
| 正式补丁 | `v2026.11.20-1`、`-2` | 同日第 N 个补丁，N 只增不减；只用于 CLI 与服务器二进制 |
| beta | `v2026.11.20-beta.1` | 日期段必须大于当前最新正式号，不与补丁号混用 |

同一日期一旦发过 `-N` 补丁，不再补发该日期无后缀的正式版。

> 桌面端更新器按标准 semver 比较，`-N` 会被判为低于同号正式版。因此**桌面端只发三段日期号**，同一天要再发桌面版就把日期段进位；`-N` 只用于 CLI 与服务器二进制。
