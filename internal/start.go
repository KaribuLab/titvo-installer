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

const promptFileUrl = "https://raw.githubusercontent.com/KaribuLab/titvo-installer/main/system_prompt.md"
const contentTemplateFileUrl = "https://raw.githubusercontent.com/KaribuLab/titvo-installer/main/content_template.md"
const apiKeyCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func downloadPromptFile(dir string) (string, error) {
	url := promptFileUrl
	err := downloadFile(url, dir, "system_prompt.md")
	if err != nil {
		return "", err
	}
	return path.Join(dir, "system_prompt.md"), nil
}

func downloadContentTemplateFile(dir string) (string, error) {
	url := contentTemplateFileUrl
	err := downloadFile(url, dir, "content_template.md")
	if err != nil {
		return "", err
	}
	return path.Join(dir, "content_template.md"), nil
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
	AIProvider     string
	AIModel        string
	AIApiKey       string
	AESSecret      string
	TitvoDir       string
}

// StartConfiguration starts the configuration
func StartConfiguration(config *StartConfig) error {
	printInfo("Starting configuration")
	dynamoUserTableName, err := GetParameter(config.AWSCredentials, "/tvo/security-scan/prod/infra/dynamo/user-table-name")
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
	dynamoAPIKeyTableName, err := GetParameter(config.AWSCredentials, "/tvo/security-scan/prod/infra/dynamo/apikey-table-name")
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
	dynamoConfigurationTableName, err := GetParameter(config.AWSCredentials, "/tvo/security-scan/prod/infra/dynamo/parameter-table-name")
	if err != nil {
		return err
	}
	err = PutRecord(config.AWSCredentials, dynamoConfigurationTableName, map[string]interface{}{
		"parameter_id": "ai_provider",
		"value":        config.AIProvider,
	})
	if err != nil {
		return err
	}
	err = PutRecord(config.AWSCredentials, dynamoConfigurationTableName, map[string]interface{}{
		"parameter_id": "ai_model",
		"value":        config.AIModel,
	})
	if err != nil {
		return err
	}
	cliFilesBucketName, err := GetParameter(config.AWSCredentials, "/tvo/security-scan/prod/infra/s3/cli-files/bucket_name")
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
	aiApiKey, err := encrypt(config.AIApiKey, config.AESSecret)
	if err != nil {
		return err
	}
	err = PutRecord(config.AWSCredentials, dynamoConfigurationTableName, map[string]interface{}{
		"parameter_id": "ai_api_key",
		"value":        aiApiKey,
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
	securityScanJobQueueName, err := GetParameter(config.AWSCredentials, "/tvo/security-scan/prod/infra/batch/agent/job_queue_name")
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
	// Read content template file
	contentTemplateFilePath, err := downloadContentTemplateFile(config.TitvoDir)
	if err != nil {
		return err
	}
	contentTemplateFile, err := os.ReadFile(contentTemplateFilePath)
	if err != nil {
		return err
	}
	err = PutRecord(config.AWSCredentials, dynamoConfigurationTableName, map[string]interface{}{
		"parameter_id": "content_template",
		"value":        string(contentTemplateFile),
	})
	if err != nil {
		return err
	}
	taskEndpoint, err := GetParameter(config.AWSCredentials, "/tvo/security-scan/prod/infra/apigateway/task/api_gateway_api_full_endpoint")
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
	err = PutRecord(config.AWSCredentials, dynamoConfigurationTableName, map[string]interface{}{
		"parameter_id": "mcp_server_url",
		"value":        "http://gateway.internal.titvo.com:3000/mcp",
	})
	if err != nil {
		return err
	}
	securityScanJobDefinitionName, err := GetParameter(config.AWSCredentials, "/tvo/security-scan/prod/infra/batch/agent/job_definition_name")
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
	setupEndpoint, err := GetParameter(config.AWSCredentials, "/tvo/security-scan/prod/infra/apigateway/task/api_gateway_api_full_endpoint")
	if err != nil {
		return err
	}
	printInfo("----------------------------------------------------------------")
	printInfo(fmt.Sprintf("- Setup Endpoint: %s", setupEndpoint))
	printInfo(fmt.Sprintf("- User ID: %s", userId))
	printInfo(fmt.Sprintf("- API Key: %s", apiKey))
	printInfo("----------------------------------------------------------------")
	printInfo("* Remember to keep your API Key and User ID in a safe place")
	printInfo("----------------------------------------------------------------")
	printInfo("Now download the Titvo CLI from the following link:")
	printInfo("https://github.com/KaribuLab/tli/releases")
	printInfo("----------------------------------------------------------------")
	printInfo("And run the following command to setup the Titvo CLI:")
	printInfo("tli setup")
	printInfo("----------------------------------------------------------------")
	return nil
}
