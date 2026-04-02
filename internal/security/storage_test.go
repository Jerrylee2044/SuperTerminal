package security

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSecureStorageEnv tests environment variable storage.
func TestSecureStorageEnv(t *testing.T) {
	s, _ := NewSecureStorage(StorageOptions{PreferEnv: true})

	// Set secret in environment
	err := s.SetSecret(SecretAPIKeyAnthropic, "test-key-123", StorageEnv)
	if err != nil {
		t.Errorf("Failed to set secret: %v", err)
	}

	// Get secret from environment
	value, ok := s.GetSecret(SecretAPIKeyAnthropic)
	if !ok {
		t.Error("Failed to get secret")
	}
	if value != "test-key-123" {
		t.Errorf("Expected 'test-key-123', got '%s'", value)
	}

	// Clean up
	os.Unsetenv("ANTHROPIC_API_KEY")
}

// TestSecureStorageMemory tests memory storage.
func TestSecureStorageMemory(t *testing.T) {
	s, _ := NewSecureStorage(StorageOptions{PreferEnv: false})

	// Set secret in memory
	err := s.SetSecret(SecretAPIKey, "memory-key", StorageMemory)
	if err != nil {
		t.Errorf("Failed to set secret: %v", err)
	}

	// Get secret from memory
	value, ok := s.GetSecret(SecretAPIKey)
	if !ok {
		t.Error("Failed to get secret")
	}
	if value != "memory-key" {
		t.Errorf("Expected 'memory-key', got '%s'", value)
	}
}

// TestSecureStorageFile tests encrypted file storage.
func TestSecureStorageFile(t *testing.T) {
	tmpDir := t.TempDir()
	secretsFile := filepath.Join(tmpDir, "secrets.enc")

	s, err := NewSecureStorage(StorageOptions{
		MasterPassword: "test-password-123",
		SecretsFile:    secretsFile,
		PreferEnv:      false,
	})
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	// Set secret in file
	err = s.SetSecret(SecretAPIKeyAnthropic, "file-key-456", StorageFile)
	if err != nil {
		t.Errorf("Failed to set secret: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(secretsFile); err != nil {
		t.Errorf("Secrets file should exist: %v", err)
	}

	// Get secret from file
	value, ok := s.GetSecret(SecretAPIKeyAnthropic)
	if !ok {
		t.Error("Failed to get secret")
	}
	if value != "file-key-456" {
		t.Errorf("Expected 'file-key-456', got '%s'", value)
	}
}

// TestSecureStorageFileReload tests reloading secrets from file.
func TestSecureStorageFileReload(t *testing.T) {
	tmpDir := t.TempDir()
	secretsFile := filepath.Join(tmpDir, "secrets.enc")

	// Create first storage and save
	s1, err := NewSecureStorage(StorageOptions{
		MasterPassword: "test-password",
		SecretsFile:    secretsFile,
		PreferEnv:      false,
	})
	if err != nil {
		t.Fatal(err)
	}

	s1.SetSecret(SecretAPIKeyAnthropic, "saved-key", StorageFile)

	// Create second storage with same file
	s2, err := NewSecureStorage(StorageOptions{
		MasterPassword: "test-password",
		SecretsFile:    secretsFile,
		PreferEnv:      false,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Should load saved secret
	value, ok := s2.GetSecret(SecretAPIKeyAnthropic)
	if !ok {
		t.Error("Failed to load secret from file")
	}
	if value != "saved-key" {
		t.Errorf("Expected 'saved-key', got '%s'", value)
	}
}

// TestSecureStorageDelete tests secret deletion.
func TestSecureStorageDelete(t *testing.T) {
	s, _ := NewSecureStorage(StorageOptions{PreferEnv: false})

	// Set secret
	s.SetSecret(SecretAPIKey, "to-delete", StorageMemory)

	// Delete secret
	err := s.DeleteSecret(SecretAPIKey)
	if err != nil {
		t.Errorf("Failed to delete secret: %v", err)
	}

	// Should not exist
	_, ok := s.GetSecret(SecretAPIKey)
	if ok {
		t.Error("Secret should be deleted")
	}
}

// TestSecureStorageHasSecret tests secret existence check.
func TestSecureStorageHasSecret(t *testing.T) {
	s, _ := NewSecureStorage(StorageOptions{PreferEnv: false})

	// Should not exist
	if s.HasSecret(SecretAPIKey) {
		t.Error("Secret should not exist initially")
	}

	// Set secret
	s.SetSecret(SecretAPIKey, "test", StorageMemory)

	// Should exist
	if !s.HasSecret(SecretAPIKey) {
		t.Error("Secret should exist")
	}
}

// TestSecureStorageListSecrets tests secret listing.
func TestSecureStorageListSecrets(t *testing.T) {
	s, _ := NewSecureStorage(StorageOptions{PreferEnv: false})

	// Set multiple secrets
	s.SetSecret(SecretAPIKeyAnthropic, "key1", StorageMemory)
	s.SetSecret(SecretAPIKeyOpenAI, "key2", StorageMemory)

	// List secrets
	types := s.ListSecrets()
	if len(types) < 2 {
		t.Errorf("Expected at least 2 secrets, got %d", len(types))
	}
}

// TestSecureStorageNoMasterKey tests file storage without master key.
func TestSecureStorageNoMasterKey(t *testing.T) {
	s, _ := NewSecureStorage(StorageOptions{
		PreferEnv: false,
	})

	// Should fail to store in file
	err := s.SetSecret(SecretAPIKey, "test", StorageFile)
	if err == nil {
		t.Error("Should fail without master password")
	}
}

// TestTypeToEnvKey tests environment key conversion.
func TestTypeToEnvKey(t *testing.T) {
	tests := []struct {
		typ      SecretType
		expected string
	}{
		{SecretAPIKeyAnthropic, "ANTHROPIC_API_KEY"},
		{SecretAPIKeyOpenAI, "OPENAI_API_KEY"},
		{SecretAPIKey, "API_KEY"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			key := typeToEnvKey(tt.typ)
			if key != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, key)
			}
		})
	}
}

