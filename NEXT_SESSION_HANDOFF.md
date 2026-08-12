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
- 新增代码注释使用中文；新增错误、状态提示和提示语使用英文。
- 不调用真实模型或钉钉，不改 HexClaw Desktop UI，不使用旧 DMG 冒充当前源码构建。

## 2. 当前事实（三平台远端 CI 已全绿）

- **远端门禁已通过**：CI（16 个 job 全绿）、Downstream Contract（ai-core + hexagon + examples + hexclaw 全适配）、Sandbox CodeExec（Linux/Windows/macOS）均已成功。
- 修复已全部推送 `main`；`v0.3.0` 的打 Tag 与发布仍由用户自行完成（本仓不打 Tag）。
- 下游仓库（hexagon、hexclaw、ai-core）已按 v0.3.0 API 适配并推送各自 `main`；codeup 侧消费者的合同验证改由消费者仓库的 `scripts/verify-toolkit-upstream.sh` 自验脚本接管。
- Windows 平台根因修复已落地：句柄重开权限回退（`reopenWindowsHandle` 统一按路径 `CreateFile`）、junction/reparse 的内核信息类权威检测、NTFS 创建时间身份判据（fail-closed）、blobstore 原子替换有界重试、行尾归一化（`git checkout-index -f -u -a` + `git update-index --refresh`）。
- darwin 平台修复已落地：homebrew 安装树（含 Cellar）纳入读挂载与可执行放行；CodeExec 的 go 运行时测试仅 Linux 强制执行（untrusted deny process-fork 与 go build 语义冲突）。
- AppLocker 门禁在托管 runner 强制不可用时条件化跳过（与 root Linux bwrap 门禁同语义）；codecov 上传步骤已移除。
- `go.mod` 声明的最低 Go 版本是 `1.25.12`，不是要求所有开发机只能安装该版本。发布门禁使用 Go 1.25.12、`GOWORK=off` 和 `-mod=readonly` 固定最低版本证据。
- API 目标已统一为 `v0.3.0`；`gorelease -base=v0.2.6 -version=v0.3.0` 与当前 34 个不兼容包段的 baseline 校验已通过。源码再次变化后必须按最终指纹重跑。

## 3. 已取得的证据

| 范围 | 证据 |
|---|---|
| 根模块 | 全仓普通测试、Race、build、vet、golangci-lint 均通过（docker 中 Linux 视角 0 issues）。 |
| 远端 CI | CI 16/16 job 成功；Downstream Contract 成功；Sandbox CodeExec 成功（三 workflow 均以当前 main 推送验证）。 |
| macOS Sandbox | `os/sandbox` 关键原生门禁通过；homebrew 运行时（pip3/npm）在沙箱内可执行。 |
| Linux Sandbox | bwrap 可用环境全量通过；root 门禁在 bwrap 不可用 runner 上按环境探测条件化。 |
| Windows | Server 2022 原生门禁通过（junction、DACL/integrity、Job 限制、网络隔离、close 语义等 28 项）；AppLocker 强制不可用时跳过。 |
| 集成与安全 | MySQL 8.4、Redis 7.4 ACL 集成通过；`govulncheck` 无可达或导入漏洞。 |
| 下游 | HexClaw、Hexagon、ai-core 对 v0.3.0 API 的编译与测试门禁通过（Downstream 全绿）。 |
| 文档 | 中英文 README、CHANGELOG（v0.3.0 发布记录含本次平台修复）、COMPATIBILITY 均已同步。 |

## 4. 已知遗留（不阻塞发布）

- `TestCache_Del` 在 race 模式下偶发失败（cache/multi 的时序敏感，失败时已输出层数据诊断）；常规测试与 CI 非 race 路径稳定。
- 本机 macOS 全量测试有环境相关的失败（project 模式 scratch 位于 /tmp 下的 go build cache 初始化、darwin untrusted 下 go build 派生），均不在 CI 门禁 required 列表内。
- Windows runner 无 AppLocker 强制执行能力，对应门禁按环境探测跳过（策略应用与探测逻辑保留）。

## 5. 发布检查清单

1. 源码冻结后按最终指纹重跑范围、敏感信息、tidy/verify、build、vet、lint、全仓普通/Race、受影响 Sandbox 与 API 门禁。
2. 使用同一冻结源码执行 HexClaw Desktop `make package-local`、`make verify-package-local`，并挂载新 DMG 校验应用树、版本、主程序摘要与未签名策略。
3. 远端 CI（CI + Downstream Contract + Sandbox CodeExec）已以当前 main 全部通过；用户打 `v0.3.0` Tag 后由 Tag 触发发布工作流收口。

Desktop 打包结果不会在构建后回写本文件，避免修改五仓源码清单并使刚生成的制品失去精确源码身份；请以最终会话报告中的 generation、DMG 摘要和验收命令为准。

## 6. 完成标准

当前 main 已满足发布前置：三平台远端 CI、Downstream Contract、Sandbox CodeExec 全部通过，API baseline 与下游兼容已验证，文档同步完成。剩余动作仅为用户打 `v0.3.0` Tag 并触发发布工作流。
