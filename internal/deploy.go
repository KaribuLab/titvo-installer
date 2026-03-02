package internal

import (
	"encoding/base64"
	"fmt"
	"os"
	"path"
)

const titvoInfraSource = "https://github.com/KaribuLab/titvo-security-scan-infra-aws.git"
const titvoSecurityScanInfraSource = "https://github.com/KaribuLab/titvo-security-scan.git"
const titvoAuthSetupSource = "https://github.com/KaribuLab/titvo-auth-setup-aws.git"
const titvoTaskCliFilesSource = "https://github.com/KaribuLab/titvo-task-cli-files-aws.git"
const titvoTaskTriggerSource = "https://github.com/KaribuLab/titvo-task-trigger-aws.git"
const titvoTaskStatusSource = "https://github.com/KaribuLab/titvo-task-status-aws.git"
const titvoInstallerECRPublisherSource = "https://github.com/KaribuLab/titvo-installer-ecr-publisher.git"

func DownloadInfraSource(dir string) error {
	err := ExecuteWithOptions("git", &ExecuteOptions{
		WorkingDir: dir,
	}, "clone", titvoInfraSource)
	printInfo(fmt.Sprintf("Downloaded infra from %s to %s", titvoInfraSource, dir))
	if err != nil {
		return err
	}
	return nil
}

func DownloadSecurityScanInfraSource(dir string) error {
	err := ExecuteWithOptions("git", &ExecuteOptions{
		WorkingDir: dir,
	}, "clone", titvoSecurityScanInfraSource)
	printInfo(fmt.Sprintf("Downloaded security scan infra from %s to %s", titvoSecurityScanInfraSource, dir))
	if err != nil {
		return err
	}
	return nil
}

func DownloadAuthSetupSource(dir string) error {
	err := ExecuteWithOptions("git", &ExecuteOptions{
		WorkingDir: dir,
	}, "clone", titvoAuthSetupSource)
	printInfo(fmt.Sprintf("Downloaded auth setup from %s to %s", titvoAuthSetupSource, dir))
	if err != nil {
		return err
	}
	return nil
}

func DownloadTaskCliFilesSource(dir string) error {
	err := ExecuteWithOptions("git", &ExecuteOptions{
		WorkingDir: dir,
	}, "clone", titvoTaskCliFilesSource)
	printInfo(fmt.Sprintf("Downloaded task cli files from %s to %s", titvoTaskCliFilesSource, dir))
	if err != nil {
		return err
	}
	return nil
}

func DownloadTaskTriggerSource(dir string) error {
	err := ExecuteWithOptions("git", &ExecuteOptions{
		WorkingDir: dir,
	}, "clone", titvoTaskTriggerSource)
	printInfo(fmt.Sprintf("Downloaded task trigger from %s to %s", titvoTaskTriggerSource, dir))
	if err != nil {
		return err
	}
	return nil
}

func DownloadTaskStatusSource(dir string) error {
	err := ExecuteWithOptions("git", &ExecuteOptions{
		WorkingDir: dir,
	}, "clone", titvoTaskStatusSource)
	printInfo(fmt.Sprintf("Downloaded task status from %s to %s", titvoTaskStatusSource, dir))
	if err != nil {
		return err
	}
	return nil
}

func DownloadInstallerECRPublisherSource(dir string) error {
	err := ExecuteWithOptions("git", &ExecuteOptions{
		WorkingDir: dir,
	}, "clone", titvoInstallerECRPublisherSource)
	printInfo(fmt.Sprintf("Downloaded installer ecr publisher from %s to %s", titvoInstallerECRPublisherSource, dir))
	if err != nil {
		return err
	}
	return nil
}

type DeployConfig struct {
	AWSCredentials    AWSCredentials
	InstallToolConfig InstallToolConfig
	VPCID             string
	SubnetID          string
	AESSecret         string
	Debug             bool
}

