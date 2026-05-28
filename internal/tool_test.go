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

func TestToolBinaryVersion_ShebangEnvResolvesFromBinaryDirPATH(t *testing.T) {
	dir := t.TempDir()
	nodeBinDir := filepath.Join(dir, "node", "bin")
	if err := os.MkdirAll(nodeBinDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Simulate npm's typical shebang: `#!/usr/bin/env node`
	// and make sure `node` is only resolvable if the binary dir is in PATH.
	t.Setenv("PATH", "/usr/bin")

	nodePath := filepath.Join(nodeBinDir, "node")
	npmPath := filepath.Join(nodeBinDir, "npm")

	nodeStub := "#!/bin/sh\necho \"fake-node v1.2.3\"\n"
	if err := os.WriteFile(nodePath, []byte(nodeStub), 0o755); err != nil {
		t.Fatal(err)
	}

	npmShebang := "#!/usr/bin/env node\nconsole.log('this is never parsed');\n"
	if err := os.WriteFile(npmPath, []byte(npmShebang), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := toolBinaryVersion(npmPath)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "fake-node v1.2.3" {
		t.Fatalf("version = %q, want %q", got, "fake-node v1.2.3")
	}
}
