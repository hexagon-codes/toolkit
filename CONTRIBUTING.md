# 贡献指南 — toolkit

toolkit 是 Hexagon 生态的 **L0 通用工具库**（零 AI 依赖：语言增强、加密、网络、缓存、并发、集合、OS 沙箱、blobstore）。被 ai-core/hexagon/hexclaw 广泛依赖，故对质量与零外部依赖要求最高。

## 分层铁律
- toolkit 是最底层，**不得依赖** ai-core / hexagon / hexclaw 或任何业务/AI 包。
- 新增能力必须是**通用、零业务语义**的；带 AI 语义的请放 ai-core。
- 尽量零第三方依赖；引入新依赖需在 PR 说明理由（尤其避免拉入 cgo / 重型传递依赖）。

## 本地开发

macOS CI 固定使用 `macos-15`，Linux 原生沙箱使用 `ubuntu-22.04`，Windows 使用 `windows-2022`。Action 固定为经过验证的 Node 24 版本及完整 SHA，工具升级作为独立维护批次。

最低 Go 版本以根 `go.mod` 为唯一来源，`examples/go.mod` 同步维护。三平台 lint、普通全量与 race、前向工具链探针、tidy、govulncheck、API breaking、隔离 MySQL/Redis 和原生 sandbox 合同继续保留。macOS 普通测试输出 JSON 后直接核验必需行为，不再次运行同一组用例；Python 由 `setup-python` 安装到受支持运行时路径，必须实际执行对应 pip 场景。

Windows Job 内存门禁使用已有 `gcc` 编译原生 Win32 载荷，保留 512 MiB 对照、160 MiB 总限额及两个各 96 MiB 分配的真实错误验证。Linux root/non-root 独立探测：只绑定探针及必要加载依赖；环境能力不足时打印原因，已声明可用后的失败仍阻断。

消费者基线集中在 `.github/consumer-baselines.json`，只在兼容合同变化时更新，不随普通版本刷新。下游契约仅执行 build 和测试编译。CodeExec 调用消费者 `scripts/verify-code-exec.py`，toolkit 核验稳定行为 ID、平台及唯一通过结果；自动运行使用受控 HTTP，公网验证保留手动入口。

API 检查覆盖 PR、main 和 tag，以最新稳定已发布 tag 为 base（tag 检查排除自身），保留批准 breaking 清单和 `--allow-empty`。比较输入与实际消费者 SHA 自动归档，失败先保存测试及服务日志再清理，不向源码提交运行结果账本。

发布入口：`python3 scripts/release.py vX.Y.Z` 只核对资格；`--prepare` 重跑失败检查或为当前 main 候选触发缺失检查；`--publish` 在资格齐备后创建并推送不可变 tag。入口核对事件、检出及候选 SHA、固定消费者、API base 与工具版本。旧候选仅复用或重跑其已有结果，缺失证据不借用新 main 的结果。主 CI、API、Downstream 支持手动补验；CodeExec 补验自动选择固定消费者并关闭公网检查。main 不设置分支保护，提交和推送仍按维护者授权执行。

```bash
go build ./...
go vet ./...
go test -race ./...
golangci-lint run        # 配置见 .golangci.yml
govulncheck ./...        # 漏洞扫描
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
