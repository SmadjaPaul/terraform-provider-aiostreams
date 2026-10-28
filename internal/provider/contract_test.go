package provider

import (
	"encoding/json"
	"os"
	"testing"
)

func TestUserDataV237ContractFixture(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/userdata-v2.37.0.json")
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"formatter", "sortCriteria", "preferredLanguages", "preferredSubtitles", "services", "presets", "jellyfin"} {
		if _, ok := document[key]; !ok {
			t.Fatalf("v2.37.0 fixture missing %q", key)
		}
	}
	if managedFingerprint(raw) == "invalid" {
		t.Fatal("fixture is not a valid object")
	}
}
