package cicheck

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

const (
	bubblewrapVersion = "0.11.2"
	bubblewrapSHA256  = "69abc30005d2186baf7737feacd8da35633b93cf5af38838ecff17c5f8e924f6"
	installerRootPath = "./scripts/install-bubblewrap-ci.sh"
	installerSubPath  = "./toolkit/scripts/install-bubblewrap-ci.sh"
)

type workflowDocument struct {
	triggers map[string]workflowTrigger
	jobs     map[string]workflowJob
}

type workflowTrigger struct {
	paths []string
	tags  []string
}

type workflowJob struct {
	ifExpr string
	runsOn string
	steps  []workflowStep
}

type workflowStep struct {
	name   string
	uses   string
	ifExpr string
	shell  string
	run    string
}

type installerRequirement struct {
	workflow string
	job      string
	command  string
	ifExpr   string
}

type shellCommand struct {
	words          []string
	depth          int
	function       string
	operatorBefore string
	operatorAfter  string
}

type shellSegment struct {
	words          []string
	operatorBefore string
	operatorAfter  string
}

type shellBlock struct {
	kind     string
	function string
}

func TestBubblewrapWorkflowUsesSharedPinnedInstaller(t *testing.T) {
	t.Parallel()

	workflows := readWorkflowFiles(t, repositoryRoot(t))
	if err := validateBubblewrapWorkflows(workflows); err != nil {
		t.Fatal(err)
	}
}

func TestBubblewrapInstallerSecurityContract(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	path := filepath.Join(root, "scripts", "install-bubblewrap-ci.sh")
	body := readContractFile(t, path)
	// Windows 工作树不保留 exec 位（core.filemode=false），改查 git index 的
	// 文件模式（跨平台一致），确认安装器在仓库中登记为可执行。
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat Bubblewrap installer: %v", err)
		}
		if info.Mode().Perm()&0o111 == 0 {
			t.Fatal("Bubblewrap installer must be executable")
		}
	}
	if err := validateInstallerIndexMode(path); err != nil {
		t.Fatal(err)
	}
	if err := validateBubblewrapInstaller(body); err != nil {
		t.Fatal(err)
	}
}

func TestBubblewrapWorkflowContractRejectsCommentOnlyInstaller(t *testing.T) {
	t.Parallel()

	const source = `jobs:
  test:
    steps:
      - uses: actions/checkout@0000000000000000000000000000000000000000
      - name: Install Linux sandbox dependencies
        if: runner.os == 'Linux'
        run: ./scripts/install-bubblewrap-ci.sh
`
	requirement := installerRequirement{
		workflow: "fixture.yml",
		job:      "test",
		command:  installerRootPath,
		ifExpr:   "runner.os == 'Linux'",
	}
	document, err := parseWorkflow(source)
	if err != nil {
		t.Fatalf("parse valid workflow fixture: %v", err)
	}
	if _, contractErr := requireInstallerStep(document.jobs["test"], requirement); contractErr != nil {
		t.Fatalf("valid workflow fixture must satisfy installer contract: %v", contractErr)
	}
	mutated, ok := replaceExactlyOnce(
		source,
		"run: "+installerRootPath,
		"run: /bin/true # "+installerRootPath,
	)
	if !ok {
		t.Fatal("prepare workflow mutation: installer run node not found exactly once")
	}
	document, err = parseWorkflow(mutated)
	if err != nil {
		t.Fatalf("parse mutated workflow fixture: %v", err)
	}
	if _, err := requireInstallerStep(document.jobs["test"], requirement); err == nil {
		t.Fatal("workflow contract accepted an installer path present only in a YAML comment")
	}
}

func TestBubblewrapWorkflowParserDoesNotTreatOtherTriggerListsAsPaths(t *testing.T) {
	t.Parallel()

	const source = `on:
  push:
    branches:
      - scripts/install-bubblewrap-ci.sh
    paths:
      - go.mod
jobs:
  test:
    steps:
      - run: /bin/true
`
	document, err := parseWorkflow(source)
	if err != nil {
		t.Fatalf("parse trigger fixture: %v", err)
	}
	if containsExact(document.triggers["push"].paths, "scripts/install-bubblewrap-ci.sh") {
		t.Fatal("workflow parser treated a branches entry as a paths entry")
	}
}

func TestBubblewrapInstallerContractRejectsCommentOnlySafetyFlags(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		source   string
		old      string
		mutated  string
		comment  string
		validate func([]shellCommand) error
	}{
		"setuid": {
			source: `build_bubblewrap() {
  /usr/bin/meson setup "$1" "$source_dir" --buildtype=debugoptimized -Db_ndebug=false -Dman=disabled -Dselinux=disabled -Dsupport_setuid=false -Dtests=false "-Dc_args=-ffile-prefix-map=${source_dir}=/usr/src/toolkit-bubblewrap/source -fdebug-prefix-map=${source_dir}=/usr/src/toolkit-bubblewrap/source -ffile-prefix-map=${output_dir}=/usr/src/toolkit-bubblewrap/build -fdebug-prefix-map=${output_dir}=/usr/src/toolkit-bubblewrap/build"
  /usr/bin/meson compile -C "$1"
}`,
			old:      "-Dsupport_setuid=false",
			mutated:  "-Dsupport_setuid=true # -Dsupport_setuid=false",
			validate: validateMesonBuildFunction,
		},
		"curl_config": {
			source:   `/usr/bin/curl --disable --fail --proto '=https' --proto-redir '=https' "$BUBBLEWRAP_URL"`,
			old:      "/usr/bin/curl --disable",
			mutated:  "/usr/bin/curl --fail # /usr/bin/curl --disable",
			validate: validateCurlCommand,
		},
		"prefix_map": {
			source: `build_bubblewrap() {
  /usr/bin/meson setup "$1" "$source_dir" --buildtype=debugoptimized -Db_ndebug=false -Dman=disabled -Dselinux=disabled -Dsupport_setuid=false -Dtests=false "-Dc_args=-ffile-prefix-map=${source_dir}=/usr/src/toolkit-bubblewrap/source -fdebug-prefix-map=${source_dir}=/usr/src/toolkit-bubblewrap/source -ffile-prefix-map=${output_dir}=/usr/src/toolkit-bubblewrap/build -fdebug-prefix-map=${output_dir}=/usr/src/toolkit-bubblewrap/build"
  /usr/bin/meson compile -C "$1"
}`,
			old:      "-ffile-prefix-map=${source_dir}=",
			mutated:  "-fno-file-prefix-map=${source_dir}=",
			comment:  "-ffile-prefix-map=${source_dir}=",
			validate: validateMesonBuildFunction,
		},
	}

	for name, test := range tests {
		name, test := name, test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			commands, err := parseShell(test.source)
			if err != nil {
				t.Fatalf("parse valid %s shell fixture: %v", name, err)
			}
			if validationErr := test.validate(commands); validationErr != nil {
				t.Fatalf("valid %s shell fixture must satisfy contract: %v", name, validationErr)
			}
			mutated, ok := replaceExactlyOnce(test.source, test.old, test.mutated)
			if !ok {
				t.Fatalf("prepare %s mutation: target not found exactly once", name)
			}
			if test.comment != "" {
				mutated += "\n# " + test.comment + "\n"
			}
			commands, err = parseShell(mutated)
			if err != nil {
				t.Fatalf("parse mutated %s shell fixture: %v", name, err)
			}
			if err := test.validate(commands); err == nil {
				t.Fatalf("installer contract accepted %s safety syntax present only in a shell comment", name)
			}
		})
	}
}

