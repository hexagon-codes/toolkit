//go:build linux

package sandbox

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const linuxBoundaryProbeResultPrefix = "TOOLKIT_LINUX_BOUNDARY"

// linuxNamespaceRoot 是 Linux 进程命名空间符号链接目录。
const linuxNamespaceRoot = "/proc/self/ns"

type linuxObjectIdentity struct {
	device uint64
	inode  uint64
	mode   os.FileMode
	size   int64
	mtime  syscall.Timespec
	ctime  syscall.Timespec
}

type linuxExecutionGuard struct {
	workspacePath     string
	workspaceIdentity linuxObjectIdentity
	commandPath       string
	commandIdentity   linuxObjectIdentity
	directoryPath     string
	directoryIdentity linuxObjectIdentity
	policyPaths       []linuxGuardedPath
}

type linuxGuardedPath struct {
	label    string
	path     string
	identity linuxObjectIdentity
}

func newLinuxExecutionGuardContext(ctx context.Context, cfg Config, command Command) (*linuxExecutionGuard, error) {
	if err := auditLinuxWorkspaceContext(ctx, cfg.Workspace); err != nil {
		return nil, err
	}
	if err := checkPOSIXPreparationContext(ctx, "capture Linux execution identity"); err != nil {
		return nil, err
	}
	workspaceIdentity, err := readLinuxObjectIdentity(cfg.Workspace)
	if err != nil {
		return nil, fmt.Errorf("inspect sandbox workspace identity: %w", err)
	}
	commandIdentity, err := readLinuxObjectIdentity(command.Path)
	if err != nil {
		return nil, fmt.Errorf("inspect sandbox command identity: %w", err)
	}
	directoryIdentity, err := readLinuxObjectIdentity(command.Dir)
	if err != nil {
		return nil, fmt.Errorf("inspect sandbox command directory identity: %w", err)
	}
	guard := &linuxExecutionGuard{
		workspacePath:     cfg.Workspace,
		workspaceIdentity: workspaceIdentity,
		commandPath:       command.Path,
		commandIdentity:   commandIdentity,
		directoryPath:     command.Dir,
		directoryIdentity: directoryIdentity,
	}
	for _, item := range []struct {
		label string
		paths []string
	}{
		{label: "readable path", paths: cfg.ReadablePaths},
		{label: "denied path", paths: cfg.DeniedPaths},
	} {
		for _, path := range item.paths {
			if err := checkPOSIXPreparationContext(ctx, "capture Linux policy identity"); err != nil {
				return nil, err
			}
			identity, identityErr := readLinuxObjectIdentity(path)
			if identityErr != nil {
				return nil, fmt.Errorf("inspect sandbox %s identity: %w", item.label, identityErr)
			}
			guard.policyPaths = append(guard.policyPaths, linuxGuardedPath{
				label: item.label, path: path, identity: identity,
			})
		}
	}
	return guard, nil
}

func (g *linuxExecutionGuard) Verify() error {
	return g.VerifyContext(context.Background())
}

func (g *linuxExecutionGuard) VerifyContext(ctx context.Context) error {
	if g == nil {
		return fmt.Errorf("linux execution guard is unavailable")
	}
	if err := checkPOSIXPreparationContext(ctx, "revalidate Linux execution identity"); err != nil {
		return err
	}
	if err := verifyLinuxObjectIdentity("workspace", g.workspacePath, g.workspaceIdentity); err != nil {
		return err
	}
	if err := verifyLinuxObjectIdentity("command", g.commandPath, g.commandIdentity); err != nil {
		return err
	}
	if err := verifyLinuxObjectIdentity("command directory", g.directoryPath, g.directoryIdentity); err != nil {
		return err
	}
	for _, path := range g.policyPaths {
		if err := checkPOSIXPreparationContext(ctx, "revalidate Linux policy identity"); err != nil {
			return err
		}
		if err := verifyLinuxObjectIdentity(path.label, path.path, path.identity); err != nil {
			return err
		}
	}
	return auditLinuxWorkspaceContext(ctx, g.workspacePath)
}

