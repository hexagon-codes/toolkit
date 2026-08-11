//go:build windows

package sandbox

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	windowsFileDeleteChild              = 0x00000040
	windowsAccessDeniedObjectACEType    = 0x06
	windowsAccessAllowedCallbackACEType = 0x09
	windowsAccessDeniedCallbackACEType  = 0x0A
	windowsAccessAllowedCallbackObjType = 0x0B
	windowsAccessDeniedCallbackObjType  = 0x0C
	windowsReparseFlag                  = windows.FILE_FLAG_OPEN_REPARSE_POINT | windows.FILE_FLAG_BACKUP_SEMANTICS
)

var procReOpenFile = modKernel32.NewProc("ReOpenFile")

// windowsFileIdentity 冻结一个 NTFS 文件对象的稳定标识和可执行内容元数据。
type windowsFileIdentity struct {
	volumeSerial uint32
	fileIndex    uint64
	lastWrite    windows.Filetime
	size         uint64
	attributes   uint32
	links        uint32
}

func (i windowsFileIdentity) sameObjectAndContent(other windowsFileIdentity) bool {
	return i.volumeSerial == other.volumeSerial &&
		i.fileIndex == other.fileIndex &&
		i.lastWrite.HighDateTime == other.lastWrite.HighDateTime &&
		i.lastWrite.LowDateTime == other.lastWrite.LowDateTime &&
		i.size == other.size &&
		i.attributes == other.attributes &&
		i.links == other.links
}

// windowsWorkspace 持有工作区根句柄和稳定 AppContainer 身份。
type windowsWorkspace struct {
	root            *os.Root
	rootGuard       *os.File
	canonicalPath   string
	identity        windowsFileIdentity
	ownerSID        *windows.SID
	appContainerSID []byte
}

func prepareWindowsWorkspace(cfg Config) (_ *windowsWorkspace, resultErr error) {
	rootGuard, guardIdentity, guardCanonicalPath, err := openWindowsWorkspaceRootGuard(cfg.Workspace)
	if err != nil {
		return nil, err
	}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, rootGuard.Close())
		}
	}()

	root, err := os.OpenRoot(cfg.Workspace)
	if err != nil {
		return nil, fmt.Errorf("open Windows workspace root: %w", err)
	}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, root.Close())
		}
	}()

	rootFile, err := root.Open(".")
	if err != nil {
		return nil, fmt.Errorf("open Windows workspace handle: %w", err)
	}
	defer func() {
		resultErr = errors.Join(resultErr, rootFile.Close())
	}()

	identity, err := inspectWindowsFileHandle(rootFile)
	if err != nil {
		return nil, fmt.Errorf("inspect Windows workspace handle: %w", err)
	}
	if identity.attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return nil, fmt.Errorf("windows sandbox workspace must not be a reparse point")
	}
	if identity.attributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
		return nil, fmt.Errorf("windows sandbox workspace must be a directory")
	}
	if !guardIdentity.sameObjectAndContent(identity) {
		return nil, fmt.Errorf("windows sandbox workspace changed while opening the root")
	}

	canonicalPath, err := canonicalWindowsPathFromHandle(rootFile)
	if err != nil {
		return nil, fmt.Errorf("resolve Windows workspace handle: %w", err)
	}
	if pathErr := validateWindowsPath(canonicalPath); pathErr != nil {
		return nil, fmt.Errorf("windows sandbox workspace must use a local DOS path: %w", pathErr)
	}
	if !strings.EqualFold(filepath.Clean(guardCanonicalPath), filepath.Clean(canonicalPath)) {
		return nil, fmt.Errorf("windows sandbox workspace path changed while opening the root")
	}
	if isFilesystemRoot(canonicalPath) {
		return nil, fmt.Errorf("windows sandbox workspace must not be a filesystem root")
	}

	ownerSID, err := currentWindowsUserSID()
	if err != nil {
		return nil, err
	}
	appContainerSID, err := stableWindowsAppContainerSID(identity, ownerSID)
	if err != nil {
		return nil, err
	}
	allowedSIDs, err := privateWindowsWorkspaceSIDs(ownerSID, appContainerSID)
	if err != nil {
		return nil, err
	}
	entries, err := rootFile.ReadDir(-1)
	if err != nil {
		return nil, fmt.Errorf("list Windows workspace root: %w", err)
	}
	if err := auditWindowsHandleOwner(rootFile, ownerSID); err != nil {
		return nil, fmt.Errorf("audit Windows workspace root ownership: %w", err)
	}
	if len(entries) != 0 {
		rootHasIdentity, err := auditPrivateWindowsHandle(rootFile, identity, ownerSID, allowedSIDs, appContainerSID)
		if err != nil {
			return nil, fmt.Errorf("audit Windows workspace root: %w", err)
		}
		if !rootHasIdentity {
			return nil, fmt.Errorf("windows sandbox workspace must be empty before first initialization")
		}
	}

	workspace := &windowsWorkspace{
		root:            root,
		rootGuard:       rootGuard,
		canonicalPath:   canonicalPath,
		identity:        identity,
		ownerSID:        ownerSID,
		appContainerSID: appContainerSID,
	}
	if err := setPersistentWindowsWorkspaceACL(rootFile, ownerSID, appContainerSID); err != nil {
		return nil, fmt.Errorf("authorize Windows workspace root: %w", err)
	}
	if err := setPersistentWindowsWorkspaceIntegrity(rootFile); err != nil {
		return nil, fmt.Errorf("set Windows workspace integrity: %w", err)
	}

	for _, directory := range []string{
		"_tmp",
		"_appdata",
		"_localappdata",
		"_gocache",
		"_gomodcache",
		"_gopath",
	} {
		if err := root.MkdirAll(directory, 0o700); err != nil {
			return nil, fmt.Errorf("create Windows sandbox directory %q: %w", directory, err)
		}
	}
	if err := workspace.auditAndAuthorizeTree(); err != nil {
		return nil, err
	}
	return workspace, nil
}

