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
	subnetID, err := askForInput("Enter your Subnet ID (Recommended to use a private subnet with int)", "Subnet ID")
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
	openAIModel, err := askForInput("Enter your OpenAI Model", "OpenAI Model")
	if err != nil {
		printErrorAndExit(err)
	}
	openAIApiKey, err := askForPassword("Enter your OpenAI API Key", "OpenAI API Key")
	if err != nil {
		printErrorAndExit(err)
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
