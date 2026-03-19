package internal

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func RunInstaller(cmd *cobra.Command, args []string) {
	debug, err := cmd.Flags().GetBool("debug")
	if err != nil {
		printErrorAndExit(err)
	}
	if debug {
		printInfo("Debug mode enabled")
	}
	configFile, err := cmd.Flags().GetString("config")
	if err != nil {
		printErrorAndExit(err)
	}
	printInfo("Starting Titvo Installer")
	var setup *SetupConfig
	if configFile != "" {
		printInfo(fmt.Sprintf("Using config file %s", configFile))
		configFileBytes, err := os.ReadFile(configFile)
		if err != nil {
			printErrorAndExit(err)
		}
		var setupConfigFile SetupConfigFile
		err = json.Unmarshal(configFileBytes, &setupConfigFile)
		if err != nil {
			printErrorAndExit(err)
		}
		if len(setupConfigFile.AesSecret) != 32 {
			printErrorAndExit(fmt.Errorf("AES Secret in config file must have 32 characters in length"))
		}
		setup = &SetupConfig{
			AWSCredentialsLookup: &SetupConfigFileLookup{
				SetupConfigFile: setupConfigFile,
			},
			VPCID:                 setupConfigFile.VPCID,
			PrivateSubnetCIDR:     setupConfigFile.PrivateSubnetCIDR,
			AvailabilityZone:      setupConfigFile.AvailabilityZone,
			NatGatewayID:          setupConfigFile.NatGatewayID,
			AesSecret:             setupConfigFile.AesSecret,
			UserName:              setupConfigFile.UserName,
			OpenAIModel:           setupConfigFile.OpenAIModel,
			OpenAIApiKey:          setupConfigFile.OpenAIApiKey,
			BitbucketClientKey:    firstNonEmpty(setupConfigFile.BitbucketClientKey, setupConfigFile.BitbucketClientKeyCamel, setupConfigFile.BitbucketAccessKey, setupConfigFile.BitbucketAccessKeyCamel),
			BitbucketClientSecret: firstNonEmpty(setupConfigFile.BitbucketClientSecret, setupConfigFile.BitbucketClientSecretCamel),
			GithubAccessToken:     firstNonEmpty(setupConfigFile.GithubAccessToken, setupConfigFile.GithubAccessTokenCamel, setupConfigFile.GithubApiKey, setupConfigFile.GithubApiKeyCamel),
		}
	} else {
		setup, err = SetupInstallation()
		if err != nil {
			printErrorAndExit(err)
		}
	}
	printInfo("Setup successfully")
	tool, err := InstallTools()
	if err != nil {
		printErrorAndExit(err)
	}
	printInfo("Tools installed successfully")
	awsCredentials, err := setup.AWSCredentialsLookup.GetCredentials()
	if err != nil {
		printErrorAndExit(err)
	}
	err = DeployInfra(DeployConfig{
		AWSCredentials:        *awsCredentials,
		InstallToolConfig:     *tool,
		VPCID:                 setup.VPCID,
		PrivateSubnetCIDR:     setup.PrivateSubnetCIDR,
		AvailabilityZone:      setup.AvailabilityZone,
		NatGatewayID:          setup.NatGatewayID,
		AESSecret:             setup.AesSecret,
		BitbucketClientKey:    setup.BitbucketClientKey,
		BitbucketClientSecret: setup.BitbucketClientSecret,
		GithubAccessToken:     setup.GithubAccessToken,
		Debug:                 debug,
	})
	if err != nil {
		printErrorAndExit(err)
	}
	printInfo("Infra deployed successfully")
	startConfig := StartConfig{
		AWSCredentials: awsCredentials,
		UserName:       setup.UserName,
		OpenAIModel:    setup.OpenAIModel,
		OpenAIApiKey:   setup.OpenAIApiKey,
		AESSecret:      setup.AesSecret,
		TitvoDir:       tool.TitvoDir,
	}
	err = StartConfiguration(&startConfig)
	if err != nil {
		printErrorAndExit(err)
	}
	printInfo("Configuration started successfully")
}
