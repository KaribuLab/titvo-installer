package internal

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func successfulDeployStubs() {
	mkdirAllFn = os.MkdirAll
	downloadSourceFn = func(dir, sourceURL, component string) error { return nil }
	executeWithOptionsFn = func(command string, options *ExecuteOptions, args ...string) error { return nil }
	getAccountIDFn = func(creds *AWSCredentials) (string, error) { return "123456789012", nil }
	putParameterFn = func(creds *AWSCredentials, path, value string) error { return nil }
	createSecretFn = func(creds *AWSCredentials, name, secretValue string) (string, error) {
		return "arn:aws:secretsmanager:secret", nil
	}
	getParameterFn = func(creds *AWSCredentials, path string) (string, error) {
		if path == "/tvo/security-scan/prod/infra/ecr-publisher-job-definition-arn" {
			return "job-def", nil
		}
		return "job-queue", nil
	}
	submitBatchJobFn = func(creds *AWSCredentials, jobName, jobQueue, jobDefinition string, envVars map[string]string) error {
		return nil
	}
}

func validDeployConfig(titvoDir string) DeployConfig {
	return DeployConfig{
		AWSCredentials: AWSCredentials{
			AWSAccessKeyID:     "ak",
			AWSSecretAccessKey: "sk",
			AWSSessionToken:    "st",
			AWSRegion:          "us-east-1",
		},
		InstallToolConfig: InstallToolConfig{
			OS:               Linux,
			TitvoDir:         titvoDir,
			TerraformBinDir:  "tf",
			TerragruntBinDir: "tg",
			NodeBinDir:       "node",
		},
		VPCID:     "vpc-1",
		SubnetID:  "subnet-1",
		AESSecret: "secret",
		Debug:     false,
	}
}

func withRuntimeStubs(t *testing.T) {
	t.Helper()
	origExec := executeWithOptionsFn
	origGetAccount := getAccountIDFn
	origPut := putParameterFn
	origCreateSecret := createSecretFn
	origGetParam := getParameterFn
	origSubmit := submitBatchJobFn
	origDownload := downloadSourceFn
	origMkdirAll := mkdirAllFn

	t.Cleanup(func() {
		executeWithOptionsFn = origExec
		getAccountIDFn = origGetAccount
		putParameterFn = origPut
		createSecretFn = origCreateSecret
		getParameterFn = origGetParam
		submitBatchJobFn = origSubmit
		downloadSourceFn = origDownload
		mkdirAllFn = origMkdirAll
	})
}

func createRequiredInfraDirs(t *testing.T, titvoDir string) {
	t.Helper()
	paths := []string{
		"infra/titvo-security-scan-infra-aws/prod/us-east-1",
		"infra/titvo-agent-aws/aws",
		"infra/titvo-auth-setup-aws/aws",
		"infra/titvo-task-cli-files-aws/aws",
		"infra/titvo-task-trigger-aws/aws",
		"infra/titvo-task-status-aws/aws",
		"infra/titvo-installer-ecr-publisher/aws",
		"infra/titvo-mcp-gateway/aws",
		"infra/titvo-bitbucket-code-insights-aws/aws",
		"infra/titvo-git-commit-files-aws/aws",
		"infra/titvo-github-issue-aws/aws",
		"infra/titvo-issue-report-aws/aws",
	}

	for _, p := range paths {
		if err := os.MkdirAll(filepath.Join(titvoDir, p), 0o755); err != nil {
			t.Fatalf("failed creating test dir %s: %v", p, err)
		}
	}
}

func TestDownloadSourceRunsGitClone(t *testing.T) {
	withRuntimeStubs(t)

	called := false
	executeWithOptionsFn = func(command string, options *ExecuteOptions, args ...string) error {
		called = true
		if command != "git" {
			t.Fatalf("unexpected command: %s", command)
		}
		if options == nil || options.WorkingDir != "/tmp/work" {
			t.Fatalf("unexpected working dir")
		}
		if len(args) != 2 || args[0] != "clone" || args[1] != "https://example.com/repo.git" {
			t.Fatalf("unexpected args: %v", args)
		}
		return nil
	}

	err := downloadSource("/tmp/work", "https://example.com/repo.git", "component")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !called {
		t.Fatalf("expected executeWithOptionsFn to be called")
	}
}

func TestDownloadSourcePropagatesError(t *testing.T) {
	withRuntimeStubs(t)
	expected := errors.New("clone failed")
	executeWithOptionsFn = func(command string, options *ExecuteOptions, args ...string) error {
		return expected
	}

	err := downloadSource("/tmp/work", "https://example.com/repo.git", "component")
	if !errors.Is(err, expected) {
		t.Fatalf("expected error %v, got %v", expected, err)
	}
}

