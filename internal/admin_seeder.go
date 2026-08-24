package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/bcrypt"
)

// bcryptCost matches titvo-auth's PasswordHasherService SALT_ROUNDS (bcryptjs,
// see password-hasher.service.ts) so a hash written here verifies correctly
// against titvo-auth's login flow, and vice versa.
const bcryptCost = 10

// adminUserStore abstracts the DynamoDB operations needed to seed the first
// admin user, so seedAdminUser's business logic can be tested without real
// AWS credentials or a live DynamoDB table.
type adminUserStore interface {
	// HasAdminUser reports whether at least one user with role "admin"
	// already exists in the table.
	HasAdminUser() (bool, error)
	// PutUser inserts a new user record.
	PutUser(item map[string]interface{}) error
}

// dynamoAdminUserStore is the real AWS-backed implementation of adminUserStore.
type dynamoAdminUserStore struct {
	creds     *AWSCredentials
	tableName string
}

func (s *dynamoAdminUserStore) HasAdminUser() (bool, error) {
	return ScanForItemWithValue(s.creds, s.tableName, "role", "admin")
}

func (s *dynamoAdminUserStore) PutUser(item map[string]interface{}) error {
	return PutRecord(s.creds, s.tableName, item)
}

// seedAdminUser hashes the given password and writes the first admin user
// record to the store, in the same item shape titvo-auth-setup-aws's
// DynamoUserRepository expects (user_id, email, password_hash, role,
// created_at, updated_at) — see user.dynamo.ts's mapItemToUserEntity.
// It is idempotent: if an admin user already exists, it is a no-op and
// returns created=false.
func seedAdminUser(store adminUserStore, email, password string) (created bool, err error) {
	hasAdmin, err := store.HasAdminUser()
	if err != nil {
		return false, fmt.Errorf("error checking for existing admin user: %w", err)
	}
	if hasAdmin {
		return false, nil
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return false, fmt.Errorf("error hashing admin password: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	err = store.PutUser(map[string]interface{}{
		"user_id":       uuid.New().String(),
		"email":         email,
		"password_hash": string(passwordHash),
		"role":          "admin",
		"created_at":    now,
		"updated_at":    now,
	})
	if err != nil {
		return false, fmt.Errorf("error inserting admin user: %w", err)
	}

	return true, nil
}

// resolveAdminCredentials returns the admin email/password either from the
// --config file (if it sets both admin_email and admin_password) or by
// prompting interactively — mirroring prepareCredentials' own
// config-file-or-prompt pattern for AWS credentials.
func resolveAdminCredentials(cmd *cobra.Command) (email string, password string, err error) {
	configFile, err := cmd.Flags().GetString("config")
	if err != nil {
		return "", "", err
	}

	if configFile != "" {
		configFileBytes, err := os.ReadFile(configFile)
		if err != nil {
			return "", "", fmt.Errorf("error al leer archivo de configuración: %w", err)
		}
		var credentials SecretLoaderCredentials
		if err := json.Unmarshal(configFileBytes, &credentials); err != nil {
			return "", "", fmt.Errorf("error al parsear archivo de configuración: %w", err)
		}
		if credentials.AdminEmail != "" && credentials.AdminPassword != "" {
			return credentials.AdminEmail, credentials.AdminPassword, nil
		}
	}

	email, err = askForInput("Ingresa el email del usuario admin", "Email del usuario admin")
	if err != nil {
		return "", "", err
	}
	password, err = askForPassword("Ingresa la contraseña del usuario admin", "Contraseña del usuario admin")
	if err != nil {
		return "", "", err
	}
	return email, password, nil
}

// RunAdminSeeder runs the CLI flow to seed the first admin user of the admin
// console, mirroring the RunSecretLoader/RunParameterLoader pattern
// (prepareCredentials, prompt, PutRecord) but idempotently and exactly-once
// for a single admin. There is no signup/invite flow in scope (see design.md
// D6) — this command is the only way to create the first `role=admin` user.
func RunAdminSeeder(cmd *cobra.Command, args []string) {
	printInfo("Iniciando creación del primer usuario admin")

	awsCredentials := prepareCredentials(cmd)

	dynamoUserTableName, err := GetParameter(awsCredentials, "/tvo/security-scan/prod/infra/dynamo/user-table-name")
	if err != nil {
		printErrorAndExit(err)
	}

	email, password, err := resolveAdminCredentials(cmd)
	if err != nil {
		printErrorAndExit(err)
	}

	store := &dynamoAdminUserStore{creds: awsCredentials, tableName: dynamoUserTableName}

	created, err := seedAdminUser(store, email, password)
	if err != nil {
		printErrorAndExit(err)
	}

	if !created {
		printInfo("Ya existe un usuario admin. No se creó ninguno nuevo.")
		return
	}

	printInfo(fmt.Sprintf("Usuario admin creado exitosamente. Email: %s", email))
}
