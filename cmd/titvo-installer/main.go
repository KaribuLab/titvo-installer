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

	// Agregar subcomandos secret y parameter
	rootCmd.AddCommand(NewSecretCommand())
	rootCmd.AddCommand(NewParameterCommand())

	return rootCmd
}

func NewSecretCommand() *cobra.Command {
	secretCmd := &cobra.Command{
		Use:   "secret",
		Short: "Agrega un secreto encriptado a la tabla de configuración",
		Long:  "Agrega un secreto a la tabla de configuración de Titvo en DynamoDB. El valor del secreto será encriptado usando AES.",
		Run:   internal.RunSecretLoader,
	}
	secretCmd.Flags().BoolP("debug", "d", false, "Enable debug mode")
	secretCmd.Flags().StringP("config", "c", "", "Archivo de configuración con credenciales de AWS")
	return secretCmd
}

func NewParameterCommand() *cobra.Command {
	paramCmd := &cobra.Command{
		Use:   "parameter",
		Short: "Agrega un parámetro (sin encriptar) a la tabla de configuración",
		Long:  "Agrega un parámetro a la tabla de configuración de Titvo en DynamoDB. El valor se almacena en texto plano.",
		Run:   internal.RunParameterLoader,
	}
	paramCmd.Flags().BoolP("debug", "d", false, "Enable debug mode")
	paramCmd.Flags().StringP("config", "c", "", "Archivo de configuración con credenciales de AWS")
	return paramCmd
}

func main() {
	rootCmd := NewRootCommand()
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
