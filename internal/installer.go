package internal

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func RunInstaller(cmd *cobra.Command, args []string) {
	fmt.Println("Starting Titvo Installer")
	err, _ := SetupCredentials()
	if err != nil {
		fmt.Println("Failed to setup credentials", err)
		os.Exit(1)
	}
	fmt.Println("Credentials setup successfully")
}
