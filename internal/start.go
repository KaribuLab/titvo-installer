package internal

import (
	"crypto/aes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/google/uuid"
)

const promptFileUrl = "https://raw.githubusercontent.com/KaribuLab/titvo-installer/main/prompt.md"
const reportTemplateFileUrl = "https://raw.githubusercontent.com/KaribuLab/titvo-installer/main/report_template.html"
const apiKeyCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func downloadPromptFile(dir string) (string, error) {
	url := promptFileUrl
	err := downloadFile(url, dir, "prompt.md")
	if err != nil {
		return "", err
	}
	return path.Join(dir, "prompt.md"), nil
}

func downloadReportTemplateFile(dir string) (string, error) {
	url := reportTemplateFileUrl
	err := downloadFile(url, dir, "report_template.html")
	if err != nil {
		return "", err
	}
	return path.Join(dir, "report_template.html"), nil
}

// hashSha256 hashes data using SHA-256
func hashSha256(data []byte) string {
	hash := sha256.New()
	hash.Write(data)
	return hex.EncodeToString(hash.Sum(nil))
}

// generateAPIKey generates a random API key
func generateAPIKey() string {
	const prefix = "tvok-"
	const totalLength = 48
	const suffixLength = totalLength - len(prefix) // 43 characters after the prefix

	// Allowed characters: uppercase letters, lowercase letters and numbers

	// Generate random bytes
	bytes := make([]byte, suffixLength)
	_, err := rand.Read(bytes)
	if err != nil {
		// Fallback using UUID if crypto/rand fails using the uuid package
		uuid := strings.ReplaceAll(uuid.New().String(), "-", "")
		if len(uuid) >= suffixLength {
			return prefix + uuid[:suffixLength]
		}
		// If the UUID is too short, repeat it until it is complete
		var suffix strings.Builder
		for suffix.Len() < suffixLength {
			suffix.WriteString(uuid)
		}
		return prefix + suffix.String()[:suffixLength]
	}

	// Convert bytes to characters of the apiKeyCharset
	var suffix strings.Builder
	for _, b := range bytes {
		suffix.WriteByte(apiKeyCharset[int(b)%len(apiKeyCharset)])
	}

	return prefix + suffix.String()
}

// encrypt encrypts a text using AES in ECB mode
func encrypt(text, key string) (string, error) {
	if len(key) != 32 {
		return "", errors.New("AES_KEY must have 32 characters length")
	}

	// Create the AES cipher
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return "", err
	}

	// Convert text to bytes
	plaintext := []byte(text)

	// Apply padding PKCS7 to make it a multiple of the block size
	blockSize := block.BlockSize()
	padding := blockSize - len(plaintext)%blockSize
	// PKCS7 padding: if text is already multiple of block size, add a full block of padding
	if padding == 0 {
		padding = blockSize
	}
	padtext := make([]byte, len(plaintext)+padding)
	copy(padtext, plaintext)
	for i := len(plaintext); i < len(padtext); i++ {
		padtext[i] = byte(padding)
	}

	// Encrypt using ECB (block by block) using the AES cipher
	encrypted := make([]byte, len(padtext))
	for i := 0; i < len(padtext); i += blockSize {
		block.Encrypt(encrypted[i:i+blockSize], padtext[i:i+blockSize])
	}

	// Return in base64 format
	return base64.StdEncoding.EncodeToString(encrypted), nil
}

type StartConfig struct {
	AWSCredentials *AWSCredentials
	UserName       string
	OpenAIModel    string
	OpenAIApiKey   string
	AESSecret      string
	TitvoDir       string
}

