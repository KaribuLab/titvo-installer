package internal

import (
	"bufio"
	"fmt"
	"log/slog"
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

type SetupCredentialsConfig struct {
	AWSCredentialsLookup AWSCredentialsLookup
}

func SetupCredentials() (err error, config *SetupCredentialsConfig) {
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
		awsAccessKeyID, err = bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			fmt.Println("Failed to read AWS Access Key ID", err)
			os.Exit(1)
		}
		awsAccessKeyID = strings.TrimSpace(awsAccessKeyID)
		fmt.Println("Enter your AWS Secret Access Key:")
		awsSecretAccessKey, err = bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			fmt.Println("Failed to read AWS Secret Access Key", err)
			os.Exit(1)
		}
		awsSecretAccessKey = strings.TrimSpace(awsSecretAccessKey)
		fmt.Println("Enter your AWS Session Token:")
		awsSessionToken, err = bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			fmt.Println("Failed to read AWS Session Token", err)
			os.Exit(1)
		}
		awsSessionToken = strings.TrimSpace(awsSessionToken)
		return nil, &SetupCredentialsConfig{
			AWSCredentialsLookup: &InputCredential{
				AWSCredentials: AWSCredentials{
					AWSAccessKeyID:     awsAccessKeyID,
					AWSSecretAccessKey: awsSecretAccessKey,
					AWSSessionToken:    awsSessionToken,
					AWSRegion:          strings.TrimSpace(awsRegion),
				},
			},
		}
	case "2":
		fmt.Println("Enter your AWS Profile:")
		profile, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			fmt.Println("Failed to read profile", err)
			os.Exit(1)
		}
		profile = strings.TrimSpace(profile)
		return nil, &SetupCredentialsConfig{
			AWSCredentialsLookup: &AWSFileCredentials{
				Profile: profile,
				Region:  strings.TrimSpace(awsRegion),
			},
		}
	default:
		slog.Error("Invalid choice", "error", err)
		return fmt.Errorf("invalid choice %s", choice), nil
	}
}
