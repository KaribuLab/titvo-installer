package main

import (
	"os"

	"github.com/KaribuLab/titvo-installer/internal"
	"github.com/spf13/cobra"
)

func NewRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "titvo-installer",
		Short: "Installer for Titvo",
		Long:  "Installer for Titvo",
		Run:   internal.RunInstaller,
	}
	rootCmd.Flags().BoolP("debug", "d", false, "Enable debug mode")
	rootCmd.Flags().StringP("config", "c", "", "Configuration file")

	// Agregar subcomando add-secret
	rootCmd.AddCommand(NewAddSecretCommand())

	return rootCmd
}

func NewAddSecretCommand() *cobra.Command {
	addSecretCmd := &cobra.Command{
		Use:   "add-secret",
		Short: "Add a secret to the Titvo configuration table",
		Long:  "Add a secret to the Titvo configuration table in DynamoDB. The secret value will be encrypted using AES.",
		Run:   internal.RunSecretLoader,
	}
	addSecretCmd.Flags().BoolP("debug", "d", false, "Enable debug mode")
	addSecretCmd.Flags().StringP("config", "c", "", "Configuration file with AWS credentials")
	return addSecretCmd
}

func main() {
	rootCmd := NewRootCommand()
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
