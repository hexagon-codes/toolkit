# Toolkit 下一次执行接手文档

> 更新时间：2026-08-11（Asia/Shanghai）  
> 基线提交：`d8396a6a7373db54b513def8dcffebefc5964524`  
> 最近标签：`v0.2.6`  
> 当前状态：共享工作区存在大量未提交改动，禁止 reset、checkout、clean 或覆盖。

## 1. 本轮范围决定

- 本轮只继续闭环 macOS 沙箱及 HexClaw 在 macOS 上的实际使用链。
- Linux、Windows 沙箱立即冻结：保留已有实现和测试，不继续修改，不把阶段性结果表述为发布级 GREEN。
- Toolkit 其他尚未完成的审计任务同时冻结，详见附录。
- 不打 Tag、不 Push；不得用旧构建产物冒充当前源码结果。
- 不调用真实模型或钉钉，不修改 HexClaw Desktop UI。

## 2. 固定执行环境

```bash
TOOLKIT_ROOT=/Users/guoyanjun/work/toolkit
GO_ROOT=/Users/guoyanjun/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.12.darwin-amd64
GO_BIN="$GO_ROOT/bin/go"

cd "$TOOLKIT_ROOT"
env GOROOT="$GO_ROOT" GOTOOLCHAIN=local GOENV=off GOWORK=off \
  GOFLAGS=-mod=readonly "$GO_BIN" version
```

最低编译版本是 Go `1.25.12`。仓库没有 `toolchain` 指令；不要把最低版本误写为固定用户机器工具链。

## 3. macOS 沙箱闭环门禁

### 3.1 必须保持的不变量

- `ExecutionProfileUntrusted` 是安全零值，macOS 通过 Seatbelt no-fork 提供不可信执行边界。
- 可信构建必须同时显式设置：
  - `ExecutionProfileTrustedBuild`
  - `TrustedBuildIsolationCapabilities`
- capability 只是准入断言，不能隐式切换 execution profile。
- `Close` 只是生命周期屏障，不是构件提升。
- 可信构建产物必须在构建沙箱关闭后，按字节复制到独立 runtime workspace 的全新普通文件和全新 inode；必须校验格式、大小、哈希、身份、权限与同步结果。禁止复用构建路径、硬链接或 inode。
- 根进程成功退出、超时或取消后，都不能遗留可逃逸后代、未回收根进程、stdout/stderr FD 或复制 goroutine。
- 输出排空超时必须返回稳定错误，不得在函数返回后继续修改结果缓冲。
- `Close` 失败后必须保留资源所有权，并允许再次调用收敛。
- 命令路径只解析和冻结一次；取消在启动前已经可观察时不得产生载荷副作用。
- 内存 rlimit 能力只能通过辅助子进程探测，绝不能临时下调宿主进程限制。
- `Config.Timeout` 秒数转换必须在任何副作用前拒绝溢出。

### 3.2 重点文件

```text
os/sandbox/sandbox.go
os/sandbox/sandbox_darwin.go
os/sandbox/execution_guard_darwin.go
os/sandbox/command.go
os/sandbox/exec_posix.go
os/sandbox/posix_execution.go
os/sandbox/process_lifecycle.go
os/sandbox/env_posix.go
os/sandbox/resource_limits.go
os/sandbox/close_basic.go
os/sandbox/close_darwin.go
os/sandbox/posix_execution_lifecycle_test.go
os/sandbox/trusted_build_darwin_test.go
os/sandbox/no_child_darwin_test.go
os/sandbox/workspace_isolation_darwin_test.go
```

### 3.3 最终验证

在源码冻结后使用独立缓存，避免共享 Go 1.26 缓存污染证据：

