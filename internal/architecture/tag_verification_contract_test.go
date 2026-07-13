package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTagVerificationWorkflowUsesIsolatedProxyOnlyGate(t *testing.T) {
	root := repoRoot(t)
	workflow := readContractFile(t, filepath.Join(root, ".github", "workflows", "tag-verification.yml"))
	script := readContractFile(t, filepath.Join(root, "scripts", "ci", "verify-tag-consumer.sh"))

	for _, required := range []string{
		"cache: false",
		`./scripts/ci/verify-tag-consumer.sh`,
		`${{ github.ref_name }}`,
		`${{ github.sha }}`,
		"tag-evidence.json",
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("tag workflow missing %q", required)
		}
	}
	for _, forbidden := range []string{"proxy.golang.org,direct", "cache: true"} {
		if strings.Contains(workflow, forbidden) {
			t.Errorf("tag workflow permits shared or fallback state through %q", forbidden)
		}
	}
	for _, required := range []string{
		`GOPROXY=https://proxy.golang.org`,
		`GOVCS='*:off'`,
		`GOWORK=off`,
		`GOMODCACHE="$workdir/modcache"`,
		`GOCACHE="$workdir/buildcache"`,
		`GOPATH="$workdir/gopath"`,
		`-timeout 10m`,
		`-retry-interval 15s`,
	} {
		if !strings.Contains(script, required) {
			t.Errorf("tag consumer script missing %q", required)
		}
	}
}

func readContractFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
