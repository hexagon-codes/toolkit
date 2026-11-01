# 贡献指南 — toolkit

toolkit 是 Hexagon 生态的 **L0 通用工具库**（零 AI 依赖：语言增强、加密、网络、缓存、并发、集合、OS 沙箱、blobstore）。被 ai-core/hexagon/hexclaw 广泛依赖，故对质量与零外部依赖要求最高。

## 分层铁律
- toolkit 是最底层，**不得依赖** ai-core / hexagon / hexclaw 或任何业务/AI 包。
- 新增能力必须是**通用、零业务语义**的；带 AI 语义的请放 ai-core。
- 尽量零第三方依赖；引入新依赖需在 PR 说明理由（尤其避免拉入 cgo / 重型传递依赖）。

## 本地开发

macOS CI 固定使用 `macos-15`，Linux 原生沙箱使用 `ubuntu-22.04`，Windows 使用 `windows-2022`。Action 使用完整 SHA 固定，运行时与工具升级作为独立维护批次。

最低 Go 版本以根 `go.mod` 为唯一来源，`examples/go.mod` 同步维护。根模块的本地命令与核心 CI 使用 `GOWORK=off`，避免受到父目录工作区影响；跨仓验证另建专用 `go.work`。三平台 lint、普通全量与 race、前向工具链探针、tidy、govulncheck、API breaking、隔离 MySQL/Redis 和原生 sandbox 合同继续保留。工具链别名先解析实际 Go 版本；与最低版本相同时复用 Linux 最低版本验证，实际不同的版本仍执行兼容性探针。

macOS 普通测试输出 JSON 后直接核验必需行为，不再次运行同一组用例；Python 由 `setup-python` 安装到受支持运行时路径，必须实际执行对应 pip 场景。普通、race 与 Windows 专项测试事件落盘，失败时连同服务及探针日志归档，再清理环境。

Windows Job 内存门禁使用已有 `gcc` 编译原生 Win32 载荷，保留 512 MiB 对照、160 MiB 总限额及两个各 96 MiB 分配的真实错误验证。Linux root/non-root 独立探测，只绑定探针及必要加载依赖；仅明确的 namespace、UID/GID 映射环境限制可以说明原因后跳过。载荷、依赖、路径及挂载错误仍失败，已声明可用后的行为失败仍阻断。

工作流检查统一使用 `bash scripts/check-workflows.sh`，本地与 CI 都须安装 ShellCheck；入口固定 actionlint 版本，不因缺少 ShellCheck 静默省略 shell 检查。

## 验证与发布

PR、main 的自动验证由核心 CI 和 API Compatibility 负责。API 检查以最新稳定已发布 tag 为 base，保留批准 breaking 清单和 `--allow-empty`，归档 API 变更分类与比较输入。发布入口将分类与候选版本一起核对：本项目约定 `v0.x` patch 不得破坏导出 API；`v1` 以后兼容 API 新增至少升 minor，破坏式变更升 major。`v0.x` 的 patch 约定是项目兼容承诺，详见 [COMPATIBILITY.md](COMPATIBILITY.md)。

API 报告中的 base/release 包加载错误会在基线比对前阻断，不生成成功分类或验证凭据。

普通 patch 的 API 比较使用 `gorelease -base`，不传 `-version`。跨 major 发布可通过 API 工作流的手动输入 `release_version` 指定尚未发布的候选版本；自动检查遇到新的 `/vN` 模块路径时，可推导 `vN.0.0` 做显式版本校验。成功记录包含 `release_version`，跨 major 的发布资格要求该值与实际候选版本一致；`--prepare` 自动传入目标版本，已有 tag 的模块核验仍使用发布后入口。

Downstream Contract 和 Sandbox CodeExec 仅手动触发，不参与 toolkit 发布资格。消费者基线集中在 `.github/consumer-baselines.json`，只在兼容合同变化时更新，不随普通版本刷新。下游契约仅执行 build 和测试编译；CodeExec 保留三平台行为验证，调用消费者 `scripts/verify-code-exec.py`，toolkit 核验稳定行为 ID、平台及唯一通过结果。CodeExec 默认使用固定消费者和受控 HTTP，公网检查需要显式开启。

发布前复用同一候选提交的核心 CI 与 API 检查；tag 后只运行 Verify Published Module，核对 tag、提交 SHA、实际模块下载及校验和，不重复全量 CI 或 gorelease 版本建议。

| 发布入口 | 行为 |
|---|---|
| `python3 scripts/release.py vX.Y.Z` | 未发布版本只核对资格；已发布版本直接核验其 tag 和模块，不依赖最新版本或历史 CI 产物 |
| `--candidate COMMIT` | 显式指定预期提交；省略时依次使用现有远端 tag、本地 tag 或 HEAD |
| `--prepare` | 为当前 main 候选重新触发失败、缺失或证据过期的核心检查，跨 major 时向 API 检查传入目标版本；运行结束后再次核对资格 |
| `--publish` | 资格齐备并确认目标工具链可用后创建并推送 tag；本地已有同 SHA tag 时续推，远端已有同 SHA tag 时核验；不同 SHA 一律拒绝 |
| `--verify-published` | 只核验已有发布版本，适用于当前版本及历史版本 |

资格核验要求 workflow 事件、检出及候选 SHA 一致，API base、批准清单摘要和工具版本一致。旧候选缺少有效证据时须选择当前 main 重新验证，不能借用新提交的结果。已发布 tag 不删除或移动；main 不设置分支保护。运行结果作为产物归档，不向源码提交结果账本。

发布入口按目标提交或 tag 的 `go.mod` 在隔离目录选择工具链，设置 `GOWORK=off` 并尊重调用方的 `GOTOOLCHAIN`：`auto` 可自动选择目标要求的版本，`local` 不足时明确失败。新版本在创建 tag 前确认工具链可用；历史版本不受当前工作区 `go.mod` 的 Go 要求影响。模块下载非零退出时仍显示 JSON 中的 `Error`；非 JSON 失败保留 stderr，便于直接定位工具链、网络或下载错误。

```bash
export GOWORK=off
go build ./...
go vet ./...
go test -race ./...
golangci-lint run        # 配置见 .golangci.yml
govulncheck ./...        # 漏洞扫描
bash scripts/check-workflows.sh
```

## 提交规范
- Conventional Commits：`feat(net/sse): ...` / `fix(cache/redis): ...` / `chore: ...`
- 注释中文、只写功能描述，禁暴露内部开发文档/对标框架/调研出处。
- 每个 PR 必须：build+vet+test 全绿、golangci-lint 0 issue、受影响公共行为有必要的回归证据。

## PR Checklist
- [ ] `go test -race ./...` 全绿
- [ ] `golangci-lint run` 0 issue
- [ ] 公开 API 有 GoDoc 注释（`// Xxx ...`）
- [ ] 不引入对上层/AI 包的依赖
- [ ] CHANGELOG.md 记录用户可见变更
