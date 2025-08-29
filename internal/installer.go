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
		fmt.Println("Failed to get debug flag", err)
		os.Exit(1)
	}
	if debug {
		fmt.Println("Debug mode enabled")
	}
	configFile, err := cmd.Flags().GetString("config")
	if err != nil {
		fmt.Println("Failed to get config flag", err)
		os.Exit(1)
	}
	fmt.Println("Starting Titvo Installer")
	var setup *SetupConfig
	if configFile != "" {
		fmt.Println("Using config file", configFile)
		configFileBytes, err := os.ReadFile(configFile)
		if err != nil {
			fmt.Println("Failed to read config file", err)
			os.Exit(1)
		}
		var setupConfigFile SetupConfigFile
		err = json.Unmarshal(configFileBytes, &setupConfigFile)
		if err != nil {
			fmt.Println("Failed to unmarshal config file", err)
			os.Exit(1)
		}
		setup = &SetupConfig{
			AWSCredentialsLookup: &SetupConfigFileLookup{
				SetupConfigFile: setupConfigFile,
			},
			VPCID:     setupConfigFile.VPCID,
			SubnetID:  setupConfigFile.SubnetID,
			AesSecret: setupConfigFile.AesSecret,
		}
	} else {
		setup, err = SetupInstallation()
		if err != nil {
			fmt.Println("Failed to setup", err)
			os.Exit(1)
		}
	}
	fmt.Println("Setup successfully")
	tool, err := InstallTools()
	if err != nil {
		fmt.Println("Failed to install tools", err)
		os.Exit(1)
	}
	fmt.Println("Tools installed successfully")
	awsCredentials, err := setup.AWSCredentialsLookup.GetCredentials()
	if err != nil {
		fmt.Println("Failed to get aws setup", err)
		os.Exit(1)
	}
	err = DeployInfra(DeployConfig{
		AWSCredentials:    *awsCredentials,
		InstallToolConfig: *tool,
		VPCID:             setup.VPCID,
		SubnetID:          setup.SubnetID,
		AESSecret:         setup.AesSecret,
		Debug:             debug,
	})
	if err != nil {
		fmt.Println("Failed to deploy infra", err)
		os.Exit(1)
	}
	fmt.Println("Infra deployed successfully")
}
