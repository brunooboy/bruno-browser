//go:build !windows

package network

func NewDefaultProtector(dataRoot string) (SecretProtector, error) {
	return newFileKeyProtector(dataRoot)
}