func (w *windowsWorkspace) close() error {
	if w == nil {
		return nil
	}
	var resultErr error
	if w.root != nil {
		if err := w.root.Close(); err != nil {
			resultErr = errors.Join(resultErr, err)
		} else {
			w.root = nil
		}
	}
	if w.rootGuard != nil {
		if err := w.rootGuard.Close(); err != nil {
			resultErr = errors.Join(resultErr, err)
		} else {
			w.rootGuard = nil
		}
	}
	return resultErr
}

// openWindowsWorkspaceRootGuard 使用调用方提供的 raw absolute path 打开最终目录项，
// 在任何 canonicalize 或 os.Root 跟随链接前拒绝 symlink、junction 和其他 reparse point。
func openWindowsWorkspaceRootGuard(path string) (*os.File, windowsFileIdentity, string, error) {
	if err := validateWindowsPath(path); err != nil {
		return nil, windowsFileIdentity{}, "", fmt.Errorf("windows sandbox workspace path is invalid: %w", err)
	}
	if !filepath.IsAbs(path) {
		return nil, windowsFileIdentity{}, "", fmt.Errorf("windows sandbox workspace path must be absolute")
	}
	pathW, err := windows.UTF16PtrFromString(filepath.Clean(path))
	if err != nil {
		return nil, windowsFileIdentity{}, "", fmt.Errorf("encode Windows workspace path: %w", err)
	}
	handle, err := windows.CreateFile(
		pathW,
		windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		windowsReparseFlag,
		0,
	)
	if err != nil {
		return nil, windowsFileIdentity{}, "", fmt.Errorf("open raw Windows workspace root: %w", err)
	}
	file := os.NewFile(uintptr(handle), filepath.Clean(path))
	if file == nil {
		return nil, windowsFileIdentity{}, "", errors.Join(fmt.Errorf("wrap raw Windows workspace root handle"), windows.CloseHandle(handle))
	}
	identity, err := inspectWindowsFileHandle(file)
	if err != nil {
		return nil, windowsFileIdentity{}, "", errors.Join(fmt.Errorf("inspect raw Windows workspace root: %w", err), file.Close())
	}
	if identity.attributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
		return nil, windowsFileIdentity{}, "", errors.Join(fmt.Errorf("windows sandbox workspace must be a directory"), file.Close())
	}
	if identity.attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return nil, windowsFileIdentity{}, "", errors.Join(fmt.Errorf("windows sandbox workspace root must not be a reparse point"), file.Close())
	}
	canonicalPath, err := canonicalWindowsPathFromHandle(file)
	if err != nil {
		return nil, windowsFileIdentity{}, "", errors.Join(fmt.Errorf("resolve raw Windows workspace root: %w", err), file.Close())
	}
	return file, identity, canonicalPath, nil
}

