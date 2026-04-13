package internal

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// SecretLoaderCredentials estructura para las credenciales AWS desde archivo de configuración
type SecretLoaderCredentials struct {
	AWSAccessKeyID     string `json:"aws_access_key_id"`
	AWSSecretAccessKey string `json:"aws_secret_access_key"`
	AWSSessionToken    string `json:"aws_session_token"`
	AWSRegion          string `json:"aws_region"`
}

// SecretLoaderConfig contiene la configuración completa para el secret loader
type SecretLoaderConfig struct {
	AWSCredentials AWSCredentials
}

// RunSecretLoader ejecuta el flujo para cargar un secreto en la tabla de configuración
func RunSecretLoader(cmd *cobra.Command, args []string) {
	debug, err := cmd.Flags().GetBool("debug")
	if err != nil {
		printErrorAndExit(err)
	}
	if debug {
		printInfo("Modo debug habilitado")
	}

	configFile, err := cmd.Flags().GetString("config")
	if err != nil {
		printErrorAndExit(err)
	}

	printInfo("Iniciando cargador de secretos")
	printInfo("Este comando requiere que Titvo esté instalado. Verificando...")

	var awsCredentials *AWSCredentials

	if configFile != "" {
		printInfo(fmt.Sprintf("Usando archivo de configuración %s", configFile))
		awsCredentials, err = loadCredentialsFromFile(configFile)
		if err != nil {
			printErrorAndExit(err)
		}
	} else {
		awsCredentials, err = loadCredentialsFromInput()
		if err != nil {
			printErrorAndExit(err)
		}
	}

	// Verificar que Titvo está instalado (el secreto AES existe)
	_, err = GetSecret(awsCredentials, "/tvo/security-scan/prod/aes_secret")
	if err != nil {
		printErrorAndExit(fmt.Errorf("Error: Titvo no parece estar instalado. Por favor ejecuta primero: titvo-installer"))
	}

	printInfo("Titvo está instalado. Continuando...")

	// Pedir nombre del secreto
	secretName, err := askForInput("Ingresa el nombre del secreto (parameter_id)", "Nombre del secreto")
	if err != nil {
		printErrorAndExit(err)
	}

	// Pedir valor del secreto (input oculto)
	secretValue, err := askForPassword("Ingresa el valor del secreto", "Valor del secreto")
	if err != nil {
		printErrorAndExit(err)
	}

	// Obtener la clave AES desde Secret Manager
	printInfo("Obteniendo clave de encriptación desde Secret Manager...")
	aesSecretEncoded, err := GetSecret(awsCredentials, "/tvo/security-scan/prod/aes_secret")
	if err != nil {
		printErrorAndExit(fmt.Errorf("error al obtener clave AES: %w", err))
	}

	// Decodificar el valor base64
	aesSecretBytes, err := base64.StdEncoding.DecodeString(aesSecretEncoded)
	if err != nil {
		printErrorAndExit(fmt.Errorf("error al decodificar clave AES: %w", err))
	}
	aesSecret := string(aesSecretBytes)

	// Validar que la clave tenga 32 caracteres
	if len(aesSecret) != 32 {
		printErrorAndExit(fmt.Errorf("la clave AES debe tener 32 caracteres"))
	}

	// Encriptar el valor
	printInfo("Encriptando valor del secreto...")
	encryptedValue, err := encrypt(secretValue, aesSecret)
	if err != nil {
		printErrorAndExit(fmt.Errorf("error al encriptar el valor: %w", err))
	}

	// Insertar en DynamoDB
	printInfo("Insertando secreto en la tabla de configuración...")
	err = PutRecord(awsCredentials, "tvo-security-scan-parameter-prod", map[string]interface{}{
		"parameter_id": secretName,
		"value":        encryptedValue,
	})
	if err != nil {
		printErrorAndExit(fmt.Errorf("error al insertar secreto en DynamoDB: %w", err))
	}

	printInfo(fmt.Sprintf("Secreto '%s' cargado exitosamente en la tabla de configuración", secretName))
}

