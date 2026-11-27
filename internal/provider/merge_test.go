package provider

import (
	"encoding/json"
	"reflect"
	"testing"
)

// The provider replaces the whole UserData document on every write and
// promises that the fields it does not manage survive untouched. That promise
// is carried by the merge helpers of internal/provider/document.go, so they are
// exercised directly here: an unknown remote field must survive an overlay, a
// managed field must be updated in place, and the merge must never corrupt the
// document it was built from.

// decodeDocument parses a JSON document the way the provider reads a remote
// UserData: objects become map[string]any, arrays become []any and null
// becomes a nil interface.
func decodeDocument(t *testing.T, raw string) map[string]any {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal([]byte(raw), &document); err != nil {
		t.Fatal(err)
	}
	return document
}

func TestMergeObjectMergesNestedObjectsAndKeepsUnknownFields(t *testing.T) {
	base := decodeDocument(t, `{"uuid":"11111111-1111-1111-1111-111111111111","futureField":{"keep":true},"jellyfin":{"primary":{"name":"Paul","avatar":"paul.png"}}}`)
	overlay := decodeDocument(t, `{"jellyfin":{"primary":{"name":"Paul Sr"}}}`)

	mergeObject(base, overlay)

	if base["uuid"] != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("unknown top level field was not preserved: %#v", base["uuid"])
	}
	future := base["futureField"].(map[string]any)
	if future["keep"] != true {
		t.Fatalf("unknown nested field was not preserved: %#v", future)
	}
	primary := base["jellyfin"].(map[string]any)["primary"].(map[string]any)
	if primary["name"] != "Paul Sr" {
		t.Fatalf("managed field was not overlaid: %#v", primary)
	}
	if primary["avatar"] != "paul.png" {
		t.Fatalf("unknown nested field of a managed object was not preserved: %#v", primary)
	}
}

func TestMergeObjectKeepsTargetFieldsMissingFromTheOverlay(t *testing.T) {
	base := decodeDocument(t, `{"formatter":{"id":"torbox"},"sortCriteria":{"global":[{"key":"quality"}]}}`)
	overlay := decodeDocument(t, `{"formatter":{"id":"other"}}`)

	mergeObject(base, overlay)

	if base["formatter"].(map[string]any)["id"] != "other" {
		t.Fatalf("overlay was not applied: %#v", base["formatter"])
	}
	if _, ok := base["sortCriteria"]; !ok {
		t.Fatal("a field missing from the overlay must be preserved")
	}
}

func TestMergeObjectReplacesAnObjectWithAScalar(t *testing.T) {
	base := decodeDocument(t, `{"formatter":{"id":"torbox","extra":true}}`)
	overlay := decodeDocument(t, `{"formatter":"torbox"}`)

	mergeObject(base, overlay)

	if base["formatter"] != "torbox" {
		t.Fatalf("a scalar must replace an object: %#v", base["formatter"])
	}
}

func TestMergeObjectReplacesAScalarWithAnObject(t *testing.T) {
	base := decodeDocument(t, `{"formatter":"torbox"}`)
	overlay := decodeDocument(t, `{"formatter":{"id":"torbox"}}`)

	mergeObject(base, overlay)

	formatter, ok := base["formatter"].(map[string]any)
	if !ok || formatter["id"] != "torbox" {
		t.Fatalf("an object must replace a scalar: %#v", base["formatter"])
	}
}

func TestMergeObjectDoesNotShareTheOverlayObjectWithTheResult(t *testing.T) {
	base := decodeDocument(t, `{"formatter":"torbox"}`)
	overlay := decodeDocument(t, `{"formatter":{"id":"torbox"}}`)

	mergeObject(base, overlay)

	base["formatter"].(map[string]any)["id"] = "mutated"
	if overlay["formatter"].(map[string]any)["id"] != "torbox" {
		t.Fatal("the merged result must not alias the overlay object")
	}
}

func TestMergeObjectReplacesListsWholesale(t *testing.T) {
	base := decodeDocument(t, `{"presets":[{"type":"cinemeta","instanceId":"cinemeta"}]}`)
	overlay := decodeDocument(t, `{"presets":[{"type":"tmdb-addon","instanceId":"smadja-tmdb"}]}`)

	mergeObject(base, overlay)

	presets := base["presets"].([]any)
	if len(presets) != 1 {
		t.Fatalf("a list must be replaced, not appended: %#v", presets)
	}
	preset := presets[0].(map[string]any)
	if preset["type"] != "tmdb-addon" || preset["instanceId"] != "smadja-tmdb" {
		t.Fatalf("the overlay list was not applied: %#v", preset)
	}
}

