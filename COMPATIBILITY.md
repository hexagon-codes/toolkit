# 兼容性与稳定性策略 — toolkit

toolkit 是 Hexagon 生态的**共享底座**，被多个独立产品依赖（toolkit ← ai-core/hexagon/hexclaw；ai-core ← hexagon/hexclaw）。底座的接口稳定性直接决定上游能否安心 pin 版本、避免 lockstep。

## SemVer 承诺
- 遵循 [SemVer](https://semver.org/lang/zh-CN/)。**导出标识符（公开 API）**是兼容性契约。
- `v0.x` 阶段的 patch 不得破坏导出 API；minor 可以包含破坏式变更，但必须在 CHANGELOG 显著标注 `BREAKING` 并给出完整迁移清单。
- `v1.0.0` 以后，minor 与 patch 只能兼容演进；破坏式变更只能进入 major。
- 内部包（`internal/`）、未导出标识符、`examples/` 不在契约内。

## 自动门禁
1. **API 兼容性检测**：`.github/workflows/api-compat.yml` 用 `gorelease` 对照上一 tag 检测破坏式变更，提示版本号应如何升。
2. **下游接缝契约**：`.github/workflows/downstream.yml` 在 go.work 下用本仓改动跑全部直接消费者的 build+test —— 下游绿才算接口未破。

## 弃用流程
- `v1.0.0` 以后，弃用先标 `// Deprecated: 用 X 替代。将在 vN 移除。`，至少保留一个 minor 周期并记录到 CHANGELOG，到期后只能在下一个 major 移除。
- `v0.x` 的破坏式 minor 若明确采用一次性迁移，可以直接删除旧 API；不得以兼容别名、双实现或隐藏回退延长两套合同。此类删除必须逐项标注 `BREAKING`。

## 升级建议（给上游 hexagon / hexclaw）
- pin 明确版本；`v0.x` 的 patch 可直接升级，minor 必须先检查 `BREAKING` 清单并完成迁移。
- 下游门禁用于证明当前源码组合能够协同构建和测试，不替代调用方对破坏式 minor 的升级评估。
