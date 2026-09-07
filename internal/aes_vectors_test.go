package internal

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"
)

// Cross-implementation byte-compatibility lock-down.
//
// These vectors are shared verbatim (byte-identical copies) across
// titvo-installer (Go), titvo-shared (TS) and titvo-rag-indexer (Python).
// All three implementations derive the same 32-byte AES-256 key by
// base64-decoding the same Secrets Manager value and must produce the
// exact same ciphertext for the exact same plaintext (AES-256-ECB + PKCS7).
//
// titvo-installer only ever encrypts (the CLI writes secrets; other
// services decrypt them), so this test only exercises encrypt().

type aesVector struct {
	Name                     string `json:"name"`
	Plaintext                string `json:"plaintext"`
	ExpectedCiphertextBase64 string `json:"expected_ciphertext_base64"`
}

type aesFixture struct {
	KeyBase64 string      `json:"key_base64"`
	Vectors   []aesVector `json:"vectors"`
}

func loadAesFixture(t *testing.T) aesFixture {
	t.Helper()

	data, err := os.ReadFile("testdata/aes-test-vectors.json")
	if err != nil {
		t.Fatalf("failed to read shared AES test vectors fixture: %v", err)
	}

	var fixture aesFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("failed to parse shared AES test vectors fixture: %v", err)
	}

	return fixture
}

func TestEncryptMatchesSharedVectors(t *testing.T) {
	fixture := loadAesFixture(t)

	keyBytes, err := base64.StdEncoding.DecodeString(fixture.KeyBase64)
	if err != nil {
		t.Fatalf("failed to decode fixture key: %v", err)
	}
	key := string(keyBytes)

	for _, vector := range fixture.Vectors {
		vector := vector
		t.Run(vector.Name, func(t *testing.T) {
			got, err := encrypt(vector.Plaintext, key)
			if err != nil {
				t.Fatalf("encrypt(%q) returned error: %v", vector.Name, err)
			}
			if got != vector.ExpectedCiphertextBase64 {
				t.Fatalf("encrypt(%q) = %q, want %q (byte-compatibility with titvo-shared/rag-indexer broken)", vector.Name, got, vector.ExpectedCiphertextBase64)
			}
		})
	}
}
