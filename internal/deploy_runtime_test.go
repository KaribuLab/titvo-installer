package internal

import (
	"encoding/json"
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
		if path == "/tvo/security-scan/prod/infra/ecr/publisher/job_definition_arn" {
			return "job-def", nil
		}
		return "job-queue", nil
	}
	submitBatchJobFn = func(creds *AWSCredentials, jobName, jobQueue, jobDefinition string, envVars map[string]string) error {
		return nil
	}
	putRecordFn = func(creds *AWSCredentials, tableName string, item map[string]interface{}) error {
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
		VPCID:             "vpc-1",
		PrivateSubnetCIDR: "172.31.64.0/20",
		AvailabilityZone:  "us-east-1a",
		NatGatewayID:      "nat-00000000000000001",
		AESSecret:         "12345678901234567890123456789012",
		Debug:             false,
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
	origPutRecord := putRecordFn
	origDownload := downloadSourceFn
	origMkdirAll := mkdirAllFn

	t.Cleanup(func() {
		executeWithOptionsFn = origExec
		getAccountIDFn = origGetAccount
		putParameterFn = origPut
		createSecretFn = origCreateSecret
		getParameterFn = origGetParam
		submitBatchJobFn = origSubmit
		putRecordFn = origPutRecord
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
		"infra/titvo-mcp-gateway/aws/ecr",
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
	writtenParams := map[string]string{}
	terragruntApplyDirs := []string{}
	mcpRanNpm := false
	mcpRanSubmodule := false
	executeWithOptionsFn = func(command string, options *ExecuteOptions, args ...string) error {
		if options != nil && strings.Contains(options.WorkingDir, "titvo-mcp-gateway") {
			if command == "npm" {
				mcpRanNpm = true
			}
			if command == "git" && len(args) > 0 && args[0] == "submodule" {
				mcpRanSubmodule = true
			}
		}
		if command == "terragrunt" && options != nil && len(args) > 1 && args[0] == "run-all" && args[1] == "apply" {
			terragruntApplyDirs = append(terragruntApplyDirs, options.WorkingDir)
		}
		return nil
	}
	putParameterFn = func(creds *AWSCredentials, path, value string) error {
		writtenParams[path] = value
		return nil
	}
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
	privateSubnetsValue, ok := writtenParams["/tvo/security-scan/prod/infra/vpc/installer/subnets/private"]
	if !ok {
		t.Fatalf("expected private subnets parameter to be written")
	}
	var privateSubnets []privateSubnetConfig
	if err := json.Unmarshal([]byte(privateSubnetsValue), &privateSubnets); err != nil {
		t.Fatalf("expected valid private subnets JSON, got error: %v", err)
	}
	if len(privateSubnets) != 1 {
		t.Fatalf("expected 1 private subnet entry, got %d", len(privateSubnets))
	}
	if privateSubnets[0].CIDRBlock != config.PrivateSubnetCIDR {
		t.Fatalf("unexpected cidr block: %s", privateSubnets[0].CIDRBlock)
	}
	if privateSubnets[0].AvailabilityZone != config.AvailabilityZone {
		t.Fatalf("unexpected availability zone: %s", privateSubnets[0].AvailabilityZone)
	}
	if privateSubnets[0].NatGatewayID != config.NatGatewayID {
		t.Fatalf("unexpected nat gateway id: %s", privateSubnets[0].NatGatewayID)
	}
	if jobsSubmitted != 2 {
		t.Fatalf("expected 2 jobs submitted, got %d", jobsSubmitted)
	}

	mcpECRDir := filepath.Join(titvoDir, "infra", "titvo-mcp-gateway", "aws", "ecr")
	ecrPublisherDir := filepath.Join(titvoDir, "infra", "titvo-installer-ecr-publisher", "aws")
	agentDir := filepath.Join(titvoDir, "infra", "titvo-agent-aws", "aws")
	mcpAWSDir := filepath.Join(titvoDir, "infra", "titvo-mcp-gateway", "aws")

	findDirIndex := func(dirs []string, target string) int {
		for i, dir := range dirs {
			if dir == target {
				return i
			}
		}
		return -1
	}

	mcpECRIndex := findDirIndex(terragruntApplyDirs, mcpECRDir)
	if mcpECRIndex == -1 {
		t.Fatalf("expected MCP gateway ECR apply to run")
	}
	ecrPublisherIndex := findDirIndex(terragruntApplyDirs, ecrPublisherDir)
	if ecrPublisherIndex == -1 {
		t.Fatalf("expected installer ecr publisher apply to run")
	}
	agentIndex := findDirIndex(terragruntApplyDirs, agentDir)
	if agentIndex == -1 {
		t.Fatalf("expected agent aws apply to run")
	}
	mcpAWSIndex := findDirIndex(terragruntApplyDirs, mcpAWSDir)
	if mcpAWSIndex == -1 {
		t.Fatalf("expected MCP gateway aws apply to run")
	}
	if mcpECRIndex >= ecrPublisherIndex {
		t.Fatalf("expected MCP gateway ECR apply before installer ecr publisher apply")
	}
	if agentIndex >= mcpECRIndex {
		t.Fatalf("expected agent aws apply before MCP gateway ECR apply")
	}
	if agentIndex >= ecrPublisherIndex {
		t.Fatalf("expected agent aws apply before installer ecr publisher apply")
	}
	if mcpAWSIndex <= ecrPublisherIndex {
		t.Fatalf("expected MCP gateway aws apply after installer ecr publisher apply")
	}
	if mcpRanNpm {
		t.Fatalf("expected MCP gateway flow to skip npm build")
	}
	if mcpRanSubmodule {
		t.Fatalf("expected MCP gateway flow to skip git submodule update")
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

func TestDeployInfraOptionalSCMIntegrations(t *testing.T) {
	tests := []struct {
		name                       string
		bitbucketClientKey         string
		bitbucketClientSecret      string
		githubAccessToken          string
		expectBitbucketApply       bool
		expectGithubApply          bool
		expectedDynamoParameterIDs []string
	}{
		{
			name:                       "none provided",
			bitbucketClientKey:         "",
			bitbucketClientSecret:      "",
			githubAccessToken:          "",
			expectBitbucketApply:       false,
			expectGithubApply:          false,
			expectedDynamoParameterIDs: []string{},
		},
		{
			name:                       "only bitbucket provided",
			bitbucketClientKey:         "bb-key",
			bitbucketClientSecret:      "bb-secret",
			githubAccessToken:          "",
			expectBitbucketApply:       true,
			expectGithubApply:          false,
			expectedDynamoParameterIDs: []string{"bitbucket_client_credentials"},
		},
		{
			name:                       "only github provided",
			bitbucketClientKey:         "",
			bitbucketClientSecret:      "",
			githubAccessToken:          "gh-token",
			expectBitbucketApply:       false,
			expectGithubApply:          true,
			expectedDynamoParameterIDs: []string{"github_access_token"},
		},
		{
			name:                       "both provided",
			bitbucketClientKey:         "bb-key",
			bitbucketClientSecret:      "bb-secret",
			githubAccessToken:          "gh-token",
			expectBitbucketApply:       true,
			expectGithubApply:          true,
			expectedDynamoParameterIDs: []string{"bitbucket_client_credentials", "github_access_token"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withRuntimeStubs(t)
			titvoDir := t.TempDir()
			createRequiredInfraDirs(t, titvoDir)
			successfulDeployStubs()

			scmCreateSecretCalled := false
			dynamoValues := map[string]string{}
			dynamoParameterIDs := []string{}
			events := []string{}
			terragruntApplyDirs := []string{}

			createSecretFn = func(creds *AWSCredentials, name, secretValue string) (string, error) {
				if name != "/tvo/security-scan/prod/aes_secret" {
					scmCreateSecretCalled = true
				}
				return "arn:" + name, nil
			}
			putRecordFn = func(creds *AWSCredentials, tableName string, item map[string]interface{}) error {
				if tableName != "tvo-security-scan-parameter-prod" {
					t.Fatalf("unexpected table name: %s", tableName)
				}
				parameterID, ok := item["parameter_id"].(string)
				if !ok {
					t.Fatalf("parameter_id missing or invalid")
				}
				value, ok := item["value"].(string)
				if !ok {
					t.Fatalf("value missing or invalid")
				}
				dynamoParameterIDs = append(dynamoParameterIDs, parameterID)
				dynamoValues[parameterID] = value
				events = append(events, "put_record")
				return nil
			}
			executeWithOptionsFn = func(command string, options *ExecuteOptions, args ...string) error {
				if command == "terragrunt" && options != nil && len(args) > 1 && args[0] == "run-all" && args[1] == "apply" {
					terragruntApplyDirs = append(terragruntApplyDirs, options.WorkingDir)
					if strings.Contains(options.WorkingDir, filepath.Join("prod", "us-east-1")) {
						events = append(events, "base_apply")
					}
				}
				return nil
			}

			config := validDeployConfig(titvoDir)
			config.BitbucketClientKey = tc.bitbucketClientKey
			config.BitbucketClientSecret = tc.bitbucketClientSecret
			config.GithubAccessToken = tc.githubAccessToken
			if err := deployInfra(config); err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			bitbucketDir := filepath.Join(titvoDir, "infra", "titvo-bitbucket-code-insights-aws", "aws")
			githubDir := filepath.Join(titvoDir, "infra", "titvo-github-issue-aws", "aws")

			foundBitbucketApply := false
			foundGithubApply := false
			for _, applyDir := range terragruntApplyDirs {
				if applyDir == bitbucketDir {
					foundBitbucketApply = true
				}
				if applyDir == githubDir {
					foundGithubApply = true
				}
			}

			if foundBitbucketApply != tc.expectBitbucketApply {
				t.Fatalf("unexpected bitbucket apply state: got %v", foundBitbucketApply)
			}
			if foundGithubApply != tc.expectGithubApply {
				t.Fatalf("unexpected github apply state: got %v", foundGithubApply)
			}

			if scmCreateSecretCalled {
				t.Fatalf("expected no SCM secret manager writes")
			}

			for _, expectedParameterID := range tc.expectedDynamoParameterIDs {
				found := false
				for _, parameterID := range dynamoParameterIDs {
					if parameterID == expectedParameterID {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("expected dynamodb parameter id %s to be written", expectedParameterID)
				}
			}

			if tc.bitbucketClientKey != "" && tc.bitbucketClientSecret != "" {
				expectedBitbucketJSON, err := json.Marshal(map[string]string{"key": tc.bitbucketClientKey, "secret": tc.bitbucketClientSecret})
				if err != nil {
					t.Fatalf("failed to marshal expected bitbucket json: %v", err)
				}
				expectedEncrypted, err := encrypt(string(expectedBitbucketJSON), config.AESSecret)
				if err != nil {
					t.Fatalf("failed to encrypt expected bitbucket json: %v", err)
				}
				actualEncrypted := dynamoValues["bitbucket_client_credentials"]
				if actualEncrypted != expectedEncrypted {
					t.Fatalf("unexpected encrypted bitbucket credentials")
				}
			}

			if tc.githubAccessToken != "" {
				expectedEncrypted, err := encrypt(tc.githubAccessToken, config.AESSecret)
				if err != nil {
					t.Fatalf("failed to encrypt expected github token: %v", err)
				}
				actualEncrypted := dynamoValues["github_access_token"]
				if actualEncrypted != expectedEncrypted {
					t.Fatalf("unexpected encrypted github access token")
				}
			}

			baseApplyIndex := -1
			firstPutRecordIndex := -1
			for i, event := range events {
				if event == "base_apply" && baseApplyIndex == -1 {
					baseApplyIndex = i
				}
				if event == "put_record" && firstPutRecordIndex == -1 {
					firstPutRecordIndex = i
				}
			}
			if firstPutRecordIndex != -1 && (baseApplyIndex == -1 || firstPutRecordIndex <= baseApplyIndex) {
				t.Fatalf("expected dynamodb writes to happen after base infra apply")
			}
		})
	}
}

func TestDeployInfraAdditionalErrorPaths(t *testing.T) {
	tests := []struct {
		name         string
		prepare      func(t *testing.T, titvoDir string)
		mutate       func()
		mutateConfig func(config *DeployConfig)
		expected     string
	}{
		{
			name: "base source dir missing",
			prepare: func(t *testing.T, titvoDir string) {
				if err := os.MkdirAll(filepath.Join(titvoDir, "infra"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			mutate:       func() {},
			mutateConfig: func(config *DeployConfig) {},
			expected:     "source directory",
		},
		{
			name:    "mkdir infra fails",
			prepare: func(t *testing.T, titvoDir string) {},
			mutate: func() {
				mkdirAllFn = func(path string, perm os.FileMode) error { return errors.New("mkdir fail") }
			},
			mutateConfig: func(config *DeployConfig) {},
			expected:     "mkdir fail",
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
			mutateConfig: func(config *DeployConfig) {},
			expected:     "failed to create plugin cache directory: plugin mkdir fail",
		},
		{
			name:    "put parameter fails",
			prepare: func(t *testing.T, titvoDir string) { createRequiredInfraDirs(t, titvoDir) },
			mutate: func() {
				putParameterFn = func(creds *AWSCredentials, path, value string) error {
					if strings.Contains(path, "vpc_id") {
						return errors.New("ssm put fail")
					}
					return nil
				}
			},
			mutateConfig: func(config *DeployConfig) {},
			expected:     "failed to put parameter vpc-id: ssm put fail",
		},
		{
			name:    "create secret fails",
			prepare: func(t *testing.T, titvoDir string) { createRequiredInfraDirs(t, titvoDir) },
			mutate: func() {
				createSecretFn = func(creds *AWSCredentials, name, secretValue string) (string, error) {
					return "", errors.New("secret fail")
				}
			},
			mutateConfig: func(config *DeployConfig) {},
			expected:     "failed to create secret aes_secret: secret fail",
		},
		{
			name:    "put encryption key arn fails",
			prepare: func(t *testing.T, titvoDir string) { createRequiredInfraDirs(t, titvoDir) },
			mutate: func() {
				putParameterFn = func(creds *AWSCredentials, path, value string) error {
					if strings.Contains(path, "secret/manager/arn") {
						return errors.New("arn put fail")
					}
					return nil
				}
			},
			mutateConfig: func(config *DeployConfig) {},
			expected:     "failed to put parameter encryption-key-arn: arn put fail",
		},
		{
			name:    "put bitbucket dynamo parameter fails",
			prepare: func(t *testing.T, titvoDir string) { createRequiredInfraDirs(t, titvoDir) },
			mutate: func() {
				putRecordFn = func(creds *AWSCredentials, tableName string, item map[string]interface{}) error {
					if item["parameter_id"] == "bitbucket_client_credentials" {
						return errors.New("bitbucket dynamo put fail")
					}
					return nil
				}
			},
			mutateConfig: func(config *DeployConfig) {
				config.BitbucketClientKey = "bb-key"
				config.BitbucketClientSecret = "bb-secret"
			},
			expected: "failed to put scm parameter bitbucket_client_credentials in dynamodb: bitbucket dynamo put fail",
		},
		{
			name:    "put github dynamo parameter fails",
			prepare: func(t *testing.T, titvoDir string) { createRequiredInfraDirs(t, titvoDir) },
			mutate: func() {
				putRecordFn = func(creds *AWSCredentials, tableName string, item map[string]interface{}) error {
					if item["parameter_id"] == "github_access_token" {
						return errors.New("github dynamo put fail")
					}
					return nil
				}
			},
			mutateConfig: func(config *DeployConfig) {
				config.GithubAccessToken = "gh-token"
			},
			expected: "failed to put scm parameter github_access_token in dynamodb: github dynamo put fail",
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
			mutateConfig: func(config *DeployConfig) {},
			expected:     "terragrunt apply failed: tg fail",
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
			mutateConfig: func(config *DeployConfig) {},
			expected:     "failed to download installer ecr publisher: download fail",
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
			mutateConfig: func(config *DeployConfig) {},
			expected:     "terragrunt apply installer ecr publisher failed: ecr apply fail",
		},
		{
			name:    "get queue arn fails",
			prepare: func(t *testing.T, titvoDir string) { createRequiredInfraDirs(t, titvoDir) },
			mutate: func() {
				getParameterFn = func(creds *AWSCredentials, path string) (string, error) {
					if strings.Contains(path, "job_queue_arn") {
						return "", errors.New("queue fail")
					}
					return "job-def", nil
				}
			},
			mutateConfig: func(config *DeployConfig) {},
			expected:     "failed to get ecr publisher job queue arn: queue fail",
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
			mutateConfig: func(config *DeployConfig) {},
			expected:     "terragrunt destroy installer ecr publisher failed: destroy fail",
		},
		{
			name:    "second stage component fails",
			prepare: func(t *testing.T, titvoDir string) { createRequiredInfraDirs(t, titvoDir) },
			mutate: func() {
				downloadSourceFn = func(dir, sourceURL, component string) error {
					if component == "bitbucket code insights aws" {
						return errors.New("bitbucket download fail")
					}
					return nil
				}
			},
			mutateConfig: func(config *DeployConfig) {
				config.BitbucketClientKey = "bb-key"
				config.BitbucketClientSecret = "bb-secret"
			},
			expected: "failed to download bitbucket code insights aws: bitbucket download fail",
		},
		{
			name:    "first stage component fails",
			prepare: func(t *testing.T, titvoDir string) { createRequiredInfraDirs(t, titvoDir) },
			mutate: func() {
				downloadSourceFn = func(dir, sourceURL, component string) error {
					if component == "auth setup" {
						return errors.New("auth download fail")
					}
					return nil
				}
			},
			mutateConfig: func(config *DeployConfig) {},
			expected:     "failed to download auth setup: auth download fail",
		},
		{
			name:    "MCP ecr apply fails",
			prepare: func(t *testing.T, titvoDir string) { createRequiredInfraDirs(t, titvoDir) },
			mutate: func() {
				executeWithOptionsFn = func(command string, options *ExecuteOptions, args ...string) error {
					if command == "terragrunt" && options != nil && strings.Contains(options.WorkingDir, "titvo-mcp-gateway/aws/ecr") && len(args) > 1 && args[1] == "apply" {
						return errors.New("mcp ecr apply fail")
					}
					return nil
				}
			},
			mutateConfig: func(config *DeployConfig) {},
			expected:     "terragrunt apply MCP gateway ECR failed: mcp ecr apply fail",
		},
		{
			name: "ecr publisher dir missing",
			prepare: func(t *testing.T, titvoDir string) {
				createRequiredInfraDirs(t, titvoDir)
				if err := os.RemoveAll(filepath.Join(titvoDir, "infra", "titvo-installer-ecr-publisher")); err != nil {
					t.Fatal(err)
				}
			},
			mutate:       func() {},
			mutateConfig: func(config *DeployConfig) {},
			expected:     "installer ecr publisher directory",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withRuntimeStubs(t)
			titvoDir := t.TempDir()
			successfulDeployStubs()
			tc.prepare(t, titvoDir)
			tc.mutate()
			config := validDeployConfig(titvoDir)
			tc.mutateConfig(&config)
			err := deployInfra(config)
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
