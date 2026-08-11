//go:build darwin

package sandbox

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

type darwinPathIdentity struct {
	path       string
	info       os.FileInfo
	mode       os.FileMode
	size       int64
	links      uint64
	changeSec  int64
	changeNsec int64
	freezeData bool
}

type darwinExecutionGuard struct {
	workspace  string
	identities []darwinPathIdentity
}

func newDarwinExecutionGuard(workspace string, command Command) (*darwinExecutionGuard, error) {
	return newDarwinExecutionGuardContext(context.Background(), workspace, command)
}

func newDarwinExecutionGuardContext(ctx context.Context, workspace string, command Command) (*darwinExecutionGuard, error) {
	if err := checkPOSIXPreparationContext(ctx, "create macOS execution guard"); err != nil {
		return nil, err
	}
	// command.Path 必须来自同一次不可变执行计划，禁止在 guard 与 runner 之间再次解析符号链接。
	resolvedCommand := filepath.Clean(command.Path)
	if !filepath.IsAbs(resolvedCommand) {
		return nil, fmt.Errorf("sandbox executable plan must use an absolute path")
	}
	commandIdentity, err := captureDarwinPathIdentity(resolvedCommand)
	if err != nil {
		return nil, err
	}
	commandIdentity.freezeData = true
	return newDarwinExecutionGuardWithIdentity(ctx, workspace, command, commandIdentity)
}

func newDarwinExecutionGuardFromPlanContext(
	ctx context.Context,
	workspace string,
	plan darwinCommandPlan,
) (*darwinExecutionGuard, error) {
	if plan.err != nil {
		return nil, plan.err
	}
	if plan.commandIdentity.info == nil {
		return nil, fmt.Errorf("sandbox executable plan identity is unavailable")
	}
	return newDarwinExecutionGuardWithIdentity(ctx, workspace, plan.command, plan.commandIdentity)
}

func newDarwinExecutionGuardWithIdentity(
	ctx context.Context,
	workspace string,
	command Command,
	commandIdentity darwinPathIdentity,
) (*darwinExecutionGuard, error) {
	guard := &darwinExecutionGuard{workspace: filepath.Clean(workspace)}
	if err := rejectDarwinWorkspaceHardlinksContext(ctx, workspace); err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, 3)
	for _, path := range []string{workspace, command.Dir} {
		if err := checkPOSIXPreparationContext(ctx, "capture macOS execution identity"); err != nil {
			return nil, err
		}
		path = filepath.Clean(path)
		if _, exists := seen[path]; exists {
			continue
		}
		identity, err := captureDarwinPathIdentity(path)
		if err != nil {
			return nil, err
		}
		guard.identities = append(guard.identities, identity)
		seen[path] = struct{}{}
	}
	if _, exists := seen[commandIdentity.path]; !exists {
		guard.identities = append(guard.identities, commandIdentity)
	}
	return guard, nil
}

func rejectDarwinWorkspaceHardlinksContext(ctx context.Context, workspace string) error {
	return filepath.WalkDir(workspace, func(path string, entry fs.DirEntry, walkErr error) error {
		if err := checkPOSIXPreparationContext(ctx, "audit macOS workspace"); err != nil {
			return err
		}
		if walkErr != nil {
			if path != workspace && errors.Is(walkErr, fs.ErrNotExist) {
				return nil
			}
			return fmt.Errorf("inspect sandbox workspace entry %q: %w", path, walkErr)
		}
		if !entry.Type().IsRegular() && entry.Type() != 0 {
			return nil
		}
		identity, err := captureDarwinPathIdentity(path)
		if err != nil {
			if path != workspace && errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if identity.mode.IsRegular() && identity.links != 1 {
			return fmt.Errorf("sandbox workspace file %q has multiple hard links", path)
		}
		return nil
	})
}

func captureDarwinPathIdentity(path string) (darwinPathIdentity, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return darwinPathIdentity{}, fmt.Errorf("inspect sandbox path identity %q: %w", path, err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return darwinPathIdentity{}, fmt.Errorf("inspect sandbox path identity %q: unsupported metadata", path)
	}
	return darwinPathIdentity{
		path:       filepath.Clean(path),
		info:       info,
		mode:       info.Mode(),
		size:       info.Size(),
		links:      uint64(stat.Nlink),
		changeSec:  stat.Ctimespec.Sec,
		changeNsec: stat.Ctimespec.Nsec,
	}, nil
}

func (g *darwinExecutionGuard) Revalidate() error {
	return g.RevalidateContext(context.Background())
}

func (g *darwinExecutionGuard) RevalidateContext(ctx context.Context) error {
	if g == nil {
		return fmt.Errorf("sandbox Darwin execution guard is unavailable")
	}
	for _, expected := range g.identities {
		if err := checkPOSIXPreparationContext(ctx, "revalidate macOS execution identity"); err != nil {
			return err
		}
		current, err := captureDarwinPathIdentity(expected.path)
		if err != nil {
			return err
		}
		if !os.SameFile(expected.info, current.info) || expected.mode != current.mode {
			return fmt.Errorf("sandbox path identity changed before execution: %q", expected.path)
		}
		if current.mode.IsRegular() && expected.links != current.links {
			return fmt.Errorf("sandbox path identity changed before execution: %q", expected.path)
		}
		if expected.freezeData && (expected.size != current.size ||
			expected.changeSec != current.changeSec ||
			expected.changeNsec != current.changeNsec) {
			return fmt.Errorf("sandbox executable identity changed before execution: %q", expected.path)
		}
	}
	return rejectDarwinWorkspaceHardlinksContext(ctx, g.workspace)
}