func (w *windowsWorkspace) auditAndAuthorizeTree() error {
	return w.walk(func(relativePath string, file *os.File, identity windowsFileIdentity) error {
		allowedSIDs, err := privateWindowsWorkspaceSIDs(w.ownerSID, w.appContainerSID)
		if err != nil {
			return err
		}
		appPresent, err := auditPrivateWindowsHandle(file, identity, w.ownerSID, allowedSIDs, w.appContainerSID)
		if err != nil {
			return fmt.Errorf("audit Windows workspace entry %q: %w", relativePath, err)
		}
		if !appPresent {
			return fmt.Errorf("audit Windows workspace entry %q: stable AppContainer identity is missing", relativePath)
		}
		if err := setPersistentWindowsWorkspaceACL(file, w.ownerSID, w.appContainerSID); err != nil {
			return fmt.Errorf("authorize Windows workspace entry %q: %w", relativePath, err)
		}
		if err := setPersistentWindowsWorkspaceIntegrity(file); err != nil {
			return fmt.Errorf("set Windows workspace entry integrity %q: %w", relativePath, err)
		}
		return nil
	})
}

func (w *windowsWorkspace) walk(visit func(string, *os.File, windowsFileIdentity) error) error {
	var walk func(string) error
	walk = func(relativePath string) (resultErr error) {
		if err := rejectWindowsRootReparsePoint(w.root, relativePath); err != nil {
			return fmt.Errorf("audit Windows workspace entry %q: %w", relativePath, err)
		}
		file, err := w.root.Open(relativePath)
		if err != nil {
			return fmt.Errorf("open Windows workspace entry %q: %w", relativePath, err)
		}
		defer func() {
			resultErr = errors.Join(resultErr, file.Close())
		}()

		identity, err := inspectWindowsFileHandle(file)
		if err != nil {
			return fmt.Errorf("inspect Windows workspace entry %q: %w", relativePath, err)
		}
		if identity.attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return fmt.Errorf("windows workspace entry %q is a reparse point", relativePath)
		}
		isDirectory := identity.attributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0
		if !isDirectory && identity.links != 1 {
			return fmt.Errorf("windows workspace entry %q has %d hard links; exactly one is required", relativePath, identity.links)
		}
		if visitErr := visit(relativePath, file, identity); visitErr != nil {
			return visitErr
		}
		if !isDirectory {
			return nil
		}
		entries, err := file.ReadDir(-1)
		if err != nil {
			return fmt.Errorf("list Windows workspace entry %q: %w", relativePath, err)
		}
		for _, entry := range entries {
			child := entry.Name()
			if relativePath != "." {
				child = filepath.Join(relativePath, child)
			}
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(".")
}

// rejectWindowsRootReparsePoint 在 os.Root 解析链接前检查最终目录项，确保工作区
// 不只是“无法逃逸”，而是完全不接受 symlink、junction 或其他 reparse point。
func rejectWindowsRootReparsePoint(root *os.Root, relativePath string) error {
	if root == nil {
		return fmt.Errorf("windows workspace root is unavailable")
	}
	info, err := root.Lstat(relativePath)
	if err != nil {
		return fmt.Errorf("inspect Windows root entry without following links: %w", err)
	}
	attributes, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok || attributes == nil {
		return fmt.Errorf("windows root entry attributes are unavailable")
	}
	if attributes.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fmt.Errorf("windows root entry is a reparse point")
	}
	return nil
}

func inspectWindowsFileHandle(file *os.File) (windowsFileIdentity, error) {
	if file == nil {
		return windowsFileIdentity{}, fmt.Errorf("file handle is required")
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &info); err != nil {
		return windowsFileIdentity{}, err
	}
	runtime.KeepAlive(file)
	return windowsFileIdentity{
		volumeSerial: info.VolumeSerialNumber,
		fileIndex:    uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow),
		lastWrite:    info.LastWriteTime,
		size:         uint64(info.FileSizeHigh)<<32 | uint64(info.FileSizeLow),
		attributes:   info.FileAttributes,
		links:        info.NumberOfLinks,
	}, nil
}

