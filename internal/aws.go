package internal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/batch"
	batchtypes "github.com/aws/aws-sdk-go-v2/service/batch/types"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
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

// GetParameter obtiene el valor de un parámetro del Parameter Store por su path
func GetParameter(creds *AWSCredentials, path string) (string, error) {
	cfg, err := creds.getAWSConfig(context.TODO())
	if err != nil {
		return "", fmt.Errorf("error al cargar configuración de AWS: %w", err)
	}

	client := ssm.NewFromConfig(cfg)

	input := &ssm.GetParameterInput{
		Name:           aws.String(path),
		WithDecryption: aws.Bool(true), // Permitir desencriptar parámetros SecureString
	}

	result, err := client.GetParameter(context.TODO(), input)
	if err != nil {
		return "", fmt.Errorf("error al obtener parámetro '%s': %w", path, err)
	}

	if result.Parameter == nil || result.Parameter.Value == nil {
		return "", fmt.Errorf("parámetro '%s' no tiene valor", path)
	}

	return *result.Parameter.Value, nil
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

// SubmitBatchJob envía un job de AWS Batch con variables de ambiente personalizadas y espera a que termine
func SubmitBatchJob(creds *AWSCredentials, jobName, jobQueue, jobDefinition string, envVars map[string]string) error {
	ctx := context.TODO()
	cfg, err := creds.getAWSConfig(ctx)
	if err != nil {
		return fmt.Errorf("error al cargar configuración de AWS: %w", err)
	}

	client := batch.NewFromConfig(cfg)

	// Convertir el mapa de variables de entorno al formato requerido por AWS Batch
	var environment []batchtypes.KeyValuePair
	for key, value := range envVars {
		environment = append(environment, batchtypes.KeyValuePair{
			Name:  aws.String(key),
			Value: aws.String(value),
		})
	}

	// Crear la solicitud para enviar el trabajo
	input := &batch.SubmitJobInput{
		JobName:       aws.String(jobName),
		JobQueue:      aws.String(jobQueue),
		JobDefinition: aws.String(jobDefinition),
		ContainerOverrides: &batchtypes.ContainerOverrides{
			Environment: environment,
		},
	}

	// Enviar el trabajo
	result, err := client.SubmitJob(ctx, input)
	if err != nil {
		return fmt.Errorf("error al enviar job de Batch '%s': %w", jobName, err)
	}

	if result.JobId == nil {
		return fmt.Errorf("job ID no disponible en la respuesta")
	}

	jobID := *result.JobId
	fmt.Printf("Job de Batch enviado con ID: %s\n", jobID)

	// Monitorear el estado del trabajo hasta que termine
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("contexto cancelado mientras se esperaba el job: %w", ctx.Err())
		case <-ticker.C:
			// Describir el job para obtener su estado actual
			describeInput := &batch.DescribeJobsInput{
				Jobs: []string{jobID},
			}

			describeOutput, err := client.DescribeJobs(ctx, describeInput)
			if err != nil {
				return fmt.Errorf("error describing job '%s': %w", jobID, err)
			}

			if len(describeOutput.Jobs) == 0 {
				return fmt.Errorf("job not found '%s'", jobID)
			}

			job := describeOutput.Jobs[0]
			fmt.Printf("Current job status (%s): %s\n", jobID, job.Status)

			// Verificar si el job ha terminado
			switch job.Status {
			case batchtypes.JobStatusSucceeded:
				fmt.Printf("Job '%s' completed successfully\n", jobID)
				return nil
			case batchtypes.JobStatusFailed:
				reason := "unknown reason"
				if job.StatusReason != nil {
					reason = *job.StatusReason
				}
				return fmt.Errorf("job '%s' failed with reason: %s", jobID, reason)
				// Estados intermedios: SUBMITTED, PENDING, RUNNABLE, STARTING, RUNNING
				// Continúa esperando en el siguiente ciclo
			}
		}
	}
}

// PutRecord insert a record in a DynamoDB table
func PutRecord(creds *AWSCredentials, tableName string, item map[string]interface{}) error {
	cfg, err := creds.getAWSConfig(context.TODO())
	if err != nil {
		return fmt.Errorf("error loading AWS configuration: %w", err)
	}

	client := dynamodb.NewFromConfig(cfg)

	// Convert the map to DynamoDB attributes
	dynamoItem, err := attributevalue.MarshalMap(item)
	if err != nil {
		return fmt.Errorf("error converting item to DynamoDB attributes: %w", err)
	}

	// Create the PutItem request
	input := &dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item:      dynamoItem,
	}

	// Execute the PutItem operation
	_, err = client.PutItem(context.TODO(), input)
	if err != nil {
		return fmt.Errorf("error inserting item in table '%s': %w", tableName, err)
	}

	return nil
}
