package internal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLogConfiguredToolBinaries(t *testing.T) {
	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}

	terragruntPath := filepath.Join(binDir, "terragrunt")
	terraformPath := filepath.Join(binDir, "terraform")
	nodePath := filepath.Join(dir, "node", "bin", "node")
	npmPath := filepath.Join(dir, "node", "bin", "npm")
	if err := os.MkdirAll(filepath.Dir(nodePath), 0o755); err != nil {
		t.Fatal(err)
	}

	versionScript := "#!/bin/sh\necho \"test-tool v1.2.3\"\n"
	for _, binaryPath := range []string{terragruntPath, terraformPath, nodePath, npmPath} {
		if err := os.WriteFile(binaryPath, []byte(versionScript), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	config := InstallToolConfig{
		OS:               Linux,
		TerraformBinDir:  binDir,
		TerragruntBinDir: binDir,
		NodeBinDir:       filepath.Join(dir, "node", "bin"),
	}

	if err := LogConfiguredToolBinaries(config); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestLogConfiguredToolBinariesMissingBinary(t *testing.T) {
	config := InstallToolConfig{
		OS:               Linux,
		TerraformBinDir:  t.TempDir(),
		TerragruntBinDir: t.TempDir(),
		NodeBinDir:       t.TempDir(),
	}

	if err := LogConfiguredToolBinaries(config); err == nil {
		t.Fatal("expected error for missing binaries")
	}
}