func TestMergeObjectKeepsNullValues(t *testing.T) {
	base := decodeDocument(t, `{"formatter":{"id":"torbox"},"other":"kept"}`)
	overlay := decodeDocument(t, `{"formatter":null}`)

	mergeObject(base, overlay)

	if value, ok := base["formatter"]; !ok || value != nil {
		t.Fatalf("a null must replace the previous value: %#v", base["formatter"])
	}
	if base["other"] != "kept" {
		t.Fatalf("an unrelated field was lost: %#v", base["other"])
	}
	encoded, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"formatter":null,"other":"kept"}` {
		t.Fatalf("a null must be written as a JSON null: %s", encoded)
	}
}

func TestMergeObjectWithAnEmptyOverlayChangesNothing(t *testing.T) {
	base := decodeDocument(t, `{"formatter":{"id":"torbox"},"presets":[]}`)

	mergeObject(base, map[string]any{})

	expected := decodeDocument(t, `{"formatter":{"id":"torbox"},"presets":[]}`)
	if !reflect.DeepEqual(base, expected) {
		t.Fatalf("an empty overlay must leave the document untouched: %#v", base)
	}
}

func TestMergeObjectSharesTheOverlayList(t *testing.T) {
	// Known limitation: a list is copied by reference, so the merged document
	// and the overlay share the same backing array. The overlay is parsed from
	// the configuration on every call, so the aliasing stays inside one write.
	base := decodeDocument(t, `{"presets":[{"type":"cinemeta"}]}`)
	overlay := decodeDocument(t, `{"presets":[{"type":"tmdb-addon"}]}`)

	mergeObject(base, overlay)

	base["presets"].([]any)[0].(map[string]any)["type"] = "mutated"
	if overlay["presets"].([]any)[0].(map[string]any)["type"] != "mutated" {
		t.Fatal("documented limitation: the merged list is shared with the overlay")
	}
}

func TestCloneObjectDoesNotShareNestedObjectsWithTheOriginal(t *testing.T) {
	original := decodeDocument(t, `{"formatter":{"id":"torbox"},"options":{"depth":{"level":1}}}`)
	clone := cloneObject(original)

	clone["formatter"].(map[string]any)["id"] = "mutated"
	if original["formatter"].(map[string]any)["id"] != "torbox" {
		t.Fatal("the clone must not share a nested object with the original")
	}

	clone["options"].(map[string]any)["depth"].(map[string]any)["level"] = 2
	if original["options"].(map[string]any)["depth"].(map[string]any)["level"] != float64(1) {
		t.Fatal("the clone must not share a deeply nested object with the original")
	}
}

func TestCloneObjectCopiesTheListContainer(t *testing.T) {
	original := decodeDocument(t, `{"services":[{"id":"torbox"}]}`)
	clone := cloneObject(original)

	clone["services"].([]any)[0] = "replaced"

	if original["services"].([]any)[0].(map[string]any)["id"] != "torbox" {
		t.Fatal("the list container must be copied, not shared")
	}
}

func TestCloneObjectSharesTheObjectsContainedInAList(t *testing.T) {
	// Documented behaviour: the nested objects of the cloned map are copied,
	// but only the container of a list is copied, the objects it contains stay
	// shared with the original. A caller must not mutate them in place, as
	// the provider does not for the entries it clones.
	original := decodeDocument(t, `{"services":[{"id":"torbox","credentials":{"apiKey":"one"}}]}`)
	clone := cloneObject(original)

	clone["services"].([]any)[0].(map[string]any)["credentials"].(map[string]any)["apiKey"] = "mutated"

	if original["services"].([]any)[0].(map[string]any)["credentials"].(map[string]any)["apiKey"] != "mutated" {
		t.Fatal("documented limitation: the objects contained in a list stay shared with the original")
	}
}

func TestCloneObjectTurnsAnEmptyListIntoNull(t *testing.T) {
	// Known limitation: an empty list is copied as a nil slice, so it is
	// written back as a JSON null instead of an empty JSON array.
	original := decodeDocument(t, `{"options":[]}`)
	clone := cloneObject(original)

	options, ok := clone["options"].([]any)
	if !ok || options != nil {
		t.Fatalf("documented limitation: an empty list is cloned as a nil list: %#v", clone["options"])
	}
	encoded, err := json.Marshal(clone)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"options":null}` {
		t.Fatalf("documented limitation: an empty list is written as a null: %s", encoded)
	}
}

func TestCloneObjectKeepsScalarValues(t *testing.T) {
	original := decodeDocument(t, `{"name":"torbox","count":3,"enabled":true,"missing":null}`)
	clone := cloneObject(original)

	if clone["name"] != "torbox" || clone["count"] != float64(3) || clone["enabled"] != true {
		t.Fatalf("scalar values were not copied: %#v", clone)
	}
	if value, ok := clone["missing"]; !ok || value != nil {
		t.Fatalf("a null value was not copied: %#v", clone["missing"])
	}
}

func TestClearNativeCatalogBoundaryRemovesLegacyCatalogModifications(t *testing.T) {
	base := decodeDocument(t, `{"catalogModifications":[{"id":"cinemeta","type":"movie"}],"futureField":{"keep":true}}`)
	overlay := decodeDocument(t, `{"presets":[{"type":"tmdb-addon","instanceId":"smadja-tmdb"}]}`)

	clearNativeCatalogBoundary(base, overlay)

	if _, ok := base["catalogModifications"]; ok {
		t.Fatal("the legacy catalog modifications must be removed for a native preset document")
	}
	if base["futureField"].(map[string]any)["keep"] != true {
		t.Fatal("an unknown field must be preserved")
	}
	if len(overlay["presets"].([]any)) != 1 {
		t.Fatal("the overlay must not be modified")
	}
}

