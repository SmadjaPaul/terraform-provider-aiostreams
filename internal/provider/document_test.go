package provider

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestDesiredDocumentPreservesUnknownRemoteFields(t *testing.T) {
	config := configurationModel{
		ConfigurationJSON: types.StringValue(`{"jellyfin":{"primary":{"name":"Paul"}}}`),
	}
	remote := json.RawMessage(`{"uuid":"11111111-1111-1111-1111-111111111111","futureField":{"keep":true},"presets":[{"type":"cinemeta","instanceId":"cinemeta","enabled":true,"options":{}}]}`)

	got, err := desiredDocument(context.Background(), config, remote)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(got, &document); err != nil {
		t.Fatal(err)
	}
	if document["futureField"].(map[string]any)["keep"] != true {
		t.Fatal("unknown remote field was not preserved")
	}
	if document["presets"] == nil {
		t.Fatal("remote presets were not preserved")
	}
	primary := document["jellyfin"].(map[string]any)["primary"].(map[string]any)
	if primary["name"] != "Paul" {
		t.Fatalf("primary profile not overlaid: %#v", primary)
	}
}

func TestDesiredDocumentDropsLegacyCatalogModificationsForNativePresets(t *testing.T) {
	config := configurationModel{
		ConfigurationJSON: types.StringValue(`{"presets":[{"type":"tmdb-addon","instanceId":"smadja-tmdb","enabled":true,"options":{}}]}`),
	}
	remote := json.RawMessage(`{"catalogModifications":[{"id":"cinemeta","type":"movie"}],"futureField":{"keep":true}}`)

	got, err := desiredDocument(context.Background(), config, remote)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(got, &document); err != nil {
		t.Fatal(err)
	}
	if _, ok := document["catalogModifications"]; ok {
		t.Fatal("legacy catalog modifications must not survive a native preset transition")
	}
	if document["futureField"].(map[string]any)["keep"] != true {
		t.Fatal("unknown remote fields must remain preserved")
	}
}

func TestDesiredDocumentPreservesCatalogModificationsWithoutNativePresetOwner(t *testing.T) {
	config := configurationModel{
		ConfigurationJSON: types.StringValue(`{"formatter":{"id":"tamtaro"}}`),
	}
	remote := json.RawMessage(`{"catalogModifications":[{"id":"cinemeta","type":"movie"}]}`)

	got, err := desiredDocument(context.Background(), config, remote)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(got, &document); err != nil {
		t.Fatal(err)
	}
	if _, ok := document["catalogModifications"]; !ok {
		t.Fatal("catalog modifications must remain when no native preset document owns the boundary")
	}
}

func TestPublicJSONRejectsSecretFields(t *testing.T) {
	config := configurationModel{
		ConfigurationJSON: types.StringValue(`{"tmdbApiKey":"must-not-be-state"}`),
	}
	_, err := desiredDocument(context.Background(), config, nil)
	if err == nil || !strings.Contains(err.Error(), "write-only") {
		t.Fatalf("expected secret rejection, got %v", err)
	}
}

func TestManagedFingerprintDoesNotContainSecretMaterial(t *testing.T) {
	first := json.RawMessage(`{"uuid":"a","services":[{"id":"torbox","credentials":{"apiKey":"one"}}],"presets":[]}`)
	second := json.RawMessage(`{"uuid":"b","services":[{"id":"torbox","credentials":{"apiKey":"two"}}],"presets":[]}`)
	if managedFingerprint(first) != managedFingerprint(second) {
		t.Fatal("credential value changed the non-secret fingerprint")
	}
	if strings.Contains(managedFingerprint(first), "one") || strings.Contains(managedFingerprint(first), "two") {
		t.Fatal("secret material entered fingerprint")
	}
}

func TestManagedFingerprintIgnoresUnknownFields(t *testing.T) {
	base := json.RawMessage(`{"formatter":{"type":"torbox"},"futureField":{"version":1}}`)
	changed := json.RawMessage(`{"formatter":{"type":"torbox"},"futureField":{"version":2}}`)
	if managedFingerprint(base) != managedFingerprint(changed) {
		t.Fatal("unknown remote fields must not create managed drift")
	}
}

func TestManagedFingerprintIncludesConfiguredJSONFields(t *testing.T) {
	config := configurationModel{FiltersJSON: types.StringValue(`{"hideErrors":true}`)}
	first := json.RawMessage(`{"hideErrors":true}`)
	second := json.RawMessage(`{"hideErrors":false}`)
	if managedFingerprintForConfig(first, config) == managedFingerprintForConfig(second, config) {
		t.Fatal("configured JSON fields must participate in drift detection")
	}
}

func TestConcurrentChangeDetected(t *testing.T) {
	state := configurationModel{
		ConfigurationJSON:  types.StringValue(`{"formatter":{"id":"torbox"}}`),
		ManagedFingerprint: types.StringValue(managedFingerprintForConfig(json.RawMessage(`{"formatter":{"id":"torbox"}}`), configurationModel{ConfigurationJSON: types.StringValue(`{"formatter":{"id":"torbox"}}`)})),
	}
	if !concurrentChangeDetected(state, json.RawMessage(`{"formatter":{"id":"other"}}`)) {
		t.Fatal("remote changes must block an update based on stale state")
	}
}

func TestImplicitDeleteIsNeverImplemented(t *testing.T) {
	resourceImpl := &configurationResource{}
	response := &resource.DeleteResponse{}
	resourceImpl.Delete(context.Background(), resource.DeleteRequest{}, response)
	if !response.Diagnostics.HasError() {
		t.Fatal("implicit deletion must return a diagnostic")
	}
}