func readLinuxObjectIdentity(path string) (linuxObjectIdentity, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return linuxObjectIdentity{}, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return linuxObjectIdentity{}, fmt.Errorf("linux stat identity is unavailable")
	}
	return linuxObjectIdentity{
		device: stat.Dev,
		inode:  stat.Ino,
		mode:   info.Mode(),
		size:   info.Size(),
		mtime:  stat.Mtim,
		ctime:  stat.Ctim,
	}, nil
}

func verifyLinuxObjectIdentity(label, path string, expected linuxObjectIdentity) error {
	actual, err := readLinuxObjectIdentity(path)
	if err != nil {
		return fmt.Errorf("sandbox %s changed before launch: %w", label, err)
	}
	changed := actual.device != expected.device || actual.inode != expected.inode || actual.mode != expected.mode
	if !expected.mode.IsDir() {
		changed = changed || actual.size != expected.size || actual.mtime != expected.mtime || actual.ctime != expected.ctime
	}
	if changed {
		return fmt.Errorf("sandbox %s changed before launch", label)
	}
	return nil
}

type linuxInodeKey struct {
	device uint64
	inode  uint64
}

type linuxInodeLinks struct {
	observed uint64
	total    uint64
}

func auditLinuxWorkspaceContext(ctx context.Context, workspace string) error {
	links := make(map[linuxInodeKey]linuxInodeLinks)
	directories := make(map[string]linuxObjectIdentity)
	err := filepath.WalkDir(workspace, func(path string, entry os.DirEntry, walkErr error) error {
		if err := checkPOSIXPreparationContext(ctx, "audit Linux workspace"); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			stat, ok := info.Sys().(*syscall.Stat_t)
			if !ok || stat == nil || stat.Uid != uint32(os.Geteuid()) {
				return fmt.Errorf("sandbox workspace entry is not owned by the current user: %q", path)
			}
			resolved, resolveErr := filepath.EvalSymlinks(path)
			if resolveErr != nil || !linuxPathWithin(workspace, resolved) {
				return fmt.Errorf("sandbox workspace symlink escapes the workspace: %q", path)
			}
			return nil
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat == nil {
			return fmt.Errorf("inspect sandbox workspace inode: %q", path)
		}
		if stat.Uid != uint32(os.Geteuid()) {
			return fmt.Errorf("sandbox workspace entry is not owned by the current user: %q", path)
		}
		if info.Mode().Perm()&0o022 != 0 {
			return fmt.Errorf("sandbox workspace entry is group or world writable: %q", path)
		}
		if info.IsDir() {
			identity, identityErr := readLinuxObjectIdentity(path)
			if identityErr != nil {
				return identityErr
			}
			directories[path] = identity
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("sandbox workspace contains an unsupported file type: %q", path)
		}
		key := linuxInodeKey{device: stat.Dev, inode: stat.Ino}
		value := links[key]
		value.observed++
		value.total = stat.Nlink
		links[key] = value
		return nil
	})
	if err != nil {
		return fmt.Errorf("audit sandbox workspace: %w", err)
	}
	for _, value := range links {
		if err := checkPOSIXPreparationContext(ctx, "audit Linux workspace links"); err != nil {
			return err
		}
		if value.total > value.observed {
			return fmt.Errorf("sandbox workspace file links outside the workspace")
		}
	}
	for path, expected := range directories {
		if err := checkPOSIXPreparationContext(ctx, "revalidate Linux workspace"); err != nil {
			return err
		}
		actual, identityErr := readLinuxObjectIdentity(path)
		if identityErr != nil || actual.device != expected.device || actual.inode != expected.inode ||
			actual.mode != expected.mode || actual.mtime != expected.mtime || actual.ctime != expected.ctime {
			return fmt.Errorf("sandbox workspace changed during audit: %q", path)
		}
	}
	return nil
}

