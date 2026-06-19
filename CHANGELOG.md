# Changelog

本文件记录 toolkit 的用户可见变更，遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本遵循 [SemVer](https://semver.org/lang/zh-CN/)。

## [Unreleased]
### Added
- `blobstore`：抽象 `Blobstore` 接口 + 流式 `SaveStream`/`OpenReader` + `ObjectBackend`(S3/R2) seam + TTL。
- `util/lease`：分布式互斥租约 `Lease` 接口 + `FencingToken` + 进程内 `MemoryLease`。
- `cache/local`：裸 `Get`/`Set` API、无后台清理构造、`Close` 别名与确定性 LRU 淘汰。
- `net/sse`：Reader 选项、累计字节上限、严格 data 前缀模式与 provider 无关 done 谓词。
- `net/ssrf`：URL 级 SSRF 校验；`net/ip.IsPrivateOrReservedIP` 补齐私有/保留地址判断。
- `os/sandbox`：跨平台命令沙箱、网络策略与代理能力。
- `util/rand`：返回 error 的 `Try*` 随机数安全变体。
- `util/retry`：最终错误可解包与 `OnRetry` 零基计数兼容选项。
- 工程治理：CI（build/vet/race/lint/govulncheck）、`.golangci.yml`、`CONTRIBUTING.md`。

### Fixed
- `infra/otel`：修复 W3C `traceparent` 解析，避免使用 `fmt.Sscanf` 贪婪匹配导致 trace 链路断开。

## [0.0.6]
- 基线版本（lang / crypto / net / cache / util / collection / infra 等通用能力）。
