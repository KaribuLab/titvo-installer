package internal

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"path"
	"strings"

	"golang.org/x/term"
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

func SetupInstallation() (config *SetupConfig, err error) {
	fmt.Println("Setting up Titvo Installer")
	var awsRegion string
	fmt.Println("Enter your AWS Region:")
	awsRegion, err = bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		fmt.Println("Failed to read AWS Region", err)
		os.Exit(1)
	}
	fmt.Println("You want to give the credentials from input or a credentials file?")
	fmt.Println("1. Input")
	fmt.Println("2. File")
	fmt.Println("Enter your choice:")
	choice, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		fmt.Println("Failed to read choice", err)
		os.Exit(1)
	}
	choice = strings.TrimSpace(choice)
	switch choice {
	case "1":
		var awsAccessKeyID string
		var awsSecretAccessKey string
		var awsSessionToken string
		fmt.Println("Enter your AWS Access Key ID:")
		accessKeyBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			fmt.Println("Failed to read AWS Access Key ID", err)
			os.Exit(1)
		}
		awsAccessKeyID = strings.TrimSpace(string(accessKeyBytes))
		fmt.Println("Enter your AWS Secret Access Key:")
		secretKeyBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			fmt.Println("Failed to read AWS Secret Access Key", err)
			os.Exit(1)
		}
		awsSecretAccessKey = strings.TrimSpace(string(secretKeyBytes))
		fmt.Println("Enter your AWS Session Token:")
		sessionTokenBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			fmt.Println("Failed to read AWS Session Token", err)
			os.Exit(1)
		}
		awsSessionToken = strings.TrimSpace(string(sessionTokenBytes))
		fmt.Println("Enter your VPC ID:")
		vpcID, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			fmt.Println("Failed to read VPC ID", err)
			os.Exit(1)
		}
		vpcID = strings.TrimSpace(vpcID)
		fmt.Println("Enter your Subnet ID (Recommended to use a private subnet with int):")
		subnetID, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			fmt.Println("Failed to read Subnet ID", err)
			os.Exit(1)
		}
		subnetID = strings.TrimSpace(subnetID)
		fmt.Println("Enter your AES Secret:")
		aesSecret, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			fmt.Println("Failed to read AES Secret", err)
			os.Exit(1)
		}
		if len(aesSecret) != 32 {
			fmt.Println("AES Secret must have 32 characters in length")
			os.Exit(1)
		}
		fmt.Println("Enter your first Titvo User Name:")
		userName, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			fmt.Println("Failed to read first Titvo User Name", err)
			os.Exit(1)
		}
		userName = strings.TrimSpace(userName)
		fmt.Println("Enter your OpenAI Model:")
		openAIModel, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			fmt.Println("Failed to read OpenAI Model", err)
			os.Exit(1)
		}
		openAIModel = strings.TrimSpace(openAIModel)
		fmt.Println("Enter your OpenAI API Key:")
		openAIApiKey, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			fmt.Println("Failed to read OpenAI API Key", err)
			os.Exit(1)
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
	case "2":
		fmt.Println("Enter your AWS Profile:")
		profile, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			fmt.Println("Failed to read profile", err)
			os.Exit(1)
		}
		profile = strings.TrimSpace(profile)
		fmt.Println("Enter your VPC ID:")
		vpcID, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			fmt.Println("Failed to read VPC ID", err)
			os.Exit(1)
		}
		vpcID = strings.TrimSpace(vpcID)
		fmt.Println("Enter your Subnet ID:")
		subnetID, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			fmt.Println("Failed to read Subnet ID", err)
			os.Exit(1)
		}
		subnetID = strings.TrimSpace(subnetID)
		fmt.Println("Enter your AES Secret:")
		aesSecret, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			fmt.Println("Failed to read AES Secret", err)
			os.Exit(1)
		}
		if len(aesSecret) != 32 {
			fmt.Println("AES Secret must have 32 characters in length")
			os.Exit(1)
		}
		fmt.Println("Enter your first Titvo User Name:")
		userName, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			fmt.Println("Failed to read first Titvo User Name", err)
			os.Exit(1)
		}
		userName = strings.TrimSpace(userName)
		fmt.Println("Enter your OpenAI Model:")
		openAIModel, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			fmt.Println("Failed to read OpenAI Model", err)
			os.Exit(1)
		}
		openAIModel = strings.TrimSpace(openAIModel)
		fmt.Println("Enter your OpenAI API Key:")
		openAIApiKey, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			fmt.Println("Failed to read OpenAI API Key", err)
			os.Exit(1)
		}
		return &SetupConfig{
			AWSCredentialsLookup: &AWSFileCredentials{
				Profile: profile,
				Region:  strings.TrimSpace(awsRegion),
			},
			VPCID:        vpcID,
			SubnetID:     subnetID,
			AesSecret:    string(aesSecret),
			UserName:     userName,
			OpenAIModel:  openAIModel,
			OpenAIApiKey: string(openAIApiKey),
		}, nil
	default:
		slog.Error("Invalid choice", "error", err)
		return nil, fmt.Errorf("invalid choice %s", choice)
	}
}
