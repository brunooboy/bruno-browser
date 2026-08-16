package updates

import (
	"context"
	"testing"
)

func TestEmbeddedManifestAndVersionComparison(t *testing.T) {
	service, err := New("")
	if err != nil {
		t.Fatal(err)
	}
	status, err := service.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.CurrentVersion != "1.0.0" || status.UpdateAvailable || len(status.Changelog) == 0 {
		t.Fatalf("unexpected status: %+v", status)
	}
	if compareVersions("0.9.0", "0.8.9") <= 0 {
		t.Fatal("version comparison failed")
	}
}
