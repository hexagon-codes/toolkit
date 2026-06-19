# 贡献指南 — toolkit

toolkit 是 Hexagon 生态的 **L0 通用工具库**（零 AI 依赖：语言增强、加密、网络、缓存、并发、集合、OS 沙箱、blobstore）。被 ai-core/hexagon/hexclaw 广泛依赖，故对质量与零外部依赖要求最高。

## 分层铁律
- toolkit 是最底层，**不得依赖** ai-core / hexagon / hexclaw 或任何业务/AI 包。
- 新增能力必须是**通用、零业务语义**的；带 AI 语义的请放 ai-core。
- 尽量零第三方依赖；引入新依赖需在 PR 说明理由（尤其避免拉入 cgo / 重型传递依赖）。

## 本地开发
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
