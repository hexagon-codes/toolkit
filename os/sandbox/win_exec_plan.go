//go:build windows

package sandbox

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// windowsExecutablePlan 将 canonical Command.Path 绑定到禁止写入和删除共享的句柄。
type windowsExecutablePlan struct {
	applicationName string
	identity        windowsFileIdentity
	file            *os.File
}

// windowsDirectoryPlan 将 Command.Dir 绑定到工作区内的目录句柄。
type windowsDirectoryPlan struct {
	path     string
	identity windowsFileIdentity
	file     *os.File
}

func resolveWindowsExecutable(workspace *windowsWorkspace, commandPath string) (*windowsExecutablePlan, error) {
	if commandPath == "" {
		return nil, fmt.Errorf("Windows Command.Path is required")
	}
	if commandPath != strings.TrimSpace(commandPath) {
		return nil, fmt.Errorf("Windows Command.Path must not contain surrounding whitespace")
	}
	if strings.ContainsAny(commandPath, "\x00\r\n\"") {
		return nil, fmt.Errorf("Windows Command.Path contains unsupported characters")
	}
	if err := validateWindowsPath(commandPath); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(commandPath) {
		return nil, fmt.Errorf("Windows Command.Path must be an absolute canonical path")
	}
	if windowsPathWithin(workspace.canonicalPath, commandPath) {
		return resolveWindowsWorkspaceExecutable(workspace, commandPath)
	}
	return resolveTrustedWindowsExecutable(commandPath)
}