func TestBubblewrapInstallerContractRejectsDeadReproducibilityBuild(t *testing.T) {
	t.Parallel()

	const body = `verify_reproducible_binary() {
  /usr/bin/sha256sum "$1"
  /usr/bin/sha256sum "$2"
}
build_bubblewrap "$primary_output_dir"
build_bubblewrap "$verification_output_dir"
verify_reproducible_binary "$primary_output_dir/bwrap" "$verification_output_dir/bwrap"
`
	commands, err := parseShell(body)
	if err != nil {
		t.Fatalf("parse valid reproducibility fixture: %v", err)
	}
	if validationErr := validateReproducibilityCalls(commands); validationErr != nil {
		t.Fatalf("valid reproducibility fixture must satisfy contract: %v", validationErr)
	}
	mutated, ok := replaceExactlyOnce(
		body,
		"build_bubblewrap \"$verification_output_dir\"",
		"if false; then\n  build_bubblewrap \"$verification_output_dir\"\nfi",
	)
	if !ok {
		t.Fatal("prepare reproducibility mutation: verification build call not found exactly once")
	}
	commands, err = parseShell(mutated)
	if err != nil {
		t.Fatalf("parse mutated reproducibility fixture: %v", err)
	}
	if err := validateReproducibilityCalls(commands); err == nil {
		t.Fatal("installer contract accepted a reproducibility build hidden in dead control flow")
	}
}

func TestBubblewrapWorkflowContractRejectsShortCircuitedInstaller(t *testing.T) {
	t.Parallel()

	const source = `jobs:
  test:
    steps:
      - uses: actions/checkout@0000000000000000000000000000000000000000
      - name: Install Linux sandbox dependencies
        if: runner.os == 'Linux'
        run: /usr/bin/true || ./scripts/install-bubblewrap-ci.sh
`
	requirement := installerRequirement{
		workflow: "fixture.yml",
		job:      "test",
		command:  installerRootPath,
		ifExpr:   "runner.os == 'Linux'",
	}
	document, err := parseWorkflow(source)
	if err != nil {
		t.Fatalf("parse workflow fixture: %v", err)
	}
	if _, err := requireInstallerStep(document.jobs["test"], requirement); err == nil {
		t.Fatal("workflow contract accepted an installer skipped by shell short-circuiting")
	}
}

func TestBubblewrapInstallerContractRejectsShortCircuitedReproducibility(t *testing.T) {
	t.Parallel()

	body := readContractFile(t, filepath.Join(repositoryRoot(t), "scripts", "install-bubblewrap-ci.sh"))
	mutated, ok := replaceExactlyOnce(
		body,
		"build_bubblewrap \"$verification_output_dir\"",
		"/usr/bin/true || build_bubblewrap \"$verification_output_dir\"",
	)
	if !ok {
		t.Fatal("prepare installer mutation: verification build call not found exactly once")
	}
	mutated, ok = replaceExactlyOnce(
		mutated,
		"verify_reproducible_binary \"$primary_output_dir/bwrap\" \"$verification_output_dir/bwrap\"",
		"/usr/bin/true || verify_reproducible_binary \"$primary_output_dir/bwrap\" \"$verification_output_dir/bwrap\"",
	)
	if !ok {
		t.Fatal("prepare installer mutation: reproducibility verification call not found exactly once")
	}
	if err := validateBubblewrapInstaller(mutated); err == nil {
		t.Fatal("installer contract accepted short-circuited reproducibility commands")
	}
}

func TestBubblewrapInstallerContractRequiresArchiveChecksumVerification(t *testing.T) {
	t.Parallel()

	body := readContractFile(t, filepath.Join(repositoryRoot(t), "scripts", "install-bubblewrap-ci.sh"))
	const checksumCommand = "/usr/bin/printf '%s  %s\\n' \"$BUBBLEWRAP_SHA256\" \"$archive\" | /usr/bin/sha256sum --check --strict -\n"
	mutated, ok := replaceExactlyOnce(body, checksumCommand, "")
	if !ok {
		t.Fatal("prepare installer mutation: archive checksum command not found exactly once")
	}
	if err := validateBubblewrapInstaller(mutated); err == nil {
		t.Fatal("installer contract accepted a download without archive checksum verification")
	}
}

func TestBubblewrapInstallerContractRequiresHashMismatchFailure(t *testing.T) {
	t.Parallel()

	body := readContractFile(t, filepath.Join(repositoryRoot(t), "scripts", "install-bubblewrap-ci.sh"))
	mutated, ok := replaceExactlyOnce(
		body,
		"if [[ \"$primary_hash\" != \"$verification_hash\" ]]; then",
		"if /usr/bin/false; then",
	)
	if !ok {
		t.Fatal("prepare installer mutation: hash comparison not found exactly once")
	}
	if err := validateBubblewrapInstaller(mutated); err == nil {
		t.Fatal("installer contract accepted a disabled reproducibility hash comparison")
	}
}

