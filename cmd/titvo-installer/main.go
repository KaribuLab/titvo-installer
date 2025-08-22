package main

import (
	"fmt"
	"os"

	"github.com/KaribuLab/titvo-installer/internal"
	"github.com/spf13/cobra"
)

func Run(cmd *cobra.Command, args []string) {
	fmt.Println("Starting Titvo Installer")
	err, _ := internal.SetupCredentials()
	if err != nil {
		fmt.Println("Failed to setup credentials", err)
		os.Exit(1)
	}
	fmt.Println("Credentials setup successfully")
}

func NewRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "titvo-installer",
		Short: "Installer for Titvo",
		Long:  "Installer for Titvo",
		Run:   Run,
	}
	return rootCmd
}

func main() {
	rootCmd := NewRootCommand()
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
