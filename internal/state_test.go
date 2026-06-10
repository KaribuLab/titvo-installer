package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadInstallStateMissingFileReturnsEmpty(t *testing.T) {
	titvoDir := t.TempDir()
	state, err := loadInstallState(titvoDir)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if state == nil || state.Modules == nil {
		t.Fatalf("expected initialized state")
	}
	if len(state.Modules) != 0 {
		t.Fatalf("expected empty modules, got %d", len(state.Modules))
	}
	if state.filePath != filepath.Join(titvoDir, installStateFileName) {
		t.Fatalf("unexpected file path: %s", state.filePath)
	}
}

func TestInstallStateSaveAndReload(t *testing.T) {
	titvoDir := t.TempDir()
	state, err := loadInstallState(titvoDir)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	m := state.module("titvo-task-trigger-aws")
	m.Cloned = true
	m.Built = true
	if err := state.save(); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	reloaded, err := loadInstallState(titvoDir)
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	got := reloaded.module("titvo-task-trigger-aws")
	if !got.Cloned || !got.Built || got.Applied {
		t.Fatalf("unexpected reloaded module state: %+v", got)
	}
}

func TestInstallStateSaveOmitsEmptyFlags(t *testing.T) {
	titvoDir := t.TempDir()
	state, _ := loadInstallState(titvoDir)
	m := state.module("titvo-security-scan-infra-aws")
	m.Cloned = true
	m.Applied = true
	if err := state.save(); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(titvoDir, installStateFileName))
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "\"cloned\": true") || !strings.Contains(content, "\"applied\": true") {
		t.Fatalf("expected cloned and applied flags, got: %s", content)
	}
	if strings.Contains(content, "built") || strings.Contains(content, "applied_ecr") || strings.Contains(content, "submitted") || strings.Contains(content, "destroyed") {
		t.Fatalf("expected empty flags to be omitted, got: %s", content)
	}
}