func TestReadWorkflowFilesIncludesYAMLAndYML(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	workflowDir := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(workflowDir, 0o700); err != nil {
		t.Fatalf("create workflow directory: %v", err)
	}
	for _, name := range []string{"primary.yml", "secondary.yaml"} {
		if err := os.WriteFile(filepath.Join(workflowDir, name), []byte("jobs: {}\n"), 0o600); err != nil {
			t.Fatalf("write workflow fixture %s: %v", name, err)
		}
	}
	workflows := readWorkflowFiles(t, root)
	for _, path := range []string{".github/workflows/primary.yml", ".github/workflows/secondary.yaml"} {
		if _, ok := workflows[path]; !ok {
			t.Errorf("workflow scan omitted %s", path)
		}
	}
}

func validateBubblewrapWorkflows(sources map[string]string) error {
	documents := make(map[string]workflowDocument, len(sources))
	for path, source := range sources {
		document, err := parseWorkflow(source)
		if err != nil {
			return fmt.Errorf("parse workflow %s: %w", path, err)
		}
		documents[path] = document
	}

	requirements := []installerRequirement{
		{workflow: ".github/workflows/ci.yml", job: "test", command: installerRootPath, ifExpr: "runner.os == 'Linux'"},
		{workflow: ".github/workflows/ci.yml", job: "linux-root-sandbox-security", command: installerRootPath},
		{workflow: ".github/workflows/ci.yml", job: "toolchain-compatibility", command: installerRootPath},
		{workflow: ".github/workflows/sandbox-code-exec.yml", job: "sandbox-code-exec", command: installerSubPath, ifExpr: "runner.os == 'Linux'"},
		{workflow: ".github/workflows/downstream.yml", job: "downstream", command: installerSubPath},
	}
	for _, requirement := range requirements {
		document, ok := documents[requirement.workflow]
		if !ok {
			return fmt.Errorf("required workflow is missing: %s", requirement.workflow)
		}
		job, ok := document.jobs[requirement.job]
		if !ok {
			return fmt.Errorf("required workflow job is missing: %s:%s", requirement.workflow, requirement.job)
		}
		index, err := requireInstallerStep(job, requirement)
		if err != nil {
			return err
		}
		if !hasCheckoutBefore(job.steps, index) {
			return fmt.Errorf("installer must run after checkout: %s:%s", requirement.workflow, requirement.job)
		}
	}

	sandbox := documents[".github/workflows/sandbox-code-exec.yml"]
	for _, event := range []string{"push", "pull_request"} {
		trigger, ok := sandbox.triggers[event]
		if !ok {
			return fmt.Errorf("sandbox workflow trigger is missing: %s", event)
		}
		if !containsExact(trigger.paths, "scripts/install-bubblewrap-ci.sh") {
			return fmt.Errorf("sandbox workflow %s paths must include scripts/install-bubblewrap-ci.sh", event)
		}
	}

	for path, document := range documents {
		for jobName, job := range document.jobs {
			for _, step := range job.steps {
				if !isShellWorkflowStep(step) {
					continue
				}
				commands, err := parseShell(step.run)
				if err != nil {
					return fmt.Errorf("parse run step %s:%s:%s: %w", path, jobName, step.name, err)
				}
				for _, command := range commands {
					executable, arguments := shellExecutable(command.words)
					base := filepath.Base(executable)
					if (base == "apt" || base == "apt-get") && containsExact(arguments, "bubblewrap") {
						return fmt.Errorf("workflow must not install the distribution Bubblewrap package: %s:%s:%s", path, jobName, step.name)
					}
				}
			}
		}
	}
	return nil
}

func requireInstallerStep(job workflowJob, requirement installerRequirement) (int, error) {
	indices := make([]int, 0, 1)
	for index, step := range job.steps {
		if !isShellWorkflowStep(step) {
			continue
		}
		commands, err := parseShell(step.run)
		if err != nil {
			return -1, fmt.Errorf("parse installer step %s:%s: %w", requirement.workflow, requirement.job, err)
		}
		for _, command := range commands {
			executable, arguments := shellExecutable(command.words)
			if isUnconditionalTopLevel(command) && executable == requirement.command && len(arguments) == 0 {
				indices = append(indices, index)
			}
		}
	}
	if len(indices) != 1 {
		return -1, fmt.Errorf("workflow job must invoke the pinned installer exactly once: %s:%s got %d", requirement.workflow, requirement.job, len(indices))
	}
	step := job.steps[indices[0]]
	if normalizeExpression(step.ifExpr) != normalizeExpression(requirement.ifExpr) {
		return -1, fmt.Errorf("unexpected installer condition in %s:%s: got %q want %q", requirement.workflow, requirement.job, step.ifExpr, requirement.ifExpr)
	}
	return indices[0], nil
}

func hasCheckoutBefore(steps []workflowStep, index int) bool {
	for _, step := range steps[:index] {
		if strings.HasPrefix(step.uses, "actions/checkout@") {
			return true
		}
	}
	return false
}

func validateBubblewrapInstaller(source string) error {
	lines := strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n")
	if len(lines) == 0 || lines[0] != "#!/bin/bash" {
		return errors.New("Bubblewrap installer must use the fixed /bin/bash interpreter")
	}

	commands, err := parseShell(source)
	if err != nil {
		return fmt.Errorf("parse Bubblewrap installer: %w", err)
	}
	assignments := collectShellAssignments(commands)
	requiredAssignments := map[string]string{
		"PATH":                    "/usr/sbin:/usr/bin:/sbin:/bin",
		"BUBBLEWRAP_VERSION":      bubblewrapVersion,
		"BUBBLEWRAP_SHA256":       bubblewrapSHA256,
		"BUBBLEWRAP_URL":          "https://github.com/containers/bubblewrap/releases/download/v${BUBBLEWRAP_VERSION}/bubblewrap-${BUBBLEWRAP_VERSION}.tar.xz",
		"BUBBLEWRAP_INSTALL_PATH": "/usr/bin/bwrap",
	}
	for name, expected := range requiredAssignments {
		actual, ok := assignments[name]
		if !ok || actual != expected {
			return fmt.Errorf("unexpected installer assignment %s: got %q want %q", name, actual, expected)
		}
	}
	if !hasTopLevelCommand(commands, "set", "-euo", "pipefail") {
		return errors.New("Bubblewrap installer must enable strict Bash mode")
	}

	requiredUnset := []string{"CURL_HOME", "XDG_CONFIG_HOME", "CURL_CA_BUNDLE", "SSL_CERT_FILE", "SSL_CERT_DIR"}
	if !hasUnsetVariables(commands, requiredUnset) {
		return fmt.Errorf("installer must unset download configuration variables: %s", strings.Join(requiredUnset, ", "))
	}

	if err := validateAbsoluteInstallerTools(commands); err != nil {
		return err
	}
	if err := validateCurlCommand(commands); err != nil {
		return err
	}
	if err := validateArchiveChecksumCommand(commands); err != nil {
		return err
	}
	if err := validateMesonBuildFunction(commands); err != nil {
		return err
	}
	if err := validateReproducibilityCalls(commands); err != nil {
		return err
	}
	if err := validateReproducibilityComparison(commands); err != nil {
		return err
	}
	return validateInstallCommand(commands)
}