// TestEncryptionDecryption tests encrypt/decrypt cycle.
func TestEncryptionDecryption(t *testing.T) {
	s, err := NewSecureStorage(StorageOptions{
		MasterPassword: "test-password",
		PreferEnv:      false,
	})
	if err != nil {
		t.Fatal(err)
	}

	plaintext := []byte("secret data to encrypt")
	
	encrypted, err := s.encrypt(plaintext)
	if err != nil {
		t.Errorf("Encryption failed: %v", err)
	}

	decrypted, err := s.decrypt(encrypted)
	if err != nil {
		t.Errorf("Decryption failed: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Errorf("Decrypted data mismatch")
	}
}

// TestEncryptionDifferentPasswords tests wrong password fails.
func TestEncryptionDifferentPasswords(t *testing.T) {
	s1, _ := NewSecureStorage(StorageOptions{
		MasterPassword: "password1",
		PreferEnv:      false,
	})

	s2, _ := NewSecureStorage(StorageOptions{
		MasterPassword: "password2",
		PreferEnv:      false,
	})

	plaintext := []byte("secret")
	encrypted, _ := s1.encrypt(plaintext)

	// Should fail to decrypt with different password
	_, err := s2.decrypt(encrypted)
	if err == nil {
		t.Error("Should fail to decrypt with different password")
	}
}

// TestGlobalFunctions tests global storage functions.
func TestGlobalFunctions(t *testing.T) {
	Init(StorageOptions{PreferEnv: false})

	// Set and get
	SetSecret(SecretAPIKey, "global-test", StorageMemory)
	
	value, ok := GetSecret(SecretAPIKey)
	if !ok || value != "global-test" {
		t.Error("Global functions should work")
	}

	// Has
	if !HasSecret(SecretAPIKey) {
		t.Error("HasSecret should return true")
	}

	// Delete
	DeleteSecret(SecretAPIKey)
	if HasSecret(SecretAPIKey) {
		t.Error("Secret should be deleted")
	}
}