func TestClearNativeCatalogBoundaryKeepsCatalogModificationsWithoutPresets(t *testing.T) {
	base := decodeDocument(t, `{"catalogModifications":[{"id":"cinemeta","type":"movie"}]}`)
	overlay := decodeDocument(t, `{"formatter":{"id":"torbox"}}`)

	clearNativeCatalogBoundary(base, overlay)

	if _, ok := base["catalogModifications"]; !ok {
		t.Fatal("the catalog modifications must be preserved when no preset owns the boundary")
	}
}

func TestClearNativeCatalogBoundaryKeepsCatalogModificationsDeclaredByTheOverlay(t *testing.T) {
	base := decodeDocument(t, `{"catalogModifications":[{"id":"cinemeta","type":"movie"}]}`)
	overlay := decodeDocument(t, `{"presets":[{"type":"tmdb-addon"}],"catalogModifications":[{"id":"other","type":"series"}]}`)

	clearNativeCatalogBoundary(base, overlay)

	modifications, ok := base["catalogModifications"].([]any)
	if !ok || len(modifications) != 1 {
		t.Fatalf("the catalog modifications must be preserved: %#v", base["catalogModifications"])
	}
	if modifications[0].(map[string]any)["id"] != "cinemeta" {
		t.Fatalf("the boundary helper must not rewrite the catalog modifications: %#v", modifications)
	}
}

func TestClearNativeCatalogBoundaryIgnoresMissingCatalogModifications(t *testing.T) {
	base := decodeDocument(t, `{"futureField":{"keep":true}}`)
	overlay := decodeDocument(t, `{"presets":[{"type":"tmdb-addon"}]}`)

	clearNativeCatalogBoundary(base, overlay)

	if base["futureField"].(map[string]any)["keep"] != true {
		t.Fatal("an unknown field must be preserved")
	}
}

func TestClearNativeCatalogBoundaryTreatsNullPresetsAsDeclared(t *testing.T) {
	// Documented behaviour: the boundary is decided on the presence of the
	// presets key, not on its value.
	base := decodeDocument(t, `{"catalogModifications":[{"id":"cinemeta","type":"movie"}]}`)
	overlay := decodeDocument(t, `{"presets":null}`)

	clearNativeCatalogBoundary(base, overlay)

	if _, ok := base["catalogModifications"]; ok {
		t.Fatal("a declared presets key is expected to remove the legacy catalog modifications")
	}
}

func TestFindObjectByStringReturnsTheMatchingEntry(t *testing.T) {
	services := decodeDocument(t, `{"services":[{"id":"torbox"},{"id":"realdebrid"}]}`)["services"]

	found := findObjectByString(services, "id", "realdebrid")

	if found == nil || found["id"] != "realdebrid" {
		t.Fatalf("the matching entry was not found: %#v", found)
	}
}

func TestFindObjectByStringReturnsNothingWithoutAMatch(t *testing.T) {
	services := decodeDocument(t, `{"services":[{"id":"torbox"}]}`)["services"]

	if found := findObjectByString(services, "id", "realdebrid"); found != nil {
		t.Fatalf("no entry must be returned: %#v", found)
	}
}

func TestFindObjectByStringReturnsNothingForANotAList(t *testing.T) {
	for name, value := range map[string]any{
		"object":       map[string]any{"id": "torbox"},
		"string":       "torbox",
		"nil":          nil,
		"list of text": []any{"torbox"},
		"empty list":   []any{},
	} {
		if found := findObjectByString(value, "id", "torbox"); found != nil {
			t.Fatalf("%s must not return an entry: %#v", name, found)
		}
	}
}

func TestFindObjectByStringComparesWithAStringConversion(t *testing.T) {
	// Documented behaviour: the comparison is a plain string conversion, so a
	// numeric identifier matches its decimal form and a missing key matches
	// the "<nil>" placeholder.
	entries := decodeDocument(t, `{"services":[{"id":42},{"id":true},{"other":"x"}]}`)["services"]

	if found := findObjectByString(entries, "id", "42"); found == nil {
		t.Fatal("a numeric identifier must match its string form")
	}
	if found := findObjectByString(entries, "id", "true"); found == nil {
		t.Fatal("a boolean identifier must match its string form")
	}
	if found := findObjectByString(entries, "id", "<nil>"); found == nil {
		t.Fatal("a missing identifier must match the nil placeholder")
	}
}

func TestFindObjectByStringReturnsTheLiveEntry(t *testing.T) {
	// Documented behaviour: the entry is returned as is, so a caller that
	// mutates it mutates the list it belongs to. The provider clones the
	// entry before merging into it.
	services := decodeDocument(t, `{"services":[{"id":"torbox","enabled":false}]}`)["services"]

	found := findObjectByString(services, "id", "torbox")
	if found == nil {
		t.Fatal("the entry was not found")
	}
	found["enabled"] = true

	if services.([]any)[0].(map[string]any)["enabled"] != true {
		t.Fatal("the returned entry is expected to be the live entry of the list")
	}
}