func validateAbsoluteInstallerTools(commands []shellCommand) error {
	required := []string{
		"/usr/bin/apt-get",
		"/usr/bin/curl",
		"/usr/bin/env",
		"/usr/bin/install",
		"/usr/bin/meson",
		"/usr/bin/mktemp",
		"/usr/bin/sha256sum",
		"/usr/bin/stat",
		"/usr/bin/tar",
		"/bin/rm",
	}
	seen := make(map[string]bool, len(required))
	for _, command := range commands {
		executable, _ := shellExecutable(command.words)
		for _, path := range required {
			if executable == path || shellCommandExecutesPath(command.words, path) {
				seen[path] = true
			}
		}
	}
	for _, path := range required {
		if !seen[path] {
			return fmt.Errorf("installer must use required absolute tool path: %s", path)
		}
	}
	return nil
}

func validateCurlCommand(commands []shellCommand) error {
	matches := findCommands(commands, "", "/usr/bin/curl")
	if len(matches) != 1 || !isUnconditionalTopLevel(matches[0]) {
		return fmt.Errorf("installer must invoke /usr/bin/curl exactly once at top level: got %d", len(matches))
	}
	_, arguments := shellExecutable(matches[0].words)
	if len(arguments) == 0 || arguments[0] != "--disable" {
		return errors.New("curl must use --disable as its first argument")
	}
	if !hasAdjacentArguments(arguments, "--proto", "=https") || !hasAdjacentArguments(arguments, "--proto-redir", "=https") {
		return errors.New("curl must restrict both direct and redirected protocols to HTTPS")
	}
	return nil
}

func validateArchiveChecksumCommand(commands []shellCommand) error {
	matches := 0
	for index := 1; index < len(commands); index++ {
		checksum := commands[index]
		checksumExecutable, checksumArguments := shellExecutable(checksum.words)
		if checksumExecutable != "/usr/bin/sha256sum" ||
			!sameArguments(checksumArguments, []string{"--check", "--strict", "-"}) ||
			!isUnconditionalTopLevel(checksum) {
			continue
		}

		input := commands[index-1]
		inputExecutable, inputArguments := shellExecutable(input.words)
		if inputExecutable != "/usr/bin/printf" ||
			!sameArguments(inputArguments, []string{"%s  %s\\n", "$BUBBLEWRAP_SHA256", "$archive"}) ||
			!isUnconditionalTopLevel(input) ||
			input.operatorAfter != "|" || checksum.operatorBefore != "|" {
			continue
		}
		matches++
	}
	if matches != 1 {
		return fmt.Errorf("installer must verify the downloaded archive exactly once with the pinned SHA-256: got %d", matches)
	}
	return nil
}

func validateMesonBuildFunction(commands []shellCommand) error {
	matches := findCommands(commands, "build_bubblewrap", "/usr/bin/meson")
	if len(matches) != 2 {
		return fmt.Errorf("build_bubblewrap must contain one Meson setup and one compile command: got %d", len(matches))
	}
	var setup shellCommand
	for _, command := range matches {
		if !isUnconditionalFunctionCommand(command, "build_bubblewrap") {
			return errors.New("build_bubblewrap must execute Meson commands unconditionally")
		}
		_, arguments := shellExecutable(command.words)
		if len(arguments) > 0 && arguments[0] == "setup" {
			setup = command
		}
	}
	if len(setup.words) == 0 {
		return errors.New("build_bubblewrap must configure Meson")
	}
	_, arguments := shellExecutable(setup.words)
	required := []string{
		"--buildtype=debugoptimized",
		"-Db_ndebug=false",
		"-Dsupport_setuid=false",
		"-Dman=disabled",
		"-Dselinux=disabled",
		"-Dtests=false",
	}
	for _, argument := range required {
		if !containsExact(arguments, argument) {
			return fmt.Errorf("Meson setup is missing required argument: %s", argument)
		}
	}
	for _, forbidden := range []string{"--buildtype=release", "-Db_ndebug=true", "-Dsupport_setuid=true"} {
		if containsExact(arguments, forbidden) {
			return fmt.Errorf("Meson setup contains forbidden argument: %s", forbidden)
		}
	}
	prefixes := []string{
		"-ffile-prefix-map=${source_dir}=/usr/src/toolkit-bubblewrap/source",
		"-fdebug-prefix-map=${source_dir}=/usr/src/toolkit-bubblewrap/source",
		"-ffile-prefix-map=${output_dir}=/usr/src/toolkit-bubblewrap/build",
		"-fdebug-prefix-map=${output_dir}=/usr/src/toolkit-bubblewrap/build",
	}
	for _, prefix := range prefixes {
		if !argumentListContains(arguments, prefix) {
			return fmt.Errorf("Meson setup is missing reproducible compiler option: %s", prefix)
		}
	}
	return nil
}

