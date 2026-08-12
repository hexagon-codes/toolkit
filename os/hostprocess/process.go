// Package hostprocess 管理单次宿主命令及其所属进程的生命周期。
package hostprocess

import (
	"context"
	"fmt"
	"os"
)

// Spec 的标准流使用已打开文件，避免取消后遗留无归属的管道复制任务。
type Spec struct {
	Executable string
	Args       []string
	// Dir 是调用方已经解析的工作目录。
	Dir string
	// Env 是调用方准备的完整环境；本层不处理继承与增量覆盖。
	Env    []string
	Stdin  *os.File
	Stdout *os.File
	Stderr *os.File
}

// Result 只描述进程实际状态，不将正常退出等同于业务任务成功。
type Result struct {
	Executable string
	Dir        string
	PID        int
	ExitCode   int
	Started    bool
	Settled    bool
	// 根进程退出不代表它启动的后台进程及输出流已经结束。
	BackgroundProcessesRunning bool
}

// Run 在取消后终止本次创建的进程组或 Job 并等待根进程回收。
// 调用方拥有标准流文件；返回后负责关闭、保存及交付完整输出。
func Run(ctx context.Context, spec Spec) (Result, error) {
	result := Result{Executable: spec.Executable, Dir: spec.Dir, ExitCode: -1}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if spec.Executable == "" {
		return result, fmt.Errorf("executable is required")
	}
	if spec.Stdin == nil || spec.Stdout == nil || spec.Stderr == nil {
		return result, fmt.Errorf("host process standard streams are required")
	}
	return run(ctx, spec, result)
}
