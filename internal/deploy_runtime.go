package internal

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
)

type privateSubnetConfig struct {
	CIDRBlock        string `json:"cidr_block"`
	AvailabilityZone string `json:"availability_zone"`
	NatGatewayID     string `json:"nat_gateway_id"`
}

var executeWithOptionsFn = ExecuteWithOptions
var logConfiguredToolBinariesFn = LogConfiguredToolBinaries
var getAccountIDFn = GetAccountID
var putParameterFn = PutParameter
var createSecretFn = CreateSecret
var getParameterFn = GetParameter
var submitBatchJobFn = SubmitBatchJob
var putRecordFn = PutRecord
var mkdirAllFn = os.MkdirAll
var gitOutputFn = gitOutput

// repoDirNameFromURL derives the directory name git clone would create from a
// repository URL (the base name without the .git suffix).
func repoDirNameFromURL(sourceURL string) string {
	return strings.TrimSuffix(path.Base(sourceURL), ".git")
}

func downloadSource(dir, sourceURL, component string) error {
	// Remove any leftover directory from a previously interrupted clone so the
	// resume flow does not fail with "destination path already exists".
	target := path.Join(dir, repoDirNameFromURL(sourceURL))
	if _, statErr := os.Stat(target); statErr == nil {
		if err := os.RemoveAll(target); err != nil {
			return fmt.Errorf("failed to remove existing clone %s: %w", target, err)
		}
	}
	err := executeWithOptionsFn("git", &ExecuteOptions{WorkingDir: dir, Env: gitExecuteEnv()}, "clone", sourceURL)
	printInfo(fmt.Sprintf("Downloaded %s from %s to %s", component, sourceURL, dir))
	return err
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func localHeadCommit(dir string) string {
	commit, err := gitOutputFn(dir, "rev-parse", "HEAD")
	if err != nil {
		return ""
	}
	return commit
}

func remoteHeadCommit(sourceURL string) (string, error) {
	out, err := gitOutputFn("", "ls-remote", sourceURL, "HEAD")
	if err != nil {
		return "", err
	}
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return "", fmt.Errorf("no HEAD ref for %s", sourceURL)
	}
	return fields[0], nil
}

// ensureModuleSource clones or refreshes a module source tree. On resume it
// compares the stored commit against the remote HEAD; when the remote changed,
// it re-clones cleanly and resets downstream build/deploy flags.
func ensureModuleSource(state *installState, infraDir, module, label, sourceURL string, downloadFn func(string) error) error {
	m := state.module(module)
	sourceDir := path.Join(infraDir, module)

	if !m.Cloned {
		if err := downloadFn(infraDir); err != nil {
			return fmt.Errorf("failed to download %s: %w", label, err)
		}
		m.Cloned = true
		m.Commit = localHeadCommit(sourceDir)
		return state.save()
	}

	localCommit := m.Commit
	if localCommit == "" {
		if _, err := os.Stat(sourceDir); err == nil {
			localCommit = localHeadCommit(sourceDir)
		}
	}

	remoteCommit, err := remoteHeadCommit(sourceURL)
	if err != nil {
		printAskQuestion(fmt.Sprintf("Warning: could not check remote for %s: %v. Using existing clone.", label, err))
		return nil
	}

	if localCommit == "" {
		printAskQuestion(fmt.Sprintf("Warning: could not determine local commit for %s. Using existing clone without refresh.", label))
		return nil
	}

	if localCommit == remoteCommit {
		if m.Commit != localCommit {
			m.Commit = localCommit
			return state.save()
		}
		printInfo(fmt.Sprintf("%s is up to date (commit %s)", label, shortCommit(m.Commit)))
		return nil
	}

	printInfo(fmt.Sprintf("Refreshing %s: remote commit changed (%s -> %s)", label, shortCommit(localCommit), shortCommit(remoteCommit)))
	if err := downloadFn(infraDir); err != nil {
		return fmt.Errorf("failed to refresh %s: %w", label, err)
	}
	m.Commit = localHeadCommit(sourceDir)
	m.resetDownstream()
	return state.save()
}

func shortCommit(commit string) string {
	if len(commit) <= 12 {
		return commit
	}
	return commit[:12]
}

