package cicheck

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseTagsTriggerRequiredValidationWorkflows(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"ci.yml", "downstream.yml", "sandbox-code-exec.yml", "api-compat.yml"} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, document := loadWorkflowContract(t, name)
			push, ok := document.triggers["push"]
			if !ok || !containsExact(push.tags, "v*") {
				t.Fatalf("%s must run for v* tag pushes", name)
			}
		})
	}
}

func TestAPICompatibilityGateIsHardAndAuditable(t *testing.T) {
	t.Parallel()

	source, document := loadWorkflowContract(t, "api-compat.yml")
	active := activeYAMLContract(source)
	// 结构合同用固定串断言；版本号只断言 SemVer 格式（发版升级版本号无需
	// 修改测试），baseline 文件名断言与主版本前缀一致。
	// 结构合同用固定串断言；版本号不再硬编码——tag 名只校验 SemVer 格式，
	// API 对比以最新已发布 tag 为动态 base，保证每次发版无需修改测试与 workflow。
	for _, required := range []string{
		`API_BREAKING_BASELINE: ".github/workflows/api-breaking-v0.3.0.txt"`,
		`API_BREAKING_CHECKER: ".github/workflows/check-api-breaking.py"`,
		`gorelease -base="$base_version"`,
		`python3 "$API_BREAKING_CHECKER" "$report_path" "$API_BREAKING_BASELINE" --allow-empty`,
		`=~ ^v[0-9]+\.[0-9]+\.[0-9]+$`,
	} {
		if !strings.Contains(active, required) {
			t.Fatalf("API compatibility gate is missing %q", required)
		}
	}
	if strings.Contains(active, "gorelease ||") {
		t.Fatal("API compatibility gate must not swallow gorelease failures")
	}

	job, ok := document.jobs["gorelease"]
	if !ok {
		t.Fatal("API compatibility workflow is missing the gorelease job")
	}
	// 关键不变量：gorelease 必须执行且带 -base 参数。参数形式（-version 是否
	// 存在、if 分支结构）随 tag/PR 场景合法变化，不作为合同断言，保证版本
	// 迭代与流程修复不触发门禁误伤。
	matched := 0
	for _, step := range job.steps {
		commands, err := parseShell(step.run)
		if err != nil {
			t.Fatalf("parse API compatibility step %q: %v", step.name, err)
		}
		for _, command := range commands {
			executable, arguments := shellExecutable(command.words)
			if filepath.Base(executable) != "gorelease" {
				continue
			}
			matched++
			if !hasArgumentPrefix(arguments, "-base=") {
				t.Fatalf("gorelease must receive an explicit base version: %v", arguments)
			}
		}
	}
	if matched != 1 {
		t.Fatalf("API compatibility gate must execute gorelease exactly once, got %d", matched)
	}

	checker := readContractFile(t, filepath.Join(repositoryRoot(t), ".github", "workflows", "check-api-breaking.py"))
	requireContractText(t, "breaking API checker", checker,
		"def extract_breaking_sections",
		"difflib.unified_diff",
		"if actual != expected:",
		"Unapproved breaking API changes detected.",
	)

	baseline := readContractFile(t, filepath.Join(repositoryRoot(t), ".github", "workflows", "api-breaking-v0.3.0.txt"))
	if strings.Count(baseline, "## incompatible changes") != 34 {
		t.Fatalf("v0.3.0 breaking API baseline must contain exactly 34 package sections")
	}
	if strings.Contains(baseline, "## compatible changes") || strings.Contains(baseline, "# summary") {
		t.Fatal("breaking API baseline must contain only incompatible change sections")
	}
	seenPackages := make(map[string]struct{})
	for _, section := range strings.Split(strings.TrimSuffix(baseline, "\n"), "\n\n") {
		lines := strings.Split(section, "\n")
		if len(lines) < 3 || !strings.HasPrefix(lines[0], "# github.com/hexagon-codes/toolkit/") || lines[1] != "## incompatible changes" {
			t.Fatalf("invalid breaking API baseline section: %q", section)
		}
		if _, exists := seenPackages[lines[0]]; exists {
			t.Fatalf("duplicate breaking API baseline package: %s", lines[0])
		}
		seenPackages[lines[0]] = struct{}{}
	}
}

