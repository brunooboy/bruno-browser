package network

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"
)

const CurrentSchemaVersion = 1

type Mode string

type DNSPreset string

const (
	ModeDirect Mode = "direct"
	ModeHTTP   Mode = "http"
	ModeSOCKS5 Mode = "socks5"
)

const (
	DNSLight   DNSPreset = "light"
	DNSNormal  DNSPreset = "normal"
	DNSHigh    DNSPreset = "high"
	DNSPro     DNSPreset = "pro"
	DNSProPlus DNSPreset = "pro_plus"
)

type Settings struct {
	SchemaVersion int       `json:"schemaVersion"`
	ProfileID     string    `json:"profileId"`
	Mode          Mode      `json:"mode"`
	DNSPreset     DNSPreset `json:"dnsPreset"`
	Host          string    `json:"host,omitempty"`
	Port          uint16    `json:"port,omitempty"`
	Username      string    `json:"username,omitempty"`
	HasPassword   bool      `json:"hasPassword"`
	BypassList    []string  `json:"bypassList,omitempty"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type SaveInput struct {
	Mode          Mode      `json:"mode"`
	DNSPreset     DNSPreset `json:"dnsPreset"`
	Host          string    `json:"host,omitempty"`
	Port          uint16    `json:"port,omitempty"`
	Username      string    `json:"username,omitempty"`
	Password      string    `json:"password,omitempty"`
	ClearPassword bool      `json:"clearPassword"`
	BypassList    []string  `json:"bypassList,omitempty"`
}

type RuntimeSettings struct {
	Settings
	Password string
}

type TestResult struct {
	ProfileID string `json:"profileId"`
	Mode      Mode   `json:"mode"`
	LatencyMs int64  `json:"latencyMs"`
	Endpoint  string `json:"endpoint"`
}

var hostnameLabelPattern = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)

func (mode Mode) Valid() bool {
	return mode == ModeDirect || mode == ModeHTTP || mode == ModeSOCKS5
}

func (preset DNSPreset) Valid() bool {
	return preset == DNSLight || preset == DNSNormal || preset == DNSHigh || preset == DNSPro || preset == DNSProPlus
}

func normalizeDNSPreset(preset DNSPreset) (DNSPreset, error) {
	if preset == "" {
		return DNSNormal, nil
	}
	if !preset.Valid() {
		return "", fmt.Errorf("unsupported DNS preset %q", preset)
	}
	return preset, nil
}

func normalizeInput(input SaveInput) (SaveInput, error) {
	input.Host = strings.TrimSpace(strings.Trim(input.Host, "[]"))
	input.Username = strings.TrimSpace(input.Username)
	input.BypassList = normalizeBypassList(input.BypassList)
	var err error
	input.DNSPreset, err = normalizeDNSPreset(input.DNSPreset)
	if err != nil {
		return SaveInput{}, err
	}
	if !input.Mode.Valid() {
		return SaveInput{}, fmt.Errorf("unsupported network mode %q", input.Mode)
	}
	if input.Mode == ModeDirect {
		return SaveInput{Mode: ModeDirect, DNSPreset: input.DNSPreset, ClearPassword: true}, nil
	}
	if err := validateHost(input.Host); err != nil {
		return SaveInput{}, err
	}
	if input.Port == 0 {
		return SaveInput{}, errors.New("proxy port must be between 1 and 65535")
	}
	if len([]rune(input.Username)) > 255 {
		return SaveInput{}, errors.New("proxy username is too long")
	}
	if len([]byte(input.Password)) > 255 && input.Mode == ModeSOCKS5 {
		return SaveInput{}, errors.New("SOCKS5 passwords cannot exceed 255 bytes")
	}
	if input.Password != "" && input.Username == "" {
		return SaveInput{}, errors.New("proxy username is required when a password is provided")
	}
	if input.ClearPassword && input.Password != "" {
		return SaveInput{}, errors.New("cannot replace and clear the proxy password together")
	}
	for _, rule := range input.BypassList {
		if err := validateBypassRule(rule); err != nil {
			return SaveInput{}, err
		}
	}
	return input, nil
}

func validateHost(host string) error {
	if host == "" {
		return errors.New("proxy host is required")
	}
	if net.ParseIP(host) != nil {
		return nil
	}
	if len(host) > 253 || strings.ContainsAny(host, "/:@?# \t\r\n") {
		return errors.New("proxy host is invalid")
	}
	for _, label := range strings.Split(host, ".") {
		if !hostnameLabelPattern.MatchString(label) {
			return errors.New("proxy host is invalid")
		}
	}
	return nil
}

func normalizeBypassList(rules []string) []string {
	result := make([]string, 0, len(rules))
	seen := make(map[string]struct{}, len(rules))
	for _, rule := range rules {
		rule = strings.ToLower(strings.TrimSpace(rule))
		if rule == "" {
			continue
		}
		if _, exists := seen[rule]; exists {
			continue
		}
		seen[rule] = struct{}{}
		result = append(result, rule)
	}
	return result
}

func validateBypassRule(rule string) error {
	if len(rule) > 255 || strings.ContainsAny(rule, ",;\r\n\t ") {
		return fmt.Errorf("invalid proxy bypass rule %q", rule)
	}
	if rule == "<local>" || rule == "<-loopback>" {
		return nil
	}
	host := strings.TrimPrefix(rule, "*.")
	if _, _, err := net.ParseCIDR(host); err == nil {
		return nil
	}
	return validateHost(host)
}

func proxyEndpoint(mode Mode, host string, port uint16) string {
	return fmt.Sprintf("%s://%s", mode, net.JoinHostPort(host, fmt.Sprintf("%d", port)))
}
