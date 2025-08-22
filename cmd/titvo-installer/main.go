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
	return rootCmd
}

func main() {
	rootCmd := NewRootCommand()
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