```bash
cd /Users/guoyanjun/work/toolkit
GO_ROOT=/Users/guoyanjun/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.12.darwin-amd64
GO_BIN="$GO_ROOT/bin/go"
MAC_CACHE="$(mktemp -d /tmp/toolkit-sandbox-macos.XXXXXX)"

git diff --check -- os/sandbox
rg --files os/sandbox -g '*.go' -0 | xargs -0 "$GO_BIN" gofmt -d

env GOROOT="$GO_ROOT" GOCACHE="$MAC_CACHE" GOTOOLCHAIN=local GOENV=off \
  GOWORK=off GOFLAGS=-mod=readonly GOOS=darwin GOARCH=amd64 CGO_ENABLED=1 \
  GOMAXPROCS=4 "$GO_BIN" test ./os/sandbox -count=1 -shuffle=on -timeout=20m

env GOROOT="$GO_ROOT" GOCACHE="$MAC_CACHE" GOTOOLCHAIN=local GOENV=off \
  GOWORK=off GOFLAGS=-mod=readonly GOOS=darwin GOARCH=amd64 CGO_ENABLED=1 \
  GOMAXPROCS=4 "$GO_BIN" test -race ./os/sandbox -count=1 -shuffle=on -timeout=30m

env GOROOT="$GO_ROOT" GOCACHE="$MAC_CACHE" GOTOOLCHAIN=local GOENV=off \
  GOWORK=off GOFLAGS=-mod=readonly "$GO_BIN" vet ./os/sandbox

env GOROOT="$GO_ROOT" GOCACHE="$MAC_CACHE" GOTOOLCHAIN=local GOENV=off \
  GOWORK=off GOFLAGS=-mod=readonly "$GO_BIN" test -json ./os/sandbox -count=1 \
  -run '^(TestDarwinTrustedBuildProfileAllowsChildrenWithoutClaimingContainment|TestDarwinTrustedBuildRunsFrozenGoCompilerChild|TestDarwinRejectsProcessCreationAndContainmentBeforePayload)$'
```

最后一条 JSON 中三个顶层用例必须全部 PASS 且没有 SKIP。只通过编译或源码字符串检查不算闭环。

## 4. HexClaw macOS 使用链

重点文件：

```text
/Users/guoyanjun/work/hexclaw/skill/builtin/code_exec.go
/Users/guoyanjun/work/hexclaw/skill/builtin/code_exec_go_execution.go
/Users/guoyanjun/work/hexclaw/skill/builtin/code_exec_go_execution_test.go
```

必须验证：可信 Go 构建显式选择 trusted-build profile；构件通过全新 inode 提升到独立 runtime workspace；运行阶段重新使用 untrusted profile；成功和失败路径都清理构建缓存及两个工作区。macOS 上不可信 npm/pnpm 项目执行必须 fail-closed，不能用 TrustedBuild 执行不可信脚本。

禁止调用真实模型或钉钉。使用临时 Go 1.25.12 workspace 绑定当前 `toolkit`、`ai-core`、`hexagon`、`hexclaw` 后运行 focused ordinary、race 和 vet。

## 5. Linux 沙箱冻结状态：已有实现，发布 NO-GO

### 已有/阶段性完成

- 已有 bubblewrap 后端、PID namespace、deny-by-default 文件系统与网络模式。
- 已改为 bubblewrap 不可用时 fail-closed，不再退化为裸进程。
- 不再用 `RLIMIT_NPROC` 冒充精确进程树总预算。
- 资源能力与调用方实际请求分开报告。

### 未完成

- `prlimit` 目前不能只凭可信路径存在就声明 Memory enforced；必须在启动载荷前用固定辅助子进程验证同一限制链。
- 最新共享 POSIX 生命周期修改对 Linux 的影响尚未原生验证。
- 新鲜 Linux 容器的 root/non-root、离线 bubblewrap、ordinary/race、no-skip 和零僵尸门禁未完成。
- 不得把旧容器或旧测试日志当作当前源码证据。

### 下一次优先 RED

- 根进程正常退出、后代关闭 stdout/stderr 后继续存活时，必须终止并证明收敛，不能误报 ProcessContainment。
- `prlimit` 可执行文件存在但实际调用失败时，载荷不得启动，Memory capability 不得报告 enforced。

## 6. Windows 沙箱冻结状态：已有实现，发布 NO-GO

### 已有/阶段性完成