func currentWindowsUserSID() (*windows.SID, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, fmt.Errorf("read current Windows user SID: %w", err)
	}
	if user == nil || user.User.Sid == nil || !user.User.Sid.IsValid() {
		return nil, fmt.Errorf("current Windows user SID is invalid")
	}
	copySID, err := user.User.Sid.Copy()
	if err != nil {
		return nil, fmt.Errorf("copy current Windows user SID: %w", err)
	}
	return copySID, nil
}

func stableWindowsAppContainerSID(identity windowsFileIdentity, owner *windows.SID) ([]byte, error) {
	if owner == nil || !owner.IsValid() {
		return nil, fmt.Errorf("workspace owner SID is invalid")
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte("hexclaw-toolkit-windows-sandbox-v1\x00"))
	_, _ = hash.Write([]byte(owner.String()))
	var identityBytes [12]byte
	binary.LittleEndian.PutUint32(identityBytes[:4], identity.volumeSerial)
	binary.LittleEndian.PutUint64(identityBytes[4:], identity.fileIndex)
	_, _ = hash.Write(identityBytes[:])
	digest := hash.Sum(nil)

	subAuthorities := make([]uint32, 0, appContainerSubAuthCount)
	subAuthorities = append(subAuthorities, appContainerBaseSID)
	for index := 0; index < appContainerSubAuthCount-1; index++ {
		rid := binary.LittleEndian.Uint32(digest[index*4 : (index+1)*4])
		if rid == 0 {
			rid = uint32(index + 1)
		}
		subAuthorities = append(subAuthorities, rid)
	}
	sid, err := allocateAppPackageSID(subAuthorities...)
	if err != nil {
		return nil, fmt.Errorf("derive stable Windows AppContainer SID: %w", err)
	}
	return copySIDBytes(sid)
}

func privateWindowsWorkspaceSIDs(owner *windows.SID, appContainerSID []byte) ([]*windows.SID, error) {
	appSID, err := windowsSIDFromBytes(appContainerSID)
	if err != nil {
		return nil, err
	}
	systemSID, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return nil, fmt.Errorf("create LocalSystem SID: %w", err)
	}
	adminSID, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return nil, fmt.Errorf("create Administrators SID: %w", err)
	}
	creatorOwnerSID, err := windows.CreateWellKnownSid(windows.WinCreatorOwnerSid)
	if err != nil {
		return nil, fmt.Errorf("create Creator Owner SID: %w", err)
	}
	return []*windows.SID{owner, systemSID, adminSID, creatorOwnerSID, appSID}, nil
}

func windowsSIDFromBytes(sidBytes []byte) (*windows.SID, error) {
	if len(sidBytes) == 0 {
		return nil, fmt.Errorf("AppContainer SID is required")
	}
	sid := (*windows.SID)(unsafe.Pointer(&sidBytes[0])) // #nosec G103 -- 字节来自已验证的 Windows SID 副本。
	if !sid.IsValid() || int(windows.GetLengthSid(sid)) != len(sidBytes) {
		return nil, fmt.Errorf("AppContainer SID is invalid")
	}
	runtime.KeepAlive(sidBytes)
	return sid, nil
}

