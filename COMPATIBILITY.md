# 兼容性与稳定性策略 — toolkit

toolkit 是 Hexagon 生态的**共享底座**，被多个独立产品依赖（toolkit ← ai-core/hexagon/hexclaw；ai-core ← hexagon/hexclaw）。底座的接口稳定性直接决定上游能否安心 pin 版本、避免 lockstep。

## SemVer 承诺
- 遵循 [SemVer](https://semver.org/lang/zh-CN/)。**导出标识符（公开 API）**是兼容性契约。
- 本项目承诺 `v0.x` 阶段的 patch 不得破坏导出 API；minor 可以包含破坏式变更，但必须在 CHANGELOG 显著标注 `BREAKING` 并给出完整迁移清单。这是项目额外约定，SemVer 本身将 `v0.x` 视为初始开发阶段。
- `v1.0.0` 以后，兼容的导出 API 新增至少升 minor；patch 仅用于兼容修复，破坏式变更只能进入 major。
- 内部包（`internal/`）、未导出标识符、`examples/` 不在契约内。

## 验证职责

1. **发布前核心验证**：PR、main 自动运行 CI 与 API Compatibility。三平台 lint、普通全量及 race、最低 Go、tidy、govulncheck、隔离集成和原生 sandbox 合同验证 toolkit 自身行为；根模块设置 `GOWORK=off`。前向工具链探针解析实际版本，与最低版本相同时复用既有验证。
2. **API 与版本核对**：`.github/workflows/api-compat.yml` 用 `gorelease` 对照最新稳定 tag 比较导出 API，保留批准 breaking 清单与 `--allow-empty`。普通 patch 不传 `-version`；跨 major 可通过手动输入 `release_version` 指定尚未发布的候选版本，自动检查遇到新的 `/vN` 模块路径时可推导 `vN.0.0` 做显式校验。成功记录包含 `release_version`，`scripts/release.py` 要求跨 major 的记录版本与实际候选版本一致，`--prepare` 自动传入目标版本。发布入口复用同 SHA 的成功检查及 API 分类，按上述版本承诺判断候选版本；批准 breaking 清单不豁免版本规则。
3. **手动消费者验证**：`.github/workflows/downstream.yml` 在专用 `go.work` 下对固定消费者执行 build 和 `go test -run '^$'`，只证明编译契约；运行行为由各下游 CI 负责。Sandbox CodeExec 保留三平台手动行为验证，默认固定消费者及受控 HTTP，公网检查显式开启。两者均不作为 toolkit 常规发布前置条件。
4. **发布后交付核验**：`.github/workflows/release-verify.yml` 在 tag 推送后核对 tag 的提交 SHA、实际模块包、go.mod 和校验和；也可手动核验历史版本。不再次运行 gorelease 的版本建议或全量源码 CI。

发布入口先识别已发布 tag，再判断新候选资格；历史版本核验不受新版本或过期 CI 产物影响。本地同 SHA tag 可续推，远端同 SHA tag 可重复核验，不同 SHA 拒绝，已发布 tag 不删除或移动。新候选的验证产物缺失或过期时，`--prepare` 可为当前 main 触发新验证；旧候选不能借用其他 SHA 的结果。

目标提交或 tag 的 `go.mod` 决定隔离目录中的工具链选择，使用 `GOWORK=off` 并尊重调用方的 `GOTOOLCHAIN=auto/local`。创建新 tag 前确认目标工具链可用；核验历史版本不依赖当前工作区的 Go 要求。模块下载失败保留 JSON `Error` 或 stderr，不以通用退出码替代真实失败原因。

## 弃用流程
- `v1.0.0` 以后，弃用先标 `// Deprecated: 用 X 替代。将在 vN 移除。`，至少保留一个 minor 周期并记录到 CHANGELOG，到期后只能在下一个 major 移除。
- `v0.x` 的破坏式 minor 若明确采用一次性迁移，可以直接删除旧 API；不得以兼容别名、双实现或隐藏回退延长两套合同。此类删除必须逐项标注 `BREAKING`。

## 升级建议（给上游 hexagon / hexclaw）
- 当前本地交付目标为 **v0.3.7**，新增 `reflectx.DeepCopyChecked`，保留既有 `DeepCopy`，最低 Go 版本仍为 **1.26.10**。版本说明不表示 tag 已创建、推送或已可由公共代理解析。
- pin 明确版本；升级前检查 `BREAKING` 清单并完成必要迁移。v0.3.6 将最低 Go 要求提升至 1.26.10，旧工具链不能继续构建。
- 手动下游编译契约用于证明所配置源码组合能够编译，不证明运行时行为，也不替代调用方对破坏式 minor 的升级评估。
