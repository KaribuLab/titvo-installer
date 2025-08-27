package internal

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func RunInstaller(cmd *cobra.Command, args []string) {
	fmt.Println("Starting Titvo Installer")
	err, setup := SetupInstallation()
	if err != nil {
		fmt.Println("Failed to setup", err)
		os.Exit(1)
	}
	fmt.Println("Setup successfully")
	err, tool := InstallTools()
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
	err = DeployInfra(*awsCredentials, tool, setup.TerraformStateBucket, setup.VPCID, setup.SubnetID, setup.AesSecret)
	if err != nil {
		fmt.Println("Failed to deploy infra", err)
		os.Exit(1)
	}
	fmt.Println("Infra deployed successfully")
}