func validateReproducibilityCalls(commands []shellCommand) error {
	buildCalls := findCommands(commands, "", "build_bubblewrap")
	if len(buildCalls) != 2 {
		return fmt.Errorf("installer must perform exactly two top-level builds: got %d", len(buildCalls))
	}
	arguments := make([]string, 0, len(buildCalls))
	for _, call := range buildCalls {
		if !isUnconditionalTopLevel(call) {
			return errors.New("both reproducibility builds must execute at top level")
		}
		_, callArguments := shellExecutable(call.words)
		if len(callArguments) != 1 {
			return errors.New("each build_bubblewrap call must receive exactly one output directory")
		}
		arguments = append(arguments, callArguments[0])
	}
	sort.Strings(arguments)
	expected := []string{"$primary_output_dir", "$verification_output_dir"}
	sort.Strings(expected)
	if strings.Join(arguments, "\x00") != strings.Join(expected, "\x00") {
		return fmt.Errorf("reproducibility builds must use distinct declared output directories: got %v", arguments)
	}

	verifyCalls := findCommands(commands, "", "verify_reproducible_binary")
	if len(verifyCalls) != 1 || !isUnconditionalTopLevel(verifyCalls[0]) {
		return fmt.Errorf("installer must verify the two binaries exactly once at top level: got %d", len(verifyCalls))
	}
	_, verifyArguments := shellExecutable(verifyCalls[0].words)
	if len(verifyArguments) != 2 || verifyArguments[0] == verifyArguments[1] {
		return errors.New("reproducibility verification must compare two distinct binaries")
	}
	verifyHashCommands := findCommands(commands, "verify_reproducible_binary", "/usr/bin/sha256sum")
	if len(verifyHashCommands) != 2 {
		return fmt.Errorf("reproducibility verifier must hash both binaries: got %d hash commands", len(verifyHashCommands))
	}
	for _, command := range verifyHashCommands {
		if !isUnconditionalFunctionCommand(command, "verify_reproducible_binary") {
			return errors.New("reproducibility verifier must hash both binaries unconditionally")
		}
	}
	return nil
}

func validateReproducibilityComparison(commands []shellCommand) error {
	conditions := findCommands(commands, "verify_reproducible_binary", "if")
	matches := 0
	conditionDepth := 0
	for _, condition := range conditions {
		_, arguments := shellExecutable(condition.words)
		if isUnconditionalFunctionCommand(condition, "verify_reproducible_binary") &&
			sameArguments(arguments, []string{"[[", "$primary_hash", "!=", "$verification_hash", "]]"}) {
			matches++
			conditionDepth = condition.depth
		}
	}
	if matches != 1 {
		return fmt.Errorf("reproducibility verifier must compare the two hashes exactly once: got %d", matches)
	}

	failureExits := 0
	for _, command := range findCommands(commands, "verify_reproducible_binary", "exit") {
		_, arguments := shellExecutable(command.words)
		if command.depth == conditionDepth+1 && commandHasUnconditionalControlFlow(command) && sameArguments(arguments, []string{"1"}) {
			failureExits++
		}
	}
	if failureExits != 1 {
		return fmt.Errorf("reproducibility hash mismatch must fail exactly once: got %d exits", failureExits)
	}
	return nil
}

func validateInstallCommand(commands []shellCommand) error {
	matches := findCommands(commands, "", "/usr/bin/install")
	if len(matches) != 1 || !isUnconditionalTopLevel(matches[0]) {
		return fmt.Errorf("installer must install Bubblewrap exactly once at top level: got %d", len(matches))
	}
	_, arguments := shellExecutable(matches[0].words)
	requiredPairs := [][2]string{{"-o", "root"}, {"-g", "root"}, {"-m", "0755"}}
	for _, pair := range requiredPairs {
		if !hasAdjacentArguments(arguments, pair[0], pair[1]) {
			return fmt.Errorf("install command is missing %s %s", pair[0], pair[1])
		}
	}
	return nil
}

