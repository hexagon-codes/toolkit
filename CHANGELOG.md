# Changelog

本文件记录 toolkit 的用户可见变更，遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本遵循 [SemVer](https://semver.org/lang/zh-CN/)。

## [Unreleased]

### Changed
- **BREAKING** `infra/otel`：公开类型统一为 `Tracer`、`Config`、`Option`、`Span`，构造函数统一为 `NewTracer`/`DefaultConfig`；`Tracer.SetExporter(ctx, exporter) error` 在调用时接管导出器所有权，替换时关闭旧导出器并返回关闭错误。
- **BREAKING** `infra/prometheus`：指标类型统一为 `Counter`、`Gauge`、`Histogram`、`Summary`，移除 `Prometheus*` 类型名。
- **BREAKING** `util/retry`：重试条件选项统一为 `If`、`IfHTTP`、`IfHTTPOrNetwork`，移除旧构造函数名。
- **BREAKING** `infra/db/mysql`：`QueryWithTimeout` 与 `QueryRowWithTimeout` 显式返回 `context.CancelFunc`，调用方必须在消费完 `Rows`/`Row` 后取消上下文；移除旧 `Ex` 变体。
- **BREAKING** `net/ip`：可能触发网络访问的函数改为 context-first API。
- **BREAKING** `util/poolx`：全局提交函数返回调度错误，`SubmitFuncCtx` 改为 context-first；`Parallel` 只等待成功提交的任务。
- **BREAKING** `lang/errorx`：`MultiError.Append`/`AppendResult` 不再返回接收者，错误聚合改为有界保留并拒绝循环引用；并行执行统一返回 `error`、按输入顺序聚合并限制默认并发；`SafeGo(func()) <-chan error` 返回唯一一次完成结果，`Result.UnwrapOrElse(func(error) T) (T, error)` 对 nil 回调返回可诊断错误。`lang/syncx.OnceErr.Value` 返回顺序改为 `(value, initialized, error)`，数据库单例 `Reset` 开始返回关闭错误。
- **BREAKING** `lang/contextx`：`Run(ctx, func(context.Context) error)` 与 `RunTimeout(parent, timeout, func(context.Context) error)` 改为同步协作取消；`NewPool(ctx, size) (*Pool, error)` 校验 nil context 与非正数容量。
- **BREAKING** `cache/multi`：`NewCache(layers, opts...) (*Cache, error)` 与 `Builder.Build() (*Cache, error)` 统一校验空层、nil 层及无效选项，不再通过 panic 或无效实例表达配置错误。
- **BREAKING** `net/httpx`：`NewHostPool(HostPoolConfig) (*HostPool, error)` 要求显式提供单主机池配置与 `MaxHosts` 容量上限；`RemoveHost` 同时关闭目标池并释放配置和实例容量。
- **BREAKING** `infra/queue/asynq`：取消状态统一采用 `StateCanceled`/`CANCELED`，不再接受旧拼写。
- **BREAKING** `event`：`New`、`Subscribe`、`SubscribeAll`、`Publish` 与 `PublishSync` 显式返回配置或生命周期错误；`Close` 仅启动非阻塞关闭流程，`Shutdown(ctx)` 负责等待活跃处理器完成；panic 回调自身的 panic 会被隔离。
- **BREAKING** `util/file`：默认文件权限收紧为 `0600`、目录权限收紧为 `0750`；写入与复制改为同目录临时文件、文件同步、原子替换和父目录同步协议，空删除路径与 nil 遍历回调会返回错误。

### Fixed
- 补齐数据库、HTTP/SSE、沙箱代理、文件复制和可观测性链路中的资源关闭与错误传播，避免关闭错误或异步导出错误被静默丢弃。
- OTLP 批量导出失败时重新入队未发送 span，并在 `Shutdown` 汇总异步导出错误。
- MySQL 超时查询不再在调用方读取结果前提前取消派生上下文。

### Tests
- 新增 MySQL 8.4 与 Redis 7.4 具名 ACL 的隔离容器集成测试脚本，并纳入 CI。
- CI 全量 lint 门禁改为检查整个仓库。

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
- `util/retry`：补充最终错误链与重试回调计数控制能力。
- 工程治理：CI（build/vet/race/lint/govulncheck）、`.golangci.yml`、`CONTRIBUTING.md`、`COMPATIBILITY.md`。

### Fixed
- `infra/otel`：修复 W3C `traceparent` 解析，避免使用 `fmt.Sscanf` 贪婪匹配导致 trace 链路断开。

## [0.0.6]
- 基线版本（lang / crypto / net / cache / util / collection / infra 等通用能力）。
