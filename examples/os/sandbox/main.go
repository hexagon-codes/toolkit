package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/hexagon-codes/toolkit/os/sandbox"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() (resultErr error) {
	workspace, err := os.MkdirTemp("", "toolkit-sandbox-")
	if err != nil {
		return fmt.Errorf("create sandbox workspace: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(workspace); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("remove sandbox workspace: %w", err))
		}
	}()

	sb, err := sandbox.New(sandbox.Config{
		Workspace:            workspace,
		Timeout:              10,
		Network:              sandbox.NetworkDisabled,
		ExecutionProfile:     sandbox.ExecutionProfileUntrusted,
		RequiredCapabilities: sandbox.UntrustedCodeIsolationCapabilities,
		MaxOutputBytes:       64 * 1024,
		MaxStderrBytes:       64 * 1024,
	})
	if err != nil {
		return fmt.Errorf("create sandbox: %w", err)
	}
	defer func() {
		resultErr = errors.Join(resultErr, sb.Close())
	}()
	command, err := echoCommand(workspace)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := sb.Exec(ctx, command)
	if err != nil {
		return fmt.Errorf("execute sandbox command: %w", err)
	}
	if _, err := fmt.Print(result.Stdout); err != nil {
		return fmt.Errorf("write sandbox output: %w", err)
	}
	return nil
}

// echoCommand 为不同平台选择明确的可执行文件，参数不经过 shell 文本拼接。
func echoCommand(workspace string) (sandbox.Command, error) {
	command := sandbox.Command{
		Path: "/bin/echo",
		Args: []string{"structured-command"},
		Dir:  workspace,
		Env:  nil,
	}
	if runtime.GOOS != "windows" {
		return command, nil
	}
	windowsDirectory := os.Getenv("SystemRoot")
	if windowsDirectory == "" {
		return sandbox.Command{}, fmt.Errorf("Windows SystemRoot is unavailable")
	}
	command.Path = filepath.Join(windowsDirectory, "System32", "cmd.exe")
	command.Args = []string{"/d", "/s", "/c", "echo structured-command"}
	return command, nil
}