func TestRootCIReleaseAndPlatformContract(t *testing.T) {
	t.Parallel()

	source, document := loadWorkflowContract(t, "ci.yml")
	if _, ok := document.triggers["schedule"]; !ok {
		t.Fatal("CI must schedule govulncheck independently of repository changes")
	}
	for name, job := range document.jobs {
		if name == "vuln" {
			continue
		}
		if normalizeExpression(job.ifExpr) != "github.event_name != 'schedule'" {
			t.Fatalf("scheduled CI must run only the vulnerability job; %s is not gated", name)
		}
	}

	testJob := activeYAMLContract(extractJobContract(t, source, "test"))
	requireContractText(t, "native root module job", testJob,
		"go mod tidy -diff",
		"go build -mod=readonly ./...",
		"go vet -mod=readonly ./...",
		"go test -mod=readonly -count=1 ./...",
		"go test -mod=readonly -race -count=1",
		"if: runner.os != 'Windows'",
		"if: runner.os == 'Windows'",
		"run: go test -mod=readonly -race -count=1 ./...",
		"git diff --exit-code",
		"os: ubuntu-22.04",
		"os: macos-14",
		"os: windows-2022",
	)

	windowsJob := activeYAMLContract(extractJobContract(t, source, "windows-sandbox-security"))
	requireContractText(t, "native Windows security job", windowsJob,
		"go test -race -tags windows_security -json",
		"go vet -tags windows_security ./os/sandbox",
		"$gitConfigExitCode = $LASTEXITCODE",
		"$gitConfigExitCode -ne 0 -and $gitConfigExitCode -ne 1",
		"$global:LASTEXITCODE = 0",
		"TestWindowsNativePostCreationFaultOwnership",
		"TestWindowsNativePostCreationFaultCloseRetriesQuarantine",
		"TestWindowsNativeRetainExecutionOwnership",
		"TestWindowsLaunchFailureTransfersOwnershipToQuarantine",
		"TestWindowsProcessQuarantineRetriesBoundedlyAndPreservesErrors",
		"TestWindowsProcessQuarantineRetainsUnconfirmedResources",
		"TestWindowsSandboxCloseRetriesAndReclaimsQuarantinedLaunch",
		"TestWindowsSandboxCloseKeepsWorkspaceProtectedUntilRetriedReclaim",
	)

	linuxRootJob := activeYAMLContract(extractJobContract(t, source, "linux-root-sandbox-security"))
	requireContractText(t, "native root Linux security job", linuxRootJob,
		"TestLinuxRootBwrapHidesHostUnixSocket",
		"TestLinuxBwrapHidesExternalWorkspaceWithoutPrivilege",
		"root_events=",
		`\"Action\":\"skip\"`,
		`\"Action\":\"pass\"`,
	)

	macOSJob := activeYAMLContract(extractJobContract(t, source, "test"))
	for _, testName := range []string{
		"TestDarwinTrustedBuildProfileAllowsChildrenWithoutClaimingContainment",
		"TestDarwinTrustedBuildRunsFrozenGoCompilerChild",
		"TestDarwinRejectsProcessCreationAndContainmentBeforePayload",
	} {
		if !strings.Contains(macOSJob, testName) {
			t.Fatalf("native macOS no-skip gate is missing %s", testName)
		}
	}

	examplesJob := activeYAMLContract(extractJobContract(t, source, "examples"))
	requireContractText(t, "examples job", examplesJob,
		"runs-on: ${{ matrix.os }}",
		"- ubuntu-24.04",
		"- macos-14",
		"- windows-2022",
		"GOWORK=off go mod tidy -diff",
		"./scripts/verify-examples.sh",
		"git diff --exit-code",
	)

	lintJob := activeYAMLContract(extractJobContract(t, source, "lint"))
	requireContractText(t, "cross-platform lint job", lintJob,
		"runs-on: ${{ matrix.os }}",
		"- ubuntu-24.04",
		"- macos-14",
		"- windows-2022",
	)

	workflowLint := activeYAMLContract(extractJobContract(t, source, "workflow-lint"))
	if !strings.Contains(workflowLint, `glob("*.yml")`) || !strings.Contains(workflowLint, `glob("*.yaml")`) {
		t.Fatal("workflow action SHA scan must inspect both .yml and .yaml files")
	}
}