func TestLoadInstallStateCorruptFileErrors(t *testing.T) {
	titvoDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(titvoDir, installStateFileName), []byte("{not-json"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := loadInstallState(titvoDir)
	if err == nil || !strings.Contains(err.Error(), "failed to parse install state file") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestRunOnceSkipsWhenAlreadyDone(t *testing.T) {
	state := newTestInstallState(t)
	done := true
	called := false
	err := runOnce(state, &done, "skipping", func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if called {
		t.Fatalf("expected action to be skipped")
	}
}

func TestRunOnceExecutesAndPersists(t *testing.T) {
	state := newTestInstallState(t)
	done := false
	called := false
	err := runOnce(state, &done, "skipping", func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !called {
		t.Fatalf("expected action to run")
	}
	if !done {
		t.Fatalf("expected done flag to be set")
	}
	if _, statErr := os.Stat(state.filePath); statErr != nil {
		t.Fatalf("expected state file to be persisted: %v", statErr)
	}
}

func TestRunOnceDoesNotPersistOnError(t *testing.T) {
	state := newTestInstallState(t)
	done := false
	err := runOnce(state, &done, "skipping", func() error {
		return os.ErrPermission
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if done {
		t.Fatalf("expected done flag to remain false")
	}
	if _, statErr := os.Stat(state.filePath); statErr == nil {
		t.Fatalf("expected no state file to be written on failure")
	}
}

func TestRepoDirNameFromURL(t *testing.T) {
	cases := map[string]string{
		"https://github.com/KaribuLab/titvo-agent-aws.git": "titvo-agent-aws",
		"https://github.com/KaribuLab/titvo-mcp-gateway":    "titvo-mcp-gateway",
	}
	for url, expected := range cases {
		if got := repoDirNameFromURL(url); got != expected {
			t.Fatalf("repoDirNameFromURL(%q) = %q, want %q", url, got, expected)
		}
	}
}

func TestModuleStateResetDownstream(t *testing.T) {
	m := &moduleState{
		Built:      true,
		AppliedECR: true,
		Applied:    true,
		Submitted:  true,
		Destroyed:  true,
		Cloned:     true,
		Commit:     "abc123",
	}
	m.resetDownstream()
	if m.Built || m.AppliedECR || m.Applied || m.Submitted || m.Destroyed {
		t.Fatalf("expected downstream flags cleared, got %+v", m)
	}
	if !m.Cloned || m.Commit != "abc123" {
		t.Fatalf("expected cloned and commit preserved, got %+v", m)
	}
}

func TestInstallStateCommitPersistence(t *testing.T) {
	titvoDir := t.TempDir()
	state, err := loadInstallState(titvoDir)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	m := state.module("titvo-task-trigger-aws")
	m.Cloned = true
	m.Commit = "deadbeef1234567890"
	if err := state.save(); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	reloaded, err := loadInstallState(titvoDir)
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	got := reloaded.module("titvo-task-trigger-aws")
	if got.Commit != "deadbeef1234567890" {
		t.Fatalf("unexpected commit: %s", got.Commit)
	}
}

func TestEnsureModuleSourceUpToDateSkipsDownload(t *testing.T) {
	withRuntimeStubs(t)
	state := newTestInstallState(t)
	infraDir := t.TempDir()
	m := state.module("titvo-task-trigger-aws")
	m.Cloned = true
	m.Commit = "testsha"

	downloads := 0
	err := ensureModuleSource(state, infraDir, "titvo-task-trigger-aws", "task trigger", titvoTaskTriggerSource, func(dir string) error {
		downloads++
		return nil
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if downloads != 0 {
		t.Fatalf("expected no download when up to date, got %d", downloads)
	}
}

func TestEnsureModuleSourceRemoteChangedRefreshesAndResets(t *testing.T) {
	withRuntimeStubs(t)
	state := newTestInstallState(t)
	infraDir := t.TempDir()
	m := state.module("titvo-task-trigger-aws")
	m.Cloned = true
	m.Commit = "oldsha"
	m.Built = true
	m.Applied = true

	gitOutputFn = func(dir string, args ...string) (string, error) {
		if len(args) > 0 && args[0] == "ls-remote" {
			return "newsha\tHEAD", nil
		}
		return "newsha", nil
	}

	downloads := 0
	err := ensureModuleSource(state, infraDir, "titvo-task-trigger-aws", "task trigger", titvoTaskTriggerSource, func(dir string) error {
		downloads++
		return nil
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if downloads != 1 {
		t.Fatalf("expected one refresh download, got %d", downloads)
	}
	if m.Commit != "newsha" {
		t.Fatalf("expected commit updated to newsha, got %s", m.Commit)
	}
	if m.Built || m.Applied {
		t.Fatalf("expected downstream flags reset, got built=%v applied=%v", m.Built, m.Applied)
	}
}

func TestDeployInfraResumesFromSavedState(t *testing.T) {
	withRuntimeStubs(t)
	titvoDir := t.TempDir()
	createRequiredInfraDirs(t, titvoDir)
	successfulDeployStubs()

	clones := 0
	applies := 0
	jobs := 0
	countingStubs := func() {
		downloadSourceFn = func(dir, sourceURL, component string) error {
			clones++
			return nil
		}
		submitBatchJobFn = func(creds *AWSCredentials, jobName, jobQueue, jobDefinition string, envVars map[string]string) error {
			jobs++
			return nil
		}
		executeWithOptionsFn = func(command string, options *ExecuteOptions, args ...string) error {
			if isTerragruntCommand(command) && len(args) > 1 && args[0] == "run-all" && args[1] == "apply" {
				applies++
			}
			return nil
		}
	}

	countingStubs()
	config := validDeployConfig(titvoDir)
	if err := deployInfra(config); err != nil {
		t.Fatalf("first run failed: %v", err)
	}
	if clones == 0 || applies == 0 || jobs == 0 {
		t.Fatalf("expected work on first run, got clones=%d applies=%d jobs=%d", clones, applies, jobs)
	}

	clones, applies, jobs = 0, 0, 0
	if err := deployInfra(config); err != nil {
		t.Fatalf("resume run failed: %v", err)
	}
	if clones != 0 {
		t.Fatalf("expected no clones on resume, got %d", clones)
	}
	if applies != 0 {
		t.Fatalf("expected no terragrunt applies on resume, got %d", applies)
	}
	if jobs != 0 {
		t.Fatalf("expected no batch jobs on resume, got %d", jobs)
	}
}