func copyLinuxBoundaryProbeExecutable(workspace string) (resultPath string, resultErr error) {
	sourcePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve Linux boundary probe executable: %w", err)
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return "", fmt.Errorf("open Linux boundary probe executable: %w", err)
	}
	defer func() {
		if closeErr := source.Close(); closeErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close Linux boundary probe executable source: %w", closeErr))
		}
	}()

	destinationPath := filepath.Join(workspace, ".toolkit-linux-boundary-probe")
	destination, err := os.OpenFile(destinationPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o500)
	if err != nil {
		return "", fmt.Errorf("create Linux boundary probe executable: %w", err)
	}
	if _, err := io.Copy(destination, source); err != nil {
		return "", fmt.Errorf("copy Linux boundary probe executable: %w", errors.Join(err, destination.Close()))
	}
	if err := destination.Sync(); err != nil {
		return "", fmt.Errorf("sync Linux boundary probe executable: %w", errors.Join(err, destination.Close()))
	}
	if err := destination.Close(); err != nil {
		return "", fmt.Errorf("close Linux boundary probe executable: %w", err)
	}
	return destinationPath, nil
}

func runLinuxBoundaryProbePayload(networkMustDiffer bool) int {
	if !linuxCapabilitiesAreEmpty() {
		fmt.Fprintln(os.Stderr, "Linux boundary probe detected residual capabilities")
		return 126
	}
	for _, namespace := range []string{"user", "mnt", "pid", "ipc", "uts"} {
		if !linuxNamespaceDiffersFromParent(namespace) {
			fmt.Fprintf(os.Stderr, "Linux boundary probe did not isolate %s namespace\n", namespace)
			return 126
		}
	}
	if networkMustDiffer && !linuxNamespaceDiffersFromParent("net") {
		fmt.Fprintln(os.Stderr, "Linux boundary probe did not isolate network namespace")
		return 126
	}
	fmt.Printf("%s isolation=%s\n", linuxBoundaryProbeResultPrefix, LimitStatusEnforced)
	return 0
}

func linuxCapabilitiesAreEmpty() bool {
	file, err := os.Open("/proc/self/status")
	if err != nil {
		return false
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "sandbox: close Linux capability probe status file: %v\n", closeErr)
		}
	}()
	wanted := map[string]bool{"CapInh:": false, "CapPrm:": false, "CapEff:": false, "CapBnd:": false, "CapAmb:": false}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			continue
		}
		if _, exists := wanted[fields[0]]; exists {
			wanted[fields[0]] = fields[1] == "0000000000000000"
		}
	}
	if scanner.Err() != nil {
		return false
	}
	for _, empty := range wanted {
		if !empty {
			return false
		}
	}
	return true
}

func linuxNamespaceDiffersFromParent(namespace string) bool {
	parent := os.Getenv("TOOLKIT_LINUX_PARENT_NS_" + strings.ToUpper(namespace))
	current, err := os.Readlink(filepath.Join(linuxNamespaceRoot, namespace))
	return err == nil && parent != "" && current != parent
}

func parseLinuxBoundaryProbeResult(output string) linuxBwrapProbeResult {
	fields := strings.Fields(strings.TrimSpace(output))
	if len(fields) != 2 || fields[0] != linuxBoundaryProbeResultPrefix {
		return linuxBwrapProbeResult{}
	}
	result := linuxBwrapProbeResult{}
	for _, field := range fields[1:] {
		name, value, ok := strings.Cut(field, "=")
		if !ok {
			return linuxBwrapProbeResult{}
		}
		status := LimitStatus(value)
		if status != LimitStatusEnforced && status != LimitStatusUnsupported {
			return linuxBwrapProbeResult{}
		}
		if name != "isolation" {
			return linuxBwrapProbeResult{}
		}
		result.Isolation = status
	}
	return result
}

func appendLinuxParentNamespaceEnvironment(env []string) ([]string, error) {
	result := append([]string(nil), env...)
	for _, namespace := range []string{"user", "mnt", "pid", "ipc", "uts", "net"} {
		identity, err := os.Readlink(filepath.Join(linuxNamespaceRoot, namespace))
		if err != nil {
			return nil, fmt.Errorf("inspect parent Linux %s namespace: %w", namespace, err)
		}
		result = append(result, "TOOLKIT_LINUX_PARENT_NS_"+strings.ToUpper(namespace)+"="+identity)
	}
	return result, nil
}
