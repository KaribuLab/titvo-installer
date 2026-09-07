package internal

import (
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// fakeAdminUserStore is a test double for adminUserStore so seedAdminUser's
// business logic (idempotency, hashing, item shape) can be verified without
// real AWS credentials or a live DynamoDB table.
type fakeAdminUserStore struct {
	hasAdmin    bool
	hasAdminErr error
	putErr      error
	putItems    []map[string]interface{}
}

func (f *fakeAdminUserStore) HasAdminUser() (bool, error) {
	return f.hasAdmin, f.hasAdminErr
}

func (f *fakeAdminUserStore) PutUser(item map[string]interface{}) error {
	if f.putErr != nil {
		return f.putErr
	}
	f.putItems = append(f.putItems, item)
	return nil
}

func TestSeedAdminCreatesExactlyOneAdminUserGSIQueryableByEmail(t *testing.T) {
	store := &fakeAdminUserStore{hasAdmin: false}

	created, err := seedAdminUser(store, "admin@titvo.dev", "s3cr3t-password")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !created {
		t.Fatal("expected seedAdminUser to report the admin user was created")
	}
	if len(store.putItems) != 1 {
		t.Fatalf("expected exactly one PutUser call, got %d", len(store.putItems))
	}

	item := store.putItems[0]

	// email is the attribute the EmailIndex GSI projects on (matches
	// titvo-auth-setup-aws's DynamoUserRepository.findByEmail query key),
	// so asserting its exact name/value is what makes the seeded user
	// GSI-queryable by email.
	if item["email"] != "admin@titvo.dev" {
		t.Fatalf("expected email attribute 'admin@titvo.dev', got %v", item["email"])
	}
	if item["role"] != "admin" {
		t.Fatalf("expected role attribute 'admin', got %v", item["role"])
	}
	userID, ok := item["user_id"].(string)
	if !ok || userID == "" {
		t.Fatalf("expected non-empty user_id attribute, got %v", item["user_id"])
	}
	if item["created_at"] == "" {
		t.Fatal("expected non-empty created_at attribute")
	}
	if item["updated_at"] == "" {
		t.Fatal("expected non-empty updated_at attribute")
	}
}

func TestSeedAdminHashesPasswordWithBcryptCompatibleWithTitvoAuth(t *testing.T) {
	store := &fakeAdminUserStore{hasAdmin: false}

	_, err := seedAdminUser(store, "admin@titvo.dev", "s3cr3t-password")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	hash, ok := store.putItems[0]["password_hash"].(string)
	if !ok || hash == "" {
		t.Fatalf("expected non-empty password_hash attribute, got %v", store.putItems[0]["password_hash"])
	}
	if hash == "s3cr3t-password" {
		t.Fatal("password_hash must not equal the plaintext password")
	}
	// titvo-auth's PasswordHasherService uses bcryptjs with SALT_ROUNDS=10.
	// bcrypt.CompareHashAndPassword only succeeds against a genuine bcrypt
	// hash, proving both the format and the cost are interchangeable.
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("s3cr3t-password")); err != nil {
		t.Fatalf("expected password_hash to verify against the plaintext password via bcrypt: %v", err)
	}
	cost, err := bcrypt.Cost([]byte(hash))
	if err != nil {
		t.Fatalf("expected a valid bcrypt hash: %v", err)
	}
	if cost != 10 {
		t.Fatalf("expected bcrypt cost 10 to match titvo-auth's SALT_ROUNDS, got %d", cost)
	}
}

func TestSeedAdminIsIdempotentWhenAdminAlreadyExists(t *testing.T) {
	store := &fakeAdminUserStore{hasAdmin: true}

	created, err := seedAdminUser(store, "second-admin@titvo.dev", "another-password")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created {
		t.Fatal("expected seedAdminUser to be a no-op when an admin already exists")
	}
	if len(store.putItems) != 0 {
		t.Fatalf("expected zero PutUser calls when an admin already exists, got %d", len(store.putItems))
	}
}

func TestSeedAdminPropagatesHasAdminUserError(t *testing.T) {
	store := &fakeAdminUserStore{hasAdminErr: errors.New("dynamodb unavailable")}

	created, err := seedAdminUser(store, "admin@titvo.dev", "s3cr3t-password")
	if err == nil {
		t.Fatal("expected an error when HasAdminUser fails")
	}
	if created {
		t.Fatal("expected created=false when HasAdminUser fails")
	}
	if len(store.putItems) != 0 {
		t.Fatal("expected zero PutUser calls when HasAdminUser fails")
	}
}

func TestSeedAdminPropagatesPutUserError(t *testing.T) {
	store := &fakeAdminUserStore{hasAdmin: false, putErr: errors.New("dynamodb write failed")}

	created, err := seedAdminUser(store, "admin@titvo.dev", "s3cr3t-password")
	if err == nil {
		t.Fatal("expected an error when PutUser fails")
	}
	if created {
		t.Fatal("expected created=false when PutUser fails")
	}
}
