package profile

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"bruno-browser/internal/domain"
)

func TestStoreCreatesAndUpdatesDurableProfile(t *testing.T) {
	createdAt := time.Date(2026, time.August, 5, 12, 0, 0, 0, time.UTC)
	currentTime := createdAt
	store, err := NewStore(t.TempDir(), WithClock(func() time.Time { return currentTime }))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	metadata, err := store.Create(context.Background(), validFields())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	paths, err := store.Paths(metadata.ID)
	if err != nil {
		t.Fatalf("Paths: %v", err)
	}
	for _, directory := range []string{paths.Root, paths.UserData, paths.Extensions} {
		info, statErr := os.Stat(directory)
		if statErr != nil || !info.IsDir() {
			t.Fatalf("expected physical directory %s: %v", directory, statErr)
		}
	}
	payload, err := os.ReadFile(paths.Metadata)
	if err != nil {
		t.Fatalf("read metadata.json: %v", err)
	}
	var diskMetadata domain.Metadata
	if err := json.Unmarshal(payload, &diskMetadata); err != nil {
		t.Fatalf("decode metadata.json: %v", err)
	}
	if diskMetadata.ID != metadata.ID || diskMetadata.StartURL != "https://accounts.google.com/" {
		t.Fatalf("unexpected disk metadata: %#v", diskMetadata)
	}

	currentTime = createdAt.Add(2 * time.Hour)
	updatedFields := validFields()
	updatedFields.Name = "Conta principal Google"
	updated, err := store.Update(context.Background(), metadata.ID, updatedFields)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !updated.CreatedAt.Equal(createdAt) || !updated.UpdatedAt.Equal(currentTime) {
		t.Fatalf("timestamps were not preserved correctly: %#v", updated)
	}

	launchedAt := currentTime.Add(time.Minute)
	launched, err := store.RecordLaunch(context.Background(), metadata.ID, launchedAt)
	if err != nil {
		t.Fatalf("RecordLaunch: %v", err)
	}
	if launched.LaunchCount != 1 || launched.LastLaunchedAt == nil || !launched.LastLaunchedAt.Equal(launchedAt) {
		t.Fatalf("launch metadata was not recorded: %#v", launched)
	}

	currentTime = launchedAt.Add(time.Minute)
	visited, err := store.RecordLastURL(context.Background(), metadata.ID, "https://accounts.google.com/signin/v2/")
	if err != nil {
		t.Fatalf("RecordLastURL: %v", err)
	}
	if visited.LastURL != "https://accounts.google.com/signin/v2/" || !visited.UpdatedAt.Equal(currentTime) {
		t.Fatalf("last navigation was not recorded: %#v", visited)
	}

	profiles, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(profiles) != 1 || profiles[0].ID != metadata.ID {
		t.Fatalf("unexpected profile list: %#v", profiles)
	}
}

func TestStoreRejectsInvalidProfileID(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if _, err := store.Paths(filepath.Join("..", "outside")); err != ErrInvalidID {
		t.Fatalf("expected ErrInvalidID, got %v", err)
	}
}

func TestStoreRestoresCorruptMetadataFromAtomicBackup(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := store.Create(context.Background(), validFields())
	if err != nil {
		t.Fatal(err)
	}
	updatedFields := validFields()
	updatedFields.Name = "Perfil recuperável"
	updated, err := store.Update(context.Background(), metadata.ID, updatedFields)
	if err != nil {
		t.Fatal(err)
	}
	paths, err := store.Paths(metadata.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(paths.MetadataBackup); err != nil {
		t.Fatalf("metadata backup was not created: %v", err)
	}
	if err := os.WriteFile(paths.Metadata, []byte(`{"schemaVersion":`), 0o600); err != nil {
		t.Fatal(err)
	}
	recovered, err := store.Get(context.Background(), metadata.ID)
	if err != nil {
		t.Fatalf("Get did not recover the profile: %v", err)
	}
	if recovered.ID != updated.ID || recovered.Name != "Perfil recuperável" {
		t.Fatalf("unexpected recovered metadata: %+v", recovered)
	}
	payload, err := os.ReadFile(paths.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	var repaired domain.Metadata
	if err := json.Unmarshal(payload, &repaired); err != nil || repaired.Name != recovered.Name {
		t.Fatalf("primary metadata was not repaired: %+v, %v", repaired, err)
	}
}

func validFields() Fields {
	return Fields{
		Name:      "Conta Google 01",
		Color:     "#36F58B",
		Platforms: []domain.Platform{domain.PlatformGoogle},
		Status:    domain.StatusStarting,
		Tags: []domain.Tag{{
			ID: "starting", Label: "Iniciando", Color: "#70A5FF", Kind: domain.TagKindStatus,
		}},
		Notes:    "Primeiro acesso acompanhado.",
		StartURL: "https://accounts.google.com/",
	}
}
