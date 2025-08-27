package internal

import (
	"fmt"
	"os"
	"path"
)

const titvoInfraSource = "https://github.com/KaribuLab/titvo-security-scan-infra-aws/archive/refs/heads/main.zip"
const titvoSecurityScanInfraSource = "https://github.com/KaribuLab/titvo-security-scan/archive/refs/heads/main.zip"
const titvoAuthSetupSource = "https://github.com/KaribuLab/titvo-auth-setup-aws/archive/refs/heads/main.zip"
const titvoTaskCliFilesSource = "https://github.com/KaribuLab/titvo-task-cli-files-aws/archive/refs/heads/main.zip"
const titvoTaskTriggerSource = "https://github.com/KaribuLab/titvo-task-trigger-aws/archive/refs/heads/main.zip"
const titvoTaskStatusSource = "https://github.com/KaribuLab/titvo-task-status-aws/archive/refs/heads/main.zip"

func DownloadInfraSource(dir string) error {
	fileName := "titvo-security-scan-infra-aws.zip"
	err := downloadFile(titvoInfraSource, dir, fileName)
	fmt.Println("Downloaded infra from ", titvoInfraSource, " to ", path.Join(dir, fileName))
	if err != nil {
		return err
	}
	err = extractZip(path.Join(dir, fileName), dir)
	fmt.Println("Extracted infra from ", path.Join(dir, fileName), " to ", dir)
	if err != nil {
		return err
	}
	os.Remove(path.Join(dir, fileName))
	return nil
}

func DownloadSecurityScanInfraSource(dir string) error {
	fileName := "titvo-security-scan.zip"
	err := downloadFile(titvoSecurityScanInfraSource, dir, fileName)
	fmt.Println("Downloaded security scan infra from ", titvoSecurityScanInfraSource, " to ", path.Join(dir, fileName))
	if err != nil {
		return err
	}
	err = extractZip(path.Join(dir, fileName), dir)
	fmt.Println("Extracted security scan infra from ", path.Join(dir, fileName), " to ", dir)
	if err != nil {
		return err
	}
	os.Remove(path.Join(dir, fileName))
	return nil
}

func DownloadAuthSetupSource(dir string) error {
	fileName := "titvo-auth-setup.zip"
	err := downloadFile(titvoAuthSetupSource, dir, fileName)
	fmt.Println("Downloaded auth setup from ", titvoAuthSetupSource, " to ", path.Join(dir, fileName))
	if err != nil {
		return err
	}
	err = extractZip(path.Join(dir, fileName), dir)
	fmt.Println("Extracted auth setup from ", path.Join(dir, fileName), " to ", dir)
	if err != nil {
		return err
	}
	os.Remove(path.Join(dir, fileName))
	return nil
}

func DownloadTaskCliFilesSource(dir string) error {
	fileName := "titvo-task-cli-files.zip"
	err := downloadFile(titvoTaskCliFilesSource, dir, fileName)
	fmt.Println("Downloaded task cli files from ", titvoTaskCliFilesSource, " to ", path.Join(dir, fileName))
	if err != nil {
		return err
	}
	err = extractZip(path.Join(dir, fileName), dir)
	fmt.Println("Extracted task cli files from ", path.Join(dir, fileName), " to ", dir)
	if err != nil {
		return err
	}
	os.Remove(path.Join(dir, fileName))
	return nil
}

func DownloadTaskTriggerSource(dir string) error {
	fileName := "titvo-task-trigger.zip"
	err := downloadFile(titvoTaskTriggerSource, dir, fileName)
	fmt.Println("Downloaded task trigger from ", titvoTaskTriggerSource, " to ", path.Join(dir, fileName))
	if err != nil {
		return err
	}
	err = extractZip(path.Join(dir, fileName), dir)
	fmt.Println("Extracted task trigger from ", path.Join(dir, fileName), " to ", dir)
	if err != nil {
		return err
	}
	os.Remove(path.Join(dir, fileName))
	return nil
}