// parseWorkflow 只解析合同需要的 GitHub Actions YAML 节点，注释和普通文本不会成为 run step。
func parseWorkflow(source string) (workflowDocument, error) {
	document := workflowDocument{
		triggers: make(map[string]workflowTrigger),
		jobs:     make(map[string]workflowJob),
	}
	lines := strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n")
	section := ""
	currentTrigger := ""
	currentTriggerField := ""
	currentJob := ""
	inSteps := false
	stepIndex := -1

	for index := 0; index < len(lines); index++ {
		raw := lines[index]
		if strings.ContainsRune(raw, '\t') {
			return workflowDocument{}, fmt.Errorf("line %d contains a tab", index+1)
		}
		indent := leadingSpaces(raw)
		trimmed := strings.TrimSpace(stripYAMLComment(raw))
		if trimmed == "" {
			continue
		}

		if indent == 0 {
			key, _, ok := splitYAMLKeyValue(trimmed)
			if !ok {
				return workflowDocument{}, fmt.Errorf("line %d has invalid top-level syntax", index+1)
			}
			section = key
			currentTrigger = ""
			currentTriggerField = ""
			currentJob = ""
			inSteps = false
			stepIndex = -1
			continue
		}

		switch section {
		case "on":
			if indent == 2 {
				key, _, ok := splitYAMLKeyValue(trimmed)
				if !ok {
					return workflowDocument{}, fmt.Errorf("line %d has invalid trigger syntax", index+1)
				}
				currentTrigger = key
				currentTriggerField = ""
				if _, exists := document.triggers[key]; !exists {
					document.triggers[key] = workflowTrigger{}
				}
				continue
			}
			if indent == 4 && currentTrigger != "" {
				key, _, ok := splitYAMLKeyValue(trimmed)
				if ok {
					currentTriggerField = key
				}
				continue
			}
			if indent == 6 && currentTrigger != "" && (currentTriggerField == "paths" || currentTriggerField == "tags") && strings.HasPrefix(trimmed, "- ") {
				value, err := parseYAMLScalar(strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
				if err != nil {
					return workflowDocument{}, fmt.Errorf("line %d: %w", index+1, err)
				}
				trigger := document.triggers[currentTrigger]
				if currentTriggerField == "paths" {
					trigger.paths = append(trigger.paths, value)
				} else {
					trigger.tags = append(trigger.tags, value)
				}
				document.triggers[currentTrigger] = trigger
			}
		case "jobs":
			if indent == 2 {
				key, _, ok := splitYAMLKeyValue(trimmed)
				if !ok {
					return workflowDocument{}, fmt.Errorf("line %d has invalid job syntax", index+1)
				}
				currentJob = key
				document.jobs[key] = workflowJob{}
				inSteps = false
				stepIndex = -1
				continue
			}
			if currentJob == "" {
				continue
			}
			if indent == 4 && trimmed == "steps:" {
				inSteps = true
				continue
			}
			if indent == 4 && !inSteps {
				key, value, ok := splitYAMLKeyValue(trimmed)
				if !ok {
					continue
				}
				parsed, err := parseYAMLScalar(value)
				if err != nil {
					return workflowDocument{}, fmt.Errorf("line %d: %w", index+1, err)
				}
				job := document.jobs[currentJob]
				switch key {
				case "if":
					job.ifExpr = parsed
				case "runs-on":
					job.runsOn = parsed
				}
				document.jobs[currentJob] = job
				continue
			}
			if !inSteps {
				continue
			}
			job := document.jobs[currentJob]
			if indent == 6 && strings.HasPrefix(trimmed, "-") {
				job.steps = append(job.steps, workflowStep{})
				stepIndex = len(job.steps) - 1
				remainder := strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
				if remainder != "" {
					key, value, ok := splitYAMLKeyValue(remainder)
					if !ok {
						return workflowDocument{}, fmt.Errorf("line %d has invalid step syntax", index+1)
					}
					if err := assignWorkflowStepField(&job.steps[stepIndex], key, value); err != nil {
						return workflowDocument{}, fmt.Errorf("line %d: %w", index+1, err)
					}
				}
				document.jobs[currentJob] = job
				continue
			}
			if indent != 8 || stepIndex < 0 {
				continue
			}
			key, value, ok := splitYAMLKeyValue(trimmed)
			if !ok {
				continue
			}
			if key == "run" && (value == "|" || value == "|-" || value == ">" || value == ">-") {
				block, next := readYAMLBlockScalar(lines, index+1, indent, strings.HasPrefix(value, ">"))
				job.steps[stepIndex].run = block
				document.jobs[currentJob] = job
				index = next - 1
				continue
			}
			if err := assignWorkflowStepField(&job.steps[stepIndex], key, value); err != nil {
				return workflowDocument{}, fmt.Errorf("line %d: %w", index+1, err)
			}
			document.jobs[currentJob] = job
		}
	}
	return document, nil
}

func assignWorkflowStepField(step *workflowStep, key, raw string) error {
	value, err := parseYAMLScalar(raw)
	if err != nil {
		return err
	}
	switch key {
	case "name":
		step.name = value
	case "uses":
		step.uses = value
	case "if":
		step.ifExpr = value
	case "shell":
		step.shell = value
	case "run":
		step.run = value
	}
	return nil
}

func readYAMLBlockScalar(lines []string, start, parentIndent int, folded bool) (string, int) {
	contentIndent := -1
	var content []string
	index := start
	for ; index < len(lines); index++ {
		raw := lines[index]
		if strings.TrimSpace(raw) == "" {
			content = append(content, "")
			continue
		}
		indent := leadingSpaces(raw)
		if indent <= parentIndent {
			break
		}
		if contentIndent < 0 {
			contentIndent = indent
		}
		if indent < contentIndent {
			contentIndent = indent
		}
		content = append(content, raw)
	}
	if contentIndent < 0 {
		return "", index
	}
	for i, raw := range content {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		remove := contentIndent
		if len(raw) < remove {
			remove = len(raw)
		}
		content[i] = raw[remove:]
	}
	separator := "\n"
	if folded {
		separator = " "
	}
	return strings.Join(content, separator), index
}

func splitYAMLKeyValue(line string) (string, string, bool) {
	quote := rune(0)
	escaped := false
	for index, character := range line {
		if escaped {
			escaped = false
			continue
		}
		if quote == '"' && character == '\\' {
			escaped = true
			continue
		}
		if character == '\'' || character == '"' {
			switch quote {
			case 0:
				quote = character
			case character:
				quote = 0
			}
			continue
		}
		if character == ':' && quote == 0 {
			return strings.TrimSpace(line[:index]), strings.TrimSpace(line[index+1:]), true
		}
	}
	return "", "", false
}

func stripYAMLComment(line string) string {
	quote := rune(0)
	escaped := false
	for index, character := range line {
		if escaped {
			escaped = false
			continue
		}
		if quote == '"' && character == '\\' {
			escaped = true
			continue
		}
		if character == '\'' || character == '"' {
			switch quote {
			case 0:
				quote = character
			case character:
				quote = 0
			}
			continue
		}
		if character == '#' && quote == 0 && (index == 0 || unicode.IsSpace(rune(line[index-1]))) {
			return strings.TrimRightFunc(line[:index], unicode.IsSpace)
		}
	}
	return line
}

func parseYAMLScalar(raw string) (string, error) {
	raw = strings.TrimSpace(stripYAMLComment(raw))
	if raw == "" {
		return "", nil
	}
	if strings.HasPrefix(raw, "\"") {
		value, err := strconv.Unquote(raw)
		if err != nil {
			return "", fmt.Errorf("invalid double-quoted YAML scalar: %w", err)
		}
		return value, nil
	}
	if strings.HasPrefix(raw, "'") {
		if len(raw) < 2 || raw[len(raw)-1] != '\'' {
			return "", errors.New("invalid single-quoted YAML scalar")
		}
		return strings.ReplaceAll(raw[1:len(raw)-1], "''", "'"), nil
	}
	return raw, nil
}

// parseShell 生成实际命令节点，并保留函数和控制流深度，避免注释或死分支满足合同。
func parseShell(source string) ([]shellCommand, error) {
	logicalLines, err := shellLogicalLines(source)
	if err != nil {
		return nil, err
	}
	blocks := make([]shellBlock, 0, 4)
	commands := make([]shellCommand, 0, len(logicalLines))
	for lineNumber, line := range logicalLines {
		segments, err := shellCommandSegments(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNumber+1, err)
		}
		if len(segments) == 0 {
			continue
		}

		first := segments[0].words
		if closesShellBlock(first, "fi") {
			blocks, err = popShellBlock(blocks, "if")
		} else if closesShellBlock(first, "done") {
			blocks, err = popShellLoop(blocks)
		} else if closesShellBlock(first, "esac") {
			blocks, err = popShellBlock(blocks, "case")
		} else if closesShellBlock(first, "}") {
			blocks, err = popShellBlock(blocks, "function")
		}
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNumber+1, err)
		}

		functionName := currentShellFunction(blocks)
		for _, segment := range segments {
			words := segment.words
			if len(words) == 0 || isFunctionDeclaration(words) {
				continue
			}
			if isShellControlCommand(words[0]) && words[0] != "if" {
				continue
			}
			commands = append(commands, shellCommand{
				words:          words,
				depth:          len(blocks),
				function:       functionName,
				operatorBefore: segment.operatorBefore,
				operatorAfter:  segment.operatorAfter,
			})
		}

		if name, ok := shellFunctionDeclaration(first); ok {
			blocks = append(blocks, shellBlock{kind: "function", function: name})
			continue
		}
		switch first[0] {
		case "if":
			blocks = append(blocks, shellBlock{kind: "if"})
		case "for", "while", "until":
			blocks = append(blocks, shellBlock{kind: "loop"})
		case "case":
			blocks = append(blocks, shellBlock{kind: "case"})
		}
	}
	if len(blocks) != 0 {
		return nil, fmt.Errorf("unclosed shell block: %s", blocks[len(blocks)-1].kind)
	}
	return commands, nil
}

