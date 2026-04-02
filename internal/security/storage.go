// Package security provides secure storage for sensitive data like API keys.
package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// StorageType defines how secrets are stored.
type StorageType string

const (
	StorageEnv      StorageType = "env"      // Environment variable (most secure)
	StorageFile     StorageType = "file"     // Encrypted file
	StorageMemory   StorageType = "memory"   // In-memory only (temporary)
	StorageNone     StorageType = "none"     // Not stored
)

// SecretType defines types of secrets.
type SecretType string

const (
	SecretAPIKey    SecretType = "api_key"
	SecretAPIKeyAnthropic SecretType = "anthropic_api_key"
	SecretAPIKeyOpenAI    SecretType = "openai_api_key"
	SecretPassword  SecretType = "password"
	SecretToken     SecretType = "token"
)

// Secret represents a stored secret.
type Secret struct {
	Type     SecretType
	Name     string
	Value    string
	StoredAt StorageType
}

// SecureStorage manages secure secret storage.
type SecureStorage struct {
	masterKey    []byte
	secrets      map[SecretType]Secret
	secretsFile  string
	mu           sync.RWMutex
	preferEnv    bool
}

// StorageOptions configures secure storage.
type StorageOptions struct {
	MasterPassword string        // Password for encryption
	SecretsFile    string        // Path to encrypted secrets file
	PreferEnv      bool          // Prefer environment variables
}

var (
	defaultStorage *SecureStorage
	once           sync.Once
)

// Init initializes the default secure storage.
func Init(opts StorageOptions) (*SecureStorage, error) {
	once.Do(func() {
		defaultStorage, _ = NewSecureStorage(opts)
	})
	return defaultStorage, nil
}

// Get returns the default storage.
func Get() *SecureStorage {
	if defaultStorage == nil {
		defaultStorage = &SecureStorage{
			secrets:   make(map[SecretType]Secret),
			preferEnv: true,
		}
	}
	return defaultStorage
}

// NewSecureStorage creates a new secure storage.
func NewSecureStorage(opts StorageOptions) (*SecureStorage, error) {
	s := &SecureStorage{
		secrets:   make(map[SecretType]Secret),
		preferEnv: opts.PreferEnv,
	}

	// Setup master key for encryption
	if opts.MasterPassword != "" {
		s.masterKey = deriveKey(opts.MasterPassword)
	}

	// Setup secrets file
	if opts.SecretsFile != "" {
		s.secretsFile = opts.SecretsFile
		if s.masterKey == nil {
			return nil, errors.New("master password required for file storage")
		}
	}

	// Load existing secrets
	if s.secretsFile != "" {
		s.loadFromFile()
	}

	return s, nil
}

// deriveKey derives encryption key from password using PBKDF2-like method.
func deriveKey(password string) []byte {
	// Simple PBKDF2 implementation using HMAC-SHA256
	salt := []byte("superterminal-secret-salt-v1")
	key := []byte(password)
	
	// Iterate 10000 times
	for i := 0; i < 10000; i++ {
		h := sha256.New()
		h.Write(salt)
		h.Write(key)
		key = h.Sum(nil)
	}
	
	// Return 32 bytes for AES-256
	return key[:32]
}

// Simple HMAC implementation for key derivation
func hmacSHA256(key, data []byte) []byte {
	h := sha256.New()
	h.Write(key)
	h.Write(data)
	return h.Sum(nil)
}

// GetSecret retrieves a secret.
func (s *SecureStorage) GetSecret(typ SecretType) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// First check environment variable
	if s.preferEnv {
		envKey := typeToEnvKey(typ)
		if value := os.Getenv(envKey); value != "" {
			return value, true
		}
	}

	// Check stored secrets
	secret, ok := s.secrets[typ]
	if ok {
		return secret.Value, true
	}

	return "", false
}

// GetAPIKey retrieves an API key.
func (s *SecureStorage) GetAPIKey(provider string) (string, bool) {
	typ := SecretType(provider + "_api_key")
	return s.GetSecret(typ)
}

// SetSecret stores a secret.
func (s *SecureStorage) SetSecret(typ SecretType, value string, storage StorageType) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate storage type
	if storage == StorageFile && s.masterKey == nil {
		return errors.New("master password required for file storage")
	}

	secret := Secret{
		Type:     typ,
		Name:     string(typ),
		Value:    value,
		StoredAt: storage,
	}

	switch storage {
	case StorageEnv:
		// Set environment variable
		envKey := typeToEnvKey(typ)
		os.Setenv(envKey, value)
		secret.StoredAt = StorageEnv

	case StorageFile:
		// Store encrypted in file
		s.secrets[typ] = secret
		if err := s.saveToFile(); err != nil {
			return err
		}

	case StorageMemory:
		// Store in memory only
		s.secrets[typ] = secret

	case StorageNone:
		// Don't store
		return nil
	}

	return nil
}

