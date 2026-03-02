package internal

import (
	"fmt"
	"os"
	"path"
	"strings"

	"gopkg.in/ini.v1"
)

type AWSCredentials struct {
	AWSAccessKeyID     string
	AWSSecretAccessKey string
	AWSSessionToken    string
	AWSRegion          string
}

type AWSCredentialsLookup interface {
	GetCredentials() (*AWSCredentials, error)
}

type InputCredential struct {
	AWSCredentials AWSCredentials
}

func (c *InputCredential) GetCredentials() (*AWSCredentials, error) {
	return &c.AWSCredentials, nil
}

type AWSFileCredentials struct {
	Profile string
	Region  string
}

func (c *AWSFileCredentials) GetCredentials() (*AWSCredentials, error) {
	// Get user home directory
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}
	// Look for credentials file in home directory
	credentialsFile := path.Join(home, ".aws", "credentials")
	// Check if credentials file exists
	if _, err := os.Stat(credentialsFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("credentials file not found: %w", err)
	}
	// Load credentials file using INI parser
	cfg, err := ini.Load(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	// Get the profile section
	section, err := cfg.GetSection(c.Profile)
	if err != nil {
		return nil, fmt.Errorf("profile '%s' not found in credentials file: %w", c.Profile, err)
	}

	return &AWSCredentials{
		AWSAccessKeyID:     section.Key("aws_access_key_id").String(),
		AWSSecretAccessKey: section.Key("aws_secret_access_key").String(),
		AWSSessionToken:    section.Key("aws_session_token").String(),
		AWSRegion:          c.Region,
	}, nil
}

type SetupConfigFile struct {
	AWSAccessKeyID     string `json:"aws_access_key_id"`
	AWSSecretAccessKey string `json:"aws_secret_access_key"`
	AWSSessionToken    string `json:"aws_session_token"`
	AWSRegion          string `json:"aws_region"`
	VPCID              string `json:"vpc_id"`
	SubnetID           string `json:"subnet_id"`
	AesSecret          string `json:"aes_secret"`
	UserName           string `json:"user_name"`
	OpenAIModel        string `json:"open_ai_model"`
	OpenAIApiKey       string `json:"open_ai_api_key"`
}

type SetupConfigFileLookup struct {
	SetupConfigFile SetupConfigFile
}

func (c *SetupConfigFileLookup) GetCredentials() (*AWSCredentials, error) {
	return &AWSCredentials{
		AWSAccessKeyID:     c.SetupConfigFile.AWSAccessKeyID,
		AWSSecretAccessKey: c.SetupConfigFile.AWSSecretAccessKey,
		AWSSessionToken:    c.SetupConfigFile.AWSSessionToken,
		AWSRegion:          c.SetupConfigFile.AWSRegion,
	}, nil
}

type SetupConfig struct {
	AWSCredentialsLookup AWSCredentialsLookup
	VPCID                string
	SubnetID             string
	AesSecret            string
	UserName             string
	OpenAIModel          string
	OpenAIApiKey         string
}

func askForPromptInput(awsRegion string) (*SetupConfig, error) {
	var awsAccessKeyID string
	var awsSecretAccessKey string
	var awsSessionToken string
	var vpcID string
	var subnetID string
	var aesSecret string
	var userName string
	var openAIModel string
	var openAIApiKey string
	var err error
	awsAccessKeyID, err = askForPassword("Enter your AWS Access Key ID", "AWS Access Key ID")
	if err != nil {
		printErrorAndExit(err)
	}
	awsSecretAccessKey, err = askForPassword("Enter your AWS Secret Access Key", "AWS Secret Access Key")
	if err != nil {
		printErrorAndExit(err)
	}
	awsSessionToken, err = askForPassword("Enter your AWS Session Token", "AWS Session Token")
	if err != nil {
		printErrorAndExit(err)
	}
	vpcID, err = askForInput("Enter your VPC ID", "VPC ID")
	if err != nil {
		printErrorAndExit(err)
	}
	subnetID, err = askForInput("Enter your Subnet ID (Recommended to use a private subnet with int)", "Subnet ID")
	if err != nil {
		printErrorAndExit(err)
	}
	aesSecret, err = askForPassword("Enter your AES Secret", "AES Secret")
	if err != nil {
		printErrorAndExit(err)
	}
	if len(aesSecret) != 32 {
		printErrorAndExit(fmt.Errorf("AES Secret must have 32 characters in length"))
	}
	userName, err = askForInput("Enter your first Titvo User Name", "Titvo User Name")
	if err != nil {
		printErrorAndExit(err)
	}
	openAIModel, err = askForInput("Enter your OpenAI Model", "OpenAI Model")
	if err != nil {
		printErrorAndExit(err)
	}
	openAIApiKey, err = askForPassword("Enter your OpenAI API Key", "OpenAI API Key")
	if err != nil {
		printErrorAndExit(err)
	}
	return &SetupConfig{
		AWSCredentialsLookup: &InputCredential{
			AWSCredentials: AWSCredentials{
				AWSAccessKeyID:     awsAccessKeyID,
				AWSSecretAccessKey: awsSecretAccessKey,
				AWSSessionToken:    awsSessionToken,
				AWSRegion:          strings.TrimSpace(awsRegion),
			},
		},
		VPCID:        vpcID,
		SubnetID:     subnetID,
		AesSecret:    string(aesSecret),
		UserName:     userName,
		OpenAIModel:  openAIModel,
		OpenAIApiKey: string(openAIApiKey),
	}, nil
}

func SetupInstallation() (config *SetupConfig, err error) {
	printInfo("Setting up Titvo Installer")
	awsRegion, err := askForInput("Enter your AWS Region", "AWS Region")
	if err != nil {
		printErrorAndExit(err)
	}
	choices := []choice{
		{
			Label: "Input",
			Value: "1",
			Callback: func() (any, error) {
				return askForPromptInput(awsRegion)
			},
		},
		{
			Label: "File",
			Value: "2",
			Callback: func() (any, error) {
				return askForCredentialsFile(awsRegion)
			},
		},
	}
	result, err := askForChoices("You want to give the credentials from input or a credentials file?", choices)
	if err != nil {
		printErrorAndExit(err)
	}
	config, ok := result.(*SetupConfig)
	if !ok {
		return nil, fmt.Errorf("unexpected type returned from askForChoices")
	}
	return config, nil
}
