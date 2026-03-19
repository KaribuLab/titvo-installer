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
	AWSAccessKeyID             string `json:"aws_access_key_id"`
	AWSSecretAccessKey         string `json:"aws_secret_access_key"`
	AWSSessionToken            string `json:"aws_session_token"`
	AWSRegion                  string `json:"aws_region"`
	VPCID                      string `json:"vpc_id"`
	PrivateSubnetCIDR          string `json:"private_subnet_cidr"`
	AvailabilityZone           string `json:"availability_zone"`
	NatGatewayID               string `json:"nat_gateway_id"`
	AesSecret                  string `json:"aes_secret"`
	UserName                   string `json:"user_name"`
	OpenAIModel                string `json:"open_ai_model"`
	OpenAIApiKey               string `json:"open_ai_api_key"`
	BitbucketClientKey         string `json:"bitbucket_client_key"`
	BitbucketClientSecret      string `json:"bitbucket_client_secret"`
	GithubAccessToken          string `json:"github_access_token"`
	BitbucketClientKeyCamel    string `json:"bitbucketClientKey"`
	BitbucketClientSecretCamel string `json:"bitbucketClientSecret"`
	GithubAccessTokenCamel     string `json:"githubAccessToken"`
	BitbucketAccessKey         string `json:"bitbucket_access_key"`
	GithubApiKey               string `json:"github_api_key"`
	BitbucketAccessKeyCamel    string `json:"bitbucketAccessKey"`
	GithubApiKeyCamel          string `json:"githubApiKey"`
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
	AWSCredentialsLookup  AWSCredentialsLookup
	VPCID                 string
	PrivateSubnetCIDR     string
	AvailabilityZone      string
	NatGatewayID          string
	AesSecret             string
	UserName              string
	OpenAIModel           string
	OpenAIApiKey          string
	BitbucketClientKey    string
	BitbucketClientSecret string
	GithubAccessToken     string
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func askForPromptInput(awsRegion string) (*SetupConfig, error) {
	var awsAccessKeyID string
	var awsSecretAccessKey string
	var awsSessionToken string
	var vpcID string
	var privateSubnetCIDR string
	var availabilityZone string
	var natGatewayID string
	var aesSecret string
	var userName string
	var openAIModel string
	var openAIApiKey string
	var bitbucketClientKey string
	var bitbucketClientSecret string
	var githubAccessToken string
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
	printAskQuestion("These values will be used to create an isolated private network for Titvo.")
	privateSubnetCIDR, err = askForInput("Enter your private subnet CIDR (e.g. 172.31.64.0/20)", "Private Subnet CIDR")
	if err != nil {
		printErrorAndExit(err)
	}
	availabilityZone, err = askForInput("Enter your Availability Zone (e.g. us-east-1a)", "Availability Zone")
	if err != nil {
		printErrorAndExit(err)
	}
	natGatewayID, err = askForInput("Enter your NAT Gateway ID (e.g. nat-xxxxxxxxxxxxxxxxx)", "NAT Gateway ID")
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
	configureBitbucket, err := askForYesNo("Do you want to configure Bitbucket credentials? (y/N)")
	if err != nil {
		printErrorAndExit(err)
	}
	if configureBitbucket {
		bitbucketClientKey, err = askForPassword("Enter Bitbucket Client Key", "Bitbucket Client Key")
		if err != nil {
			printErrorAndExit(err)
		}
		bitbucketClientSecret, err = askForPassword("Enter Bitbucket Client Secret", "Bitbucket Client Secret")
		if err != nil {
			printErrorAndExit(err)
		}
	} else {
		printAskQuestion("Warning: Bitbucket credentials were not provided. Bitbucket integration deployment will be skipped.")
	}

	configureGithub, err := askForYesNo("Do you want to configure GitHub credentials? (y/N)")
	if err != nil {
		printErrorAndExit(err)
	}
	if configureGithub {
		githubAccessToken, err = askForPassword("Enter GitHub Access Token", "GitHub Access Token")
		if err != nil {
			printErrorAndExit(err)
		}
	} else {
		printAskQuestion("Warning: GitHub access token was not provided. GitHub integration deployment will be skipped.")
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
		VPCID:                 vpcID,
		PrivateSubnetCIDR:     privateSubnetCIDR,
		AvailabilityZone:      availabilityZone,
		NatGatewayID:          natGatewayID,
		AesSecret:             string(aesSecret),
		UserName:              userName,
		OpenAIModel:           openAIModel,
		OpenAIApiKey:          string(openAIApiKey),
		BitbucketClientKey:    string(bitbucketClientKey),
		BitbucketClientSecret: string(bitbucketClientSecret),
		GithubAccessToken:     string(githubAccessToken),
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
