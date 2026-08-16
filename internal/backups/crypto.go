package backups

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"bruno-browser/internal/storage"

	"golang.org/x/crypto/scrypt"
)

const (
	containerMagic    = "BRUNOPROFILE\x00\x01"
	containerVersion  = 1
	containerChunk    = 4 << 20
	maxHeaderBytes    = 64 << 10
	maxContainerBytes = int64(128) << 30
)

type containerHeader struct {
	Version     int    `json:"version"`
	KDF         string `json:"kdf"`
	Salt        string `json:"salt"`
	NoncePrefix string `json:"noncePrefix"`
	N           int    `json:"n"`
	R           int    `json:"r"`
	P           int    `json:"p"`
	ChunkSize   int    `json:"chunkSize"`
	PlainBytes  int64  `json:"plainBytes"`
	PlainSHA256 string `json:"plainSha256"`
}

func validatePassword(password string) error {
	count := len([]rune(password))
	if count < 8 {
		return errors.New("backup password must contain at least 8 characters")
	}
	if count > 256 {
		return errors.New("backup password is too long")
	}
	return nil
}

func encryptFile(source, destination, password string) (int64, error) {
	if err := validatePassword(password); err != nil {
		return 0, err
	}
	sourceFile, err := os.Open(source)
	if err != nil {
		return 0, err
	}
	defer sourceFile.Close()
	info, err := sourceFile.Stat()
	if err != nil {
		return 0, err
	}
	if info.Size() <= 0 || info.Size() > maxContainerBytes {
		return 0, errors.New("backup payload size is outside the supported range")
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, sourceFile); err != nil {
		return 0, fmt.Errorf("hash backup payload: %w", err)
	}
	if _, err := sourceFile.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	salt := make([]byte, 16)
	noncePrefix := make([]byte, 8)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return 0, err
	}
	if _, err := io.ReadFull(rand.Reader, noncePrefix); err != nil {
		return 0, err
	}
	header := containerHeader{
		Version: containerVersion, KDF: "scrypt", Salt: base64.RawURLEncoding.EncodeToString(salt),
		NoncePrefix: base64.RawURLEncoding.EncodeToString(noncePrefix), N: 1 << 15, R: 8, P: 1,
		ChunkSize: containerChunk, PlainBytes: info.Size(), PlainSHA256: hex.EncodeToString(hash.Sum(nil)),
	}
	headerPayload, err := json.Marshal(header)
	if err != nil {
		return 0, err
	}
	key, err := scrypt.Key([]byte(password), salt, header.N, header.R, header.P, 32)
	if err != nil {
		return 0, err
	}
	defer wipe(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return 0, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return 0, err
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".bruno-backup-*")
	if err != nil {
		return 0, err
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return 0, err
	}
	if _, err := temporary.Write([]byte(containerMagic)); err != nil {
		return 0, err
	}
	if err := binary.Write(temporary, binary.BigEndian, uint32(len(headerPayload))); err != nil {
		return 0, err
	}
	if _, err := temporary.Write(headerPayload); err != nil {
		return 0, err
	}
	buffer := make([]byte, header.ChunkSize)
	for counter := uint32(0); ; counter++ {
		read, readErr := io.ReadFull(sourceFile, buffer)
		if readErr == io.EOF {
			break
		}
		if readErr != nil && readErr != io.ErrUnexpectedEOF {
			return 0, readErr
		}
		nonce := chunkNonce(noncePrefix, counter)
		sealed := gcm.Seal(nil, nonce, buffer[:read], chunkAAD(headerPayload, counter))
		if err := binary.Write(temporary, binary.BigEndian, uint32(len(sealed))); err != nil {
			return 0, err
		}
		if _, err := temporary.Write(sealed); err != nil {
			return 0, err
		}
		if readErr == io.ErrUnexpectedEOF {
			break
		}
	}
	wipe(buffer)
	if err := temporary.Sync(); err != nil {
		return 0, err
	}
	if err := temporary.Close(); err != nil {
		return 0, err
	}
	if err := storage.CommitFileAtomic(temporaryPath, destination); err != nil {
		return 0, err
	}
	committed = true
	result, err := os.Stat(destination)
	if err != nil {
		return 0, err
	}
	return result.Size(), nil
}

