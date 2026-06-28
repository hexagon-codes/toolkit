# Changelog

本文件记录 toolkit 的用户可见变更，遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本遵循 [SemVer](https://semver.org/lang/zh-CN/)。

## [Unreleased]

## [0.2.3] - 2026-06-28
向后兼容的 PATCH 版本（新增 `lang/stringx` 按字节封顶截断能力，无导出 API 破坏）。

### Added
- `lang/stringx`：新增 `TruncateBytes(s, maxBytes, suffix)`，按【字节预算】截断字符串并回退到完整 rune 边界后再附加后缀（后缀不计入预算）。截断点落在多字节 UTF-8 字符中间时向前回退到边界，绝不劈裂多字节字符产生乱码。适用于工具 stdout/stderr 上限、知识库分块、文档标题等「按字节封顶」场景，替代裸 `s[:n]`。与按 rune 数截断、后缀计入预算的 `Truncate`/`TruncateWithSuffix` 语义互补。

## [0.2.2] - 2026-06-28
向后兼容的 PATCH 版本（新增 `os/sandbox` 只读授权能力，无导出 API 破坏）。

### Added
- `os/sandbox`：新增 `Config.ReadablePaths`，在 `Workspace` 之外额外授予「只读」访问的宿主路径（用于用户经数据连接器等显式授权的本地目录，让沙箱内 `code_exec` 能读到）。仅授读不授写；darwin seatbelt profile 为每个授权路径追加 `file-read*` 放行，并对路径做安全校验（须为绝对路径、不含会破坏或注入 SBPL 字面量的字符），非法路径跳过、不污染整张 profile。

## [0.2.1] - 2026-06-22
向后兼容的 PATCH 版本（修正默认行为，无导出 API 变更）。

### Fixed
- `net/httpx`：`RawClient` 默认 transport 现设置 `Proxy: http.ProxyFromEnvironment`，与 `net/http.DefaultTransport` 一致地遵循 `HTTP(S)_PROXY`/`NO_PROXY` 环境变量。此前基于 RawClient 的客户端在以代理上网的宿主机上会绕过代理，导致无法访问外网。

## [0.2.0] - 2026-06-21
含破坏性变更的 MINOR 版本（SemVer 0.x：BREAKING 提升 MINOR）。`crypto/sign` 签名 wire 格式变更，下游（含 ai-core）升级前需评估签名跨版本兼容性。

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
