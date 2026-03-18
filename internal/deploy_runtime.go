package internal

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path"
)

type privateSubnetConfig struct {
	CIDRBlock        string `json:"cidr_block"`
	AvailabilityZone string `json:"availability_zone"`
	NatGatewayID     string `json:"nat_gateway_id"`
}

var executeWithOptionsFn = ExecuteWithOptions
var getAccountIDFn = GetAccountID
var putParameterFn = PutParameter
var createSecretFn = CreateSecret
var getParameterFn = GetParameter
var submitBatchJobFn = SubmitBatchJob
var mkdirAllFn = os.MkdirAll

func downloadSource(dir, sourceURL, component string) error {
	err := executeWithOptionsFn("git", &ExecuteOptions{WorkingDir: dir}, "clone", sourceURL)
	printInfo(fmt.Sprintf("Downloaded %s from %s to %s", component, sourceURL, dir))
	return err
}

func ensureDirExists(dir, errMsg string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf(errMsg, dir)
	}
	return nil
}

func runTerragrunt(dir string, env map[string]string, action string) error {
	return executeWithOptionsFn("terragrunt", &ExecuteOptions{WorkingDir: dir, Env: env}, "run-all", action, "-input=false", "-auto-approve", "--terragrunt-non-interactive")
}

func runBuild(sourceDir string, repeats int) error {
	for i := 0; i < repeats; i++ {
		printInfo("Executing build with npm")
		if err := executeWithOptionsFn("npm", &ExecuteOptions{WorkingDir: sourceDir}, "ci"); err != nil {
			return fmt.Errorf("npm ci failed: %w", err)
		}
		if err := executeWithOptionsFn("npm", &ExecuteOptions{WorkingDir: sourceDir}, "run", "build"); err != nil {
			return fmt.Errorf("npm run build failed: %w", err)
		}
	}
	return nil
}

func applyTerragruntInDir(dir, label string, env map[string]string) error {
	if err := ensureDirExists(dir, "%s directory does not exist"); err != nil {
		return err
	}
	printInfo(fmt.Sprintf("Executing terragrunt apply %s", label))
	if err := runTerragrunt(dir, env, "apply"); err != nil {
		return fmt.Errorf("terragrunt apply %s failed: %w", label, err)
	}
	return nil
}

func deployTerraformComponentFromSource(sourceDir, label string, env map[string]string) error {
	if err := ensureDirExists(sourceDir, "%s directory does not exist"); err != nil {
		return err
	}
	prodDir := path.Join(sourceDir, "aws")
	printInfo(fmt.Sprintf("Deploying %s to %s", label, prodDir))
	return applyTerragruntInDir(prodDir, label, env)
}

func deployNodeComponentFromSource(sourceDir, label string, env map[string]string, buildRepeats int, needsSubmodules bool) error {
	if err := ensureDirExists(sourceDir, "%s directory does not exist"); err != nil {
		return err
	}

	if needsSubmodules {
		printInfo("Updating git submodules")
		if err := executeWithOptionsFn("git", &ExecuteOptions{WorkingDir: sourceDir}, "submodule", "update", "--init"); err != nil {
			return fmt.Errorf("git submodule update failed: %w", err)
		}
	}

	if buildRepeats > 0 {
		if err := runBuild(sourceDir, buildRepeats); err != nil {
			return err
		}
	}

	return deployTerraformComponentFromSource(sourceDir, label, env)
}

func deployNodeComponent(infraDir, repoDirName, label string, downloadFn func(string) error, env map[string]string, buildRepeats int, needsSubmodules bool) error {
	if err := downloadFn(infraDir); err != nil {
		return fmt.Errorf("failed to download %s: %w", label, err)
	}

	sourceDir := path.Join(infraDir, repoDirName)
	return deployNodeComponentFromSource(sourceDir, label, env, buildRepeats, needsSubmodules)
}

