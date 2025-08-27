package internal

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
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

func CreateSecret(creds *AWSCredentials, name, secretValue string) error {
	cfg, err := creds.getAWSConfig(context.TODO())
	if err != nil {
		return fmt.Errorf("error al cargar configuración de AWS: %w", err)
	}

	client := secretsmanager.NewFromConfig(cfg)

	input := &secretsmanager.CreateSecretInput{
		Name:         aws.String(name),
		SecretString: aws.String(secretValue),
		Description:  aws.String(fmt.Sprintf("Secreto creado para %s", name)),
	}

	_, err = client.CreateSecret(context.TODO(), input)
	if err != nil {
		return fmt.Errorf("error al crear secreto '%s': %w", name, err)
	}

	return nil
}
