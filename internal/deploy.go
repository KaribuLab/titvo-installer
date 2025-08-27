package internal

import (
	"fmt"
	"os"
	"path"
)

const titvoInfraSource = "https://github.com/KaribuLab/titvo-security-scan-infra-aws/archive/refs/heads/main.zip"

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
	return nil
}