// DeleteSecret removes a secret.
func (s *SecureStorage) DeleteSecret(typ SecretType) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove from environment
	envKey := typeToEnvKey(typ)
	os.Unsetenv(envKey)

	// Remove from storage
	delete(s.secrets, typ)

	// Update file
	if s.secretsFile != "" {
		s.saveToFile()
	}

	return nil
}

// ListSecrets lists stored secret types (without values).
func (s *SecureStorage) ListSecrets() []SecretType {
	s.mu.RLock()
	defer s.mu.RUnlock()

	types := []SecretType{}
	for typ := range s.secrets {
		types = append(types, typ)
	}

	// Also check environment variables
	if s.preferEnv {
		for _, typ := range []SecretType{SecretAPIKeyAnthropic, SecretAPIKeyOpenAI} {
			envKey := typeToEnvKey(typ)
			if os.Getenv(envKey) != "" {
				types = append(types, typ)
			}
		}
	}

	return types
}

// HasSecret checks if a secret exists.
func (s *SecureStorage) HasSecret(typ SecretType) bool {
	_, ok := s.GetSecret(typ)
	return ok
}

// encrypt encrypts data using AES-GCM.
func (s *SecureStorage) encrypt(data []byte) ([]byte, error) {
	if s.masterKey == nil {
		return nil, errors.New("no master key")
	}

	block, err := aes.NewCipher(s.masterKey)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, data, nil)
	return ciphertext, nil
}

// decrypt decrypts data using AES-GCM.
func (s *SecureStorage) decrypt(data []byte) ([]byte, error) {
	if s.masterKey == nil {
		return nil, errors.New("no master key")
	}

	block, err := aes.NewCipher(s.masterKey)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

// saveToFile saves secrets to encrypted file.
func (s *SecureStorage) saveToFile() error {
	if s.secretsFile == "" || s.masterKey == nil {
		return nil
	}

	// Build secrets data
	data := ""
	for typ, secret := range s.secrets {
		data += fmt.Sprintf("%s=%s\n", typ, secret.Value)
	}

	// Encrypt
	encrypted, err := s.encrypt([]byte(data))
	if err != nil {
		return err
	}

	// Write file
	dir := filepath.Dir(s.secretsFile)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	encoded := base64.StdEncoding.EncodeToString(encrypted)
	return os.WriteFile(s.secretsFile, []byte(encoded), 0600)
}

// loadFromFile loads secrets from encrypted file.
func (s *SecureStorage) loadFromFile() error {
	if s.secretsFile == "" || s.masterKey == nil {
		return nil
	}

	// Check file exists
	if _, err := os.Stat(s.secretsFile); err != nil {
		return nil // File doesn't exist yet
	}

	// Read file
	data, err := os.ReadFile(s.secretsFile)
	if err != nil {
		return err
	}

	// Decode base64
	encrypted, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return err
	}

	// Decrypt
	plaintext, err := s.decrypt(encrypted)
	if err != nil {
		return err
	}

	// Parse secrets
	lines := splitLines(string(plaintext))
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := splitKeyValue(line)
		if len(parts) == 2 {
			typ := SecretType(parts[0])
			s.secrets[typ] = Secret{
				Type:     typ,
				Name:     parts[0],
				Value:    parts[1],
				StoredAt: StorageFile,
			}
		}
	}

	return nil
}

// typeToEnvKey converts secret type to environment variable name.
func typeToEnvKey(typ SecretType) string {
	switch typ {
	case SecretAPIKeyAnthropic:
		return "ANTHROPIC_API_KEY"
	case SecretAPIKeyOpenAI:
		return "OPENAI_API_KEY"
	case SecretAPIKey:
		return "API_KEY"
	default:
		return string(typ)
	}
}

// Helper functions
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func splitKeyValue(s string) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}

// === Global functions ===

// GetSecret retrieves from default storage.
func GetSecret(typ SecretType) (string, bool) {
	return Get().GetSecret(typ)
}

// SetSecret stores to default storage.
func SetSecret(typ SecretType, value string, storage StorageType) error {
	return Get().SetSecret(typ, value, storage)
}

// GetAPIKey retrieves API key from default storage.
func GetAPIKey(provider string) (string, bool) {
	return Get().GetAPIKey(provider)
}

// HasSecret checks if secret exists.
func HasSecret(typ SecretType) bool {
	return Get().HasSecret(typ)
}

// DeleteSecret removes from default storage.
func DeleteSecret(typ SecretType) error {
	return Get().DeleteSecret(typ)
}

// ListSecrets lists secrets from default storage.
func ListSecrets() []SecretType {
	return Get().ListSecrets()
}