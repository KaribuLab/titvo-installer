package internal

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"golang.org/x/term"
)

func printError(err error) {
	fmt.Println(color.RedString(err.Error()))
}

func printErrorAndExit(err error) {
	printError(err)
	os.Exit(1)
}

func printInfo(message string) {
	fmt.Println(color.GreenString(message))
}

func printAskQuestion(message string) {
	fmt.Println(color.YellowString(message))
}

func askForCredentialsFile(awsRegion string) (*SetupConfig, error) {
	profile, err := askForInput("Enter your AWS Profile", "AWS Profile")
	if err != nil {
		printErrorAndExit(err)
	}
	vpcID, err := askForInput("Enter your VPC ID", "VPC ID")
	if err != nil {
		printErrorAndExit(err)
	}
	printAskQuestion("These values will be used to create an isolated private network for Titvo.")
	privateSubnetCIDR, err := askForInput("Enter your private subnet CIDR (e.g. 172.31.64.0/20)", "Private Subnet CIDR")
	if err != nil {
		printErrorAndExit(err)
	}
	availabilityZone, err := askForInput("Enter your Availability Zone (e.g. us-east-1a)", "Availability Zone")
	if err != nil {
		printErrorAndExit(err)
	}
	natGatewayID, err := askForInput("Enter your NAT Gateway ID (e.g. nat-xxxxxxxxxxxxxxxxx)", "NAT Gateway ID")
	if err != nil {
		printErrorAndExit(err)
	}
	aesSecret, err := askForPassword("Enter your AES Secret", "AES Secret")
	if err != nil {
		printErrorAndExit(err)
	}
	userName, err := askForInput("Enter your first Titvo User Name", "Titvo User Name")
	if err != nil {
		printErrorAndExit(err)
	}
	aiProvider, err := askForAIProvider()
	if err != nil {
		printErrorAndExit(err)
	}
	aiModel, err := askForInput("Enter your AI Model", "AI Model")
	if err != nil {
		printErrorAndExit(err)
	}
	aiApiKey, err := askForPassword("Enter your AI API Key", "AI API Key")
	if err != nil {
		printErrorAndExit(err)
	}

	bitbucketAPIToken := ""
	configureBitbucket, err := askForYesNo("Do you want to configure Bitbucket credentials? (y/N)")
	if err != nil {
		printErrorAndExit(err)
	}
	if configureBitbucket {
		bitbucketAPIToken, err = askForPassword("Enter Bitbucket API Token", "Bitbucket API Token")
		if err != nil {
			printErrorAndExit(err)
		}
	} else {
		printAskQuestion("Warning: Bitbucket credentials were not provided. Bitbucket integration deployment will be skipped.")
	}

	githubAccessToken := ""
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
		AWSCredentialsLookup: &AWSFileCredentials{
			Profile: profile,
			Region:  strings.TrimSpace(awsRegion),
		},
		VPCID:             vpcID,
		PrivateSubnetCIDR: privateSubnetCIDR,
		AvailabilityZone:  availabilityZone,
		NatGatewayID:      natGatewayID,
		AesSecret:         string(aesSecret),
		UserName:          userName,
		AIProvider:        aiProvider,
		AIModel:           aiModel,
		AIApiKey:          string(aiApiKey),
		BitbucketAPIToken: string(bitbucketAPIToken),
		GithubAccessToken: string(githubAccessToken),
	}, nil
}

func askForInputWithDefault(question string, inputName string, defaultValue string) (string, error) {
	if defaultValue != "" {
		printAskQuestion(fmt.Sprintf("%s: (default: %s)", question, defaultValue))
	} else {
		printAskQuestion(question)
	}
	reader := bufio.NewReader(os.Stdin)
	answer, err := reader.ReadString('\n')
	if err != nil {
		printError(fmt.Errorf("error reading %s: %v", inputName, err))
		return "", err
	}
	if len(strings.TrimSpace(answer)) == 0 && defaultValue == "" {
		return "", fmt.Errorf("%s is empty", inputName)
	}
	if len(strings.TrimSpace(answer)) == 0 && defaultValue != "" {
		return defaultValue, nil
	}
	return strings.TrimSpace(answer), nil
}

func askForInput(question string, inputName string) (string, error) {
	return askForInputWithDefault(question, inputName, "")
}

func askForPassword(question string, inputName string) (string, error) {
	printAskQuestion(fmt.Sprintf("%s: ", question))
	answer, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		printError(fmt.Errorf("error reading %s: %v", inputName, err))
		return "", err
	}
	if len(strings.TrimSpace(string(answer))) == 0 {
		return "", fmt.Errorf("%s is empty", inputName)
	}
	return strings.TrimSpace(string(answer)), nil
}

func askForYesNo(question string) (bool, error) {
	printAskQuestion(question)
	reader := bufio.NewReader(os.Stdin)
	answer, err := reader.ReadString('\n')
	if err != nil {
		printError(fmt.Errorf("error reading yes/no input: %v", err))
		return false, err
	}
	normalized := strings.ToLower(strings.TrimSpace(answer))
	if normalized == "" || normalized == "n" || normalized == "no" {
		return false, nil
	}
	if normalized == "y" || normalized == "yes" {
		return true, nil
	}
	return false, fmt.Errorf("invalid choice: %s", strings.TrimSpace(answer))
}

type choiceCallback func() (any, error)

type choice struct {
	Label    string
	Value    string
	Callback choiceCallback
}

func askForChoices(question string, choices []choice) (any, error) {
	printAskQuestion(question)
	for _, choice := range choices {
		printAskQuestion(fmt.Sprintf("- %s: %s", choice.Label, choice.Value))
	}
	answer, err := askForInput("Enter your choice", "Choice")
	if err != nil {
		return nil, err
	}
	for _, choice := range choices {
		if choice.Value == answer {
			return choice.Callback()
		}
	}
	return nil, fmt.Errorf("invalid choice: %s", answer)
}