func ensureDirExists(dir, errMsg string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf(errMsg, dir)
	}
	return nil
}

func runTerragrunt(dir, terragruntPath string, env map[string]string, action string) error {
	args := []string{
		"run-all", action,
		"-input=false", "-auto-approve", "--terragrunt-non-interactive",
	}
	if pluginCacheDir := env["TERRAGRUNT_PROVIDER_CACHE_DIR"]; pluginCacheDir != "" {
		args = append(args, "--terragrunt-provider-cache", "--terragrunt-provider-cache-dir", pluginCacheDir)
	}
	return executeWithOptionsFn(terragruntPath, &ExecuteOptions{WorkingDir: dir, Env: env}, args...)
}

func runBuild(sourceDir, npmPath, nodeBinDir string, repeats int) error {
	for range repeats {
		printInfo("Executing build with npm")
		npmEnv := map[string]string{"PATH": pathWithBinDirFirst(nodeBinDir)}
		if err := executeWithOptionsFn(npmPath, &ExecuteOptions{WorkingDir: sourceDir, Env: npmEnv}, "install"); err != nil {
			return fmt.Errorf("npm install failed: %w", err)
		}
		if err := executeWithOptionsFn(npmPath, &ExecuteOptions{WorkingDir: sourceDir, Env: npmEnv}, "run", "build"); err != nil {
			return fmt.Errorf("npm run build failed: %w", err)
		}
	}
	return nil
}

func applyTerragruntInDir(dir, label, terragruntPath string, env map[string]string) error {
	if err := ensureDirExists(dir, "%s directory does not exist"); err != nil {
		return err
	}
	printInfo(fmt.Sprintf("Executing terragrunt apply %s", label))
	if err := runTerragrunt(dir, terragruntPath, env, "apply"); err != nil {
		return fmt.Errorf("terragrunt apply %s failed: %w", label, err)
	}
	return nil
}

func deployTerraformComponentFromSource(state *installState, module, sourceDir, label, terragruntPath string, env map[string]string) error {
	if err := ensureDirExists(sourceDir, "%s directory does not exist"); err != nil {
		return err
	}
	prodDir := path.Join(sourceDir, "aws")
	m := state.module(module)
	return runOnce(state, &m.Applied, fmt.Sprintf("Skipping apply of %s (already applied)", label), func() error {
		printInfo(fmt.Sprintf("Deploying %s to %s", label, prodDir))
		return applyTerragruntInDir(prodDir, label, terragruntPath, env)
	})
}

