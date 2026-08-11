package sandbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type capabilityRecordingBackend struct {
	available CapabilitySet
	executed  bool
}

func TestNewRejectsEmptyRequiredCapabilitiesBeforeWorkspaceMutation(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	_, err := New(Config{Workspace: workspace})
	if !errors.Is(err, ErrInvalidCapabilityContract) {
		t.Fatalf("New() error = %v, want ErrInvalidCapabilityContract", err)
	}
	if _, statErr := os.Stat(workspace); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("workspace state after rejected configuration = %v, want not created", statErr)
	}
}

func TestRequestedResourceCapabilitiesAcceptCompleteContract(t *testing.T) {
	config := Config{
		MaxOutputBytes:    1,
		MaxStderrBytes:    1,
		MaxWorkspaceBytes: 1,
		MaxArtifactBytes:  1,
		MaxMemoryBytes:    1,
		MaxProcesses:      1,
		RequiredCapabilities: UntrustedCodeIsolationCapabilities |
			CapabilityMemory |
			CapabilityProcesses |
			CapabilityStorage,
	}
	if err := validateRequiredCapabilityContract(config); err != nil {
		t.Fatalf("complete capability contract was rejected: %v", err)
	}
}

func TestUntrustedCodeIsolationCapabilitiesAreExplicit(t *testing.T) {
	want := CapabilityFilesystem | CapabilityNetwork | CapabilityProcessContainment | CapabilityOutput
	if UntrustedCodeIsolationCapabilities != want {
		t.Fatalf("UntrustedCodeIsolationCapabilities = %s, want %s", UntrustedCodeIsolationCapabilities, want)
	}
	if got := CapabilityProcessContainment.String(); got != "process-containment" {
		t.Fatalf("CapabilityProcessContainment.String() = %q, want process-containment", got)
	}
}

func TestTrustedBuildIsolationCapabilitiesAreExplicit(t *testing.T) {
	want := CapabilityFilesystem | CapabilityNetwork | CapabilityProcessCreation | CapabilityOutput
	if TrustedBuildIsolationCapabilities != want {
		t.Fatalf("TrustedBuildIsolationCapabilities = %s, want %s", TrustedBuildIsolationCapabilities, want)
	}
	if TrustedBuildIsolationCapabilities.Has(CapabilityProcessContainment) {
		t.Fatal("trusted build capabilities must not claim process containment")
	}
	if got := CapabilityProcessCreation.String(); got != "process-creation" {
		t.Fatalf("CapabilityProcessCreation.String() = %q, want process-creation", got)
	}
}

func TestLimitReportExposesOnlyProcessContainment(t *testing.T) {
	reportType := reflect.TypeOf(LimitReport{})
	if _, ok := reportType.FieldByName("ProcessContainment"); !ok {
		t.Fatal("LimitReport.ProcessContainment is missing")
	}
	if _, ok := reportType.FieldByName("ProcessTree"); ok {
		t.Fatal("legacy LimitReport process lifecycle field must remain removed")
	}
}

func TestSandboxConfigKeepsUnrequestedDoSLimitsDisabled(t *testing.T) {
	config, err := validateSandboxConfigSemantics(Config{
		Workspace:            filepath.Join(t.TempDir(), "workspace"),
		RequiredCapabilities: UntrustedCodeIsolationCapabilities,
	})
	if err != nil {
		t.Fatal(err)
	}
	if config.MaxMemoryBytes != 0 || config.MaxProcesses != 0 ||
		config.MaxWorkspaceBytes != 0 || config.MaxArtifactBytes != 0 {
		t.Fatalf("unrequested DoS limits were injected: %+v", config)
	}
	if config.MaxOutputBytes <= 0 || config.MaxStderrBytes <= 0 {
		t.Fatalf("safe output defaults were not applied: %+v", config)
	}
}

func TestNewValidatesPurePathSemanticsBeforeWorkspaceMutation(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	_, err := New(Config{
		Workspace:            workspace,
		DeniedPaths:          []string{"relative/path"},
		RequiredCapabilities: UntrustedCodeIsolationCapabilities,
	})
	if err == nil {
		t.Fatal("New() accepted a relative denied path")
	}
	if _, statErr := os.Stat(workspace); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("workspace state after rejected path semantics = %v, want not created", statErr)
	}
}

func TestNewRequiresOutputCapabilityForSafeDefaultBeforeWorkspaceMutation(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	_, err := New(Config{
		Workspace:            workspace,
		RequiredCapabilities: CapabilityFilesystem,
	})
	if !errors.Is(err, ErrInvalidCapabilityContract) {
		t.Fatalf("New() error = %v, want ErrInvalidCapabilityContract", err)
	}
	if _, statErr := os.Stat(workspace); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("workspace state after rejected output contract = %v, want not created", statErr)
	}
}