func auditPrivateWindowsHandle(
	file *os.File,
	identity windowsFileIdentity,
	ownerSID *windows.SID,
	allowedSIDs []*windows.SID,
	appContainerSID []byte,
) (bool, error) {
	if identity.attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return false, fmt.Errorf("reparse points are not allowed")
	}
	if identity.attributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 && identity.links != 1 {
		return false, fmt.Errorf("hard-linked files are not allowed")
	}
	if err := auditWindowsHandleOwner(file, ownerSID); err != nil {
		return false, err
	}
	descriptor, err := windows.GetSecurityInfo(
		windows.Handle(file.Fd()),
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION,
	)
	runtime.KeepAlive(file)
	if err != nil {
		return false, fmt.Errorf("read handle security descriptor: %w", err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return false, fmt.Errorf("read handle DACL: %w", err)
	}
	if dacl == nil {
		return false, fmt.Errorf("workspace objects must have a private DACL")
	}
	appSID, err := windowsSIDFromBytes(appContainerSID)
	if err != nil {
		return false, err
	}
	appPresent := false
	for index := uint16(0); index < dacl.AceCount; index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, uint32(index), &ace); err != nil {
			return false, fmt.Errorf("read DACL entry %d: %w", index, err)
		}
		if ace == nil || ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return false, fmt.Errorf("workspace DACL contains unsupported allow entry type %d", ace.Header.AceType)
		}
		aceSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart)) // #nosec G103 -- GetAce 返回的 SID 位于 ACE 固定布局尾部。
		if !aceSID.IsValid() || !windowsSIDAllowed(aceSID, allowedSIDs) {
			return false, fmt.Errorf("workspace DACL grants access to an untrusted SID")
		}
		if aceSID.Equals(appSID) {
			appPresent = true
			permissions := windowsAppContainerWorkspacePermissions()
			if uint32(ace.Mask)&^uint32(permissions) != 0 {
				return false, fmt.Errorf("AppContainer DACL entry grants permissions outside the workspace capability")
			}
		}
	}
	runtime.KeepAlive(appContainerSID)
	return appPresent, nil
}

func auditWindowsHandleOwner(file *os.File, ownerSID *windows.SID) error {
	descriptor, err := windows.GetSecurityInfo(
		windows.Handle(file.Fd()),
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION,
	)
	runtime.KeepAlive(file)
	if err != nil {
		return fmt.Errorf("read handle owner descriptor: %w", err)
	}
	actualOwner, defaulted, err := descriptor.Owner()
	if err != nil {
		return fmt.Errorf("read handle owner: %w", err)
	}
	// 管理员运行的进程（如 CI runner）创建的临时目录 owner 是 BUILTIN\Administrators
	// 组而非具体用户 SID；两者均视为受信 owner。
	administratorsSID, sidErr := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if sidErr != nil {
		return fmt.Errorf("resolve builtin administrators SID: %w", sidErr)
	}
	runtime.KeepAlive(administratorsSID)
	if defaulted || actualOwner == nil ||
		(!actualOwner.Equals(ownerSID) && !actualOwner.Equals(administratorsSID)) {
		actualDescription := "<nil>"
		if actualOwner != nil {
			actualDescription = actualOwner.String()
		}
		return fmt.Errorf("workspace objects must be owned by the current Windows user (got %s, want %s, defaulted=%v)",
			actualDescription, ownerSID.String(), defaulted)
	}
	return nil
}

func windowsSIDAllowed(sid *windows.SID, allowed []*windows.SID) bool {
	for _, candidate := range allowed {
		if candidate != nil && sid.Equals(candidate) {
			return true
		}
	}
	return false
}

func setPersistentWindowsWorkspaceACL(file *os.File, ownerSID *windows.SID, appContainerSID []byte) (resultErr error) {
	appSID, err := windowsSIDFromBytes(appContainerSID)
	if err != nil {
		return err
	}
	systemSID, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return fmt.Errorf("create LocalSystem SID: %w", err)
	}
	adminSID, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return fmt.Errorf("create Administrators SID: %w", err)
	}
	appPermissions := windowsAppContainerWorkspacePermissions()
	entries := []windows.EXPLICIT_ACCESS{
		windowsAllowEntry(ownerSID, windows.GENERIC_ALL),
		windowsAllowEntry(systemSID, windows.GENERIC_ALL),
		windowsAllowEntry(adminSID, windows.GENERIC_ALL),
		windowsAllowEntry(appSID, uint32(appPermissions)),
	}
	dacl, err := windows.ACLFromEntries(entries, nil)
	if err != nil {
		return fmt.Errorf("build persistent workspace DACL: %w", err)
	}

	// ReOpenFile 不能提升访问权限（请求超过原句柄权限会得到 ACCESS_DENIED），
	// DACL 更新必须按路径重新打开文件请求 WRITE_DAC。
	objectPath, pathErr := canonicalWindowsPathFromHandle(file)
	if pathErr != nil {
		return fmt.Errorf("resolve workspace object path for DACL update: %w", pathErr)
	}
	pathPointer, pathErr := windows.UTF16PtrFromString(objectPath)
	if pathErr != nil {
		return fmt.Errorf("encode workspace object path for DACL update: %w", pathErr)
	}
	writableHandle, err := windows.CreateFile(
		pathPointer,
		windows.READ_CONTROL|windows.WRITE_DAC,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windowsReparseFlag,
		0,
	)
	if err != nil {
		return fmt.Errorf("open workspace object for DACL update: %w", err)
	}
	defer func() {
		resultErr = errors.Join(resultErr, windows.CloseHandle(writableHandle))
	}()

	if err := windows.SetSecurityInfo(
		writableHandle,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		dacl,
		nil,
	); err != nil {
		return fmt.Errorf("set persistent workspace DACL: %w", err)
	}
	runtime.KeepAlive(ownerSID)
	runtime.KeepAlive(appContainerSID)
	return nil
}

