package network

type SecretProtector interface {
	Protect([]byte) (string, error)
	Unprotect(string) ([]byte, error)
}
