# Changelog

本文件记录 toolkit 的用户可见变更，遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本遵循 [SemVer](https://semver.org/lang/zh-CN/)。

## [Unreleased]

### Changed
- **BREAKING** `crypto/sign`：`APISigner.Sign` 改为长度前缀规范化编码（netstring 风格），消除「参数 + timestamp + nonce」无分隔符直接拼接导致的签名串碰撞（如 `{a:"1"}, ts=23` 与 `{a:"12"}, ts=3` 旧实现产生相同串）。**签名 wire 格式变更，与 ≤v0.1.0 不互通**——旧版本生成的签名无法通过新版 `Verify`，跨版本部署需同步升级或在灰度期双验。

### Fixed
- `lang/contextx`：`Pool.Wait()` 现返回任务错误（首个错误，多个时合并）；此前仅返回 `ctx.Err()`，导致 `Go()` 中任务返回的 error 被静默吞掉。**行为变更**：依赖「任务失败时 `Wait()` 仍返回 nil」的调用方需复核。
- `lang/conv`：修复 `Int64`/`TryInt64`/`Uint64` 对 float 输入的溢出边界判断——`math.MaxInt64` 转 `float64` 会向上取整为 2^63 导致边界漏判（恰为 2^63 时旧实现回绕成 `MinInt64`），改用可精确表示的 2^63 / 2^64 边界常量。
- `util/rate`：`LeakyBucket` 速率 `<=0` 时不再除零 panic（改为不限流放行）；`SlidingWindow.Record` 在小容量（< 50）下不再因 `len > cap` panic。

## [0.1.0] - 2026-06-19
向后兼容的 MINOR 版本（仅新增 API，不破坏 v0.0.x 导出契约），被 ai-core v0.1.4 依赖。

### Added
- `blobstore`：抽象 `Blobstore` 接口 + 流式 `SaveStream`/`OpenReader` + `ObjectBackend`(S3/R2) seam + TTL。
- `util/lease`：分布式互斥租约 `Lease` 接口 + `FencingToken` + 进程内 `MemoryLease`。
- `cache/local`：裸 `Get`/`Set` API、无后台清理构造 `NewCacheNoCleanup`、`Close` 别名与确定性 LRU 淘汰。
- `net/sse`：Reader 选项（`WithMaxTotalBytes` 累计字节上限、`WithStrictDataPrefix` 严格 data 前缀、`WithDoneFunc` provider 无关 done 谓词）+ `ReadUntilDone`/`Each` 消费 API。
- `net/ssrf`：URL 级 SSRF 校验（`ValidateURL`/`ValidateLocalURL`）；`net/ip.IsPrivateOrReservedIP` 补齐私有/保留地址判断。
- `os/sandbox`：跨平台命令沙箱、网络策略与代理能力。
- `util/rand`：返回 error 的 `Try*` 随机数安全变体（`TryToken`/`TryString` 等）。
- `util/retry`：最终错误可解包 `WithUnwrapFinalError`（别名 `WithReturnLastError`）与 `OnRetry` 零基计数兼容选项 `WithOnRetryZeroBased`。
- 工程治理：CI（build/vet/race/lint/govulncheck）、`.golangci.yml`、`CONTRIBUTING.md`、`COMPATIBILITY.md`。

### Fixed
- `infra/otel`：修复 W3C `traceparent` 解析，避免使用 `fmt.Sscanf` 贪婪匹配导致 trace 链路断开。

## [0.0.6]
- 基线版本（lang / crypto / net / cache / util / collection / infra 等通用能力）。