- 已有 AppContainer、工作区 ACL、LowBox token、Job Object 和受限句柄继承。
- 六个 post-creation 故障阶段已统一为单向所有权状态机。
- 已覆盖同步释放、quarantine 单次转移、Close 失败重试，以及 normal-wait/timeout/cancel 的 retained ownership。
- 阶段性普通/race 与 Windows amd64/arm64 交叉编译曾通过。

### 未完成

- 最后修改后的 ordinary/race 尚未复验。
- 本机没有 Windows 原生证据；`windows_security` 测试必须在 Windows CI 运行。
- CI 原生必跑名单尚未纳入三个新增生命周期测试。

### 下一次验证

```powershell
go test -race -tags windows_security ./os/sandbox `
  -run '^(TestWindowsNativePostCreationFaultOwnership|TestWindowsNativePostCreationFaultCloseRetriesQuarantine|TestWindowsNativeRetainExecutionOwnership)$' `
  -count=10 -parallel=1
```

## 7. 其他冻结工作（不是本轮 macOS 闭环范围）

- `cache/crypto`：`TestCacheRecognizesLocalNegativeCacheByDefault` 仍是预期 RED；缺少加载期间 `Del` 的代际回填保护，AES 未完成专项审计。
- `collection/lang`：`lang/timex.TestNowProviderConcurrentAccess` 暴露可变全局时钟 race，尚未修复；最终 ordinary/race/vet 未跑。
- `util/logger`：`TestNewWithHandlerSetLevelControlsOutput` 已新增但未执行/修复；最终验证早于该测试。
- `config/env/encoding/json/validator`：主体修复已完成，最新 SSE/NDJSON 修改后的五包最终 ordinary/race/vet 尚未跑。
- `idgen/rand/rate/lease/pagination`：focused 已 GREEN，最终全组 ordinary/race/count/vet 与 README 同步未完成。
- `infra/db/queue/redisconn`：Manager/Redis/Lease/凭据错误链已有 focused GREEN；各数据库生命周期与最终全组门禁未完成。
- `examples`：大量示例已加固，最终 tidy/mod verify/test/race/vet/build 未完成。
- `net/ip/ssrf/sse`：当前实现曾通过 ordinary/race/vet/staticcheck；仓库全量、README/CHANGELOG 和最终下游验证未完成。
- 根模块和 examples 的 `go mod tidy -diff` 均要求删除不再可达的 `github.com/bytedance/gopkg v0.1.3`；只能在源码最终冻结后执行 tidy。
- README/README.en/CHANGELOG 仍需同步 context-first HTTPX、TrustedBuild 构件提升、event/poolx/blobstore/OTel 生命周期，并固化 `## [0.3.0] - 2026-08-11`。
- API breaking baseline 在后续导出 API 变化前生成，不能作为最终基线。

## 8. 临时资源与清理边界

已知由本轮 Agent 创建、可在确认不再复验后单独删除的临时目录：

```text
/tmp/toolkit-ssrf-downstream.re1hGC
/tmp/toolkit-readonly-audit.66lKaD
/tmp/toolkit-cross-verify.PmzsaF
/tmp/toolkit-event-stress-20260811.LYJ2HF
```

Linux 容器/临时资源的历史精确名称：

```text
toolkit-linux-native-20260811
toolkit-bwrap-probe-20260811
toolkit-linux-final-20260811
/tmp/toolkit-bwrap-final-20260811.imo2n0
```

不要执行广泛 `rm -rf`、`docker system prune` 或清理未知资源。只处理已经逐项确认归属的精确目标。

## 9. 接手顺序

1. 先读取本文件和 `git status --short --untracked-files=all`；不要恢复或覆盖共享改动。
2. 确认没有遗留 `go test`/sandbox 测试进程。
3. 完成并冻结 macOS/POSIX 生命周期，再跑第 3 节完整门禁。
4. 验证 HexClaw macOS `code_exec` 的 trusted build → fresh inode → untrusted runtime 链。
5. 若恢复 Linux，先修 `prlimit` 预检，再创建全新容器；不要复用旧结果。
6. 若恢复 Windows，先更新 CI 原生必跑名单，再在真实 Windows Runner 验证。
7. 最后恢复附录中的其他 Toolkit RED，完成 tidy、文档、全量发布矩阵后才可打 Tag。

