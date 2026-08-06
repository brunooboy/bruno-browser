//go:build windows

package network

import (
	"encoding/base64"
	"errors"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

type dpapiProtector struct{}

func NewDefaultProtector(dataRoot string) (SecretProtector, error) {
	protector := dpapiProtector{}
	probe, err := protector.Protect([]byte("bruno-browser-dpapi-probe"))
	if err == nil {
		if plaintext, unprotectErr := protector.Unprotect(probe); unprotectErr == nil && string(plaintext) == "bruno-browser-dpapi-probe" {
			for index := range plaintext {
				plaintext[index] = 0
			}
			return protector, nil
		}
	}
	return newFileKeyProtector(dataRoot)
}

func (dpapiProtector) Protect(plaintext []byte) (string, error) {
	if len(plaintext) == 0 {
		return "", nil
	}
	input := windows.DataBlob{Size: uint32(len(plaintext)), Data: &plaintext[0]}
	var output windows.DataBlob
	name, err := windows.UTF16PtrFromString("bruno browser proxy credential")
	if err != nil {
		return "", err
	}
	if err := windows.CryptProtectData(
		&input,
		name,
		nil,
		0,
		nil,
		windows.CRYPTPROTECT_UI_FORBIDDEN,
		&output,
	); err != nil {
		return "", err
	}
	defer windows.LocalFree(windows.Handle(uintptr(unsafe.Pointer(output.Data))))
	protected := append([]byte(nil), unsafe.Slice(output.Data, output.Size)...)
	runtime.KeepAlive(plaintext)
	return base64.StdEncoding.EncodeToString(protected), nil
}

func (dpapiProtector) Unprotect(ciphertext string) ([]byte, error) {
	if ciphertext == "" {
		return nil, nil
	}
	protected, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return nil, err
	}
	if len(protected) == 0 {
		return nil, errors.New("protected proxy credential is empty")
	}
	input := windows.DataBlob{Size: uint32(len(protected)), Data: &protected[0]}
	var output windows.DataBlob
	if err := windows.CryptUnprotectData(
		&input,
		nil,
		nil,
		0,
		nil,
		windows.CRYPTPROTECT_UI_FORBIDDEN,
		&output,
	); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(uintptr(unsafe.Pointer(output.Data))))
	plaintext := append([]byte(nil), unsafe.Slice(output.Data, output.Size)...)
	runtime.KeepAlive(protected)
	return plaintext, nil
}
