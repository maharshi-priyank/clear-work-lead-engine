package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

// Format: iv:tag:ciphertext (all hex) — identical to NestJS vault-crypto.util.ts
// so the same encrypted rows work for both services.

func Encrypt(plaintext, hexKey string) (string, error) {
	key, err := hex.DecodeString(hexKey[:64])
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	iv := make([]byte, 12)
	if _, err := rand.Read(iv); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nil, iv, []byte(plaintext), nil)
	// gcm.Seal appends tag at the end; split it out
	ciphertext := sealed[:len(sealed)-gcm.Overhead()]
	tag := sealed[len(sealed)-gcm.Overhead():]
	return fmt.Sprintf("%s:%s:%s",
		hex.EncodeToString(iv),
		hex.EncodeToString(tag),
		hex.EncodeToString(ciphertext),
	), nil
}

func Decrypt(stored, hexKey string) (string, error) {
	parts := strings.SplitN(stored, ":", 3)
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid vault format")
	}
	iv, err := hex.DecodeString(parts[0])
	if err != nil {
		return "", err
	}
	tag, err := hex.DecodeString(parts[1])
	if err != nil {
		return "", err
	}
	data, err := hex.DecodeString(parts[2])
	if err != nil {
		return "", err
	}
	key, err := hex.DecodeString(hexKey[:64])
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	// Reconstitute ciphertext+tag the way Go's GCM expects
	combined := append(data, tag...)
	plain, err := gcm.Open(nil, iv, combined, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt failed: %w", err)
	}
	return string(plain), nil
}
