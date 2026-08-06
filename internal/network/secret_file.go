package network

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"bruno-browser/internal/storage"
)

type fileKeyProtector struct {
	key []byte
}

func newFileKeyProtector(dataRoot string) (SecretProtector, error) {
	keyPath := filepath.Join(dataRoot, "secrets", "network.key")
	key, err := os.ReadFile(keyPath)
	if errors.Is(err, os.ErrNotExist) {
		key = make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, key); err != nil {
			return nil, fmt.Errorf("generate network encryption key: %w", err)
		}
		if err := storage.WriteFileAtomic(keyPath, key, 0o600); err != nil {
			return nil, fmt.Errorf("write network encryption key: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("read network encryption key: %w", err)
	}
	if len(key) != 32 {
		return nil, errors.New("network encryption key has an invalid length")
	}
	return &fileKeyProtector{key: append([]byte(nil), key...)}, nil
}

func (protector *fileKeyProtector) Protect(plaintext []byte) (string, error) {
	if len(plaintext) == 0 {
		return "", nil
	}
	aead, err := protector.aead()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	payload := aead.Seal(nonce, nonce, plaintext, []byte("bruno-browser-network-v1"))
	return base64.StdEncoding.EncodeToString(payload), nil
}

func (protector *fileKeyProtector) Unprotect(ciphertext string) ([]byte, error) {
	if ciphertext == "" {
		return nil, nil
	}
	payload, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return nil, err
	}
	aead, err := protector.aead()
	if err != nil {
		return nil, err
	}
	if len(payload) < aead.NonceSize() {
		return nil, errors.New("protected proxy credential is truncated")
	}
	nonce, ciphertextBytes := payload[:aead.NonceSize()], payload[aead.NonceSize():]
	return aead.Open(nil, nonce, ciphertextBytes, []byte("bruno-browser-network-v1"))
}

func (protector *fileKeyProtector) aead() (cipher.AEAD, error) {
	block, err := aes.NewCipher(protector.key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