func shellLogicalLines(source string) ([]string, error) {
	scanner := bufio.NewScanner(strings.NewReader(strings.ReplaceAll(source, "\r\n", "\n")))
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	var lines []string
	var current strings.Builder
	heredocDelimiter := ""
	for scanner.Scan() {
		line := scanner.Text()
		if heredocDelimiter != "" {
			if strings.TrimSpace(line) == heredocDelimiter {
				heredocDelimiter = ""
			}
			continue
		}
		if current.Len() > 0 {
			current.WriteByte(' ')
		}
		current.WriteString(line)
		if hasShellContinuation(line) {
			value := current.String()
			current.Reset()
			current.WriteString(strings.TrimSuffix(value, "\\"))
			continue
		}
		logicalLine := current.String()
		lines = append(lines, logicalLine)
		heredocDelimiter = shellHeredocDelimiter(logicalLine)
		current.Reset()
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if current.Len() != 0 {
		return nil, errors.New("shell source ends with a line continuation")
	}
	if heredocDelimiter != "" {
		return nil, fmt.Errorf("shell source ends with an unterminated heredoc: %s", heredocDelimiter)
	}
	return lines, nil
}

func shellHeredocDelimiter(line string) string {
	for index := 0; index+1 < len(line); index++ {
		if line[index] != '<' || line[index+1] != '<' ||
			(index > 0 && line[index-1] == '<') ||
			(index+2 < len(line) && line[index+2] == '<') {
			continue
		}
		index += 2
		if index < len(line) && line[index] == '-' {
			index++
		}
		for index < len(line) && unicode.IsSpace(rune(line[index])) {
			index++
		}
		if index >= len(line) {
			return ""
		}
		quote := byte(0)
		if line[index] == '\'' || line[index] == '"' {
			quote = line[index]
			index++
		}
		start := index
		for index < len(line) {
			character := line[index]
			if quote != 0 {
				if character == quote {
					break
				}
			} else if unicode.IsSpace(rune(character)) || strings.ContainsRune(";|&<>", rune(character)) {
				break
			}
			index++
		}
		if index > start {
			return line[start:index]
		}
	}
	return ""
}

func hasShellContinuation(line string) bool {
	trimmed := strings.TrimRightFunc(line, unicode.IsSpace)
	count := 0
	for index := len(trimmed) - 1; index >= 0 && trimmed[index] == '\\'; index-- {
		count++
	}
	return count%2 == 1
}

func shellCommandSegments(line string) ([]shellSegment, error) {
	var segments []shellSegment
	var words []string
	var word strings.Builder
	quote := byte(0)
	escaped := false
	wordStarted := false
	pendingOperator := ""
	flushWord := func() {
		if wordStarted {
			words = append(words, word.String())
			word.Reset()
			wordStarted = false
		}
	}
	flushCommand := func() {
		flushWord()
		if len(words) > 0 {
			segments = append(segments, shellSegment{
				words:          words,
				operatorBefore: pendingOperator,
			})
			words = nil
			pendingOperator = ""
		}
	}
	recordOperator := func(operator string) {
		flushCommand()
		if len(segments) > 0 {
			segments[len(segments)-1].operatorAfter = operator
		}
		pendingOperator = operator
	}

	for index := 0; index < len(line); index++ {
		character := line[index]
		if escaped {
			word.WriteByte(character)
			wordStarted = true
			escaped = false
			continue
		}
		if quote != 0 {
			if quote == '"' && character == '\\' {
				escaped = true
				continue
			}
			if character == quote {
				quote = 0
				wordStarted = true
				continue
			}
			word.WriteByte(character)
			wordStarted = true
			continue
		}
		switch character {
		case '\\':
			escaped = true
			wordStarted = true
		case '\'', '"':
			quote = character
			wordStarted = true
		case '#':
			if !wordStarted {
				flushCommand()
				return segments, nil
			}
			word.WriteByte(character)
		case ' ', '\t':
			flushWord()
		case ';', '|', '&':
			operator := string(character)
			if index+1 < len(line) && line[index+1] == character {
				operator += string(character)
				index++
			}
			recordOperator(operator)
		case '(', ')', '{', '}':
			if wordStarted {
				word.WriteByte(character)
				continue
			}
			words = append(words, string(character))
		default:
			word.WriteByte(character)
			wordStarted = true
		}
	}
	if escaped || quote != 0 {
		return nil, errors.New("unterminated shell quote or escape")
	}
	flushCommand()
	return segments, nil
}

func isFunctionDeclaration(words []string) bool {
	_, ok := shellFunctionDeclaration(words)
	return ok
}

func shellFunctionDeclaration(words []string) (string, bool) {
	if len(words) == 2 && strings.HasSuffix(words[0], "()") && words[1] == "{" {
		name := strings.TrimSuffix(words[0], "()")
		return name, isShellIdentifier(name)
	}
	if len(words) == 4 && isShellIdentifier(words[0]) && words[1] == "(" && words[2] == ")" && words[3] == "{" {
		return words[0], true
	}
	return "", false
}

func isShellIdentifier(value string) bool {
	if value == "" || (value[0] != '_' && !unicode.IsLetter(rune(value[0]))) {
		return false
	}
	for _, character := range value[1:] {
		if character != '_' && !unicode.IsLetter(character) && !unicode.IsDigit(character) {
			return false
		}
	}
	return true
}

func isShellControlCommand(value string) bool {
	switch value {
	case "if", "then", "elif", "else", "fi", "for", "while", "until", "do", "done", "case", "in", "esac", "{", "}":
		return true
	default:
		return false
	}
}

func closesShellBlock(words []string, token string) bool {
	return len(words) > 0 && words[0] == token
}

func popShellBlock(blocks []shellBlock, expected string) ([]shellBlock, error) {
	if len(blocks) == 0 || blocks[len(blocks)-1].kind != expected {
		return nil, fmt.Errorf("unexpected shell block terminator for %s", expected)
	}
	return blocks[:len(blocks)-1], nil
}

func popShellLoop(blocks []shellBlock) ([]shellBlock, error) {
	return popShellBlock(blocks, "loop")
}

func currentShellFunction(blocks []shellBlock) string {
	for index := len(blocks) - 1; index >= 0; index-- {
		if blocks[index].kind == "function" {
			return blocks[index].function
		}
	}
	return ""
}

func shellExecutable(words []string) (string, []string) {
	if len(words) == 0 {
		return "", nil
	}
	index := 0
	for index < len(words) && isShellAssignment(words[index]) {
		index++
	}
	if index < len(words) && words[index] == "${privilege[@]}" {
		index++
	}
	if index >= len(words) {
		return "", nil
	}
	if filepath.Base(words[index]) == "sudo" {
		index++
		for index < len(words) && strings.HasPrefix(words[index], "-") {
			index++
		}
	}
	if index >= len(words) {
		return "", nil
	}
	return words[index], words[index+1:]
}

func isShellAssignment(word string) bool {
	index := strings.IndexByte(word, '=')
	return index > 0 && isShellIdentifier(word[:index])
}

func collectShellAssignments(commands []shellCommand) map[string]string {
	assignments := make(map[string]string)
	for _, command := range commands {
		if !isUnconditionalTopLevel(command) || len(command.words) == 0 {
			continue
		}
		words := command.words
		if words[0] == "readonly" || words[0] == "export" {
			words = words[1:]
		}
		for _, word := range words {
			if !isShellAssignment(word) {
				continue
			}
			parts := strings.SplitN(word, "=", 2)
			assignments[parts[0]] = parts[1]
		}
	}
	return assignments
}

func hasTopLevelCommand(commands []shellCommand, executable string, arguments ...string) bool {
	for _, command := range commands {
		actualExecutable, actualArguments := shellExecutable(command.words)
		if isUnconditionalTopLevel(command) && actualExecutable == executable && sameArguments(actualArguments, arguments) {
			return true
		}
	}
	return false
}

func hasUnsetVariables(commands []shellCommand, variables []string) bool {
	seen := make(map[string]bool, len(variables))
	for _, command := range commands {
		executable, arguments := shellExecutable(command.words)
		if !isUnconditionalTopLevel(command) || executable != "unset" {
			continue
		}
		for _, argument := range arguments {
			seen[argument] = true
		}
	}
	for _, variable := range variables {
		if !seen[variable] {
			return false
		}
	}
	return true
}

func findCommands(commands []shellCommand, function, executable string) []shellCommand {
	var matches []shellCommand
	for _, command := range commands {
		actualExecutable, _ := shellExecutable(command.words)
		if command.function == function && actualExecutable == executable {
			matches = append(matches, command)
		}
	}
	return matches
}

func isUnconditionalTopLevel(command shellCommand) bool {
	return command.depth == 0 && command.function == "" && commandHasUnconditionalControlFlow(command)
}

func isUnconditionalFunctionCommand(command shellCommand, function string) bool {
	return command.depth == 1 && command.function == function && commandHasUnconditionalControlFlow(command)
}

func commandHasUnconditionalControlFlow(command shellCommand) bool {
	return !isShortCircuitOperator(command.operatorBefore) && !isShortCircuitOperator(command.operatorAfter)
}

func isShortCircuitOperator(operator string) bool {
	return operator == "&&" || operator == "||"
}

func sameArguments(actual, expected []string) bool {
	return strings.Join(actual, "\x00") == strings.Join(expected, "\x00")
}

func hasAdjacentArguments(arguments []string, first, second string) bool {
	for index := 0; index+1 < len(arguments); index++ {
		if arguments[index] == first && arguments[index+1] == second {
			return true
		}
	}
	return false
}

func argumentListContains(arguments []string, fragment string) bool {
	for _, argument := range arguments {
		value := argument
		if name, raw, ok := strings.Cut(argument, "="); ok && name == "-Dc_args" {
			value = raw
		}
		for _, item := range strings.Fields(value) {
			if item == fragment {
				return true
			}
		}
	}
	return false
}

func containsExact(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func shellCommandExecutesPath(words []string, path string) bool {
	for _, word := range words {
		if word == path || strings.Contains(word, "$("+path) {
			return true
		}
	}
	return false
}

func isShellWorkflowStep(step workflowStep) bool {
	shell := strings.ToLower(strings.TrimSpace(step.shell))
	return shell == "" || shell == "bash" || shell == "sh" || strings.HasPrefix(shell, "bash ") || strings.HasPrefix(shell, "sh ")
}

func normalizeExpression(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func leadingSpaces(value string) int {
	return len(value) - len(strings.TrimLeft(value, " "))
}

func replaceExactlyOnce(source, old, replacement string) (string, bool) {
	if strings.Count(source, old) != 1 {
		return source, false
	}
	return strings.Replace(source, old, replacement, 1), true
}

// repositoryRoot 以当前测试文件定位仓库，避免依赖调用方工作目录。
func repositoryRoot(t *testing.T) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate contract test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), ".."))
}

