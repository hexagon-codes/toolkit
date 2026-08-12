# Changelog

本文件记录 toolkit 的用户可见变更，遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本遵循 [SemVer](https://semver.org/lang/zh-CN/)。

## [0.3.0] - 2026-08-12

### Changed
- **BREAKING** 构建基线：`go.mod` 的最低 Go 版本由 v0.2.6 的 1.25.7 提升至 1.25.12；仓库未声明 `toolchain`，因此这是最低编译版本要求，不是对本机 Go 可执行文件的额外固定。
- **BREAKING** `os/sandbox`：`Sandbox.Exec` 由旧三参数形式改为 `Exec(ctx, Command)`，不保留旧 facade；`Command.Path`、`Args`、`Dir` 与 `Env` 分别表达可执行文件、参数、工作目录和完整环境。`Env == nil` 使用平台最小安全环境，非 nil 环境不会增量继承宿主变量。
- **BREAKING** `os/sandbox`：移除 `Sandbox.ExecCode` 以及平台后端内建的 Python、JavaScript、Go 语言分派和临时源码文件管理；调用方负责准备语言产物，并统一通过结构化 `Exec` 提交可审计命令。Toolkit 不再把 `go run` 等多进程构建编排混入跨平台隔离层。
- **BREAKING** `os/sandbox`：`Sandbox` 新增 `Close() error` 生命周期合同；关闭会先拒绝新操作、等待已经进入的操作收敛，再确定性释放平台资源，并保留后端关闭 panic 或多重错误的完整诊断链。
- **BREAKING** `os/sandbox`：`Config.Network` 由内置 `bool` 改为 `NetworkMode`，仅保留 `NetworkDisabled` 与 `NetworkHost` 两种精确语义；移除 `Config.DenyLoopback`、`NetPolicy`、`NetProxy`、`NetProxyConfig`、`NewDefaultPolicy`、`NewNetProxy`、`ProxyEnvVars` 以及 Darwin/Linux/Windows proxy integration facade。Windows 同时移除旧 `SandboxMode`、字符串型 `NetworkMode`、`WindowsSandboxPolicy`、`DefaultWindowsPolicy` 和公开 Win32 ACL/Token/Job 常量族。Windows 后端现在会在 `New` 阶段拒绝 `NetworkHost` 和任何非空 `ReadablePaths`；v0.2.6 中使用 `Network: true` 或 `ReadablePaths` 的 Windows 配置升级后会在运行时构造失败，调用方必须改用 `NetworkDisabled` 并清空 `ReadablePaths`，需要宿主网络或只读宿主路径映射的负载应改由 macOS/Linux 后端承载。
- **BREAKING** `os/sandbox`：移除 `LimitStatusWeak`，限制状态统一为 `not_requested`/`enforced`/`unsupported`，明确区分未请求配额、已真实执行和后端不支持；新增独立的 `LimitReport.ProcessContainment`，并将仅有执行前后 walk 检查的已请求 `Storage` 如实报告为 `unsupported`。
- **BREAKING** `os/sandbox`：新增 `CapabilityProcessContainment` 与 `LimitReport.ProcessContainment`；macOS no-fork、Linux PID namespace 与 Windows Job Object 证明同一不可逃逸不变量，只有确认全部退出后才能报告 `enforced`。
- **BREAKING** `os/sandbox`：`Config` 新增必填的 `RequiredCapabilities` 执行前安全合同，`RequiredCapabilities == 0` 会在工作区创建前被拒绝；新增 `UntrustedCodeIsolationCapabilities`，它只保证 filesystem、network、process-containment 与 output 隔离。Memory、Processes、Storage 限额的零值表示未请求且不设置上限，正值必须额外声明对应能力；`New` 在产生文件系统副作用前完成纯配置语义校验。
- **BREAKING** `os/sandbox`：新增只供固定可信构建工具使用的 `TrustedBuildIsolationCapabilities` 与 `CapabilityProcessCreation`。macOS 选择该合同后允许工具链派生子进程，同时明确将 `ProcessContainment` 报告为 `unsupported`；构建沙箱不得执行不可信产物，产物必须在关闭构建沙箱并完成校验后交给新的严格沙箱执行。
- **BREAKING** `blobstore`：`Blobstore` 接口和本地 `Store` 新增 `Close() error` 生命周期合同；`Store` 不再可比较。TTL 元数据文件由 `<blob>.ttl` 改为 `<blob>.blobstore.ttl`，旧 sidecar 不会被读取或自动迁移；升级前必须重命名或重新生成现有 TTL 元数据，否则对应 blob 将被视为未设置 TTL。`NewStore` 现在拒绝空路径和文件系统根目录，将存储根目录规范化并收紧为 `0700`；扩展名仅接受 1–16 位 ASCII 字母或数字并统一为小写。文件访问改用 `os.Root` 防止符号链接逃逸，blob 文件使用 `0600`，TTL 更新与清理通过跨进程文件锁线性化；`SaveFromURL` 对超过 200 MiB 的响应返回错误，不再静默截断后落盘。
- **BREAKING** `util/hash`：移除弱摘要公共 API `MD5`、`MD5Bytes`、`SHA1` 与 `SHA1Bytes`；内容摘要统一使用 `SHA256`/`SHA256Bytes` 或 `SHA512`/`SHA512Bytes`。
- **BREAKING** `crypto/aes`：移除 CBC/CTR 公共 API `EncryptCBC`、`DecryptCBC`、`EncryptCBCString`、`DecryptCBCString`、`EncryptCTR`、`DecryptCTR` 及对应 `ErrInvalidBlockSize`、`ErrInvalidPadding`，加密调用统一迁移到 AEAD GCM。
- **BREAKING** `crypto/rsa`：移除 PKCS#1 v1.5 加解密及签名 API `EncryptPKCS1v15*`、`DecryptPKCS1v15*`、`SignPKCS1v15`、`VerifyPKCS1v15`，分别迁移到 OAEP 和 PSS；`KeyPair.PrivateKeyToPEM`、`PublicKeyToPEM` 改为返回 `(string, error)`。
- **BREAKING** `util/circuit`：`New`、`NewAIBreaker` 开始返回构造错误；`NewBreakerManager` 不再接受 `func() *Breaker` 工厂回调，改为 `NewBreakerManager(opts ...Option) (*BreakerManager, error)`。`OpenAIConfig`、`ClaudeConfig`、`GeminiConfig`、`AggressiveConfig`、`ConservativeConfig` 由共享变量改为返回独立选项切片的函数。移除 `Breaker.Allow/Success/Failure`，每次调用必须使用同一次 `Acquire() (*Permit, error)` 获得的许可并且只调用一次 `Permit.Complete(error) error`；`Reset`、`OnStateChange` 及 manager 的 `Get`/`Reset`/`ResetAll` 开始返回错误。
- **BREAKING** `util/retry`：重试条件入口统一为 `If`、`IfHTTP` 与 `IfHTTPOrNetwork`，移除 `RetryIf`、`RetryIfHTTP`、`RetryIfHTTPOrNetwork`；所有 AI/网络/数据库预设策略由共享变量改为函数。移除 `ExponentialBackoffWithJitter`、`LinearBackoffWithJitter`、`WithOnRetryZeroBased`、`WithReturnLastError` 与 `WithUnwrapFinalError`；`OnRetry` 固定公开一基尝试次数，重试耗尽错误固定同时包装 `ErrMaxAttemptsReached` 与最终业务错误。
- **BREAKING** `util/rate`：`NewTokenBucket`、`NewLeakyBucket`、`NewSlidingWindow`、`NewTokenBucketV2`、`NewTokenRateLimiter`、`NewMultiDimensionLimiter` 全部改为返回 `(*T, error)`。`TokenBucketV2` 移除 `ConsumeN`、`Reserve`、`ReserveN`、`TryAllowN`，`TokenRateLimiter` 移除 `ConsumeN`、`Reserve`、`TryAllowN`，统一使用 `AllowN`/`WaitN`。
- **BREAKING** `net/sse`：`ReaderOption`、`ClientOption` 改为返回错误；`NewReader`、`NewReaderWithSize`、`NewReaderWithOptions`、`NewWriter`、`NewClient` 改为返回 `(*T, error)`，静态可信配置可使用对应 `MustNew*`。`Reader.Close` 改为返回关闭错误，`FormatEvent` 改为 `(string, error)`，`CollectOpenAIStream` 改为必须显式传入 `CollectConfig`。移除客户端内建重连配置 `RetryInterval`、`MaxRetries`、`WithRetryInterval`、`WithMaxRetries` 与 `ErrConnectionFailed`；`Stream` 因持有取消函数而不再可比较。
- **BREAKING** `net/sse`：`Reader` 默认增加单行 1 MiB、单事件 8 MiB 的硬上限；`Client` 拒绝非 `text/event-stream` 响应，其 `Timeout` 只约束连接、TLS 握手和响应头阶段，不限制长连接的完整生命周期。
- **BREAKING** `net/httpx`：`Option`、`RawOption` 改为返回错误；`NewClient` 以及 `OpenAIClient`、`AzureOpenAIClient`、`ClaudeClient`、`GeminiClient`、`DeepSeekClient`、`QwenClient`、`DoubaoClient`、`MoonshotClient`、`ZhipuClient`、`BaichuanClient`、`SparkClient`、`CohereClient`、`MistralClient`、`VertexAIClient` 和 `CustomAIClient*` 等全部 AI 客户端构造函数开始返回错误。移除 `RawClient`，改用 `NewRawClient`/`MustNewRawClient`；移除 `WithTransport`，原生客户端 transport 注入使用 `WithRawTransport`。
- **BREAKING** `net/httpx`：包级 `Get`、`Post`、`PostForm`、`Put`、`Delete`，以及 `Client.R`、`Client.ChatCompletion`、`Client.ChatCompletionStream` 改为 context-first；移除 `GetWithContext`、`PostWithContext` 与 `Request.SetContext`。
- **BREAKING** `net/httpx`：`NewPool(PoolConfig)`、`NewRetryPool(*Pool, RetryConfig)`、`NewRateLimitedPool`、`NewCircuitBreakerPool`、`NewHostPool(HostPoolConfig)` 全部开始返回错误，`DefaultPoolConfig` 与 `DefaultRetryConfig` 由变量改为函数。移除私有熔断状态机的 `CircuitBreakerConfig`、`CircuitBreakerState` 与状态常量，统一使用 `util/circuit`；`CircuitBreakerPool.Reset`、`HostPool.GetPool`、`HostPool.SetHostConfig`、`SetGlobalPool` 开始返回错误，`RateLimitedPool.Close` 不再返回错误。`PoolStats` 移除连接数、等待及响应时间公开字段，统计读取统一使用 `GetStats() PoolStatsSnapshot`。
- **BREAKING** `net/httpx`：客户端和 `RetryPool` 只重试幂等方法或携带 `Idempotency-Key` 的请求；需要重试的请求体必须可重放，否则返回 `ErrRequestBodyNotReplayable`。普通响应超过 `maxBodySize` 时改为返回 `ErrResponseBodyTooLarge`，不再静默截断；流式 SSE 单事件默认上限为 1 MiB。
- **BREAKING** 数据库生命周期：`clickhouse.Reset`、`elasticsearch.Reset`、`mongodb.Reset` 开始返回资源关闭错误。
- **BREAKING** 数据库 TLS：ClickHouse 移除 `Config.InsecureSkipVerify`，`WithTLS(bool)` 改为无参数 `WithTLS()` 并始终启用证书校验；Elasticsearch 移除 `Config.InsecureSkipVerify` 与 `WithInsecureSkipVerify`，自定义信任链仅通过 `CACert`/`WithCACert` 配置。
- **BREAKING** `infra/db/mysql`：`QueryWithTimeout` 与 `QueryRowWithTimeout` 显式返回 `context.CancelFunc`，调用方必须在消费完 `Rows`/`Row` 后取消上下文；移除旧 `Ex` 变体。
- **BREAKING** `infra/db/redis`：Redis 拓扑、ACL、TLS 与连接池配置统一由 `infra/redisconn.Config`/`Mode` 拥有，旧 `Config`/`Mode` 改为该包的别名；`DefaultConfig` 改为 `DefaultConfig(mode, addrs...) Config`，`New` 改为 `New(ctx, Config)` 并在返回前验证连接。移除 `DefaultClusterConfig`、`Init`、`GetGlobal`、`Logger`、`StdLogger` 等进程全局 API；`Client.GetWithDefault` 改为返回 `(string, error)`，只在 Redis key 不存在时返回默认值。
- **BREAKING** `infra/queue/asynq`：`Config` 改为持有唯一的 `redisconn.Config`，移除顶层 `RedisAddrs`、`Username`、`Password`；`DefaultConfig(redisConfig) Config`、`NewManager(ctx, Config)`、`InitManager(ctx, Config)` 改为显式配置和 context-first。移除 `ConfigProvider`、`RedisConfig`、`RedisClient`、`DefaultConfigProvider`、`InitManagerFromConfig`、`InitWithRedisConfig`、`Set/GetConfigProvider`、`Set/GetRedisClient` 等重复配置和全局 Redis facade；调用方必须把 Redis 拓扑、凭据、TLS 与连接池配置迁入 `redisconn.Config`，由 `Manager` 独占 Redis 客户端及其生命周期。
- **BREAKING** `infra/queue/asynq`：`InitQueueNames` 已删除，调用方应删除初始化调用，通过 `Config.QueuePrefix` 配置命名空间，并使用 `Config.QueueName`/`Manager.QueueName` 派生队列名。`NewCircuitBreaker` 与 `NewRateLimiter` 分别改为返回 `(*CircuitBreaker, error)` 与 `(*RateLimiter, error)`，调用方必须处理配置校验错误。`InitPolling` 改为 `InitPolling(ctx, PollingConfig, Config, WorkerDependencies) (func() error, error)`，`PollingConfig` 移除 `RedisAddr` 与 `Concurrency`；handler、middleware、schedule 注册开始返回错误。`InitMetrics` 改为 `InitMetrics(queueNames []string)`，`UpdateQueueMetrics` 改为 `UpdateQueueMetrics(ctx, manager) error`，`StartMetricsUpdater` 改为 `StartMetricsUpdater(ctx, manager, interval) error`；backpressure、rate-limit 和 circuit manager 的配置、启动、查询、清理与重置操作开始返回错误。
- **BREAKING** `infra/queue/asynq`：熔断器移除 `Allow`、`RecordSuccess`、`RecordFailure`，改用 `Acquire` 与同一次 `CircuitPermit.Complete`，状态类型统一为 `util/circuit.State`；移除全局 channel/platform breaker getter 和旧 `SetConfig`，删除 `CircuitBreakerStats.ConsecutiveErrors`。轮询/迁移锁的包级函数及任务轮询全局标记被移除，统一使用 `Manager.AcquirePollingLease`/`AcquireMigrationLease` 返回的 `Lease`。队列名由变量改为常量，`PollingLockTTL` 改为 `time.Duration`；移除 `StateCancelled`，统一使用 `StateCanceled`/`CANCELED`；`HealthChecker` 不再可比较。
- **BREAKING** `infra/observe`/`infra/prometheus`：指标标签由 `...string` 改为 `...observe.Tag`，`Metrics.Counter/Gauge/Histogram/Timer` 及 Prometheus registry/adapter 构造与操作开始返回错误；`NewExporter` 改为返回 `(*Exporter, error)`，调用方必须处理运行时收集器注册和工厂构造错误；`Counter.Add` 返回错误，`Gauge` 不再实现 `Counter`。移除 `Prometheus*` 类型、`Collector`、`NewCollector` 与 `Exporter.Collector`，`DefaultBuckets`/`DefaultQuantiles` 由变量改为函数，`Registry.Gather` 返回 `(string, error)`，`Exporter.Shutdown` 改为 `Shutdown(ctx) error`。
- **BREAKING** `infra/otel`：移除 `OTelTracer`、`OTelConfig`、`OTelOption`、`OTelSpan`、`NewOTelTracer`、`DefaultOTelConfig`、`WithEndpoint` 与 `WithBatchConfig`，统一使用 `Tracer`、`Config`、`Option`、`Span`、`NewTracer` 与 `DefaultConfig`。`NewOTLPExporter` 改为返回 `(*OTLPExporter, error)`；`Tracer.SetExporter(ctx, exporter) error` 接管 exporter 生命周期并返回旧 exporter 的关闭错误。
- **BREAKING** `lang/errorx`：`MultiError.Append`/`AppendResult` 不再返回接收者，错误聚合改为有界保留并拒绝循环引用；并行执行统一返回 `error`、按输入顺序聚合并限制默认并发；`SafeGo(func()) <-chan error` 返回唯一一次完成结果，`Result.UnwrapOrElse(func(error) T) (T, error)` 对 nil 回调返回可诊断错误。`lang/syncx.OnceErr.Value` 返回顺序改为 `(value, initialized, error)`。
- **BREAKING** `lang/contextx`：包级 `Go`、`(*WaitGroupContext).Go` 与 `(*Pool).Go` 开始返回 `error`；`Run(ctx, func(context.Context) error)` 与 `RunTimeout(parent, timeout, func(context.Context) error)` 改为同步协作取消；`NewPool(ctx, size) (*Pool, error)` 校验 nil context 与非正数容量。
- **BREAKING** `lang/timex`：`Now` 由可替换的函数变量改为并发安全函数；需要替换时间提供函数的调用方改用 `SetNowProvider(provider)`，并调用其返回的恢复函数。
- **BREAKING** `cache/multi`：`Option` 由 `func(*Options)` 改为 `func(*Options) error`；`NewCache(layers, opts...) (*Cache, error)` 与 `Builder.Build() (*Cache, error)` 统一校验空层、nil 层及无效选项，不再通过 panic 或无效实例表达配置错误。
- **BREAKING** `event`：`BusOption` 由 `func(*Bus)` 改为 `func(*Bus) error`；`New`、`Subscribe`、`SubscribeAll`、`Publish` 与 `PublishSync` 显式返回配置或生命周期错误；`Close` 仅启动非阻塞关闭流程，`Shutdown(ctx)` 负责等待活跃处理器完成；panic 回调自身的 panic 会被隔离。
- **BREAKING** `util/poolx`：`Go`、`GoWait`、`GoBatch`、`Parallel`、`SetDefaultPool` 开始返回调度错误；`GoCtx` 在 v0.2.6 已返回 `error`，本次不兼容变更是回调由 `func()` 改为 `func(context.Context)`。`SubmitWithContext` 回调同样改为 `func(context.Context)`，`SubmitFuncCtx` 改为 context-first。`NewMultiPool`、`NewObjectPool` 开始返回构造错误。移除任务级 `Timeout` 字段、`WithTaskTimeout`、`HookOnTimeout`、`HookBuilder.OnTimeout` 与 `StealingScheduler.Steal`；删除 timeout hook 后，`HookOnWorkerStart`、`HookOnWorkerStop`、`HookOnScaleUp`、`HookOnScaleDown` 的数值发生重排，依赖枚举数值的调用方必须迁移。
- **BREAKING** `net/ip`：`GetLocalIP`、`GetOutboundIP`、`ResolveHost`、`ReverseLookup` 全部改为 context-first 签名。
- **BREAKING** `net/ssrf`：`ValidateURL` 改为 `ValidateURL(ctx, rawURL)` context-first 签名。
- **BREAKING** `util/config`：`Config.LoadEnv` 与 `SetGlobal` 开始返回错误。
- **BREAKING** `util/encoding`：`JoinURL` 由返回 `string` 改为返回 `(string, error)`。
- **BREAKING** `util/idgen`：`Snowflake` 不再可比较。
- **BREAKING** `util/lease`：`ErrNotHolder` 的静态类型由私有 `*leaseError` 改为 `error`。
- **BREAKING** `util/pagination`：`Pagination.Page`、`Offset`、`TotalPages`，`New`/`NewWithOffset` 的页码或偏移量参数，以及 `GetRange`、`PrevPage`、`NextPage` 的返回值由 `int` 改为 `int64`；`GetPageNumbers` 改为返回 `[]int64`。
- **BREAKING** `util/validator`：`FieldError` 移除公开字段 `Value`。
- **BREAKING** `util/file`：默认文件权限收紧为 `0600`、目录权限收紧为 `0750`；写入与复制改为同目录临时文件、文件同步、原子替换和父目录同步协议，空删除路径与 nil 遍历回调会返回错误。
- 依赖治理：根模块与 `examples` 移除不再可达的 `github.com/bytedance/gopkg`，减少无效依赖与供应链表面。
- `cache/local`、`cache/redis`、`cache/multi`：通过私有 NotFound 标记合同识别跨缓存层负缓存错误，避免包耦合、字符串匹配及重复回源。
- `util/logger`：自定义 `slog.Handler` 共享动态 `LevelVar`，`SetLevel` 现在可以控制 `UseHandler`、`UseHandlerWithConfig` 和 `NewWithHandler` 创建的日志器。

### Fixed
- Windows：`os/sandbox` 的句柄重开在 `ReOpenFile` 无法提升访问权限（请求超过原句柄权限即 `ACCESS_DENIED`）时，统一回退为按 `GetFinalPathNameByHandle` 解析出的真实路径重新 `CreateFile`；工作区 DACL/integrity 更新与目录句柄冻结共同受益，不再逐处按路径打开。
- Windows：文件与目录身份检测以内核信息类查询为权威来源——`FileStandardInformation.Directory` 判定目录性、`FileAttributeTagInformation.ReparseTag` 判定 reparse point、`IO_REPARSE_TAG_MOUNT_POINT` 补全 junction 的目录位；修复 `FILE_FLAG_OPEN_REPARSE_POINT` 打开时 `GetFileInformationByHandle` 属性位缺失导致的 junction 误判为普通目录。
- Windows：目录替换复核以 NTFS 创建时间为额外判据（文件索引可能被新目录复用），任一环节取不到创建时间时按 fail-closed 视为身份已变化，不再把替换后的目录误判为原身份。
- macOS：sandbox 系统读挂载与可执行放行覆盖 homebrew 安装树（`/opt/homebrew`、`/usr/local` 含 `Cellar`），修复沙箱内 homebrew 运行时的 `import` 与 `exec` 失败（如 pip 的站点包与解释器 shebang 位于 Cellar 深层路径）。
- `blobstore`：Windows 上原子替换对瞬时占用（目标被无 `FILE_SHARE_DELETE` 的句柄短暂打开，如防病毒扫描）做有界指数退避重试；`Go` 的 `os.Open` 在 Windows 上共享模式不含 `FILE_SHARE_DELETE`，测试改用带删除共享的句柄验证"替换时句柄仍有效"语义。
- Windows：AppContainer 沙箱内子进程未显式设置 stdio 时默认打开 `NUL` 设备会被拒；测试载荷统一显式继承父进程 stdio。
- 补齐数据库、HTTP/SSE、文件复制和可观测性链路中的资源关闭与错误传播，避免关闭错误或异步导出错误被静默丢弃。
- `infra/otel`：OTLP 批量导出失败时重新入队未发送 Span；`Tracer.Shutdown` 先等待代际排空并刷新最终 Span，再使用调用方 context 停止 exporter 自有任务，并汇总异步导出、刷新和关闭错误。
- MySQL 超时查询不再在调用方读取结果前提前取消派生上下文。
- `crypto/sign`：未知 `HMACHash` 不再静默降级为 SHA-256，而是使签名失败；时间窗口验证拒绝负时间戳、非正 `maxAge` 和整数溢出，nonce 验证拒绝空 nonce、nil/typed-nil checker；`TimestampSigner` 复制并独占调用方提供的密钥字节。
- `util/rand`：Unicode 字符集改为按 rune 采样，整数随机区间改用无溢出计算，完整 `int`/`int64` 范围不再触发区间溢出。
- `util/encoding`：`JoinURL` 重新校验拼接后的最终 URL，畸形 authority 不再作为无效 URL 静默返回。
- `os/sandbox`：macOS/Linux 的 hostedtoolcache 路径改为从当前用户主目录派生，不再把固定 Runner 主目录写入二进制。

### Tests
- CI 的 Windows 作业统一在 checkout 后以 `git checkout-index -f -u -a` 重写工作树为 LF 并刷新索引 stat 缓存，消除 `autocrlf` 导致的 gofmt/tidy/合同测试伪失败；收尾门禁改为内容级校验（`git diff` 权威），不再被平台行尾表示差异误伤。
- Windows AppLocker 门禁在托管 runner 上强制执行不可用（策略已应用但执行未被阻止）时条件化跳过，与 root Linux bwrap 门禁的环境探测语义一致。
- Sandbox CodeExec 的 go 运行时测试改为仅 Linux 强制执行；darwin 不可信沙箱的 deny process-fork 与 go build 派生工具链子进程的语义冲突，不作为 macOS 门禁要求。
- Sandbox CodeExec 的 go.work 纳入 ai-core 与 hexagon 源码，避免解析到未适配 v0.3.0 的历史发布版本；Downstream Contract 的 codeup 侧消费者合同作业移除，改由消费者侧自验脚本接管。
- 移除 codecov 上传步骤（tokenless 上传已被拒绝），覆盖率仍由 `-coverprofile` 本地生成。
- 新增 MySQL 8.4 与 Redis 7.4 具名 ACL 的隔离容器集成测试脚本，并纳入 CI。
- CI 全量 lint 门禁改为检查整个仓库。
- API 兼容工作流新增 `v*` Tag 触发与候选版本一致性校验，`v0.3.0` breaking baseline 固定为 34 个不兼容包段。
- Windows 原生沙箱门禁扩充为 28 项，并修复 PowerShell 将 `git config` 的预期退出码 1 遗留到后续命令的问题。
- Linux 原生沙箱门禁修正 root/non-root 测试归属，分别验证宿主 Unix Socket 隐藏和非特权外部工作区隔离。

## [0.2.6] - 2026-07-04

`os/sandbox` 安全加固版本。相对 v0.2.3，导出 API 仅包含兼容性新增，但默认资源限额和 Linux 沙箱后端降级策略发生了用户可见变化。

### Added
- `os/sandbox`：`Config` 新增 `DenyLoopback`、`MaxOutputBytes`、`MaxStderrBytes`、`MaxWorkspaceBytes`、`MaxArtifactBytes`、`MaxMemoryBytes` 与 `MaxProcesses`。
- `os/sandbox`：新增 `LimitStatus`、`LimitReport`、`LimitStatusEnforced`、`LimitStatusUnsupported` 与 `LimitStatusWeak`，用于报告资源限制及文件系统隔离的实际强度。
- `os/sandbox`：`ExecResult` 新增 `StdoutBytes`、`StderrBytes`、`StdoutTruncated`、`StderrTruncated` 与 `Limits`。
- `os/sandbox`：新增可通过 `errors.Is` 判断的 `ErrStorageLimitExceeded` 与 `ErrFilesystemContainmentUnavailable`。
- `os/sandbox`：Windows 构建新增公开 ACL 常量 `DENY_ACCESS` 与 `TRUSTEE_IS_SID`。

### Changed
- 构建基线：最低 Go 版本从 1.25.5 提升至 1.25.7。
- `os/sandbox`：零值资源配置会应用安全默认值：stdout 64 KiB、stderr 64 KiB、工作区 1 GiB、单产物 50 MiB、内存 256 MiB、进程数 64。
- `os/sandbox`：Linux 优先使用 bubblewrap；仅有 unshare 且配置了 `DeniedPaths` 时 fail-closed，未声明机密路径时才允许以 `LimitStatusWeak` 运行；没有可用隔离后端时不再退化为裸进程执行。
- `os/sandbox`：macOS Seatbelt 在 `Network=true` 且 `DenyLoopback=true` 时拒绝回环访问；Linux 此版本尚不能落实 `DenyLoopback`，调用方不得依赖该字段隔离本机端口。
- `os/sandbox`：Windows 使用真实进程等待结果、退出码、有界 stdout/stderr 和超时终止结果，资源限制状态通过 `LimitReport` 返回。

### Fixed
- `os/sandbox`：存储限额违规保留已经产生的 `ExecResult`，并保持 `ErrStorageLimitExceeded` 错误链。
- `os/sandbox`：工作区检查允许文件在并发执行期间消失，不再把正常清理竞态误判为扫描失败。
- `os/sandbox`：修复 Windows 非零退出码、输出截断统计以及超时后进程等待行为。

### Tests
- 新增跨平台沙箱隔离、资源限制、输出截断、存储限额、回环网络和 Linux 后端 fail-closed 回归测试。
- 新增独立的 sandbox code-exec CI 工作流，并固定 Linux 沙箱 Runner 为 Ubuntu 22.04。

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
