package internal

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

const apiKeyCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

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

type StartConfig struct {
	AWSCredentials    *AWSCredentials
	UserName          string
	AIProvider        string
	AIModel           string
	AIApiKey          string
	EmbeddingProvider string
	EmbeddingModel    string
	EmbeddingApiKey   string
	AESSecret         string
	TitvoDir          string
}

// StartConfiguration starts the configuration, skipping it when a previous run
// already completed it. This step is not idempotent (it creates a new user and
// API key on every run), so it must only run once per installation.
func StartConfiguration(config *StartConfig) error {
	state, err := loadInstallState(config.TitvoDir)
	if err != nil {
		return err
	}
	configMod := state.module("configuration")
	return runOnce(state, &configMod.Applied, "Skipping configuration (already completed)", func() error {
		return runStartConfiguration(config)
	})
}

// runStartConfiguration performs the actual configuration work.
func runStartConfiguration(config *StartConfig) error {
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

	// RAG index bucket from SSM
	ragIndexBucketName, err := GetParameter(config.AWSCredentials, "/tvo/security-scan/prod/infra/s3/rag-index/bucket_name")
	if err != nil {
		return err
	}
	err = PutRecord(config.AWSCredentials, dynamoConfigurationTableName, map[string]interface{}{
		"parameter_id": "rag_index_bucket",
		"value":        ragIndexBucketName,
	})
	if err != nil {
		return err
	}

	// embedding_provider: use config value or fall back to ai_provider
	embeddingProvider := config.EmbeddingProvider
	if embeddingProvider == "" {
		embeddingProvider = config.AIProvider
	}
	err = PutRecord(config.AWSCredentials, dynamoConfigurationTableName, map[string]interface{}{
		"parameter_id": "embedding_provider",
		"value":        embeddingProvider,
	})
	if err != nil {
		return err
	}

	// embedding_model (required, plain text)
	err = PutRecord(config.AWSCredentials, dynamoConfigurationTableName, map[string]interface{}{
		"parameter_id": "embedding_model",
		"value":        config.EmbeddingModel,
	})
	if err != nil {
		return err
	}

	// embedding_api_key: use config value or fall back to ai_api_key
	embeddingRawApiKey := config.EmbeddingApiKey
	if embeddingRawApiKey == "" {
		embeddingRawApiKey = config.AIApiKey
	}
	embeddingApiKey, err := encrypt(embeddingRawApiKey, config.AESSecret)
	if err != nil {
		return err
	}
	err = PutRecord(config.AWSCredentials, dynamoConfigurationTableName, map[string]interface{}{
		"parameter_id": "embedding_api_key",
		"value":        embeddingApiKey,
	})
	if err != nil {
		return err
	}

	setupEndpoint, err := GetParameter(config.AWSCredentials, "/tvo/security-scan/prod/infra/apigateway/task/api_gateway_api_full_endpoint")
	if err != nil {
		return err
	}
	// Admin console URL: the CloudFront domain fronting titvo-admin-web (see
	// deploy_runtime.go's adminWebDomainNameParam). A missing/unreadable
	// parameter here must not fail the whole setup summary — the admin
	// console is deployed alongside everything else in DeployInfra, but a
	// user who only cares about the User ID/API Key should still get them.
	adminConsoleURL, adminConsoleErr := GetParameter(config.AWSCredentials, adminWebDomainNameParam)
	printInfo("----------------------------------------------------------------")
	printInfo(fmt.Sprintf("- Setup Endpoint: %s", setupEndpoint))
	printInfo(fmt.Sprintf("- User ID: %s", userId))
	printInfo(fmt.Sprintf("- API Key: %s", apiKey))
	if adminConsoleErr == nil && adminConsoleURL != "" {
		printInfo(fmt.Sprintf("- Admin Console: https://%s", adminConsoleURL))
	}
	printInfo("----------------------------------------------------------------")
	printInfo("* Remember to keep your API Key and User ID in a safe place")
	printInfo("----------------------------------------------------------------")
	return nil
}
