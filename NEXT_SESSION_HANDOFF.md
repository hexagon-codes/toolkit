# Toolkit 当前接手文档

> 更新时间：2026-08-12（Asia/Shanghai）
>
> 目标版本：`v0.3.0`
>
> 发布基线：以 `v0.3.0` 标签指向的提交为准
>
> 最近已发布标签：`v0.2.6`

## 1. 范围与约束

- 只闭环已经重构或当前已修改的代码；未修改代码不再扩展审计或重构。
- 只做有确定 RED 证据的最小修复。任何新重构都必须先说明原因并取得用户审批。
- macOS、Linux、Windows 都要保留平台合同；本地交叉编译不能冒充 Windows 原生运行证据。
- 共享工作树中的既有改动必须全部保留，禁止 `reset`、`checkout`、`clean`、覆盖或回滚。
- 最终门禁通过后仅提交 Toolkit 当前改动；不打 Tag、不 Push，`v0.3.0` 的发布由用户自行完成。
- 新增代码注释使用中文；新增错误、状态提示和提示语使用英文。
- 不调用真实模型或钉钉，不改 HexClaw Desktop UI，不使用旧 DMG 冒充当前源码构建。

## 2. 当前事实

- `go.mod` 声明的最低 Go 版本是 `1.25.12`，不是要求所有开发机只能安装该版本。发布门禁使用 Go 1.25.12、`GOWORK=off` 和 `-mod=readonly` 固定最低版本证据。
- 根模块与 `examples` 已删除不再可达的 `github.com/bytedance/gopkg`；tidy、verify、list、test、vet、build 门禁均取得过通过证据。
- API 目标已统一为 `v0.3.0`；`gorelease -base=v0.2.6 -version=v0.3.0` 与当前 34 个不兼容包段的 baseline 校验已通过。源码再次变化后必须按最终指纹重跑。
- `util/retry` 只保留 `If`，旧 `RetryIf` 等入口有意删除；下游已经迁移，不恢复兼容 facade。
- `util/hash` 不恢复 MD5/SHA1 公共 API；只有下游明确公开的 `text_hash(md5)` 合同在工具内部使用标准库 `crypto/md5`。

## 3. 已取得的本地证据

以下记录最终冻结源码已经取得的本地证据。

| 范围 | 证据 |
|---|---|
| 根模块 | 全仓普通测试、Race、build、vet、golangci-lint 均通过。 |
| macOS Sandbox | `os/sandbox` 全包普通、Race 与关键原生 no-skip 门禁通过；hostedtoolcache 改为基于当前用户主目录后，聚焦回归及全包普通/Race 均通过。 |
| Linux Sandbox | Ubuntu 22.04、Go 1.25.12 的隔离 Docker 环境中，root 13/13、non-root 4/4 原生测试零 skip 通过。 |
| Windows | Windows/Linux 交叉 build、vet 通过；当前候选提交的 Windows Server 2022 原生 28 项 `windows_security` 只能在推送后由 CI 证明。 |
| 集成与安全 | MySQL 8.4、Redis 7.4 ACL 集成通过；`govulncheck` 无可达或导入漏洞；敏感信息与二进制范围扫描通过。 |
| API 与工作流 | 4 个 workflow 的 actionlint、cicheck、API checker 及 34 段 breaking baseline 校验通过；远端 CI 状态仍以当前提交推送后的结果为准。 |
| 下游 | legacy-downstream、HexClaw、Hexagon、ai-core 对当前 Toolkit API 的编译与定向合同通过。 |
| Desktop | Ollama 固定归档的 PAX/AppleDouble 处理已完成 RED→GREEN，相关解包与敏感边界测试 50/50 通过，真实固定归档 49 个成员成功解析且未发布元数据伪文件。正式 package-local、verify 与 DMG 挂载结果以最终会话报告为准。 |
| 文档示例 | 中英文 README 的 API 签名与本轮新增注释已按当前合同更新；CHANGELOG 已固化 `v0.3.0` 发布记录。 |

## 4. 发布检查清单

1. 源码冻结后按最终指纹重跑范围、敏感信息、tidy/verify、build、vet、lint、全仓普通/Race、受影响 Sandbox 与 API 门禁。
2. 使用同一冻结源码执行 HexClaw Desktop `make package-local`、`make verify-package-local`，并挂载新 DMG 校验应用树、版本、主程序摘要与未签名策略。
3. Toolkit 改动只做本地 Commit；不 Push、不打 Tag。
4. 用户 Push/Tag 后，由远端 CI 补齐 Windows 原生 28 项 `windows_security`、Tag 触发与发布工作流证据。

Desktop 打包结果不会在构建后回写本文件，避免修改五仓源码清单并使刚生成的制品失去精确源码身份；请以最终会话报告中的 generation、DMG 摘要和验收命令为准。

## 5. 完成标准

Toolkit 本地可发布条件是：最终冻结源码的全仓门禁、macOS/Linux 原生 Sandbox、Windows 交叉门禁、API baseline 与下游兼容全部通过，并完成 Toolkit Commit。HexClaw Desktop 当前源码 DMG 属于后续下游打包验证，不阻塞 Toolkit Tag。Windows 当前提交的原生运行证据必须在用户 Push 后由远端 CI 补齐；在该证据出现前，不把“本地可发布”表述为“三平台远端 CI 已通过”。
