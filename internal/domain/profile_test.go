package domain

import (
	"testing"
	"time"
)

func TestMetadataMaturity(t *testing.T) {
	createdAt := time.Date(2026, time.August, 1, 10, 0, 0, 0, time.UTC)
	metadata := Metadata{CreatedAt: createdAt}
	maturity := metadata.Maturity(createdAt.Add(51*time.Hour + 20*time.Minute))
	if maturity.Days != 2 || maturity.Hours != 3 || maturity.TotalHours != 51 {
		t.Fatalf("unexpected maturity: %#v", maturity)
	}
}

func TestValidateStartURL(t *testing.T) {
	for _, valid := range []string{"", "about:blank", "https://accounts.google.com/", "http://localhost:8080/login"} {
		if err := ValidateStartURL(valid); err != nil {
			t.Errorf("ValidateStartURL(%q) returned %v", valid, err)
		}
	}
	for _, invalid := range []string{"javascript:alert(1)", "file:///etc/passwd", "accounts.google.com"} {
		if err := ValidateStartURL(invalid); err == nil {
			t.Errorf("ValidateStartURL(%q) accepted an unsafe URL", invalid)
		}
	}
}