func decryptFile(source, destination, password string) error {
	if err := validatePassword(password); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil || info.Size() <= int64(len(containerMagic)+4) || info.Size() > maxContainerBytes {
		return ErrInvalidBackup
	}
	magic := make([]byte, len(containerMagic))
	if _, err := io.ReadFull(input, magic); err != nil || !bytes.Equal(magic, []byte(containerMagic)) {
		return ErrInvalidBackup
	}
	var headerLength uint32
	if err := binary.Read(input, binary.BigEndian, &headerLength); err != nil || headerLength == 0 || headerLength > maxHeaderBytes {
		return ErrInvalidBackup
	}
	headerPayload := make([]byte, headerLength)
	if _, err := io.ReadFull(input, headerPayload); err != nil {
		return ErrInvalidBackup
	}
	var header containerHeader
	decoder := json.NewDecoder(bytes.NewReader(headerPayload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&header); err != nil || header.Version != containerVersion || header.KDF != "scrypt" || header.N != 1<<15 || header.R != 8 || header.P != 1 || header.ChunkSize != containerChunk || header.PlainBytes <= 0 || header.PlainBytes > maxContainerBytes || len(header.PlainSHA256) != 64 {
		return ErrInvalidBackup
	}
	salt, err := base64.RawURLEncoding.DecodeString(header.Salt)
	if err != nil || len(salt) != 16 {
		return ErrInvalidBackup
	}
	noncePrefix, err := base64.RawURLEncoding.DecodeString(header.NoncePrefix)
	if err != nil || len(noncePrefix) != 8 {
		return ErrInvalidBackup
	}
	key, err := scrypt.Key([]byte(password), salt, header.N, header.R, header.P, 32)
	if err != nil {
		return err
	}
	defer wipe(key)
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	valid := false
	defer func() {
		_ = output.Close()
		if !valid {
			_ = os.Remove(destination)
		}
	}()
	hash := sha256.New()
	var written int64
	for counter := uint32(0); ; counter++ {
		var frameLength uint32
		err := binary.Read(input, binary.BigEndian, &frameLength)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil || frameLength < uint32(gcm.Overhead()) || frameLength > uint32(header.ChunkSize+gcm.Overhead()) {
			return ErrInvalidBackup
		}
		frame := make([]byte, frameLength)
		if _, err := io.ReadFull(input, frame); err != nil {
			return ErrInvalidBackup
		}
		plain, err := gcm.Open(nil, chunkNonce(noncePrefix, counter), frame, chunkAAD(headerPayload, counter))
		if err != nil {
			return ErrWrongPassword
		}
		if written+int64(len(plain)) > header.PlainBytes {
			wipe(plain)
			return ErrInvalidBackup
		}
		if _, err := output.Write(plain); err != nil {
			wipe(plain)
			return err
		}
		_, _ = hash.Write(plain)
		written += int64(len(plain))
		wipe(plain)
	}
	if written != header.PlainBytes || !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), header.PlainSHA256) {
		return ErrWrongPassword
	}
	if err := output.Sync(); err != nil {
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	valid = true
	return nil
}

func chunkNonce(prefix []byte, counter uint32) []byte {
	nonce := make([]byte, 12)
	copy(nonce, prefix)
	binary.BigEndian.PutUint32(nonce[8:], counter)
	return nonce
}

func chunkAAD(header []byte, counter uint32) []byte {
	aad := make([]byte, len(header)+4)
	copy(aad, header)
	binary.BigEndian.PutUint32(aad[len(header):], counter)
	return aad
}

func wipe(payload []byte) {
	for index := range payload {
		payload[index] = 0
	}
}