// loadCredentialsFromFile carga las credenciales AWS desde un archivo JSON
func loadCredentialsFromFile(configFile string) (*AWSCredentials, error) {
	configFileBytes, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("error al leer archivo de configuración: %w", err)
	}

	var credentials SecretLoaderCredentials
	err = json.Unmarshal(configFileBytes, &credentials)
	if err != nil {
		return nil, fmt.Errorf("error al parsear archivo de configuración: %w", err)
	}

	// Validar campos requeridos
	if credentials.AWSAccessKeyID == "" {
		return nil, fmt.Errorf("aws_access_key_id es requerido en el archivo de configuración")
	}
	if credentials.AWSSecretAccessKey == "" {
		return nil, fmt.Errorf("aws_secret_access_key es requerido en el archivo de configuración")
	}
	if credentials.AWSRegion == "" {
		return nil, fmt.Errorf("aws_region es requerido en el archivo de configuración")
	}

	return &AWSCredentials{
		AWSAccessKeyID:     credentials.AWSAccessKeyID,
		AWSSecretAccessKey: credentials.AWSSecretAccessKey,
		AWSSessionToken:    credentials.AWSSessionToken,
		AWSRegion:          credentials.AWSRegion,
	}, nil
}

// loadCredentialsFromInput solicita las credenciales AWS mediante prompts interactivos
func loadCredentialsFromInput() (*AWSCredentials, error) {
	awsRegion, err := askForInput("Ingresa tu región de AWS", "AWS Region")
	if err != nil {
		return nil, err
	}

	choices := []choice{
		{
			Label: "Input",
			Value: "1",
			Callback: func() (any, error) {
				return askForAWSCredentialsInput(awsRegion)
			},
		},
		{
			Label: "Archivo de credenciales",
			Value: "2",
			Callback: func() (any, error) {
				return askForAWSCredentialsFile(awsRegion)
			},
		},
	}

	result, err := askForChoices("¿Cómo deseas proporcionar las credenciales de AWS?", choices)
	if err != nil {
		return nil, err
	}

	awsCredentials, ok := result.(*AWSCredentials)
	if !ok {
		return nil, fmt.Errorf("tipo inesperado retornado de askForChoices")
	}

	return awsCredentials, nil
}

// askForAWSCredentialsInput solicita credenciales AWS directamente por input
func askForAWSCredentialsInput(awsRegion string) (*AWSCredentials, error) {
	awsAccessKeyID, err := askForPassword("Ingresa tu AWS Access Key ID", "AWS Access Key ID")
	if err != nil {
		return nil, err
	}

	awsSecretAccessKey, err := askForPassword("Ingresa tu AWS Secret Access Key", "AWS Secret Access Key")
	if err != nil {
		return nil, err
	}

	awsSessionToken, err := askForPassword("Ingresa tu AWS Session Token (opcional, presiona Enter para omitir)", "AWS Session Token")
	if err != nil {
		// Permitir session token vacío
		awsSessionToken = ""
	}

	return &AWSCredentials{
		AWSAccessKeyID:     awsAccessKeyID,
		AWSSecretAccessKey: awsSecretAccessKey,
		AWSSessionToken:    strings.TrimSpace(awsSessionToken),
		AWSRegion:          strings.TrimSpace(awsRegion),
	}, nil
}

// askForAWSCredentialsFile solicita credenciales AWS desde un archivo de credenciales
func askForAWSCredentialsFile(awsRegion string) (*AWSCredentials, error) {
	profile, err := askForInput("Ingresa tu perfil de AWS", "AWS Profile")
	if err != nil {
		return nil, err
	}

	fileCredentials := &AWSFileCredentials{
		Profile: profile,
		Region:  strings.TrimSpace(awsRegion),
	}

	return fileCredentials.GetCredentials()
}