func windowsAppContainerWorkspacePermissions() windows.ACCESS_MASK {
	return windows.ACCESS_MASK(
		windows.FILE_GENERIC_READ |
			windows.FILE_GENERIC_WRITE |
			windows.FILE_GENERIC_EXECUTE |
			windows.DELETE |
			windowsFileDeleteChild,
	)
}

func setPersistentWindowsWorkspaceIntegrity(file *os.File) (resultErr error) {
	descriptor, err := windows.SecurityDescriptorFromString("S:(ML;OICI;NW;;;LW)")
	if err != nil {
		return fmt.Errorf("build low-integrity security descriptor: %w", err)
	}
	labelACL, _, err := descriptor.SACL()
	if err != nil {
		return fmt.Errorf("read low-integrity label ACL: %w", err)
	}
	if labelACL == nil {
		return fmt.Errorf("low-integrity label ACL is unavailable")
	}
	// ReOpenFile 不能提升访问权限，integrity 更新同样按路径重新打开。
	objectPath, pathErr := canonicalWindowsPathFromHandle(file)
	if pathErr != nil {
		return fmt.Errorf("resolve workspace object path for integrity update: %w", pathErr)
	}
	pathPointer, pathErr := windows.UTF16PtrFromString(objectPath)
	if pathErr != nil {
		return fmt.Errorf("encode workspace object path for integrity update: %w", pathErr)
	}
	writableHandle, err := windows.CreateFile(
		pathPointer,
		windows.READ_CONTROL|windows.WRITE_OWNER,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windowsReparseFlag,
		0,
	)
	if err != nil {
		return fmt.Errorf("open workspace object for integrity update: %w", err)
	}
	defer func() {
		resultErr = errors.Join(resultErr, windows.CloseHandle(writableHandle))
	}()
	if err := windows.SetSecurityInfo(
		writableHandle,
		windows.SE_FILE_OBJECT,
		windows.LABEL_SECURITY_INFORMATION,
		nil,
		nil,
		nil,
		labelACL,
	); err != nil {
		return fmt.Errorf("set low-integrity workspace label: %w", err)
	}
	return nil
}

func windowsAllowEntry(sid *windows.SID, permissions uint32) windows.EXPLICIT_ACCESS {
	return windows.EXPLICIT_ACCESS{
		AccessPermissions: windows.ACCESS_MASK(permissions),
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_UNKNOWN,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}
}

func reopenWindowsHandle(
	handle windows.Handle,
	desiredAccess, shareMode, flags uint32,
) (windows.Handle, error) {
	result, _, callErr := procReOpenFile.Call(
		uintptr(handle),
		uintptr(desiredAccess),
		uintptr(shareMode),
		uintptr(flags),
	)
	if windows.Handle(result) == windows.InvalidHandle {
		return windows.InvalidHandle, callErr
	}
	return windows.Handle(result), nil
}

