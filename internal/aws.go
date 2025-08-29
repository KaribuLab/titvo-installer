package internal

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	secretsmanagertypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

func (creds *AWSCredentials) getAWSConfig(ctx context.Context) (aws.Config, error) {
	configOptions := []func(*config.LoadOptions) error{
		config.WithRegion(creds.AWSRegion),
	}

	if creds.AWSAccessKeyID != "" && creds.AWSSecretAccessKey != "" {
		credProvider := credentials.NewStaticCredentialsProvider(
			creds.AWSAccessKeyID,
			creds.AWSSecretAccessKey,
			creds.AWSSessionToken,
		)
		configOptions = append(configOptions, config.WithCredentialsProvider(credProvider))
	}

	return config.LoadDefaultConfig(ctx, configOptions...)
}

// GetAccountID obtiene el Account ID de AWS usando las credenciales proporcionadas
func GetAccountID(creds *AWSCredentials) (string, error) {
	cfg, err := creds.getAWSConfig(context.TODO())
	if err != nil {
		return "", fmt.Errorf("error al cargar configuración de AWS: %w", err)
	}

	client := sts.NewFromConfig(cfg)

	result, err := client.GetCallerIdentity(context.TODO(), &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", fmt.Errorf("error al obtener identity del caller: %w", err)
	}

	if result.Account == nil {
		return "", fmt.Errorf("account ID no disponible en la respuesta")
	}

	return *result.Account, nil
}
func PutParameter(creds *AWSCredentials, path, value string) error {
	cfg, err := creds.getAWSConfig(context.TODO())
	if err != nil {
		return fmt.Errorf("error al cargar configuración de AWS: %w", err)
	}

	client := ssm.NewFromConfig(cfg)

	input := &ssm.PutParameterInput{
		Name:      aws.String(path),
		Value:     aws.String(value),
		Type:      types.ParameterTypeString,   // Tipo String
		Tier:      types.ParameterTierStandard, // Capa estándar
		Overwrite: aws.Bool(true),              // Permitir sobrescribir si ya existe
	}

	_, err = client.PutParameter(context.TODO(), input)
	if err != nil {
		return fmt.Errorf("error al insertar parámetro '%s': %w", path, err)
	}

	return nil
}

func CreateSecret(creds *AWSCredentials, name, secretValue string) (string, error) {
	cfg, err := creds.getAWSConfig(context.TODO())
	if err != nil {
		return "", fmt.Errorf("error al cargar configuración de AWS: %w", err)
	}

	client := secretsmanager.NewFromConfig(cfg)

	// Intentar obtener el secreto para ver si existe
	_, err = client.GetSecretValue(context.TODO(), &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(name),
	})

	var notFound *secretsmanagertypes.ResourceNotFoundException
	if errors.As(err, &notFound) {
		// El secreto no existe, crearlo
		output, err := client.CreateSecret(context.TODO(), &secretsmanager.CreateSecretInput{
			Name:         aws.String(name),
			SecretString: aws.String(secretValue),
			Description:  aws.String(fmt.Sprintf("Secreto creado para %s", name)),
		})
		if err != nil {
			return "", fmt.Errorf("error al crear secreto '%s': %w", name, err)
		}
		return *output.ARN, nil
	} else if err != nil {
		// Otro tipo de error
		return "", fmt.Errorf("error al verificar secreto '%s': %w", name, err)
	} else {
		// El secreto existe, actualizarlo
		output, err := client.UpdateSecret(context.TODO(), &secretsmanager.UpdateSecretInput{
			SecretId:     aws.String(name),
			SecretString: aws.String(secretValue),
		})
		if err != nil {
			return "", fmt.Errorf("error al actualizar secreto '%s': %w", name, err)
		}
		return *output.ARN, nil
	}
}
