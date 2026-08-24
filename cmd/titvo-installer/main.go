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

	// Agregar subcomandos secret, parameter y seed-admin
	rootCmd.AddCommand(NewSecretCommand())
	rootCmd.AddCommand(NewParameterCommand())
	rootCmd.AddCommand(NewAdminSeedCommand())

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

func NewAdminSeedCommand() *cobra.Command {
	adminSeedCmd := &cobra.Command{
		Use:   "seed-admin",
		Short: "Crea el primer usuario admin de la consola de administración",
		Long:  "Crea el primer usuario con role=admin en la tabla de usuarios de Titvo, para acceder a la consola de administración. Es idempotente: si ya existe un admin, no crea uno nuevo.",
		Run:   internal.RunAdminSeeder,
	}
	adminSeedCmd.Flags().BoolP("debug", "d", false, "Enable debug mode")
	adminSeedCmd.Flags().StringP("config", "c", "", "Archivo de configuración con credenciales de AWS (y opcionalmente admin_email/admin_password)")
	return adminSeedCmd
}

func main() {
	rootCmd := NewRootCommand()
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
