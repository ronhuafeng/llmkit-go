package main

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateResolvedModuleRequiresExactTagIntegrityAndCommit(t *testing.T) {
	valid := moduleInfo{
		Path:     modulePath,
		Version:  "v0.4.1",
		Sum:      "h1:module",
		GoModSum: "h1:gomod",
		Origin:   moduleOrigin{VCS: "git", Hash: strings.Repeat("a", 40), Ref: "refs/tags/v0.4.1"},
	}
	if err := validateResolvedModule(valid, "v0.4.1", strings.Repeat("a", 40)); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		change func(*moduleInfo)
		commit string
		want   string
	}{
		{name: "wrong path", change: func(info *moduleInfo) { info.Path = "example.com/wrong" }, want: "module path"},
		{name: "wrong version", change: func(info *moduleInfo) { info.Version = "v0.4.0" }, want: "resolved version"},
		{name: "missing module sum", change: func(info *moduleInfo) { info.Sum = "" }, want: "module sum"},
		{name: "missing go.mod sum", change: func(info *moduleInfo) { info.GoModSum = "" }, want: "go.mod sum"},
		{name: "missing origin", change: func(info *moduleInfo) { info.Origin.Hash = "" }, want: "origin commit"},
		{name: "wrong commit", change: func(*moduleInfo) {}, commit: strings.Repeat("b", 40), want: "tag commit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := valid
			tt.change(&info)
			commit := tt.commit
			if commit == "" {
				commit = strings.Repeat("a", 40)
			}
			if err := validateResolvedModule(info, "v0.4.1", commit); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validateResolvedModule error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestProxyEnvironmentIgnoresPrefilledHostCachesAndDisablesFallback(t *testing.T) {
	t.Setenv("GOPROXY", "https://untrusted.invalid,direct")
	t.Setenv("GOMODCACHE", "/shared/module-cache")
	t.Setenv("GOCACHE", "/shared/build-cache")
	t.Setenv("GOPATH", "/shared/gopath")
	t.Setenv("GOFLAGS", "-modfile=/tmp/untrusted.mod")

	workdir := filepath.Join(string(filepath.Separator), "isolated")
	values := environmentValues(proxyEnvironment(workdir))
	wants := map[string]string{
		"GO111MODULE": "on",
		"GOCACHE":     filepath.Join(workdir, "buildcache"),
		"GOENV":       "off",
		"GOFLAGS":     "",
		"GOMODCACHE":  filepath.Join(workdir, "modcache"),
		"GOPATH":      filepath.Join(workdir, "gopath"),
		"GOTOOLCHAIN": "local",
		"GOVCS":       "*:off",
		"GOPROXY":     publicProxy,
		"GOPRIVATE":   "",
		"GONOPROXY":   "",
		"GONOSUMDB":   "",
		"GOSUMDB":     "sum.golang.org",
		"GOWORK":      "off",
	}
	for key, want := range wants {
		if got := values[key]; got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestRetryUntilAvailableIsBoundedAndRetainsLastProxyFailure(t *testing.T) {
	attempts := 0
	err := retryUntilAvailable(context.Background(), time.Millisecond, func() error {
		attempts++
		if attempts < 3 {
			return errors.New("not propagated")
		}
		return nil
	})
	if err != nil || attempts != 3 {
		t.Fatalf("err=%v attempts=%d", err, attempts)
	}

	ctx, cancel := context.WithCancel(context.Background())
	err = retryUntilAvailable(ctx, time.Millisecond, func() error {
		cancel()
		return errors.New("tag unavailable")
	})
	if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "tag unavailable") {
		t.Fatalf("bounded unavailable-tag error = %v", err)
	}
}

func TestWriteEvidenceRecordsExactReviewableFacts(t *testing.T) {
	evidence := tagEvidence{
		FormatVersion: 1,
		Proxy:         publicProxy,
		Tag:           "v0.4.1",
		TagCommit:     strings.Repeat("a", 40),
		Module: evidenceModule{
			Path: modulePath, Version: "v0.4.1", Sum: "h1:module", GoModSum: "h1:gomod",
			Origin: moduleOrigin{VCS: "git", Hash: strings.Repeat("a", 40), Ref: "refs/tags/v0.4.1"},
		},
	}
	var output bytes.Buffer
	if err := writeEvidence(&output, evidence); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"format_version": 1`, `"proxy": "https://proxy.golang.org"`, `"tag": "v0.4.1"`,
		`"tag_commit": "` + strings.Repeat("a", 40) + `"`, `"version": "v0.4.1"`,
		`"sum": "h1:module"`, `"go_mod_sum": "h1:gomod"`, `"hash": "` + strings.Repeat("a", 40) + `"`,
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("evidence missing %s:\n%s", want, output.String())
		}
	}
}

func environmentValues(environment []string) map[string]string {
	values := make(map[string]string, len(environment))
	for _, item := range environment {
		key, value, _ := strings.Cut(item, "=")
		values[key] = value
	}
	return values
}
