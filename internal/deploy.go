package internal

import (
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

func DownloadInfraSource(dir string) error {
	err := ExecuteWithOptions("git", &ExecuteOptions{
		WorkingDir: dir,
	}, "clone", titvoInfraSource)
	fmt.Println("Downloaded infra from ", titvoInfraSource, " to ", dir)
	if err != nil {
		return err
	}
	return nil
}

func DownloadSecurityScanInfraSource(dir string) error {
	err := ExecuteWithOptions("git", &ExecuteOptions{
		WorkingDir: dir,
	}, "clone", titvoSecurityScanInfraSource)
	fmt.Println("Downloaded security scan infra from ", titvoSecurityScanInfraSource, " to ", dir)
	if err != nil {
		return err
	}
	return nil
}

func DownloadAuthSetupSource(dir string) error {
	err := ExecuteWithOptions("git", &ExecuteOptions{
		WorkingDir: dir,
	}, "clone", titvoAuthSetupSource)
	fmt.Println("Downloaded auth setup from ", titvoAuthSetupSource, " to ", dir)
	if err != nil {
		return err
	}
	return nil
}

func DownloadTaskCliFilesSource(dir string) error {
	err := ExecuteWithOptions("git", &ExecuteOptions{
		WorkingDir: dir,
	}, "clone", titvoTaskCliFilesSource)
	fmt.Println("Downloaded task cli files from ", titvoTaskCliFilesSource, " to ", dir)
	if err != nil {
		return err
	}
	return nil
}

func DownloadTaskTriggerSource(dir string) error {
	err := ExecuteWithOptions("git", &ExecuteOptions{
		WorkingDir: dir,
	}, "clone", titvoTaskTriggerSource)
	fmt.Println("Downloaded task trigger from ", titvoTaskTriggerSource, " to ", dir)
	if err != nil {
		return err
	}
	return nil
}

func DownloadTaskStatusSource(dir string) error {
	err := ExecuteWithOptions("git", &ExecuteOptions{
		WorkingDir: dir,
	}, "clone", titvoTaskStatusSource)
	fmt.Println("Downloaded task status from ", titvoTaskStatusSource, " to ", dir)
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
	fmt.Println("Deploying infra to ", prodDir)
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

	fmt.Println("Setting up parameters")
	err = PutParameter(&config.AWSCredentials, "/tvo/security-scan/prod/infra/vpc-id", config.VPCID)
	if err != nil {
		return fmt.Errorf("failed to put parameter vpc-id: %w", err)
	}
	err = PutParameter(&config.AWSCredentials, "/tvo/security-scan/prod/infra/subnet1", config.SubnetID)
	if err != nil {
		return fmt.Errorf("failed to put parameter subnet1: %w", err)
	}
	secretARN, err := CreateSecret(&config.AWSCredentials, "/tvo/security-scan/prod/aes_secret", config.AESSecret)
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
	fmt.Println("Executing terragrunt apply base infra")
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
	fmt.Println("Deploying security scan to ", prodDir)
	fmt.Println("Executing terragrunt apply security scan")
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
	fmt.Println("Deploying auth setup to ", prodDir)
	fmt.Println("Updating git submodules")
	err = ExecuteWithOptions("git", &ExecuteOptions{
		WorkingDir: sourceDir,
	}, "submodule", "update", "--init")
	if err != nil {
		return fmt.Errorf("git submodule update failed: %w", err)
	}
	fmt.Println("Executing build with npm")
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
	fmt.Println("Executing terragrunt apply auth setup")
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
	fmt.Println("Deploying task cli files to ", prodDir)
	fmt.Println("Updating git submodules")
	err = ExecuteWithOptions("git", &ExecuteOptions{
		WorkingDir: sourceDir,
	}, "submodule", "update", "--init")
	if err != nil {
		return fmt.Errorf("git submodule update failed: %w", err)
	}
	fmt.Println("Executing build with npm")
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
	fmt.Println("Executing build with npm")
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
	fmt.Println("Executing terragrunt apply task cli files")
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
	fmt.Println("Deploying task trigger to ", prodDir)
	fmt.Println("Updating git submodules")
	err = ExecuteWithOptions("git", &ExecuteOptions{
		WorkingDir: sourceDir,
	}, "submodule", "update", "--init")
	if err != nil {
		return fmt.Errorf("git submodule update failed: %w", err)
	}
	fmt.Println("Executing build with npm")
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
	fmt.Println("Executing build with npm")
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
	fmt.Println("Executing terragrunt apply task trigger")
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
	fmt.Println("Deploying task status to ", prodDir)
	fmt.Println("Updating git submodules")
	err = ExecuteWithOptions("git", &ExecuteOptions{
		WorkingDir: sourceDir,
	}, "submodule", "update", "--init")
	if err != nil {
		return fmt.Errorf("git submodule update failed: %w", err)
	}
	fmt.Println("Executing build with npm")
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
	fmt.Println("Executing build with npm")
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
	fmt.Println("Executing terragrunt apply task status")
	err = ExecuteWithOptions("terragrunt", &ExecuteOptions{
		WorkingDir: prodDir,
		Env:        env,
	}, "run-all", "apply", "-input=false", "-auto-approve", "--terragrunt-non-interactive")
	if err != nil {
		return fmt.Errorf("terragrunt apply task status failed: %w", err)
	}
	fmt.Println("Deployed all services")
	return nil
}
