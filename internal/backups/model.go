package backups

import (
	"errors"
	"time"

	"bruno-browser/internal/domain"
	"bruno-browser/internal/network"
)

const (
	CurrentSchemaVersion = 1
	AppVersion           = "1.5.0"
)

var (
	ErrInvalidBackup  = errors.New("invalid or corrupted Bruno profile backup")
	ErrWrongPassword  = errors.New("backup password is incorrect or the file was modified")
	ErrProfileRunning = errors.New("all selected profiles must be closed before backup")
)

type HistoryEntry struct {
	ID           string    `json:"id"`
	Operation    string    `json:"operation"`
	Status       string    `json:"status"`
	ArchivePath  string    `json:"archivePath"`
	Bytes        int64     `json:"bytes"`
	ProfileCount int       `json:"profileCount"`
	ProfileNames []string  `json:"profileNames,omitempty"`
	Message      string    `json:"message,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}

type ExportResult struct {
	Cancelled bool         `json:"cancelled"`
	Path      string       `json:"path,omitempty"`
	Bytes     int64        `json:"bytes,omitempty"`
	Profiles  int          `json:"profiles,omitempty"`
	History   HistoryEntry `json:"history"`
}

type ImportedProfile struct {
	SourceID string `json:"sourceId"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	Rekeyed  bool   `json:"rekeyed"`
}

type ImportResult struct {
	Cancelled bool              `json:"cancelled"`
	Path      string            `json:"path,omitempty"`
	Bytes     int64             `json:"bytes,omitempty"`
	Profiles  []ImportedProfile `json:"profiles,omitempty"`
	History   HistoryEntry      `json:"history"`
}

type archiveManifest struct {
	SchemaVersion int                `json:"schemaVersion"`
	AppVersion    string             `json:"appVersion"`
	ExportedAt    time.Time          `json:"exportedAt"`
	Profiles      []archiveProfile   `json:"profiles"`
	Extensions    []archiveExtension `json:"extensions,omitempty"`
}

type archiveProfile struct {
	Metadata     domain.Metadata `json:"metadata"`
	Network      archiveNetwork  `json:"network"`
	ExtensionIDs []string        `json:"extensionIds,omitempty"`
}

type archiveNetwork struct {
	Mode       network.Mode      `json:"mode"`
	DNSPreset  network.DNSPreset `json:"dnsPreset"`
	Host       string            `json:"host,omitempty"`
	Port       uint16            `json:"port,omitempty"`
	Username   string            `json:"username,omitempty"`
	Password   string            `json:"password,omitempty"`
	BypassList []string          `json:"bypassList,omitempty"`
}

type archiveExtension struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Entry   string `json:"entry"`
}