func deployInfra(config DeployConfig) error {
	infraDir := path.Join(config.InstallToolConfig.TitvoDir, "infra")
	if err := mkdirAllFn(infraDir, 0755); err != nil {
		return err
	}
	if err := DownloadInfraSource(infraDir); err != nil {
		return fmt.Errorf("failed to download infra: %w", err)
	}

	baseSourceDir := path.Join(infraDir, "titvo-security-scan-infra-aws")
	if err := ensureDirExists(baseSourceDir, "source directory %s does not exist"); err != nil {
		return err
	}
	baseProdDir := path.Join(baseSourceDir, "prod", "us-east-1")
	printInfo(fmt.Sprintf("Deploying infra to %s", baseProdDir))

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

	env := map[string]string{
		"AWS_ACCESS_KEY_ID":     config.AWSCredentials.AWSAccessKeyID,
		"AWS_SECRET_ACCESS_KEY": config.AWSCredentials.AWSSecretAccessKey,
		"AWS_REGION":            config.AWSCredentials.AWSRegion,
		"AWS_ACCOUNT_ID":        accountID,
		"TG_PLUGIN_CACHE_DIR":   pluginCacheDir,
		"AWS_STAGE":             "prod",
		"PATH":                  newPathEnv,
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

	printInfo("Executing terragrunt apply base infra")
	if err := runTerragrunt(baseProdDir, env, "apply"); err != nil {
		return fmt.Errorf("terragrunt apply failed: %w", err)
	}

	firstStageComponents := []struct {
		repoDirName    string
		label          string
		downloadFn     func(string) error
		buildRepeats   int
		needsSubmodule bool
	}{
		{repoDirName: "titvo-agent-aws", label: "agent aws", downloadFn: DownloadAgentAWSSource, buildRepeats: 0, needsSubmodule: false},
		{repoDirName: "titvo-auth-setup-aws", label: "auth setup", downloadFn: DownloadAuthSetupSource, buildRepeats: 1, needsSubmodule: true},
		{repoDirName: "titvo-task-cli-files-aws", label: "task cli files", downloadFn: DownloadTaskCliFilesSource, buildRepeats: 2, needsSubmodule: true},
		{repoDirName: "titvo-task-trigger-aws", label: "task trigger", downloadFn: DownloadTaskTriggerSource, buildRepeats: 2, needsSubmodule: true},
		{repoDirName: "titvo-task-status-aws", label: "task status", downloadFn: DownloadTaskStatusSource, buildRepeats: 2, needsSubmodule: true},
	}
	for _, component := range firstStageComponents {
		if err := deployNodeComponent(infraDir, component.repoDirName, component.label, component.downloadFn, env, component.buildRepeats, component.needsSubmodule); err != nil {
			return err
		}
	}

	if err := DownloadMCPGatewaySource(infraDir); err != nil {
		return fmt.Errorf("failed to download MCP gateway: %w", err)
	}
	mcpGatewaySourceDir := path.Join(infraDir, "titvo-mcp-gateway")
	if err := ensureDirExists(mcpGatewaySourceDir, "MCP gateway directory %s does not exist"); err != nil {
		return err
	}
	mcpGatewayECRDir := path.Join(mcpGatewaySourceDir, "aws", "ecr")
	printInfo(fmt.Sprintf("Deploying MCP gateway ECR to %s", mcpGatewayECRDir))
	if err := applyTerragruntInDir(mcpGatewayECRDir, "MCP gateway ECR", env); err != nil {
		return err
	}

	if err := DownloadInstallerECRPublisherSource(infraDir); err != nil {
		return fmt.Errorf("failed to download installer ecr publisher: %w", err)
	}
	ecrPublisherSource := path.Join(infraDir, "titvo-installer-ecr-publisher")
	if err := ensureDirExists(ecrPublisherSource, "installer ecr publisher directory %s does not exist"); err != nil {
		return err
	}
	ecrPublisherAWSDir := path.Join(ecrPublisherSource, "aws")
	printInfo("Executing terragrunt apply installer ecr publisher")
	if err := runTerragrunt(ecrPublisherAWSDir, env, "apply"); err != nil {
		return fmt.Errorf("terragrunt apply installer ecr publisher failed: %w", err)
	}

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

	printInfo("Destroying installer ecr publisher")
	if err := runTerragrunt(ecrPublisherAWSDir, env, "destroy"); err != nil {
		return fmt.Errorf("terragrunt destroy installer ecr publisher failed: %w", err)
	}

	if err := deployTerraformComponentFromSource(mcpGatewaySourceDir, "MCP gateway", env); err != nil {
		return err
	}

	secondStageComponents := []struct {
		repoDirName string
		label       string
		downloadFn  func(string) error
	}{
		{repoDirName: "titvo-bitbucket-code-insights-aws", label: "bitbucket code insights aws", downloadFn: DownloadBitbucketCodeInsightsAWSSource},
		{repoDirName: "titvo-git-commit-files-aws", label: "git commit files aws", downloadFn: DownloadGitCommitFilesAWSSource},
		{repoDirName: "titvo-github-issue-aws", label: "github issue aws", downloadFn: DownloadGithubIssueAWSSource},
		{repoDirName: "titvo-issue-report-aws", label: "issue report aws", downloadFn: DownloadIssueReportAWSSource},
	}
	for _, component := range secondStageComponents {
		if err := deployNodeComponent(infraDir, component.repoDirName, component.label, component.downloadFn, env, 0, false); err != nil {
			return err
		}
	}

	printInfo("Deployed all services")
	return nil
}