func DeployInfra(config DeployConfig) error {
	infraDir := path.Join(config.InstallToolConfig.TitvoDir, "infra")
	err := os.MkdirAll(infraDir, 0755)
	if err != nil {
		return err
	}
	err = DownloadInfraSource(infraDir)
	if err != nil {
		return fmt.Errorf("failed to download infra: %w", err)
	}
	sourceDir := path.Join(infraDir, "titvo-security-scan-infra-aws")
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		return fmt.Errorf("source directory %s does not exist", sourceDir)
	}
	prodDir := path.Join(sourceDir, "prod", "us-east-1")
	printInfo(fmt.Sprintf("Deploying infra to %s", prodDir))
	currentPathEnv := os.Getenv("PATH")
	var newPathEnv string
	if config.InstallToolConfig.OS == Windows {
		newPathEnv = fmt.Sprintf("%s;%s;%s;%s", currentPathEnv, config.InstallToolConfig.TerraformBinDir, config.InstallToolConfig.TerragruntBinDir, config.InstallToolConfig.NodeBinDir)
	} else {
		newPathEnv = fmt.Sprintf("%s:%s:%s:%s", currentPathEnv, config.InstallToolConfig.TerraformBinDir, config.InstallToolConfig.TerragruntBinDir, config.InstallToolConfig.NodeBinDir)
	}
	// Crear directorio para cache de plugins de Terraform
	pluginCacheDir := path.Join(config.InstallToolConfig.TitvoDir, "terraform-plugins")
	err = os.MkdirAll(pluginCacheDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create plugin cache directory: %w", err)
	}

	// Obtener Account ID de AWS
	accountID, err := GetAccountID(&config.AWSCredentials)
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
	err = PutParameter(&config.AWSCredentials, "/tvo/security-scan/prod/infra/vpc-id", config.VPCID)
	if err != nil {
		return fmt.Errorf("failed to put parameter vpc-id: %w", err)
	}
	err = PutParameter(&config.AWSCredentials, "/tvo/security-scan/prod/infra/subnet1", config.SubnetID)
	if err != nil {
		return fmt.Errorf("failed to put parameter subnet1: %w", err)
	}
	base64AESSecret := base64.StdEncoding.EncodeToString([]byte(config.AESSecret))
	secretARN, err := CreateSecret(&config.AWSCredentials, "/tvo/security-scan/prod/aes_secret", base64AESSecret)
	if err != nil {
		return fmt.Errorf("failed to create secret aes_secret: %w", err)
	}
	err = PutParameter(&config.AWSCredentials, "/tvo/security-scan/prod/infra/encryption-key-name", "/tvo/security-scan/prod/aes_secret")
	if err != nil {
		return fmt.Errorf("failed to put parameter encryption-key-name: %w", err)
	}
	err = PutParameter(&config.AWSCredentials, "/tvo/security-scan/prod/infra/secret-manager-arn", secretARN)
	if err != nil {
		return fmt.Errorf("failed to put parameter encryption-key-arn: %w", err)
	}
	// NOTE: Base Infra
	printInfo("Executing terragrunt apply base infra")
	err = ExecuteWithOptions("terragrunt", &ExecuteOptions{
		WorkingDir: prodDir,
		Env:        env,
	}, "run-all", "apply", "-input=false", "-auto-approve", "--terragrunt-non-interactive")
	if err != nil {
		return fmt.Errorf("terragrunt apply failed: %w", err)
	}
	err = DownloadSecurityScanInfraSource(infraDir)
	if err != nil {
		return fmt.Errorf("failed to download security scan infra: %w", err)
	}
	// NOTE: Security Scan
	sourceDir = path.Join(infraDir, "titvo-security-scan")
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		return fmt.Errorf("security scan directory %s does not exist", sourceDir)
	}
	prodDir = path.Join(sourceDir, "aws")
	printInfo(fmt.Sprintf("Deploying security scan to %s", prodDir))
	printInfo("Executing terragrunt apply security scan")
	err = ExecuteWithOptions("terragrunt", &ExecuteOptions{
		WorkingDir: prodDir,
		Env:        env,
	}, "run-all", "apply", "-input=false", "-auto-approve", "--terragrunt-non-interactive")
	if err != nil {
		return fmt.Errorf("terragrunt apply security scan failed: %w", err)
	}
	// NOTE: Auth Setup
	err = DownloadAuthSetupSource(infraDir)
	if err != nil {
		return fmt.Errorf("failed to download auth setup: %w", err)
	}
	sourceDir = path.Join(infraDir, "titvo-auth-setup-aws")
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		return fmt.Errorf("auth setup directory %s does not exist", sourceDir)
	}
	prodDir = path.Join(sourceDir, "aws")
	printInfo(fmt.Sprintf("Deploying auth setup to %s", prodDir))
	printInfo("Updating git submodules")
	err = ExecuteWithOptions("git", &ExecuteOptions{
		WorkingDir: sourceDir,
	}, "submodule", "update", "--init")
	if err != nil {
		return fmt.Errorf("git submodule update failed: %w", err)
	}
	printInfo("Executing build with npm")
	err = ExecuteWithOptions("npm", &ExecuteOptions{
		WorkingDir: sourceDir,
	}, "ci")
	if err != nil {
		return fmt.Errorf("npm ci failed: %w", err)
	}
	err = ExecuteWithOptions("npm", &ExecuteOptions{
		WorkingDir: sourceDir,
	}, "run", "build")
	if err != nil {
		return fmt.Errorf("npm run build failed: %w", err)
	}
	printInfo("Executing terragrunt apply auth setup")
	err = ExecuteWithOptions("terragrunt", &ExecuteOptions{
		WorkingDir: prodDir,
		Env:        env,
	}, "run-all", "apply", "-input=false", "-auto-approve", "--terragrunt-non-interactive")
	if err != nil {
		return fmt.Errorf("terragrunt apply auth setup failed: %w", err)
	}
	// NOTE: Task CLI Files
	err = DownloadTaskCliFilesSource(infraDir)
	if err != nil {
		return fmt.Errorf("failed to download task cli files: %w", err)
	}
	sourceDir = path.Join(infraDir, "titvo-task-cli-files-aws")
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		return fmt.Errorf("task cli files directory %s does not exist", sourceDir)
	}
	prodDir = path.Join(sourceDir, "aws")
	printInfo(fmt.Sprintf("Deploying task cli files to %s", prodDir))
	printInfo("Updating git submodules")
	err = ExecuteWithOptions("git", &ExecuteOptions{
		WorkingDir: sourceDir,
	}, "submodule", "update", "--init")
	if err != nil {
		return fmt.Errorf("git submodule update failed: %w", err)
	}
	printInfo("Executing build with npm")
	err = ExecuteWithOptions("npm", &ExecuteOptions{
		WorkingDir: sourceDir,
	}, "ci")
	if err != nil {
		return fmt.Errorf("npm ci failed: %w", err)
	}
	err = ExecuteWithOptions("npm", &ExecuteOptions{
		WorkingDir: sourceDir,
	}, "run", "build")
	if err != nil {
		return fmt.Errorf("npm run build failed: %w", err)
	}
	printInfo("Executing terragrunt apply task cli files")
	err = ExecuteWithOptions("npm", &ExecuteOptions{
		WorkingDir: sourceDir,
	}, "ci")
	if err != nil {
		return fmt.Errorf("npm ci failed: %w", err)
	}
	err = ExecuteWithOptions("npm", &ExecuteOptions{
		WorkingDir: sourceDir,
	}, "run", "build")
	if err != nil {
		return fmt.Errorf("npm run build failed: %w", err)
	}
	printInfo("Executing terragrunt apply task cli files")
	err = ExecuteWithOptions("terragrunt", &ExecuteOptions{
		WorkingDir: prodDir,
		Env:        env,
	}, "run-all", "apply", "-input=false", "-auto-approve", "--terragrunt-non-interactive")
	if err != nil {
		return fmt.Errorf("terragrunt apply task cli files failed: %w", err)
	}
	// NOTE: Task Trigger
	err = DownloadTaskTriggerSource(infraDir)
	if err != nil {
		return fmt.Errorf("failed to download task trigger: %w", err)
	}
	sourceDir = path.Join(infraDir, "titvo-task-trigger-aws")
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		return fmt.Errorf("task trigger directory %s does not exist", sourceDir)
	}
	prodDir = path.Join(sourceDir, "aws")
	printInfo(fmt.Sprintf("Deploying task trigger to %s", prodDir))
	printInfo("Updating git submodules")
	err = ExecuteWithOptions("git", &ExecuteOptions{
		WorkingDir: sourceDir,
	}, "submodule", "update", "--init")
	if err != nil {
		return fmt.Errorf("git submodule update failed: %w", err)
	}
	printInfo("Executing build with npm")
	err = ExecuteWithOptions("npm", &ExecuteOptions{
		WorkingDir: sourceDir,
	}, "ci")
	if err != nil {
		return fmt.Errorf("npm ci failed: %w", err)
	}
	err = ExecuteWithOptions("npm", &ExecuteOptions{
		WorkingDir: sourceDir,
	}, "run", "build")
	if err != nil {
		return fmt.Errorf("npm run build failed: %w", err)
	}
	printInfo("Executing build with npm")
	err = ExecuteWithOptions("npm", &ExecuteOptions{
		WorkingDir: sourceDir,
	}, "ci")
	if err != nil {
		return fmt.Errorf("npm ci failed: %w", err)
	}
	err = ExecuteWithOptions("npm", &ExecuteOptions{
		WorkingDir: sourceDir,
	}, "run", "build")
	if err != nil {
		return fmt.Errorf("npm run build failed: %w", err)
	}
	printInfo("Executing terragrunt apply task trigger")
	err = ExecuteWithOptions("terragrunt", &ExecuteOptions{
		WorkingDir: prodDir,
		Env:        env,
	}, "run-all", "apply", "-input=false", "-auto-approve", "--terragrunt-non-interactive")
	if err != nil {
		return fmt.Errorf("terragrunt apply task trigger failed: %w", err)
	}
	// NOTE: Task Status
	err = DownloadTaskStatusSource(infraDir)
	if err != nil {
		return fmt.Errorf("failed to download task status: %w", err)
	}
	sourceDir = path.Join(infraDir, "titvo-task-status-aws")
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		return fmt.Errorf("task status directory %s does not exist", sourceDir)
	}
	prodDir = path.Join(sourceDir, "aws")
	printInfo(fmt.Sprintf("Deploying task status to %s", prodDir))
	printInfo("Updating git submodules")
	err = ExecuteWithOptions("git", &ExecuteOptions{
		WorkingDir: sourceDir,
	}, "submodule", "update", "--init")
	if err != nil {
		return fmt.Errorf("git submodule update failed: %w", err)
	}
	printInfo("Executing build with npm")
	err = ExecuteWithOptions("npm", &ExecuteOptions{
		WorkingDir: sourceDir,
	}, "ci")
	if err != nil {
		return fmt.Errorf("npm ci failed: %w", err)
	}
	err = ExecuteWithOptions("npm", &ExecuteOptions{
		WorkingDir: sourceDir,
	}, "run", "build")
	if err != nil {
		return fmt.Errorf("npm run build failed: %w", err)
	}
	printInfo("Executing build with npm")
	err = ExecuteWithOptions("npm", &ExecuteOptions{
		WorkingDir: sourceDir,
	}, "ci")
	if err != nil {
		return fmt.Errorf("npm ci failed: %w", err)
	}
	err = ExecuteWithOptions("npm", &ExecuteOptions{
		WorkingDir: sourceDir,
	}, "run", "build")
	if err != nil {
		return fmt.Errorf("npm run build failed: %w", err)
	}
	printInfo("Executing terragrunt apply task status")
	err = ExecuteWithOptions("terragrunt", &ExecuteOptions{
		WorkingDir: prodDir,
		Env:        env,
	}, "run-all", "apply", "-input=false", "-auto-approve", "--terragrunt-non-interactive")
	if err != nil {
		return fmt.Errorf("terragrunt apply task status failed: %w", err)
	}
	// NOTE: Installer ECR Publisher
	err = DownloadInstallerECRPublisherSource(infraDir)
	if err != nil {
		return fmt.Errorf("failed to download installer ecr publisher: %w", err)
	}
	sourceDir = path.Join(infraDir, "titvo-installer-ecr-publisher")
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		return fmt.Errorf("installer ecr publisher directory %s does not exist", sourceDir)
	}
	prodDir = path.Join(sourceDir, "aws")
	printInfo("Executing terragrunt apply installer ecr publisher")
	err = ExecuteWithOptions("terragrunt", &ExecuteOptions{
		WorkingDir: prodDir,
		Env:        env,
	}, "run-all", "apply", "-input=false", "-auto-approve", "--terragrunt-non-interactive")
	if err != nil {
		return fmt.Errorf("terragrunt apply installer ecr publisher failed: %w", err)
	}
	jobDefinitionARN, err := GetParameter(&config.AWSCredentials, "/tvo/security-scan/prod/infra/ecr-publisher-job-definition-arn")
	if err != nil {
		return fmt.Errorf("failed to get ecr publisher job definition arn: %w", err)
	}
	jobQueueARN, err := GetParameter(&config.AWSCredentials, "/tvo/security-scan/prod/infra/ecr-publisher-job-queue-arn")
	if err != nil {
		return fmt.Errorf("failed to get ecr publisher job queue arn: %w", err)
	}
	printInfo("Submitting installer ecr publisher job")
	err = SubmitBatchJob(&config.AWSCredentials, "installer-ecr-publisher", jobQueueARN, jobDefinitionARN, map[string]string{
		"GIT_URL":    titvoSecurityScanInfraSource,
		"IMAGE_REPO": "tvo-security-scan-ecr-prod",
		"REGION":     config.AWSCredentials.AWSRegion,
	})
	if err != nil {
		return fmt.Errorf("failed to submit installer ecr publisher job: %w", err)
	}
	printInfo("Destroying installer ecr publisher")
	err = ExecuteWithOptions("terragrunt", &ExecuteOptions{
		WorkingDir: prodDir,
		Env:        env,
	}, "run-all", "destroy", "-input=false", "-auto-approve", "--terragrunt-non-interactive")
	if err != nil {
		return fmt.Errorf("terragrunt destroy installer ecr publisher failed: %w", err)
	}
	printInfo("Deployed all services")
	return nil
}
