# 贡献指南 — toolkit

toolkit 是 Hexagon 生态的 **L0 通用工具库**（零 AI 依赖：语言增强、加密、网络、缓存、并发、集合、OS 沙箱、blobstore）。被 ai-core/hexagon/hexclaw 广泛依赖，故对质量与零外部依赖要求最高。

## 分层铁律
- toolkit 是最底层，**不得依赖** ai-core / hexagon / hexclaw 或任何业务/AI 包。
- 新增能力必须是**通用、零业务语义**的；带 AI 语义的请放 ai-core。
- 尽量零第三方依赖；引入新依赖需在 PR 说明理由（尤其避免拉入 cgo / 重型传递依赖）。

## 本地开发

macOS CI 固定使用 `macos-15`。`macos-14` runner 已退役；迁移保留现有任务、矩阵与门禁，不使用浮动 `macos-latest` 替代版本管理。

Windows Job 内存门禁使用 CI runner 已有的 `gcc` 编译原生 Win32 载荷，避免把 Go/race 运行时计入内存预算。本地执行该门禁也需要 Windows C 编译器；测试保留 512 MiB 宽松对照和 160 MiB 总限额，要求两个各 96 MiB 分配中的第二个返回真实 Windows 分配错误。

最低 Go 版本以根 `go.mod` 为准，`examples/go.mod` 同步维护。CI 的固定工具链通过 `setup-go` 的 `go-version-file` 读取模块声明；升级最低版本时不在各工作流重复写入补丁版本号。版本线前向兼容探针和 `stable` 探针继续保留；最低 Go 版本升级须在 CHANGELOG 中说明兼容性影响。

Sandbox CodeExec 跨仓门禁按 `TestCodeExecSkill_Execute_PythonCrawler` 行为组发现爬虫用例；该组必须存在，发现的每个用例均须执行且通过，不允许跳过。后缀可随下游行为细化调整。自动门禁使用受控 HTTP 服务验证断网与联网行为；`PythonCrawlerPublicNetwork` 公网检查仅由现有手动输入开启。

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
- 每个 PR 必须：build+vet+test 全绿、golangci-lint 0 issue、新增/改动代码有单测、覆盖率不下降。

## PR Checklist
- [ ] `go test -race ./...` 全绿
- [ ] `golangci-lint run` 0 issue
- [ ] 公开 API 有 GoDoc 注释（`// Xxx ...`）
- [ ] 不引入对上层/AI 包的依赖
- [ ] CHANGELOG.md 记录用户可见变更