func TestDeployInfraSuccess(t *testing.T) {
	withRuntimeStubs(t)
	titvoDir := t.TempDir()
	createRequiredInfraDirs(t, titvoDir)

	successfulDeployStubs()
	jobsSubmitted := 0
	submitBatchJobFn = func(creds *AWSCredentials, jobName, jobQueue, jobDefinition string, envVars map[string]string) error {
		jobsSubmitted++
		return nil
	}

	config := validDeployConfig(titvoDir)
	config.InstallToolConfig.OS = Windows
	config.Debug = true
	err := deployInfra(config)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if jobsSubmitted != 2 {
		t.Fatalf("expected 2 jobs submitted, got %d", jobsSubmitted)
	}
}

func TestDeployInfraGetAccountIDError(t *testing.T) {
	withRuntimeStubs(t)
	titvoDir := t.TempDir()
	createRequiredInfraDirs(t, titvoDir)

	successfulDeployStubs()
	getAccountIDFn = func(creds *AWSCredentials) (string, error) { return "", errors.New("sts error") }

	err := deployInfra(validDeployConfig(titvoDir))
	if err == nil || err.Error() != "failed to get AWS account ID: sts error" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeployInfraSubmitBatchJobError(t *testing.T) {
	withRuntimeStubs(t)
	titvoDir := t.TempDir()
	createRequiredInfraDirs(t, titvoDir)

	successfulDeployStubs()
	submitBatchJobFn = func(creds *AWSCredentials, jobName, jobQueue, jobDefinition string, envVars map[string]string) error {
		if jobName == "installer-ecr-publisher-mcp-gateway" {
			return errors.New("batch failed")
		}
		return nil
	}

	err := deployInfra(validDeployConfig(titvoDir))
	if err == nil {
		t.Fatalf("expected error")
	}
	if err.Error() != "failed to submit installer ecr publisher job installer-ecr-publisher-mcp-gateway: batch failed" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureDirExistsMissing(t *testing.T) {
	err := ensureDirExists(filepath.Join(t.TempDir(), "missing"), "directory %s missing")
	if err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("expected missing directory error, got %v", err)
	}
}

func TestRunBuildErrors(t *testing.T) {
	withRuntimeStubs(t)
	executeWithOptionsFn = func(command string, options *ExecuteOptions, args ...string) error {
		if command == "npm" && len(args) > 0 && args[0] == "ci" {
			return errors.New("ci failed")
		}
		return nil
	}
	err := runBuild(t.TempDir(), 1)
	if err == nil || err.Error() != "npm ci failed: ci failed" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunBuildRunBuildError(t *testing.T) {
	withRuntimeStubs(t)
	executeWithOptionsFn = func(command string, options *ExecuteOptions, args ...string) error {
		if command == "npm" && len(args) > 1 && args[0] == "run" && args[1] == "build" {
			return errors.New("build failed")
		}
		return nil
	}
	err := runBuild(t.TempDir(), 1)
	if err == nil || err.Error() != "npm run build failed: build failed" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunBuildZeroRepeats(t *testing.T) {
	withRuntimeStubs(t)
	executeWithOptionsFn = func(command string, options *ExecuteOptions, args ...string) error {
		t.Fatalf("execute should not be called")
		return nil
	}
	if err := runBuild(t.TempDir(), 0); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestDeployNodeComponentMissingDir(t *testing.T) {
	withRuntimeStubs(t)
	downloadCalled := false
	err := deployNodeComponent(t.TempDir(), "missing-repo", "comp", func(dir string) error {
		downloadCalled = true
		return nil
	}, map[string]string{}, 0, false)
	if !downloadCalled {
		t.Fatalf("expected download to be called")
	}
	if err == nil || !strings.Contains(err.Error(), "directory does not exist") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeployNodeComponentDownloadError(t *testing.T) {
	withRuntimeStubs(t)
	err := deployNodeComponent(t.TempDir(), "repo", "comp", func(dir string) error {
		return errors.New("download failed")
	}, map[string]string{}, 0, false)
	if err == nil || err.Error() != "failed to download comp: download failed" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeployNodeComponentSubmoduleError(t *testing.T) {
	withRuntimeStubs(t)
	infraDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(infraDir, "repo", "aws"), 0o755); err != nil {
		t.Fatal(err)
	}
	executeWithOptionsFn = func(command string, options *ExecuteOptions, args ...string) error {
		if command == "git" {
			return errors.New("submodule failed")
		}
		return nil
	}
	err := deployNodeComponent(infraDir, "repo", "comp", func(dir string) error { return nil }, map[string]string{}, 1, true)
	if err == nil || err.Error() != "git submodule update failed: submodule failed" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeployNodeComponentTerragruntError(t *testing.T) {
	withRuntimeStubs(t)
	infraDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(infraDir, "repo", "aws"), 0o755); err != nil {
		t.Fatal(err)
	}
	executeWithOptionsFn = func(command string, options *ExecuteOptions, args ...string) error {
		if command == "terragrunt" {
			return errors.New("apply failed")
		}
		return nil
	}
	err := deployNodeComponent(infraDir, "repo", "comp", func(dir string) error { return nil }, map[string]string{}, 0, false)
	if err == nil || err.Error() != "terragrunt apply comp failed: apply failed" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeployNodeComponentBuildError(t *testing.T) {
	withRuntimeStubs(t)
	infraDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(infraDir, "repo", "aws"), 0o755); err != nil {
		t.Fatal(err)
	}
	executeWithOptionsFn = func(command string, options *ExecuteOptions, args ...string) error {
		if command == "npm" && len(args) > 0 && args[0] == "ci" {
			return errors.New("ci failed")
		}
		return nil
	}
	err := deployNodeComponent(infraDir, "repo", "comp", func(dir string) error { return nil }, map[string]string{}, 1, false)
	if err == nil || err.Error() != "npm ci failed: ci failed" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeployInfraDownloadInfraError(t *testing.T) {
	withRuntimeStubs(t)
	successfulDeployStubs()
	downloadSourceFn = func(dir, sourceURL, component string) error {
		if component == "infra" {
			return errors.New("clone failed")
		}
		return nil
	}
	err := deployInfra(DeployConfig{InstallToolConfig: InstallToolConfig{TitvoDir: t.TempDir()}})
	if err == nil || err.Error() != "failed to download infra: clone failed" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeployInfraGetParameterError(t *testing.T) {
	withRuntimeStubs(t)
	titvoDir := t.TempDir()
	createRequiredInfraDirs(t, titvoDir)
	successfulDeployStubs()
	getParameterFn = func(creds *AWSCredentials, path string) (string, error) {
		return "", errors.New("ssm failed")
	}

	err := deployInfra(validDeployConfig(titvoDir))
	if err == nil || err.Error() != "failed to get ecr publisher job definition arn: ssm failed" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeployInfraAdditionalErrorPaths(t *testing.T) {
	tests := []struct {
		name     string
		prepare  func(t *testing.T, titvoDir string)
		mutate   func()
		expected string
	}{
		{
			name: "base source dir missing",
			prepare: func(t *testing.T, titvoDir string) {
				if err := os.MkdirAll(filepath.Join(titvoDir, "infra"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			mutate:   func() {},
			expected: "source directory",
		},
		{
			name:    "mkdir infra fails",
			prepare: func(t *testing.T, titvoDir string) {},
			mutate: func() {
				mkdirAllFn = func(path string, perm os.FileMode) error { return errors.New("mkdir fail") }
			},
			expected: "mkdir fail",
		},
		{
			name:    "plugin cache mkdir fails",
			prepare: func(t *testing.T, titvoDir string) { createRequiredInfraDirs(t, titvoDir) },
			mutate: func() {
				mkdirAllFn = func(path string, perm os.FileMode) error {
					if strings.Contains(path, "terraform-plugins") {
						return errors.New("plugin mkdir fail")
					}
					return os.MkdirAll(path, perm)
				}
			},
			expected: "failed to create plugin cache directory: plugin mkdir fail",
		},
		{
			name:    "put parameter fails",
			prepare: func(t *testing.T, titvoDir string) { createRequiredInfraDirs(t, titvoDir) },
			mutate: func() {
				putParameterFn = func(creds *AWSCredentials, path, value string) error {
					if strings.Contains(path, "vpc-id") {
						return errors.New("ssm put fail")
					}
					return nil
				}
			},
			expected: "failed to put parameter vpc-id: ssm put fail",
		},
		{
			name:    "create secret fails",
			prepare: func(t *testing.T, titvoDir string) { createRequiredInfraDirs(t, titvoDir) },
			mutate: func() {
				createSecretFn = func(creds *AWSCredentials, name, secretValue string) (string, error) {
					return "", errors.New("secret fail")
				}
			},
			expected: "failed to create secret aes_secret: secret fail",
		},
		{
			name:    "put encryption key arn fails",
			prepare: func(t *testing.T, titvoDir string) { createRequiredInfraDirs(t, titvoDir) },
			mutate: func() {
				putParameterFn = func(creds *AWSCredentials, path, value string) error {
					if strings.Contains(path, "secret-manager-arn") {
						return errors.New("arn put fail")
					}
					return nil
				}
			},
			expected: "failed to put parameter encryption-key-arn: arn put fail",
		},
		{
			name:    "base terragrunt fails",
			prepare: func(t *testing.T, titvoDir string) { createRequiredInfraDirs(t, titvoDir) },
			mutate: func() {
				executeWithOptionsFn = func(command string, options *ExecuteOptions, args ...string) error {
					if command == "terragrunt" && options != nil && strings.Contains(options.WorkingDir, "prod/us-east-1") {
						return errors.New("tg fail")
					}
					return nil
				}
			},
			expected: "terragrunt apply failed: tg fail",
		},
		{
			name:    "download ecr publisher fails",
			prepare: func(t *testing.T, titvoDir string) { createRequiredInfraDirs(t, titvoDir) },
			mutate: func() {
				downloadSourceFn = func(dir, sourceURL, component string) error {
					if component == "installer ecr publisher" {
						return errors.New("download fail")
					}
					return nil
				}
			},
			expected: "failed to download installer ecr publisher: download fail",
		},
		{
			name:    "ecr apply fails",
			prepare: func(t *testing.T, titvoDir string) { createRequiredInfraDirs(t, titvoDir) },
			mutate: func() {
				executeWithOptionsFn = func(command string, options *ExecuteOptions, args ...string) error {
					if command == "terragrunt" && options != nil && strings.Contains(options.WorkingDir, "titvo-installer-ecr-publisher/aws") && len(args) > 1 && args[1] == "apply" {
						return errors.New("ecr apply fail")
					}
					return nil
				}
			},
			expected: "terragrunt apply installer ecr publisher failed: ecr apply fail",
		},
		{
			name:    "get queue arn fails",
			prepare: func(t *testing.T, titvoDir string) { createRequiredInfraDirs(t, titvoDir) },
			mutate: func() {
				getParameterFn = func(creds *AWSCredentials, path string) (string, error) {
					if strings.Contains(path, "job-queue") {
						return "", errors.New("queue fail")
					}
					return "job-def", nil
				}
			},
			expected: "failed to get ecr publisher job queue arn: queue fail",
		},
		{
			name:    "destroy ecr publisher fails",
			prepare: func(t *testing.T, titvoDir string) { createRequiredInfraDirs(t, titvoDir) },
			mutate: func() {
				executeWithOptionsFn = func(command string, options *ExecuteOptions, args ...string) error {
					if command == "terragrunt" && options != nil && strings.Contains(options.WorkingDir, "titvo-installer-ecr-publisher/aws") && len(args) > 1 && args[1] == "destroy" {
						return errors.New("destroy fail")
					}
					return nil
				}
			},
			expected: "terragrunt destroy installer ecr publisher failed: destroy fail",
		},
		{
			name:    "second stage component fails",
			prepare: func(t *testing.T, titvoDir string) { createRequiredInfraDirs(t, titvoDir) },
			mutate: func() {
				downloadSourceFn = func(dir, sourceURL, component string) error {
					if component == "MCP gateway" {
						return errors.New("mcp download fail")
					}
					return nil
				}
			},
			expected: "failed to download MCP gateway: mcp download fail",
		},
		{
			name:    "first stage component fails",
			prepare: func(t *testing.T, titvoDir string) { createRequiredInfraDirs(t, titvoDir) },
			mutate: func() {
				downloadSourceFn = func(dir, sourceURL, component string) error {
					if component == "agent aws" {
						return errors.New("agent download fail")
					}
					return nil
				}
			},
			expected: "failed to download agent aws: agent download fail",
		},
		{
			name: "ecr publisher dir missing",
			prepare: func(t *testing.T, titvoDir string) {
				createRequiredInfraDirs(t, titvoDir)
				if err := os.RemoveAll(filepath.Join(titvoDir, "infra", "titvo-installer-ecr-publisher")); err != nil {
					t.Fatal(err)
				}
			},
			mutate:   func() {},
			expected: "installer ecr publisher directory",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withRuntimeStubs(t)
			titvoDir := t.TempDir()
			successfulDeployStubs()
			tc.prepare(t, titvoDir)
			tc.mutate()
			err := deployInfra(validDeployConfig(titvoDir))
			if err == nil || !strings.Contains(err.Error(), tc.expected) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestDeployInfraWithoutSessionToken(t *testing.T) {
	withRuntimeStubs(t)
	titvoDir := t.TempDir()
	createRequiredInfraDirs(t, titvoDir)
	successfulDeployStubs()
	config := validDeployConfig(titvoDir)
	config.AWSCredentials.AWSSessionToken = ""
	if err := deployInfra(config); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}