func deployNodeComponentFromSource(state *installState, module, sourceDir, label, terragruntPath, npmPath, nodeBinDir string, env map[string]string, buildRepeats int, needsSubmodules bool) error {
	if err := ensureDirExists(sourceDir, "%s directory does not exist"); err != nil {
		return err
	}

	m := state.module(module)
	if err := runOnce(state, &m.Built, fmt.Sprintf("Skipping build of %s (already built)", label), func() error {
		if needsSubmodules {
			printInfo("Updating git submodules")
			if err := executeWithOptionsFn("git", &ExecuteOptions{WorkingDir: sourceDir, Env: gitExecuteEnv()}, "submodule", "update", "--init"); err != nil {
				return fmt.Errorf("git submodule update failed: %w", err)
			}
		}

		if buildRepeats > 0 {
			if err := runBuild(sourceDir, npmPath, nodeBinDir, buildRepeats); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}

	return deployTerraformComponentFromSource(state, module, sourceDir, label, terragruntPath, env)
}

func deployNodeComponent(state *installState, infraDir, repoDirName, label, sourceURL, terragruntPath, npmPath, nodeBinDir string, downloadFn func(string) error, env map[string]string, buildRepeats int, needsSubmodules bool) error {
	if err := ensureModuleSource(state, infraDir, repoDirName, label, sourceURL, downloadFn); err != nil {
		return err
	}

	sourceDir := path.Join(infraDir, repoDirName)
	return deployNodeComponentFromSource(state, repoDirName, sourceDir, label, terragruntPath, npmPath, nodeBinDir, env, buildRepeats, needsSubmodules)
}

func deployInfra(config DeployConfig) error {
	state, err := loadInstallState(config.InstallToolConfig.TitvoDir)
	if err != nil {
		return err
	}
	if len(state.Modules) > 0 {
		printInfo("Resuming installation from saved state")
	}

	infraDir := path.Join(config.InstallToolConfig.TitvoDir, "infra")
	if err := mkdirAllFn(infraDir, 0755); err != nil {
		return err
	}

	const infraModule = "titvo-security-scan-infra-aws"
	if err := ensureModuleSource(state, infraDir, infraModule, "infra", titvoInfraSource, DownloadInfraSource); err != nil {
		return err
	}

	baseSourceDir := path.Join(infraDir, "titvo-security-scan-infra-aws")
	if err := ensureDirExists(baseSourceDir, "source directory %s does not exist"); err != nil {
		return err
	}
	baseProdDir := path.Join(baseSourceDir, "prod", "us-east-1")
	printInfo(fmt.Sprintf("Deploying infra to %s", baseProdDir))

	terragruntPath := TerragruntBinaryPath(config.InstallToolConfig.TerragruntBinDir, config.InstallToolConfig.OS)
	terraformPath := TerraformBinaryPath(config.InstallToolConfig.TerraformBinDir, config.InstallToolConfig.OS)
	npmPath := NpmBinaryPath(config.InstallToolConfig.NodeBinDir, config.InstallToolConfig.OS)
	nodeBinDir := config.InstallToolConfig.NodeBinDir

	if err := logConfiguredToolBinariesFn(config.InstallToolConfig); err != nil {
		return fmt.Errorf("failed to validate tool binaries: %w", err)
	}

	currentPathEnv := os.Getenv("PATH")
	var newPathEnv string
	if config.InstallToolConfig.OS == Windows {
		newPathEnv = fmt.Sprintf("%s;%s;%s;%s", currentPathEnv, config.InstallToolConfig.TerraformBinDir, config.InstallToolConfig.TerragruntBinDir, config.InstallToolConfig.NodeBinDir)
	} else {
		newPathEnv = fmt.Sprintf("%s:%s:%s:%s", currentPathEnv, config.InstallToolConfig.TerraformBinDir, config.InstallToolConfig.TerragruntBinDir, config.InstallToolConfig.NodeBinDir)
	}

	pluginCacheDir := path.Join(config.InstallToolConfig.TitvoDir, "terraform-plugins")
	if err := mkdirAllFn(pluginCacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create plugin cache directory: %w", err)
	}

	accountID, err := getAccountIDFn(&config.AWSCredentials)
	if err != nil {
		return fmt.Errorf("failed to get AWS account ID: %w", err)
	}

	printInfo(fmt.Sprintf("Provider cache directory: %s", pluginCacheDir))

	env := map[string]string{
		"AWS_ACCESS_KEY_ID":             config.AWSCredentials.AWSAccessKeyID,
		"AWS_SECRET_ACCESS_KEY":         config.AWSCredentials.AWSSecretAccessKey,
		"AWS_REGION":                    config.AWSCredentials.AWSRegion,
		"AWS_ACCOUNT_ID":                accountID,
		"TERRAGRUNT_PROVIDER_CACHE":     "1",
		"TERRAGRUNT_PROVIDER_CACHE_DIR": pluginCacheDir,
		"AWS_STAGE":                     "prod",
		"PATH":                          newPathEnv,
		"TG_TF_PATH":                    terraformPath,
	}
	if config.Debug {
		env["TG_LOG"] = "debug"
		env["TF_LOG"] = "DEBUG"
	}
	if config.AWSCredentials.AWSSessionToken != "" {
		env["AWS_SESSION_TOKEN"] = config.AWSCredentials.AWSSessionToken
	}

	printInfo("Setting up parameters")
	privateSubnets, err := json.Marshal([]privateSubnetConfig{
		{
			CIDRBlock:        config.PrivateSubnetCIDR,
			AvailabilityZone: config.AvailabilityZone,
			NatGatewayID:     config.NatGatewayID,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to serialize private subnet configuration: %w", err)
	}

	parameterWrites := []struct {
		name  string
		path  string
		value string
	}{
		{name: "vpc-id", path: "/tvo/security-scan/prod/infra/vpc/vpc_id", value: config.VPCID},
		{name: "private-subnets", path: "/tvo/security-scan/prod/infra/vpc/installer/subnets/private", value: string(privateSubnets)},
	}
	for _, param := range parameterWrites {
		if err := putParameterFn(&config.AWSCredentials, param.path, param.value); err != nil {
			return fmt.Errorf("failed to put parameter %s: %w", param.name, err)
		}
	}
	base64AESSecret := base64.StdEncoding.EncodeToString([]byte(config.AESSecret))
	secretARN, err := createSecretFn(&config.AWSCredentials, "/tvo/security-scan/prod/aes_secret", base64AESSecret)
	if err != nil {
		return fmt.Errorf("failed to create secret aes_secret: %w", err)
	}
	secretParameters := []struct {
		name  string
		path  string
		value string
	}{
		{name: "encryption-key-name", path: "/tvo/security-scan/prod/infra/kms/encryption-key-name", value: "/tvo/security-scan/prod/aes_secret"},
		{name: "encryption-key-arn", path: "/tvo/security-scan/prod/infra/secret/manager/arn", value: secretARN},
	}
	for _, param := range secretParameters {
		if err := putParameterFn(&config.AWSCredentials, param.path, param.value); err != nil {
			return fmt.Errorf("failed to put parameter %s: %w", param.name, err)
		}
	}

	infraMod := state.module(infraModule)
	if err := runOnce(state, &infraMod.Applied, "Skipping base infra apply (already applied)", func() error {
		printInfo("Executing terragrunt apply base infra")
		if err := runTerragrunt(baseProdDir, terragruntPath, env, "apply"); err != nil {
			return fmt.Errorf("terragrunt apply failed: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	type scmSecretResult struct {
		parameterID string
		value       string
	}

	scmSecretResults := []scmSecretResult{}

	if config.BitbucketAPIToken == "" {
		printAskQuestion("Warning: Bitbucket credentials were not provided. Bitbucket integration deployment will be skipped.")
	} else {
		encryptedBitbucketAPIToken, err := encrypt(config.BitbucketAPIToken, config.AESSecret)
		if err != nil {
			return fmt.Errorf("failed to encrypt bitbucket api token: %w", err)
		}
		scmSecretResults = append(scmSecretResults, scmSecretResult{
			parameterID: "bitbucket_api_token",
			value:       encryptedBitbucketAPIToken,
		})
	}

	if config.GithubAccessToken == "" {
		printAskQuestion("Warning: GitHub access token was not provided. GitHub integration deployment will be skipped.")
	} else {
		encryptedGithubAccessToken, err := encrypt(config.GithubAccessToken, config.AESSecret)
		if err != nil {
			return fmt.Errorf("failed to encrypt github access token: %w", err)
		}
		scmSecretResults = append(scmSecretResults, scmSecretResult{
			parameterID: "github_access_token",
			value:       encryptedGithubAccessToken,
		})
	}

	for _, secret := range scmSecretResults {
		if err := putRecordFn(&config.AWSCredentials, "tvo-security-scan-parameter-prod", map[string]interface{}{
			"parameter_id": secret.parameterID,
			"value":        secret.value,
		}); err != nil {
			return fmt.Errorf("failed to put scm parameter %s in dynamodb: %w", secret.parameterID, err)
		}
	}

	if err := deployNodeComponent(state, infraDir, "titvo-git-commit-files-aws", "git commit files aws", titvoGitCommitFilesAWS, terragruntPath, npmPath, nodeBinDir, DownloadGitCommitFilesAWSSource, env, 1, true); err != nil {
		return err
	}

	const agentModule = "titvo-agent-aws"
	if err := ensureModuleSource(state, infraDir, agentModule, "agent aws", titvoAgentAWS, DownloadAgentAWSSource); err != nil {
		return err
	}
	agentSourceDir := path.Join(infraDir, agentModule)
	if err := ensureDirExists(agentSourceDir, "agent aws directory %s does not exist"); err != nil {
		return err
	}
	agentECRDir := path.Join(agentSourceDir, "aws", "ecr")
	agentMod := state.module(agentModule)
	if err := runOnce(state, &agentMod.AppliedECR, "Skipping agent ECR apply (already applied)", func() error {
		printInfo(fmt.Sprintf("Deploying agent ECR to %s", agentECRDir))
		return applyTerragruntInDir(agentECRDir, "agent ECR", terragruntPath, env)
	}); err != nil {
		return err
	}

	const mcpGatewayModule = "titvo-mcp-gateway"
	if err := ensureModuleSource(state, infraDir, mcpGatewayModule, "MCP gateway", titvoMCPGateway, DownloadMCPGatewaySource); err != nil {
		return err
	}
	mcpGatewaySourceDir := path.Join(infraDir, mcpGatewayModule)
	if err := ensureDirExists(mcpGatewaySourceDir, "MCP gateway directory %s does not exist"); err != nil {
		return err
	}
	mcpGatewayECRDir := path.Join(mcpGatewaySourceDir, "aws", "ecr")
	mcpGatewayMod := state.module(mcpGatewayModule)
	if err := runOnce(state, &mcpGatewayMod.AppliedECR, "Skipping MCP gateway ECR apply (already applied)", func() error {
		printInfo(fmt.Sprintf("Deploying MCP gateway ECR to %s", mcpGatewayECRDir))
		return applyTerragruntInDir(mcpGatewayECRDir, "MCP gateway ECR", terragruntPath, env)
	}); err != nil {
		return err
	}

	const ragIndexerModule = "titvo-rag-indexer"
	if err := ensureModuleSource(state, infraDir, ragIndexerModule, "rag indexer", titvoRagIndexerSource, DownloadRagIndexerSource); err != nil {
		return err
	}
	ragIndexerSourceDir := path.Join(infraDir, ragIndexerModule)
	if err := ensureDirExists(ragIndexerSourceDir, "rag indexer directory %s does not exist"); err != nil {
		return err
	}
	ragIndexerECRDir := path.Join(ragIndexerSourceDir, "aws", "ecr")
	ragIndexerMod := state.module(ragIndexerModule)
	if err := runOnce(state, &ragIndexerMod.AppliedECR, "Skipping rag indexer ECR apply (already applied)", func() error {
		printInfo(fmt.Sprintf("Deploying rag indexer ECR to %s", ragIndexerECRDir))
		return applyTerragruntInDir(ragIndexerECRDir, "rag indexer ECR", terragruntPath, env)
	}); err != nil {
		return err
	}

	const ecrPublisherModule = "titvo-installer-ecr-publisher"
	if err := ensureModuleSource(state, infraDir, ecrPublisherModule, "installer ecr publisher", titvoInstallerECRPublisherSource, DownloadInstallerECRPublisherSource); err != nil {
		return err
	}
	ecrPublisherSource := path.Join(infraDir, ecrPublisherModule)
	if err := ensureDirExists(ecrPublisherSource, "installer ecr publisher directory %s does not exist"); err != nil {
		return err
	}
	ecrPublisherAWSDir := path.Join(ecrPublisherSource, "aws")
	ecrPublisherMod := state.module(ecrPublisherModule)
	if err := runOnce(state, &ecrPublisherMod.Applied, "Skipping installer ecr publisher apply (already applied)", func() error {
		printInfo("Executing terragrunt apply installer ecr publisher")
		if err := runTerragrunt(ecrPublisherAWSDir, terragruntPath, env, "apply"); err != nil {
			return fmt.Errorf("terragrunt apply installer ecr publisher failed: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	if err := runOnce(state, &ecrPublisherMod.Submitted, "Skipping installer ecr publisher jobs (already submitted)", func() error {
		jobDefinitionARN, err := getParameterFn(&config.AWSCredentials, "/tvo/security-scan/prod/infra/ecr/publisher/job_definition_arn")
		if err != nil {
			return fmt.Errorf("failed to get ecr publisher job definition arn: %w", err)
		}
		jobQueueARN, err := getParameterFn(&config.AWSCredentials, "/tvo/security-scan/prod/infra/ecr/publisher/job_queue_arn")
		if err != nil {
			return fmt.Errorf("failed to get ecr publisher job queue arn: %w", err)
		}
		for _, job := range installerECRPublisherJobs(config.AWSCredentials.AWSRegion) {
			printInfo(fmt.Sprintf("Submitting installer ecr publisher job: %s", job.Name))
			if err := submitBatchJobFn(&config.AWSCredentials, job.Name, jobQueueARN, jobDefinitionARN, job.EnvVars); err != nil {
				return fmt.Errorf("failed to submit installer ecr publisher job %s: %w", job.Name, err)
			}
		}
		return nil
	}); err != nil {
		return err
	}

	if err := runOnce(state, &ecrPublisherMod.Destroyed, "Skipping installer ecr publisher destroy (already destroyed)", func() error {
		printInfo("Destroying installer ecr publisher")
		if err := runTerragrunt(ecrPublisherAWSDir, terragruntPath, env, "destroy"); err != nil {
			return fmt.Errorf("terragrunt destroy installer ecr publisher failed: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	reportingComponents := []struct {
		repoDirName    string
		label          string
		sourceURL      string
		downloadFn     func(string) error
		buildRepeat    int
		needsSubmodule bool
	}{
		{repoDirName: "titvo-issue-report-aws", label: "issue report aws", sourceURL: titvoIssueReportAWS, downloadFn: DownloadIssueReportAWSSource, buildRepeat: 1, needsSubmodule: true},
	}
	if config.BitbucketAPIToken != "" {
		reportingComponents = append(reportingComponents, struct {
			repoDirName    string
			label          string
			sourceURL      string
			downloadFn     func(string) error
			buildRepeat    int
			needsSubmodule bool
		}{repoDirName: "titvo-bitbucket-code-insights-aws", label: "bitbucket code insights aws", sourceURL: titvoBitbucketCodeInsightsAWS, downloadFn: DownloadBitbucketCodeInsightsAWSSource, buildRepeat: 1, needsSubmodule: true})
	}
	if config.GithubAccessToken != "" {
		reportingComponents = append(reportingComponents, struct {
			repoDirName    string
			label          string
			sourceURL      string
			downloadFn     func(string) error
			buildRepeat    int
			needsSubmodule bool
		}{repoDirName: "titvo-github-issue-aws", label: "github issue aws", sourceURL: titvoGithubIssueAWS, downloadFn: DownloadGithubIssueAWSSource, buildRepeat: 1, needsSubmodule: true})
	}
	for _, component := range reportingComponents {
		if err := deployNodeComponent(state, infraDir, component.repoDirName, component.label, component.sourceURL, terragruntPath, npmPath, nodeBinDir, component.downloadFn, env, component.buildRepeat, component.needsSubmodule); err != nil {
			return err
		}
	}

	if err := deployTerraformComponentFromSource(state, ragIndexerModule, ragIndexerSourceDir, "rag indexer", terragruntPath, env); err != nil {
		return err
	}

	if err := deployTerraformComponentFromSource(state, agentModule, agentSourceDir, "agent aws", terragruntPath, env); err != nil {
		return err
	}

	if err := deployTerraformComponentFromSource(state, mcpGatewayModule, mcpGatewaySourceDir, "MCP gateway", terragruntPath, env); err != nil {
		return err
	}

	coreComponents := []struct {
		repoDirName    string
		label          string
		sourceURL      string
		downloadFn     func(string) error
		buildRepeats   int
		needsSubmodule bool
	}{
		{repoDirName: "titvo-auth-setup-aws", label: "auth setup", sourceURL: titvoAuthSetupSource, downloadFn: DownloadAuthSetupSource, buildRepeats: 1, needsSubmodule: true},
		{repoDirName: "titvo-task-cli-files-aws", label: "task cli files", sourceURL: titvoTaskCliFilesSource, downloadFn: DownloadTaskCliFilesSource, buildRepeats: 1, needsSubmodule: true},
		{repoDirName: "titvo-task-trigger-aws", label: "task trigger", sourceURL: titvoTaskTriggerSource, downloadFn: DownloadTaskTriggerSource, buildRepeats: 1, needsSubmodule: true},
		{repoDirName: "titvo-task-status-aws", label: "task status", sourceURL: titvoTaskStatusSource, downloadFn: DownloadTaskStatusSource, buildRepeats: 1, needsSubmodule: true},
	}
	for _, component := range coreComponents {
		if err := deployNodeComponent(state, infraDir, component.repoDirName, component.label, component.sourceURL, terragruntPath, npmPath, nodeBinDir, component.downloadFn, env, component.buildRepeats, component.needsSubmodule); err != nil {
			return err
		}
	}

	printInfo("Deployed all services")
	return nil
}