func TestNewRequiresCapabilitiesForExplicitResourceLimits(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		want   CapabilitySet
	}{
		{
			name:   "memory",
			config: Config{MaxMemoryBytes: 1024},
			want:   CapabilityMemory,
		},
		{
			name:   "processes",
			config: Config{MaxProcesses: 2},
			want:   CapabilityProcesses,
		},
		{
			name:   "workspace storage",
			config: Config{MaxWorkspaceBytes: 1024},
			want:   CapabilityStorage,
		},
		{
			name:   "artifact storage",
			config: Config{MaxArtifactBytes: 1024},
			want:   CapabilityStorage,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workspace := filepath.Join(t.TempDir(), "workspace")
			config := test.config
			config.Workspace = workspace
			config.RequiredCapabilities = UntrustedCodeIsolationCapabilities

			_, err := New(config)
			if !errors.Is(err, ErrInvalidCapabilityContract) {
				t.Fatalf("New() error = %v, want ErrInvalidCapabilityContract", err)
			}
			if !strings.Contains(err.Error(), test.want.String()) {
				t.Fatalf("New() error = %q, want missing capability %q", err, test.want)
			}
			if _, statErr := os.Stat(workspace); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("workspace state after rejected configuration = %v, want not created", statErr)
			}
		})
	}
}

func (backend *capabilityRecordingBackend) sandboxCapabilities(context.Context) (CapabilitySet, error) {
	return backend.available, nil
}

func (backend *capabilityRecordingBackend) Exec(context.Context, Command) (*ExecResult, error) {
	backend.executed = true
	return &ExecResult{}, nil
}

func (backend *capabilityRecordingBackend) Close() error {
	return nil
}

func TestSandboxConfigExposesTypedRequiredCapabilities(t *testing.T) {
	configType := reflect.TypeOf(Config{})
	field, ok := configType.FieldByName("RequiredCapabilities")
	if !ok {
		t.Fatal("Config.RequiredCapabilities is missing")
	}
	if field.Type.Name() != "CapabilitySet" {
		t.Fatalf("Config.RequiredCapabilities type = %s, want CapabilitySet", field.Type)
	}
}

func TestSandboxConfigUsesExplicitNetworkMode(t *testing.T) {
	configType := reflect.TypeOf(Config{})
	field, ok := configType.FieldByName("Network")
	if !ok {
		t.Fatal("Config.Network is missing")
	}
	if field.Type.Name() != "NetworkMode" {
		t.Fatalf("Config.Network type = %s, want NetworkMode", field.Type)
	}
	if _, exists := configType.FieldByName("DenyLoopback"); exists {
		t.Fatal("Config.DenyLoopback must be removed because no backend proves this policy consistently")
	}
}

func TestNormalizeSandboxWorkspaceRejectsRootSymlink(t *testing.T) {
	realWorkspace := t.TempDir()
	link := filepath.Join(t.TempDir(), "workspace-link")
	if err := os.Symlink(realWorkspace, link); err != nil {
		t.Fatal(err)
	}
	if _, err := normalizeSandboxWorkspace(link); err == nil {
		t.Fatal("workspace root symlink was accepted")
	}
}