func resolveWindowsWorkspaceExecutable(workspace *windowsWorkspace, absolutePath string) (*windowsExecutablePlan, error) {
	relativePath, err := filepath.Rel(workspace.canonicalPath, filepath.Clean(absolutePath))
	if err != nil || relativePath == ".." || strings.HasPrefix(relativePath, `..\`) {
		return nil, fmt.Errorf("workspace executable is outside the sandbox root")
	}
	if rejectErr := rejectWindowsRootReparsePoint(workspace.root, relativePath); rejectErr != nil {
		return nil, fmt.Errorf("workspace executable path is invalid: %w", rejectErr)
	}
	original, err := workspace.root.Open(relativePath)
	if err != nil {
		return nil, fmt.Errorf("open workspace executable: %w", err)
	}
	identity, err := inspectWindowsFileHandle(original)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("inspect workspace executable: %w", err), original.Close())
	}
	if identity.attributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
		return nil, errors.Join(fmt.Errorf("workspace executable must be a regular file"), original.Close())
	}
	if identity.attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return nil, errors.Join(fmt.Errorf("workspace executable must not be a reparse point"), original.Close())
	}
	if identity.links != 1 {
		return nil, errors.Join(fmt.Errorf("workspace executable must not be hard-linked"), original.Close())
	}

	frozenHandle, err := reopenWindowsHandle(
		windows.Handle(original.Fd()),
		windows.FILE_GENERIC_READ|windows.FILE_GENERIC_EXECUTE,
		windows.FILE_SHARE_READ,
		windows.FILE_FLAG_OPEN_REPARSE_POINT,
	)
	runtime.KeepAlive(original)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("freeze workspace executable: %w", err), original.Close())
	}
	frozen := os.NewFile(uintptr(frozenHandle), filepath.Clean(absolutePath))
	if frozen == nil {
		return nil, errors.Join(
			fmt.Errorf("wrap frozen workspace executable handle"),
			windows.CloseHandle(frozenHandle),
			original.Close(),
		)
	}
	plan, planErr := finalizeWindowsExecutablePlan(frozen, identity, workspace.canonicalPath)
	closeErr := original.Close()
	if planErr != nil {
		return nil, errors.Join(planErr, closeErr)
	}
	if closeErr != nil {
		return nil, errors.Join(fmt.Errorf("close workspace executable audit handle: %w", closeErr), plan.close())
	}
	return plan, nil
}

func resolveTrustedWindowsExecutable(path string) (*windowsExecutablePlan, error) {
	cleanPath := filepath.Clean(path)
	if err := validateWindowsPath(cleanPath); err != nil {
		return nil, fmt.Errorf("trusted Windows executable path is invalid: %w", err)
	}
	if !filepath.IsAbs(cleanPath) {
		return nil, fmt.Errorf("trusted Windows executable path must be absolute")
	}
	pathW, err := windows.UTF16PtrFromString(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("encode trusted executable path: %w", err)
	}
	handle, err := windows.CreateFile(
		pathW,
		windows.FILE_GENERIC_READ|windows.FILE_GENERIC_EXECUTE,
		windows.FILE_SHARE_READ,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("open trusted Windows executable: %w", err)
	}
	file := os.NewFile(uintptr(handle), cleanPath)
	if file == nil {
		return nil, errors.Join(fmt.Errorf("wrap trusted Windows executable handle"), windows.CloseHandle(handle))
	}
	identity, err := inspectWindowsFileHandle(file)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("inspect trusted Windows executable: %w", err), file.Close())
	}
	if identity.attributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
		return nil, errors.Join(fmt.Errorf("trusted Windows executable must be a regular file"), file.Close())
	}
	if identity.attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return nil, errors.Join(fmt.Errorf("trusted Windows executable must not be a reparse point"), file.Close())
	}
	return finalizeWindowsExecutablePlan(file, identity, "")
}

func finalizeWindowsExecutablePlan(
	file *os.File,
	expectedIdentity windowsFileIdentity,
	workspacePath string,
) (_ *windowsExecutablePlan, resultErr error) {
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, file.Close())
		}
	}()
	identity, err := inspectWindowsFileHandle(file)
	if err != nil {
		return nil, fmt.Errorf("inspect frozen Windows executable: %w", err)
	}
	if !expectedIdentity.sameObjectAndContent(identity) {
		return nil, fmt.Errorf("Windows executable changed while being frozen")
	}
	canonicalPath, err := canonicalWindowsPathFromHandle(file)
	if err != nil {
		return nil, fmt.Errorf("resolve frozen Windows executable: %w", err)
	}
	if !strings.EqualFold(filepath.Clean(canonicalPath), filepath.Clean(file.Name())) {
		return nil, fmt.Errorf("Windows Command.Path must already be canonical")
	}
	if workspacePath != "" {
		if !windowsPathWithin(workspacePath, canonicalPath) {
			return nil, fmt.Errorf("workspace executable resolved outside the sandbox root")
		}
	} else {
		trustedRoots, err := trustedWindowsExecutableRoots()
		if err != nil {
			return nil, err
		}
		if !windowsPathWithinAny(trustedRoots, canonicalPath) {
			return nil, fmt.Errorf("Windows executable is outside trusted runtime roots: %s", canonicalPath)
		}
	}
	plan := &windowsExecutablePlan{
		applicationName: canonicalPath,
		identity:        identity,
		file:            file,
	}
	if err := plan.revalidate(); err != nil {
		return nil, err
	}
	return plan, nil
}

func (p *windowsExecutablePlan) revalidate() error {
	if p == nil || p.file == nil {
		return fmt.Errorf("Windows executable plan is closed")
	}
	identity, err := inspectWindowsFileHandle(p.file)
	if err != nil {
		return fmt.Errorf("revalidate executable handle: %w", err)
	}
	if !p.identity.sameObjectAndContent(identity) {
		return fmt.Errorf("Windows executable changed after validation")
	}
	canonicalPath, err := canonicalWindowsPathFromHandle(p.file)
	if err != nil {
		return fmt.Errorf("revalidate executable path: %w", err)
	}
	if !strings.EqualFold(filepath.Clean(canonicalPath), filepath.Clean(p.applicationName)) {
		return fmt.Errorf("Windows executable path changed after validation")
	}
	return nil
}

func (p *windowsExecutablePlan) close() error {
	if p == nil || p.file == nil {
		return nil
	}
	err := p.file.Close()
	if err == nil {
		p.file = nil
	}
	return err
}

func resolveWindowsWorkingDirectory(workspace *windowsWorkspace, commandDir string) (*windowsDirectoryPlan, error) {
	if workspace == nil || workspace.root == nil {
		return nil, fmt.Errorf("Windows workspace is not initialized")
	}
	if commandDir != "" {
		if err := validateWindowsPath(commandDir); err != nil {
			return nil, fmt.Errorf("Windows Command.Dir contains an unsupported path: %w", err)
		}
	}
	relativePath := "."
	if commandDir != "" {
		if filepath.IsAbs(commandDir) {
			var err error
			relativePath, err = filepath.Rel(workspace.canonicalPath, filepath.Clean(commandDir))
			if err != nil {
				return nil, fmt.Errorf("resolve Windows Command.Dir: %w", err)
			}
		} else {
			relativePath = filepath.Clean(commandDir)
		}
	}
	if relativePath == ".." || strings.HasPrefix(relativePath, `..\`) || filepath.IsAbs(relativePath) {
		return nil, fmt.Errorf("Windows Command.Dir must remain inside the sandbox workspace")
	}
	if err := rejectWindowsRootReparsePoint(workspace.root, relativePath); err != nil {
		return nil, fmt.Errorf("Windows Command.Dir is invalid: %w", err)
	}

	original, err := workspace.root.Open(relativePath)
	if err != nil {
		return nil, fmt.Errorf("open Windows Command.Dir: %w", err)
	}
	identity, err := inspectWindowsFileHandle(original)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("inspect Windows Command.Dir: %w", err), original.Close())
	}
	if identity.attributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
		return nil, errors.Join(fmt.Errorf("Windows Command.Dir must be a directory"), original.Close())
	}
	if identity.attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return nil, errors.Join(fmt.Errorf("Windows Command.Dir must not be a reparse point"), original.Close())
	}

	frozenHandle, err := reopenWindowsHandle(
		windows.Handle(original.Fd()),
		windows.FILE_GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		windowsReparseFlag,
	)
	runtime.KeepAlive(original)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("freeze Windows Command.Dir: %w", err), original.Close())
	}
	frozen := os.NewFile(uintptr(frozenHandle), commandDir)
	if frozen == nil {
		return nil, errors.Join(
			fmt.Errorf("wrap frozen Windows Command.Dir handle"),
			windows.CloseHandle(frozenHandle),
			original.Close(),
		)
	}
	frozenIdentity, err := inspectWindowsFileHandle(frozen)
	if err != nil || !identity.sameObjectAndContent(frozenIdentity) {
		return nil, errors.Join(fmt.Errorf("Windows Command.Dir changed while being frozen"), frozen.Close(), original.Close())
	}
	canonicalPath, err := canonicalWindowsPathFromHandle(frozen)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("resolve Windows Command.Dir handle: %w", err), frozen.Close(), original.Close())
	}
	if !windowsPathWithin(workspace.canonicalPath, canonicalPath) {
		return nil, errors.Join(fmt.Errorf("Windows Command.Dir resolved outside the sandbox workspace"), frozen.Close(), original.Close())
	}
	if err := original.Close(); err != nil {
		return nil, errors.Join(fmt.Errorf("close Windows Command.Dir audit handle: %w", err), frozen.Close())
	}
	return &windowsDirectoryPlan{path: canonicalPath, identity: frozenIdentity, file: frozen}, nil
}

func (p *windowsDirectoryPlan) revalidate() error {
	if p == nil || p.file == nil {
		return fmt.Errorf("Windows working directory plan is closed")
	}
	identity, err := inspectWindowsFileHandle(p.file)
	if err != nil {
		return fmt.Errorf("revalidate Windows Command.Dir: %w", err)
	}
	if !p.identity.sameObjectAndContent(identity) {
		return fmt.Errorf("Windows Command.Dir changed after validation")
	}
	canonicalPath, err := canonicalWindowsPathFromHandle(p.file)
	if err != nil {
		return fmt.Errorf("revalidate Windows Command.Dir path: %w", err)
	}
	if !strings.EqualFold(filepath.Clean(canonicalPath), filepath.Clean(p.path)) {
		return fmt.Errorf("Windows Command.Dir path changed after validation")
	}
	return nil
}

func (p *windowsDirectoryPlan) close() error {
	if p == nil || p.file == nil {
		return nil
	}
	err := p.file.Close()
	if err == nil {
		p.file = nil
	}
	return err
}

func trustedWindowsExecutableRoots() ([]string, error) {
	systemDirectory, err := windows.GetSystemDirectory()
	if err != nil {
		return nil, fmt.Errorf("resolve Windows system directory: %w", err)
	}
	candidates := []string{systemDirectory}

	key, err := registry.OpenKey(
		registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows\CurrentVersion`,
		registry.QUERY_VALUE|registry.WOW64_64KEY,
	)
	if err == nil {
		defer func() {
			if keyErr := key.Close(); keyErr != nil {
				fmt.Fprintf(os.Stderr, "sandbox: close registry key: %v\n", keyErr)
			}
		}()
		for _, valueName := range []string{"ProgramFilesDir", "ProgramFilesDir (x86)"} {
			value, _, valueErr := key.GetStringValue(valueName)
			if valueErr == nil && value != "" {
				candidates = append(candidates, value)
			}
		}
	}

	roots := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		root, err := canonicalTrustedWindowsDirectory(candidate)
		if err != nil {
			continue
		}
		duplicate := false
		for _, existing := range roots {
			if strings.EqualFold(existing, root) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			roots = append(roots, root)
		}
	}
	if len(roots) == 0 {
		return nil, fmt.Errorf("no trusted Windows runtime root is available")
	}
	return roots, nil
}

func canonicalTrustedWindowsDirectory(path string) (string, error) {
	cleanPath := filepath.Clean(path)
	if err := validateWindowsPath(cleanPath); err != nil {
		return "", err
	}
	if !filepath.IsAbs(cleanPath) {
		return "", fmt.Errorf("trusted runtime root must be absolute")
	}
	file, err := os.Open(cleanPath)
	if err != nil {
		return "", err
	}
	defer func() {
		if fileErr := file.Close(); fileErr != nil {
			fmt.Fprintf(os.Stderr, "sandbox: close trusted directory handle: %v\n", fileErr)
		}
	}()
	identity, err := inspectWindowsFileHandle(file)
	if err != nil {
		return "", err
	}
	if identity.attributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
		return "", fmt.Errorf("trusted runtime root is not a directory")
	}
	canonicalPath, err := canonicalWindowsPathFromHandle(file)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(cleanPath, canonicalPath) {
		return "", fmt.Errorf("trusted runtime root must already be canonical")
	}
	return canonicalPath, nil
}

func windowsPathWithinAny(roots []string, candidate string) bool {
	for _, root := range roots {
		if windowsPathWithin(root, candidate) {
			return true
		}
	}
	return false
}
