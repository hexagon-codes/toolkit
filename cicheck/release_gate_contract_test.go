package cicheck

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestAPICompatibilityGateIsHardAndAuditable(t *testing.T) {
	t.Parallel()

	source, document := loadWorkflowContract(t, "api-compat.yml")
	active := activeYAMLContract(source)
	// 只核对执行不变量，版本与批准基线路径由工作流声明。
	for _, required := range []string{
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

	baselineSetting := regexp.MustCompile(`(?m)^\s*API_BREAKING_BASELINE:\s*"([^"]+)"\s*$`).FindStringSubmatch(active)
	if len(baselineSetting) != 2 {
		t.Fatal("API compatibility gate must declare its approved baseline")
	}
	baseline := readContractFile(t, filepath.Join(repositoryRoot(t), baselineSetting[1]))
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