func DownloadTaskStatusSource(dir string) error {
	fileName := "titvo-task-status.zip"
	err := downloadFile(titvoTaskStatusSource, dir, fileName)
	fmt.Println("Downloaded task status from ", titvoTaskStatusSource, " to ", path.Join(dir, fileName))
	if err != nil {
		return err
	}
	err = extractZip(path.Join(dir, fileName), dir)
	fmt.Println("Extracted task status from ", path.Join(dir, fileName), " to ", dir)
	if err != nil {
		return err
	}
	os.Remove(path.Join(dir, fileName))
	return nil
}

func DeployInfra(credentials AWSCredentials, config InstallToolConfig, terraformStateBucket string, vpcID string, subnetID string, aesSecret string) error {
	infraDir := path.Join(config.TitvoDir, "infra")
	err := os.MkdirAll(infraDir, 0755)
	if err != nil {
		return err
	}
	DownloadInfraSource(infraDir)
	sourceDir := path.Join(infraDir, "titvo-security-scan-infra-aws-main")
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		return fmt.Errorf("source directory %s does not exist", sourceDir)
	}
	prodDir := path.Join(sourceDir, "prod", "us-east-1")
	fmt.Println("Deploying infra to ", prodDir)
	currentPathEnv := os.Getenv("PATH")
	var newPathEnv string
	if config.OS == Windows {
		newPathEnv = fmt.Sprintf("%s;%s", currentPathEnv, path.Join(config.TitvoDir, "bin"))
	} else {
		newPathEnv = fmt.Sprintf("%s:%s", currentPathEnv, path.Join(config.TitvoDir, "bin"))
	}
	env := map[string]string{
		"AWS_ACCESS_KEY_ID":     credentials.AWSAccessKeyID,
		"AWS_SECRET_ACCESS_KEY": credentials.AWSSecretAccessKey,
		"AWS_REGION":            credentials.AWSRegion,
		"BUCKET_STATE_NAME":     terraformStateBucket,
		"AWS_STAGE":             "prod",
		"PATH":                  newPathEnv,
	}

	if credentials.AWSSessionToken != "" {
		env["AWS_SESSION_TOKEN"] = credentials.AWSSessionToken
	}

	fmt.Println("Setting up parameters")
	err = PutParameter(&credentials, "/tvo/security-scan/prod/infra/vpc-id", vpcID)
	if err != nil {
		return fmt.Errorf("failed to put parameter vpc-id: %w", err)
	}
	err = PutParameter(&credentials, "/tvo/security-scan/prod/infra/subnet1", subnetID)
	if err != nil {
		return fmt.Errorf("failed to put parameter subnet1: %w", err)
	}
	err = CreateSecret(&credentials, "/tvo/security-scan/prod/aes_secret", aesSecret)
	if err != nil {
		return fmt.Errorf("failed to create secret aes_secret: %w", err)
	}
	err = PutParameter(&credentials, "/tvo/security-scan/prod/infra/encryption-key-name", "/tvo/security-scan/prod/aes_secret")
	if err != nil {
		return fmt.Errorf("failed to put parameter encryption-key-name: %w", err)
	}
	fmt.Println("Executing terragrunt init base infra")
	output, err := ExecuteWithOptions("terragrunt", &ExecuteOptions{
		WorkingDir: prodDir,
		Env:        env,
	}, "run-all", "init", "--terragrunt-non-interactive")
	fmt.Println("Init output:", output)
	if err != nil {
		return fmt.Errorf("terragrunt init failed: %w", err)
	}
	fmt.Println("Executing terragrunt plan base infra")
	output, err = ExecuteWithOptions("terragrunt", &ExecuteOptions{
		WorkingDir: prodDir,
		Env:        env,
	}, "run-all", "plan", "--terragrunt-non-interactive")
	fmt.Println("Plan output:", output)
	if err != nil {
		return fmt.Errorf("terragrunt plan failed: %w", err)
	}
	fmt.Println("Executing terragrunt apply base infra")
	output, err = ExecuteWithOptions("terragrunt", &ExecuteOptions{
		WorkingDir: prodDir,
		Env:        env,
	}, "run-all", "apply", "-input=false", "-auto-approve", "--terragrunt-non-interactive")
	fmt.Println("Apply output:", output)
	if err != nil {
		return fmt.Errorf("terragrunt apply failed: %w", err)
	}
	DownloadSecurityScanInfraSource(path.Join(config.TitvoDir, "security-scan"))
	securityScanDir := path.Join(config.TitvoDir, "security-scan", "titvo-security-scan-main")
	if _, err := os.Stat(securityScanDir); os.IsNotExist(err) {
		return fmt.Errorf("security scan directory %s does not exist", securityScanDir)
	}
	prodDir = path.Join(securityScanDir, "aws")
	fmt.Println("Deploying security scan to ", prodDir)
	fmt.Println("Executing terragrunt init security scan")
	output, err = ExecuteWithOptions("terragrunt", &ExecuteOptions{
		WorkingDir: prodDir,
		Env:        env,
	}, "run-all", "init", "--terragrunt-non-interactive")
	fmt.Println("Security scan init output:", output)
	fmt.Println("Executing terragrunt plan security scan")
	output, err = ExecuteWithOptions("terragrunt", &ExecuteOptions{
		WorkingDir: prodDir,
		Env:        env,
	}, "run-all", "plan", "--terragrunt-non-interactive")
	fmt.Println("Security scan plan output:", output)
	fmt.Println("Executing terragrunt apply security scan")
	output, err = ExecuteWithOptions("terragrunt", &ExecuteOptions{
		WorkingDir: prodDir,
		Env:        env,
	}, "run-all", "apply", "-input=false", "-auto-approve", "--terragrunt-non-interactive")
	fmt.Println("Security scan apply output:", output)
	DownloadAuthSetupSource(path.Join(config.TitvoDir, "auth-setup"))
	authSetupDir := path.Join(config.TitvoDir, "auth-setup", "titvo-auth-setup-aws-main")
	if _, err := os.Stat(authSetupDir); os.IsNotExist(err) {
		return fmt.Errorf("auth setup directory %s does not exist", authSetupDir)
	}
	prodDir = path.Join(authSetupDir, "aws")
	fmt.Println("Deploying auth setup to ", prodDir)
	fmt.Println("Executing terragrunt init auth setup")
	output, err = ExecuteWithOptions("terragrunt", &ExecuteOptions{
		WorkingDir: prodDir,
		Env:        env,
	}, "run-all", "init", "--terragrunt-non-interactive")
	fmt.Println("Auth setup init output:", output)
	fmt.Println("Executing terragrunt plan auth setup")
	output, err = ExecuteWithOptions("terragrunt", &ExecuteOptions{
		WorkingDir: prodDir,
		Env:        env,
	}, "run-all", "plan", "--terragrunt-non-interactive")
	fmt.Println("Auth setup plan output:", output)
	fmt.Println("Executing terragrunt apply auth setup")
	output, err = ExecuteWithOptions("terragrunt", &ExecuteOptions{
		WorkingDir: prodDir,
		Env:        env,
	}, "run-all", "apply", "-input=false", "-auto-approve", "--terragrunt-non-interactive")
	fmt.Println("Auth setup apply output:", output)
	DownloadTaskCliFilesSource(path.Join(config.TitvoDir, "task-cli-files"))
	taskCliFilesDir := path.Join(config.TitvoDir, "task-cli-files", "titvo-task-cli-files-aws-main")
	if _, err := os.Stat(taskCliFilesDir); os.IsNotExist(err) {
		return fmt.Errorf("task cli files directory %s does not exist", taskCliFilesDir)
	}
	prodDir = path.Join(taskCliFilesDir, "aws")
	fmt.Println("Deploying task cli files to ", prodDir)
	fmt.Println("Executing terragrunt init task cli files")
	output, err = ExecuteWithOptions("terragrunt", &ExecuteOptions{
		WorkingDir: prodDir,
		Env:        env,
	}, "run-all", "init", "--terragrunt-non-interactive")
	fmt.Println("Task cli files init output:", output)
	fmt.Println("Executing terragrunt plan task cli files")
	output, err = ExecuteWithOptions("terragrunt", &ExecuteOptions{
		WorkingDir: prodDir,
		Env:        env,
	}, "run-all", "plan", "--terragrunt-non-interactive")
	fmt.Println("Task cli files plan output:", output)
	fmt.Println("Executing terragrunt apply task cli files")
	output, err = ExecuteWithOptions("terragrunt", &ExecuteOptions{
		WorkingDir: prodDir,
		Env:        env,
	}, "run-all", "apply", "-input=false", "-auto-approve", "--terragrunt-non-interactive")
	fmt.Println("Task cli files apply output:", output)
	DownloadTaskTriggerSource(path.Join(config.TitvoDir, "task-trigger"))
	taskTriggerDir := path.Join(config.TitvoDir, "task-trigger", "titvo-task-trigger-aws-main")
	if _, err := os.Stat(taskTriggerDir); os.IsNotExist(err) {
		return fmt.Errorf("task trigger directory %s does not exist", taskTriggerDir)
	}
	prodDir = path.Join(taskTriggerDir, "aws")
	fmt.Println("Deploying task trigger to ", prodDir)
	fmt.Println("Executing terragrunt init task trigger")
	output, err = ExecuteWithOptions("terragrunt", &ExecuteOptions{
		WorkingDir: prodDir,
		Env:        env,
	}, "run-all", "init", "--terragrunt-non-interactive")
	fmt.Println("Task trigger init output:", output)
	fmt.Println("Executing terragrunt plan task trigger")
	output, err = ExecuteWithOptions("terragrunt", &ExecuteOptions{
		WorkingDir: prodDir,
		Env:        env,
	}, "run-all", "plan", "--terragrunt-non-interactive")
	fmt.Println("Task trigger plan output:", output)
	fmt.Println("Executing terragrunt apply task trigger")
	output, err = ExecuteWithOptions("terragrunt", &ExecuteOptions{
		WorkingDir: prodDir,
		Env:        env,
	}, "run-all", "apply", "-input=false", "-auto-approve", "--terragrunt-non-interactive")
	fmt.Println("Task trigger apply output:", output)
	DownloadTaskStatusSource(path.Join(config.TitvoDir, "task-status"))
	taskStatusDir := path.Join(config.TitvoDir, "task-status", "titvo-task-status-aws-main")
	if _, err := os.Stat(taskStatusDir); os.IsNotExist(err) {
		return fmt.Errorf("task status directory %s does not exist", taskStatusDir)
	}
	prodDir = path.Join(taskStatusDir, "aws")
	fmt.Println("Deploying task status to ", prodDir)
	fmt.Println("Executing terragrunt init task status")
	output, err = ExecuteWithOptions("terragrunt", &ExecuteOptions{
		WorkingDir: prodDir,
		Env:        env,
	}, "run-all", "init", "--terragrunt-non-interactive")
	fmt.Println("Task status init output:", output)
	fmt.Println("Executing terragrunt plan task status")
	output, err = ExecuteWithOptions("terragrunt", &ExecuteOptions{
		WorkingDir: prodDir,
		Env:        env,
	}, "run-all", "plan", "--terragrunt-non-interactive")
	fmt.Println("Task status plan output:", output)
	fmt.Println("Executing terragrunt apply task status")
	output, err = ExecuteWithOptions("terragrunt", &ExecuteOptions{
		WorkingDir: prodDir,
		Env:        env,
	}, "run-all", "apply", "-input=false", "-auto-approve", "--terragrunt-non-interactive")
	fmt.Println("Task status apply output:", output)
	return nil
}
