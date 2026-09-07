# AES cross-implementation test vectors

`aes-test-vectors.json` in this directory MUST stay byte-identical to the
copies committed in:

- `titvo-shared/test/fixtures/aes-test-vectors.json`
- `titvo-rag-indexer/tests/fixtures/aes-test-vectors.json`

All three repos independently implement AES-256-ECB + PKCS7 padding
(`titvo-shared`'s `AesService`, `titvo-installer`'s `encrypt()` in
`internal/util.go`, and `titvo-rag-indexer`'s `EncryptionService`). They all
derive the same 32-byte key by base64-decoding the same Secrets Manager
value, so ciphertext produced by one implementation must decrypt correctly
in the others.

`titvo-installer` only ever encrypts (the CLI writes secrets; other services
decrypt them), so `internal/aes_vectors_test.go` only exercises `encrypt()`.

This fixture is the shared contract that locks that byte-compatibility in
place. There is currently **no automated sync** between the three copies —
if you change or extend this file, manually copy the exact same bytes into
the other two repos and update all three test suites in the same change.
