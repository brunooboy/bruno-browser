package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"bruno-browser/internal/network"
	"bruno-browser/internal/storage"
)

const supportReportSchemaVersion = 1

type SupportCheck struct {
	ID     string      `json:"id"`
	Status CheckStatus `json:"status"`
}

type SupportIncidentSummary struct {
	Scope  string    `json:"scope"`
	Count  int       `json:"count"`
	LastAt time.Time `json:"lastAt"`
}

type SupportProfile struct {
	Reference          string `json:"reference"`
	NetworkMode        string `json:"networkMode"`
	DNSPreset          string `json:"dnsPreset"`
	NetworkReadable    bool   `json:"networkReadable"`
	Running            bool   `json:"running"`
	AssignedExtensions int    `json:"assignedExtensions"`
}

type SupportInventory struct {
	ProfileInventoryReadable   bool   `json:"profileInventoryReadable"`
	ExtensionInventoryReadable bool   `json:"extensionInventoryReadable"`
	Profiles                   int    `json:"profiles"`
	RunningProfiles            int    `json:"runningProfiles"`
	ProxyRoutes                int    `json:"proxyRoutes"`
	Extensions                 int    `json:"extensions"`
	AccountCached              bool   `json:"accountCached"`
	LicenseStatus              string `json:"licenseStatus"`
	LicensePlan                string `json:"licensePlan,omitempty"`
}

type SupportReport struct {
	SchemaVersion int                      `json:"schemaVersion"`
	GeneratedAt   time.Time                `json:"generatedAt"`
	AppVersion    string                   `json:"appVersion"`
	Platform      string                   `json:"platform"`
	Architecture  string                   `json:"architecture"`
	OverallStatus string                   `json:"overallStatus"`
	Checks        []SupportCheck           `json:"checks"`
	Incidents     []SupportIncidentSummary `json:"incidents"`
	Inventory     SupportInventory         `json:"inventory"`
	Profiles      []SupportProfile         `json:"profiles"`
}

type SupportExport struct {
	Path        string    `json:"path"`
	Bytes       int64     `json:"bytes"`
	GeneratedAt time.Time `json:"generatedAt"`
}

func (s *Service) BuildSupportReport(ctx context.Context) (SupportReport, error) {
	diagnosticReport, err := s.Run(ctx)
	if err != nil {
		return SupportReport{}, err
	}
	updateStatus, err := s.updates.Current(ctx)
	if err != nil {
		return SupportReport{}, fmt.Errorf("read app version for support report: %w", err)
	}

	report := SupportReport{
		SchemaVersion: supportReportSchemaVersion,
		GeneratedAt:   s.clock().UTC(),
		AppVersion:    updateStatus.CurrentVersion,
		Platform:      runtime.GOOS,
		Architecture:  runtime.GOARCH,
		OverallStatus: diagnosticReport.Status,
		Checks:        make([]SupportCheck, 0, len(diagnosticReport.Checks)),
		Profiles:      []SupportProfile{},
	}
	for _, check := range diagnosticReport.Checks {
		report.Checks = append(report.Checks, SupportCheck{ID: check.ID, Status: check.Status})
	}
	report.Incidents = summarizeIncidents(diagnosticReport.Incidents)

	profiles, profileErr := s.profiles.List(ctx)
	report.Inventory.ProfileInventoryReadable = profileErr == nil
	if profileErr == nil {
		report.Inventory.Profiles = len(profiles)
	}
	installed, extensionErr := s.extensions.List(ctx)
	report.Inventory.ExtensionInventoryReadable = extensionErr == nil
	if extensionErr == nil {
		report.Inventory.Extensions = len(installed)
	}

	running := make(map[string]bool)
	for _, process := range s.browser.Running() {
		running[process.ProfileID] = true
	}
	assignments := make(map[string]int)
	for _, extension := range installed {
		for _, profileID := range extension.AssignedProfileIDs {
			assignments[profileID]++
		}
	}
	for index, metadata := range profiles {
		profileReport := SupportProfile{
			Reference:          fmt.Sprintf("profile-%03d", index+1),
			NetworkMode:        "unreadable",
			DNSPreset:          "unreadable",
			Running:            running[metadata.ID],
			AssignedExtensions: assignments[metadata.ID],
		}
		settings, settingsErr := s.network.Get(ctx, metadata.ID)
		if settingsErr == nil {
			profileReport.NetworkReadable = true
			profileReport.NetworkMode = string(settings.Mode)
			profileReport.DNSPreset = string(settings.DNSPreset)
			if settings.Mode != network.ModeDirect {
				report.Inventory.ProxyRoutes++
			}
		}
		if profileReport.Running {
			report.Inventory.RunningProfiles++
		}
		report.Profiles = append(report.Profiles, profileReport)
	}

	user, accountErr := s.account.Get(ctx)
	if accountErr == nil && user != nil {
		report.Inventory.AccountCached = true
		activation, activationErr := s.license.Status(ctx, user.ID)
		if activationErr == nil {
			report.Inventory.LicenseStatus = activation.Status
			report.Inventory.LicensePlan = string(activation.Plan)
		} else {
			report.Inventory.LicenseStatus = "unreadable"
		}
	} else if accountErr != nil {
		report.Inventory.LicenseStatus = "unreadable"
	} else {
		report.Inventory.LicenseStatus = "none"
	}
	return report, nil
}

func (s *Service) ExportSupportReport(ctx context.Context, destination string) (SupportExport, error) {
	if err := ctx.Err(); err != nil {
		return SupportExport{}, err
	}
	destination = strings.TrimSpace(destination)
	if destination == "" {
		return SupportExport{}, errors.New("support report destination is required")
	}
	if !strings.EqualFold(filepath.Ext(destination), ".json") {
		destination += ".json"
	}
	absolutePath, err := filepath.Abs(filepath.Clean(destination))
	if err != nil {
		return SupportExport{}, fmt.Errorf("normalize support report destination: %w", err)
	}
	report, err := s.BuildSupportReport(ctx)
	if err != nil {
		return SupportExport{}, err
	}
	if err := storage.WriteJSONAtomic(absolutePath, report, 0o600); err != nil {
		return SupportExport{}, fmt.Errorf("write support report: %w", err)
	}
	info, err := os.Stat(absolutePath)
	if err != nil {
		return SupportExport{}, fmt.Errorf("inspect support report: %w", err)
	}
	return SupportExport{Path: absolutePath, Bytes: info.Size(), GeneratedAt: report.GeneratedAt}, nil
}

func summarizeIncidents(incidents []Incident) []SupportIncidentSummary {
	type aggregate struct {
		count  int
		lastAt time.Time
	}
	byScope := make(map[string]aggregate)
	for _, incident := range incidents {
		current := byScope[incident.Scope]
		current.count++
		if incident.At.After(current.lastAt) {
			current.lastAt = incident.At
		}
		byScope[incident.Scope] = current
	}
	result := make([]SupportIncidentSummary, 0, len(byScope))
	for scope, item := range byScope {
		result = append(result, SupportIncidentSummary{Scope: scope, Count: item.count, LastAt: item.lastAt})
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Scope < result[right].Scope })
	return result
}
