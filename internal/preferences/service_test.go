package preferences

import (
	"context"
	"testing"
)

func TestPreferencesRoundTrip(t *testing.T) {
	service, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defaults, err := service.Get(context.Background())
	if err != nil || defaults.AccentColor != "#42ff91" {
		t.Fatalf("unexpected defaults: %+v %v", defaults, err)
	}
	if _, err := service.Save(context.Background(), Preferences{AccentColor: "#70A5FF"}); err != nil {
		t.Fatal(err)
	}
	saved, err := service.Get(context.Background())
	if err != nil || saved.AccentColor != "#70a5ff" {
		t.Fatalf("unexpected saved value: %+v %v", saved, err)
	}
}