// StartConfiguration starts the configuration
func StartConfiguration(config *StartConfig) error {
	fmt.Println("Starting configuration")
	dynamoUserTableName, err := GetParameter(config.AWSCredentials, "/tvo/security-scan/prod/infra/dynamo-user-table-name")
	if err != nil {
		return err
	}
	userId := uuid.New().String()
	err = PutRecord(config.AWSCredentials, dynamoUserTableName, map[string]interface{}{
		"user_id":      userId,
		"account_type": "Team",
		"name":         config.UserName,
	})
	if err != nil {
		return err
	}
	dynamoAPIKeyTableName, err := GetParameter(config.AWSCredentials, "/tvo/security-scan/prod/infra/dynamo-api-key-table-name")
	if err != nil {
		return err
	}
	keyId := uuid.New().String()
	apiKey := generateAPIKey()
	err = PutRecord(config.AWSCredentials, dynamoAPIKeyTableName, map[string]interface{}{
		"key_id":  keyId,
		"api_key": hashSha256([]byte(apiKey)),
		"user_id": userId,
	})
	if err != nil {
		return err
	}
	dynamoConfigurationTableName, err := GetParameter(config.AWSCredentials, "/tvo/security-scan/prod/infra/dynamo-configuration-table-name")
	if err != nil {
		return err
	}
	err = PutRecord(config.AWSCredentials, dynamoConfigurationTableName, map[string]interface{}{
		"parameter_id": "open_ai_model",
		"value":        config.OpenAIModel,
	})
	if err != nil {
		return err
	}
	cliFilesBucketName, err := GetParameter(config.AWSCredentials, "/tvo/security-scan/prod/infra/cli-files-bucket-name")
	if err != nil {
		return err
	}
	err = PutRecord(config.AWSCredentials, dynamoConfigurationTableName, map[string]interface{}{
		"parameter_id": "cli_files_bucket_name",
		"value":        cliFilesBucketName,
	})
	if err != nil {
		return err
	}
	// Validate AES key has 32 characters
	if len(config.AESSecret) != 32 {
		return fmt.Errorf("AES_KEY must have 32 characters in length")
	}
	openAIApiKey, err := encrypt(config.OpenAIApiKey, config.AESSecret)
	if err != nil {
		return err
	}
	err = PutRecord(config.AWSCredentials, dynamoConfigurationTableName, map[string]interface{}{
		"parameter_id": "open_ai_api_key",
		"value":        openAIApiKey,
	})
	if err != nil {
		return err
	}
	promptFilePath, err := downloadPromptFile(config.TitvoDir)
	if err != nil {
		return err
	}
	// Read prompt file
	promptFile, err := os.ReadFile(promptFilePath)
	if err != nil {
		return err
	}
	err = PutRecord(config.AWSCredentials, dynamoConfigurationTableName, map[string]interface{}{
		"parameter_id": "scan_system_prompt",
		"value":        string(promptFile),
	})
	if err != nil {
		return err
	}
	securityScanJobQueueName, err := GetParameter(config.AWSCredentials, "/tvo/security-scan/prod/infra/security-scan-job-queue-name")
	if err != nil {
		return err
	}
	err = PutRecord(config.AWSCredentials, dynamoConfigurationTableName, map[string]interface{}{
		"parameter_id": "security-scan-job-queue",
		"value":        securityScanJobQueueName,
	})
	if err != nil {
		return err
	}
	// Read report template file
	reportTemplateFilePath, err := downloadReportTemplateFile(config.TitvoDir)
	if err != nil {
		return err
	}
	reportTemplateFile, err := os.ReadFile(reportTemplateFilePath)
	if err != nil {
		return err
	}
	err = PutRecord(config.AWSCredentials, dynamoConfigurationTableName, map[string]interface{}{
		"parameter_id": "report_html_template",
		"value":        string(reportTemplateFile),
	})
	if err != nil {
		return err
	}
	taskEndpoint, err := GetParameter(config.AWSCredentials, "/tvo/security-scan/prod/infra/api-gateway-task-api-full-endpoint")
	if err != nil {
		return err
	}
	err = PutRecord(config.AWSCredentials, dynamoConfigurationTableName, map[string]interface{}{
		"parameter_id": "task_endpoint",
		"value":        taskEndpoint,
	})
	if err != nil {
		return err
	}
	reportBucketName, err := GetParameter(config.AWSCredentials, "/tvo/security-scan/prod/infra/report-bucket-name")
	if err != nil {
		return err
	}
	err = PutRecord(config.AWSCredentials, dynamoConfigurationTableName, map[string]interface{}{
		"parameter_id": "report_bucket_name",
		"value":        reportBucketName,
	})
	if err != nil {
		return err
	}
	reportBucketWebsiteDomain, err := GetParameter(config.AWSCredentials, "/tvo/security-scan/prod/infra/report-bucket-website-domain")
	if err != nil {
		return err
	}
	err = PutRecord(config.AWSCredentials, dynamoConfigurationTableName, map[string]interface{}{
		"parameter_id": "report_bucket_domain",
		"value":        reportBucketWebsiteDomain,
	})
	if err != nil {
		return err
	}
	securityScanJobDefinitionName, err := GetParameter(config.AWSCredentials, "/tvo/security-scan/prod/infra/security-scan-batch-name")
	if err != nil {
		return err
	}
	err = PutRecord(config.AWSCredentials, dynamoConfigurationTableName, map[string]interface{}{
		"parameter_id": "security-scan-job-definition",
		"value":        securityScanJobDefinitionName,
	})
	if err != nil {
		return err
	}
	setupEndpoint, err := GetParameter(config.AWSCredentials, "/tvo/security-scan/prod/infra/api-gateway-account-api-full-endpoint")
	if err != nil {
		return err
	}
	fmt.Println("----------------------------------------------------------------")
	fmt.Printf("- Setup Endpoint: %s\n", setupEndpoint)
	fmt.Println("- User ID: ", userId)
	fmt.Println("- API Key: ", apiKey)
	fmt.Println("----------------------------------------------------------------")
	fmt.Println("* Remember to keep your API Key and User ID in a safe place")
	fmt.Println("----------------------------------------------------------------")
	fmt.Println("Now download the Titvo CLI from the following link:")
	fmt.Println("https://github.com/KaribuLab/tli/releases")
	fmt.Println("----------------------------------------------------------------")
	fmt.Println("And run the following command to setup the Titvo CLI:")
	fmt.Println("tli setup")
	fmt.Println("----------------------------------------------------------------")
	return nil
}