func TestNormalizeSandboxWorkspacePreservesRawAbsoluteIdentity(t *testing.T) {
	rawWorkspace := filepath.Join(t.TempDir(), "workspace")
	want, err := filepath.Abs(rawWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	got, err := normalizeSandboxWorkspace(rawWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Clean(want) {
		t.Fatalf("normalized workspace = %q, want raw absolute identity %q", got, filepath.Clean(want))
	}
}

func TestPrepareSandboxCommandPreservesDirectoryPathIdentity(t *testing.T) {
	workspace := t.TempDir()
	directory := filepath.Join(workspace, "project")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	prepared, err := prepareSandboxCommand(
		Config{Workspace: workspace},
		Command{Path: "/usr/bin/true", Dir: directory, Env: []string{}},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.Abs(directory)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Dir != filepath.Clean(want) {
		t.Fatalf("prepared command directory = %q, want raw absolute identity %q", prepared.Dir, filepath.Clean(want))
	}
}

func TestPrepareSandboxCommandAcceptsEquivalentCanonicalDirectoryIdentity(t *testing.T) {
	workspace := t.TempDir()
	directory := filepath.Join(workspace, "project")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	canonicalDirectory, err := filepath.EvalSymlinks(directory)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(canonicalDirectory) == filepath.Clean(directory) {
		t.Skip("test path has no canonical alias")
	}
	prepared, err := prepareSandboxCommand(
		Config{Workspace: workspace},
		Command{Path: "/usr/bin/true", Dir: canonicalDirectory, Env: []string{}},
		nil,
	)
	if err != nil {
		t.Fatalf("equivalent canonical directory was rejected: %v", err)
	}
	if prepared.Dir != filepath.Clean(canonicalDirectory) {
		t.Fatalf("prepared directory = %q, want caller identity %q", prepared.Dir, filepath.Clean(canonicalDirectory))
	}
}

func TestCapabilitySandboxRejectsMissingCapabilitiesBeforePayload(t *testing.T) {
	workspace, identity, err := snapshotSandboxWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	backend := &capabilityRecordingBackend{available: CapabilityOutput}
	sandboxInstance := &capabilitySandbox{
		backend: backend,
		cfg: Config{
			Workspace:            workspace,
			RequiredCapabilities: CapabilityFilesystem | CapabilityProcessContainment,
			workspaceIdentity:    identity,
		},
	}
	_, err = sandboxInstance.Exec(context.Background(), Command{Path: "/usr/bin/true"})
	if !errors.Is(err, ErrRequiredCapabilitiesUnavailable) {
		t.Fatalf("Exec() error = %v, want ErrRequiredCapabilitiesUnavailable", err)
	}
	if backend.executed {
		t.Fatal("payload executed before required capabilities were accepted")
	}
}

func TestCapabilitySandboxExecRejectsMissingCapabilitiesBeforePayload(t *testing.T) {
	workspace, identity, err := snapshotSandboxWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	backend := &capabilityRecordingBackend{available: CapabilityFilesystem | CapabilityOutput}
	sandboxInstance := &capabilitySandbox{
		backend: backend,
		cfg: Config{
			Workspace:            workspace,
			RequiredCapabilities: CapabilityFilesystem | CapabilityProcessContainment | CapabilityOutput,
			workspaceIdentity:    identity,
		},
	}
	_, err = sandboxInstance.Exec(context.Background(), Command{Path: "/usr/bin/true"})
	if !errors.Is(err, ErrRequiredCapabilitiesUnavailable) {
		t.Fatalf("Exec() error = %v, want ErrRequiredCapabilitiesUnavailable", err)
	}
	if backend.executed {
		t.Fatal("payload executed before required capabilities were accepted")
	}
}

func TestCapabilitySandboxAllowsPayloadOnlyAfterCompleteNegotiation(t *testing.T) {
	workspace, identity, err := snapshotSandboxWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	required := CapabilityFilesystem | CapabilityNetwork | CapabilityProcessContainment | CapabilityOutput
	backend := &capabilityRecordingBackend{available: required}
	sandboxInstance := &capabilitySandbox{
		backend: backend,
		cfg: Config{
			Workspace:            workspace,
			RequiredCapabilities: required,
			workspaceIdentity:    identity,
		},
	}
	if _, err := sandboxInstance.Exec(context.Background(), Command{Path: "/usr/bin/true"}); err != nil {
		t.Fatal(err)
	}
	if !backend.executed {
		t.Fatal("payload was not executed after complete capability negotiation")
	}
}

func TestAvailableCapabilitiesUsesSingleCapabilitySetModel(t *testing.T) {
	want := CapabilityFilesystem | CapabilityNetwork | CapabilityOutput
	backend := &capabilityRecordingBackend{available: want}
	got, err := AvailableCapabilities(context.Background(), backend)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("AvailableCapabilities() = %s, want %s", got, want)
	}
	if missing := (CapabilityFilesystem | CapabilityStorage).Missing(got); missing != CapabilityStorage {
		t.Fatalf("missing capabilities = %s, want storage", missing)
	}
}

func TestCapabilitySetRejectsUnknownBits(t *testing.T) {
	unknown := CapabilitySet(1 << 15)
	if err := requireSandboxCapabilities(unknown, unknown); err == nil {
		t.Fatal("unknown capability bits were accepted")
	}
}

func TestSandboxPathIdentityRejectsWorkspaceAndDirectoryReplacement(t *testing.T) {
	workspace, workspaceIdentity, err := snapshotSandboxWorkspace(filepath.Join(t.TempDir(), "workspace"))
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(workspace, "project")
	if mkdirErr := os.Mkdir(directory, 0o700); mkdirErr != nil {
		t.Fatal(mkdirErr)
	}
	command, err := snapshotSandboxCommandPaths(
		Config{Workspace: workspace, workspaceIdentity: workspaceIdentity},
		Command{Path: "/usr/bin/true", Dir: directory},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(directory, directory+"-old"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := revalidateSandboxExecutionPaths(command); err == nil {
		t.Fatal("replaced command directory identity was accepted")
	}
}

func TestSandboxPathIdentityRejectsWorkspaceReplacement(t *testing.T) {
	workspace, workspaceIdentity, err := snapshotSandboxWorkspace(filepath.Join(t.TempDir(), "workspace"))
	if err != nil {
		t.Fatal(err)
	}
	command, err := snapshotSandboxCommandPaths(
		Config{Workspace: workspace, workspaceIdentity: workspaceIdentity},
		Command{Path: "/usr/bin/true"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(workspace, workspace+"-old"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := revalidateSandboxExecutionPaths(command); err == nil {
		t.Fatal("replaced workspace identity was accepted")
	}
}

func TestPrepareSandboxCommandRejectsDirectorySymlink(t *testing.T) {
	workspace := t.TempDir()
	realDirectory := filepath.Join(workspace, "project")
	if err := os.Mkdir(realDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	directoryLink := filepath.Join(workspace, "project-link")
	if err := os.Symlink(realDirectory, directoryLink); err != nil {
		t.Fatal(err)
	}
	_, err := prepareSandboxCommand(
		Config{Workspace: workspace},
		Command{Path: "/usr/bin/true", Dir: directoryLink, Env: []string{}},
		nil,
	)
	if err == nil {
		t.Fatal("command directory symlink was accepted")
	}
}