func readWorkflowFiles(t *testing.T, root string) map[string]string {
	t.Helper()

	var paths []string
	for _, pattern := range []string{"*.yml", "*.yaml"} {
		matches, err := filepath.Glob(filepath.Join(root, ".github", "workflows", pattern))
		if err != nil {
			t.Fatalf("list workflow files matching %s: %v", pattern, err)
		}
		paths = append(paths, matches...)
	}
	if len(paths) == 0 {
		t.Fatal("no workflow files found")
	}
	sort.Strings(paths)
	result := make(map[string]string, len(paths))
	for _, path := range paths {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatalf("resolve workflow path %s: %v", path, err)
		}
		result[filepath.ToSlash(relative)] = readContractFile(t, path)
	}
	return result
}

func readContractFile(t *testing.T, path string) string {
	t.Helper()

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(body)
}

// validateInstallerIndexMode 校验安装器在 git index 中的可执行位（跨平台权威来源）。
func validateInstallerIndexMode(path string) error {
	cmd := exec.CommandContext(context.Background(), "git", "ls-files", "--stage", "--", path)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("query installer index mode: %w", err)
	}
	fields := strings.Fields(strings.TrimSpace(string(output)))
	if len(fields) < 1 || !strings.HasPrefix(fields[0], "1007") {
		return fmt.Errorf("Bubblewrap installer must be registered executable in the git index")
	}
	return nil
}
