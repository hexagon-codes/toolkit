package cache

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var cacheReadmes = []string{
	"README.md",
	"README.en.md",
	"local/README.md",
	"local/README.en.md",
	"multi/README.md",
	"multi/README.en.md",
	"redis/README.md",
	"redis/README.en.md",
}

var cacheRedisExamples = []string{
	"examples/cache/multi/main.go",
	"examples/cache/redis_stable/main.go",
	"examples/cache/redis_unstable/main.go",
}

func TestCacheReadmeStandaloneExamplesCompileWithCanonicalRedisContract(t *testing.T) {
	cacheDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repoRoot := filepath.Dir(cacheDir)
	exampleRoot := t.TempDir()

	goMod := fmt.Sprintf(`module cache-readme-examples

go 1.25.7

require github.com/hexagon-codes/toolkit v0.0.0

replace github.com/hexagon-codes/toolkit => %s
`, filepath.ToSlash(repoRoot))
	if writeErr := os.WriteFile(filepath.Join(exampleRoot, "go.mod"), []byte(goMod), 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}

	exampleCount := 0
	for _, relativePath := range cacheReadmes {
		path := filepath.Join(cacheDir, relativePath)
		blocks, readErr := goCodeBlocks(path)
		if readErr != nil {
			t.Fatalf("read %s: %v", relativePath, readErr)
		}
		for blockIndex, block := range blocks {
			assertCanonicalRedisDocContract(t, relativePath, blockIndex+1, block)
			if !strings.HasPrefix(strings.TrimSpace(block), "package ") {
				continue
			}

			exampleCount++
			dir := filepath.Join(exampleRoot, fmt.Sprintf("example-%02d", exampleCount))
			if mkdirErr := os.Mkdir(dir, 0o700); mkdirErr != nil {
				t.Fatal(mkdirErr)
			}
			if writeErr := os.WriteFile(filepath.Join(dir, "main.go"), []byte(block), 0o600); writeErr != nil {
				t.Fatal(writeErr)
			}
		}
	}
	if exampleCount == 0 {
		t.Fatal("no standalone Go examples found")
	}

	cmd := exec.CommandContext(t.Context(), "go", "test", "./...")
	cmd.Dir = exampleRoot
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod", "GOPROXY=off", "GOSUMDB=off")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("standalone README examples do not compile: %v\n%s", err, output)
	}
	t.Logf("compiled %d standalone README examples with GOPROXY=off and GOSUMDB=off", exampleCount)
}

func TestCacheRedisExamplesBoundStartupProbe(t *testing.T) {
	cacheDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repoRoot := filepath.Dir(cacheDir)

	for _, relativePath := range cacheRedisExamples {
		source, err := os.ReadFile(filepath.Join(repoRoot, relativePath))
		if err != nil {
			t.Fatalf("read %s: %v", relativePath, err)
		}
		assertBoundStartupProbe(t, relativePath, string(source))
	}
}

func assertCanonicalRedisDocContract(t *testing.T, file string, blockIndex int, block string) {
	t.Helper()
	location := fmt.Sprintf("%s Go block %d", file, blockIndex)

	for _, forbidden := range []string{
		"goredis.NewClient(",
		"goredis.NewClusterClient(",
		"goredis.NewFailoverClient(",
		"goredis.NewUniversalClient(",
		"redis.NewClient(",
		"redis.NewClusterClient(",
		"redis.NewFailoverClient(",
		"redis.NewUniversalClient(",
		"InsecureSkipVerify",
	} {
		if strings.Contains(block, forbidden) {
			t.Errorf("%s contains forbidden Redis connection pattern %q", location, forbidden)
		}
	}

	if strings.Contains(block, "DataCredentials =") {
		if !strings.Contains(block, `Username: os.Getenv("REDIS_USERNAME")`) ||
			!strings.Contains(block, `Password: os.Getenv("REDIS_PASSWORD")`) {
			t.Errorf("%s must configure Redis data username and password together", location)
		}
	}
	if strings.Contains(block, "connection.TLSConfig = &tls.Config") &&
		!strings.Contains(block, `if serverName := os.Getenv("REDIS_TLS_SERVER_NAME"); serverName != "" {`) {
		t.Errorf("%s enables TLS unconditionally; plaintext Redis must remain usable", location)
	}
	if strings.Contains(block, "factory.Open(") {
		assertBoundStartupProbe(t, location, block)
	}
}

func assertBoundStartupProbe(t *testing.T, location, source string) {
	t.Helper()
	for _, required := range []string{
		"startupCtx, cancelStartup := context.WithTimeout(ctx, 5*time.Second)",
		"factory.Open(startupCtx)",
		"cancelStartup()",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("%s must contain %q", location, required)
		}
	}
	for _, forbidden := range []string{
		"factory.Open(ctx)",
		"factory.Open(context.Background())",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("%s uses unbounded startup probe %q", location, forbidden)
		}
	}
}

func goCodeBlocks(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var blocks []string
	var current strings.Builder
	inGoBlock := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case !inGoBlock && line == "```go":
			inGoBlock = true
			current.Reset()
		case inGoBlock && line == "```":
			blocks = append(blocks, current.String())
			inGoBlock = false
		case inGoBlock:
			current.WriteString(line)
			current.WriteByte('\n')
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if inGoBlock {
		return nil, fmt.Errorf("unterminated Go code block")
	}
	return blocks, nil
}
