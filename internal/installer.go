package internal

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func RunInstaller(cmd *cobra.Command, args []string) {
	fmt.Println("Starting Titvo Installer")
	err, credentials := SetupCredentials()
	if err != nil {
		fmt.Println("Failed to setup credentials", err)
		os.Exit(1)
	}
	fmt.Println("Credentials setup successfully")
	err, config := InstallTools()
	if err != nil {
		fmt.Println("Failed to install tools", err)
		os.Exit(1)
	}
	fmt.Println("Tools installed successfully")
	awsCredentials, err := credentials.AWSCredentialsLookup.GetCredentials()
	if err != nil {
		fmt.Println("Failed to get aws credentials", err)
		os.Exit(1)
	}
	err = DeployInfra(*awsCredentials, config, credentials.TerraformStateBucket)
	if err != nil {
		fmt.Println("Failed to deploy infra", err)
		os.Exit(1)
	}
	fmt.Println("Infra deployed successfully")
}