func canonicalWindowsPathFromHandle(file *os.File) (string, error) {
	if file == nil {
		return "", fmt.Errorf("file handle is required")
	}
	buffer := make([]uint16, 512)
	for {
		length, err := windows.GetFinalPathNameByHandle(
			windows.Handle(file.Fd()),
			&buffer[0],
			uint32(len(buffer)),
			0,
		)
		runtime.KeepAlive(file)
		if err != nil {
			return "", err
		}
		if length < uint32(len(buffer)) {
			path := windows.UTF16ToString(buffer[:length])
			path = strings.TrimPrefix(path, `\\?\`)
			if strings.HasPrefix(strings.ToUpper(path), `UNC\`) {
				path = `\\` + path[4:]
			}
			return filepath.Clean(path), nil
		}
		buffer = make([]uint16, int(length)+1)
	}
}

func verifyWindowsDeniedPaths(cfg Config, workspace *windowsWorkspace) error {
	for _, deniedPath := range cfg.DeniedPaths {
		if err := validateWindowsPath(deniedPath); err != nil {
			return fmt.Errorf("windows DeniedPaths contains an unsupported path: %w", err)
		}
		if windowsPathsOverlap(workspace.canonicalPath, deniedPath) {
			return fmt.Errorf("windows DeniedPaths must not overlap the sandbox workspace: %s", deniedPath)
		}
		if err := verifyWindowsPathDeniedByAppContainer(deniedPath, workspace.appContainerSID, cfg.Network); err != nil {
			return fmt.Errorf("verify Windows denied path %q: %w", deniedPath, err)
		}
	}
	return nil
}

func verifyWindowsPathDeniedByAppContainer(path string, appContainerSID []byte, networkMode NetworkMode) (resultErr error) {
	existingPath := filepath.Clean(path)
	exactPathExists := true
	for {
		if _, err := os.Lstat(existingPath); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		exactPathExists = false
		parent := filepath.Dir(existingPath)
		if parent == existingPath {
			return fmt.Errorf("no existing path boundary is available")
		}
		existingPath = parent
	}
	blockedSIDs, err := windowsBlockedAppContainerSIDs(appContainerSID, networkMode)
	if err != nil {
		return err
	}

	pathW, err := windows.UTF16PtrFromString(existingPath)
	if err != nil {
		return err
	}
	handle, err := windows.CreateFile(
		pathW,
		windows.READ_CONTROL|windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windowsReparseFlag,
		0,
	)
	if err != nil {
		return err
	}
	defer func() {
		resultErr = errors.Join(resultErr, windows.CloseHandle(handle))
	}()
	var identity windows.ByHandleFileInformation
	if infoErr := windows.GetFileInformationByHandle(handle, &identity); infoErr != nil {
		return infoErr
	}
	if identity.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fmt.Errorf("denied path boundary is a reparse point")
	}
	if verifyErr := verifyWindowsHandleDeniedByAppContainer(handle, blockedSIDs); verifyErr != nil {
		return verifyErr
	}
	if !exactPathExists || identity.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
		return nil
	}

	root, err := os.OpenRoot(existingPath)
	if err != nil {
		return fmt.Errorf("open denied path root: %w", err)
	}
	defer func() {
		resultErr = errors.Join(resultErr, root.Close())
	}()
	return verifyWindowsDeniedRootTree(root, ".", blockedSIDs)
}

func windowsBlockedAppContainerSIDs(appContainerSID []byte, networkMode NetworkMode) ([]*windows.SID, error) {
	if networkMode != NetworkDisabled {
		return nil, fmt.Errorf("%w: Windows cannot provide the complete host network view", ErrUnsupportedNetworkPolicy)
	}
	appSID, err := windowsSIDFromBytes(appContainerSID)
	if err != nil {
		return nil, err
	}
	anyPackageSID, err := windows.CreateWellKnownSid(windows.WinBuiltinAnyPackageSid)
	if err != nil {
		return nil, err
	}
	anyRestrictedPackageSID, err := allocateAppPackageSID(appContainerBaseSID, 2)
	if err != nil {
		return nil, err
	}
	blockedSIDs := []*windows.SID{appSID, anyPackageSID, anyRestrictedPackageSID}
	runtime.KeepAlive(appContainerSID)
	return blockedSIDs, nil
}

func verifyWindowsDeniedRootTree(root *os.Root, relativePath string, blockedSIDs []*windows.SID) (resultErr error) {
	if rejectErr := rejectWindowsRootReparsePoint(root, relativePath); rejectErr != nil {
		return fmt.Errorf("audit denied path entry %q: %w", relativePath, rejectErr)
	}
	file, err := root.Open(relativePath)
	if err != nil {
		return fmt.Errorf("open denied path entry %q: %w", relativePath, err)
	}
	defer func() {
		resultErr = errors.Join(resultErr, file.Close())
	}()
	identity, err := inspectWindowsFileHandle(file)
	if err != nil {
		return fmt.Errorf("inspect denied path entry %q: %w", relativePath, err)
	}
	if identity.attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fmt.Errorf("denied path entry %q is a reparse point", relativePath)
	}
	if verifyErr := verifyWindowsHandleDeniedByAppContainer(windows.Handle(file.Fd()), blockedSIDs); verifyErr != nil {
		return fmt.Errorf("audit denied path entry %q: %w", relativePath, verifyErr)
	}
	if identity.attributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
		return nil
	}
	entries, err := file.ReadDir(-1)
	if err != nil {
		return fmt.Errorf("list denied path entry %q: %w", relativePath, err)
	}
	for _, entry := range entries {
		child := entry.Name()
		if relativePath != "." {
			child = filepath.Join(relativePath, child)
		}
		if err := verifyWindowsDeniedRootTree(root, child, blockedSIDs); err != nil {
			return err
		}
	}
	return nil
}

func verifyWindowsHandleDeniedByAppContainer(handle windows.Handle, blockedSIDs []*windows.SID) error {
	descriptor, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return err
	}
	if dacl == nil {
		return fmt.Errorf("denied path has a null DACL")
	}
	for index := uint16(0); index < dacl.AceCount; index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, uint32(index), &ace); err != nil {
			return err
		}
		if ace == nil {
			continue
		}
		switch ace.Header.AceType {
		case windows.ACCESS_DENIED_ACE_TYPE,
			windowsAccessDeniedObjectACEType,
			windowsAccessDeniedCallbackACEType,
			windowsAccessDeniedCallbackObjType:
			continue
		case windows.ACCESS_ALLOWED_ACE_TYPE:
			// 仅普通 allow ACE 的 SID 布局在此处可被无歧义验证。
		case windowsAccessAllowedCallbackACEType, windowsAccessAllowedCallbackObjType:
			return fmt.Errorf("denied path DACL contains an unsupported conditional allow entry")
		default:
			return fmt.Errorf("denied path DACL contains unsupported ACE type %d", ace.Header.AceType)
		}
		if ace.Mask == 0 {
			continue
		}
		aceSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart)) // #nosec G103 -- GetAce 返回的 SID 位于 ACE 固定布局尾部。
		if aceSID.IsValid() && windowsSIDAllowed(aceSID, blockedSIDs) {
			return fmt.Errorf("denied path grants an AppContainer identity access")
		}
	}
	return nil
}

func windowsPathsOverlap(first, second string) bool {
	return windowsPathWithin(first, second) || windowsPathWithin(second, first)
}

func windowsPathWithin(base, candidate string) bool {
	relative, err := filepath.Rel(filepath.Clean(base), filepath.Clean(candidate))
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, `..\`))
}

// validateWindowsPath 拒绝会绕开常规 Win32 文件名解析的路径形式。
func validateWindowsPath(path string) error {
	if strings.ContainsAny(path, "\x00\r\n\"") {
		return fmt.Errorf("windows path contains unsupported characters")
	}
	if hasWindowsAlternateDataStream(path) {
		return fmt.Errorf("alternate data streams are not allowed: %s", path)
	}
	if strings.HasPrefix(path, `\\`) {
		return fmt.Errorf("UNC paths are not allowed: %s", path)
	}
	lower := strings.ToLower(path)
	if strings.HasPrefix(lower, `\\.\`) || strings.HasPrefix(lower, `\\?\`) {
		return fmt.Errorf("device paths are not allowed: %s", path)
	}
	return nil
}

func hasWindowsAlternateDataStream(path string) bool {
	if !strings.Contains(path, ":") {
		return false
	}
	if isAbsWindowsPath(path) {
		return strings.Contains(path[2:], ":")
	}
	return true
}

func isAbsWindowsPath(path string) bool {
	return len(path) >= 3 && path[1] == ':' && (path[2] == '\\' || path[2] == '/')
}