func TestSandboxCodeExecFocusedTestsCannotPassWithZeroOrSkippedTests(t *testing.T) {
	t.Parallel()

	source, _ := loadWorkflowContract(t, "sandbox-code-exec.yml")
	job := activeYAMLContract(extractJobContract(t, source, "sandbox-code-exec"))
	requireContractText(t, "sandbox code_exec job", job,
		"go test -list",
		"go test -json",
		`\"Action\":\"skip\"`,
		`\"Action\":\"pass\"`,
		`if [[ "$RUNNER_OS" == "Windows" ]]`,
		"TestCodeExecSkill_ToolDefinitionP0Fields",
		"TestCodeExecSkill_Execute_Success",
		"TestCodeExecSkill_Execute_ModuleGoFiles",
		"TestSandboxP0_StaticGapMatrix",
		"TestBug20260626_CodeExecReadsConnectorAuthorizedDir",
		"TestCodeExecSkill_Execute_PythonCrawlerNetworkPolicy",
	)
}

func TestIntegrationScriptCollectsAndRequiresEveryMySQLLifecycleTest(t *testing.T) {
	t.Parallel()

	script := readContractFile(t, filepath.Join(repositoryRoot(t), "scripts", "verify-integrations.sh"))
	if strings.Contains(script, "TestQueryRowWithTimeout_") {
		t.Fatal("integration filter still references the nonexistent TestQueryRowWithTimeout_ prefix")
	}
	requireContractText(t, "MySQL integration script", script,
		"TestIntegration_BasicOperations",
		"TestIntegration_Transaction",
		"TestIntegration_Health",
		"TestIntegration_Stats",
		"TestIntegration_Timeout",
		"TestTransaction_Panic",
		"TestStats_NonNilDB",
		"TestExecWithTimeout_Success",
		"TestQueryWithTimeout_Success",
		"TestClose_ValidDB",
		"go test -list",
		"go test -json",
		`event.get("Action") == "skip"`,
		`event.get("Action") == "pass"`,
	)
}

func TestDownstreamContractCoversEveryModuleBoundary(t *testing.T) {
	t.Parallel()

	source, _ := loadWorkflowContract(t, "downstream.yml")
	ecosystem := activeYAMLContract(extractJobContract(t, source, "downstream"))
	requireContractText(t, "Go 1.25 ecosystem downstream job", ecosystem,
		"./hexagon/examples",
		"working-directory: hexagon/examples",
		"go build -mod=readonly ./...",
		"go test -mod=readonly -count=1 ./...",
	)
}

func loadWorkflowContract(t *testing.T, name string) (string, workflowDocument) {
	t.Helper()

	path := filepath.Join(repositoryRoot(t), ".github", "workflows", name)
	source := readContractFile(t, path)
	document, err := parseWorkflow(source)
	if err != nil {
		t.Fatalf("parse workflow %s: %v", name, err)
	}
	return source, document
}

// activeYAMLContract 去掉 YAML 注释，避免注释文本满足发布合同。
func activeYAMLContract(source string) string {
	lines := strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n")
	active := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimRight(stripYAMLComment(line), " \t")
		if strings.TrimSpace(line) != "" {
			active = append(active, line)
		}
	}
	return strings.Join(active, "\n")
}

// extractJobContract 按 jobs 下的二级键提取单个任务，避免其他任务替它满足合同。
func extractJobContract(t *testing.T, source, name string) string {
	t.Helper()

	lines := strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n")
	start := -1
	for index, line := range lines {
		if leadingSpaces(line) == 2 && strings.TrimSpace(stripYAMLComment(line)) == name+":" {
			start = index
			break
		}
	}
	if start < 0 {
		t.Fatalf("workflow job is missing: %s", name)
	}
	end := len(lines)
	for index := start + 1; index < len(lines); index++ {
		line := lines[index]
		if strings.TrimSpace(stripYAMLComment(line)) != "" && leadingSpaces(line) == 2 {
			end = index
			break
		}
	}
	return strings.Join(lines[start:end], "\n")
}

func requireContractText(t *testing.T, label, source string, required ...string) {
	t.Helper()

	for _, value := range required {
		if !strings.Contains(source, value) {
			t.Errorf("%s is missing %q", label, value)
		}
	}
}

func hasArgumentPrefix(arguments []string, prefix string) bool {
	for _, argument := range arguments {
		if strings.HasPrefix(argument, prefix) {
			return true
		}
	}
	return false
}
