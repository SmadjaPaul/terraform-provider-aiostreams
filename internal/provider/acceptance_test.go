package provider

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/SmadjaPaul/terraform-provider-aiostreams/internal/aiostreams"
)

// This test is intentionally opt-in. The disposable endpoint must be supplied
// by the operator; credentials are never printed and the provider has no
// DELETE operation, so the endpoint must be disposable or reset separately.
func TestAccAIOStreamsConfigurationLifecycle(t *testing.T) {
	if os.Getenv("TF_ACC") != "1" {
		t.Skip("set TF_ACC=1 to run against an explicitly provisioned disposable AIOStreams instance")
	}
	endpoint := os.Getenv("AIOSTREAMS_ACC_ENDPOINT")
	password := os.Getenv("AIOSTREAMS_ACC_PASSWORD")
	if endpoint == "" || password == "" {
		t.Fatal("AIOSTREAMS_ACC_ENDPOINT and AIOSTREAMS_ACC_PASSWORD are required when TF_ACC=1")
	}
	client, err := aiostreams.New(endpoint)
	if err != nil {
		t.Fatal("invalid acceptance endpoint")
	}

	ctx := context.Background()
	initial := json.RawMessage(`{"formatter":{"id":"torbox"},"sortCriteria":{"global":[{"key":"quality","direction":"desc"}]},"presets":[]}`)
	created, err := client.Create(ctx, password, initial)
	if err != nil {
		t.Fatal("AIOStreams acceptance create failed")
	}
	read, err := client.Get(ctx, aiostreams.Credentials{UUID: created.UUID, Password: password})
	if err != nil {
		t.Fatal("AIOStreams acceptance read failed")
	}
	var document map[string]any
	if err := json.Unmarshal(read, &document); err != nil || document["presets"] == nil {
		t.Fatal("AIOStreams acceptance read did not return UserData")
	}

	updated := json.RawMessage(`{"formatter":{"id":"torbox"},"sortCriteria":{"global":[{"key":"quality","direction":"desc"}]},"presets":[],"manifestNotice":"never"}`)
	if err := client.Update(ctx, aiostreams.Credentials{UUID: created.UUID, Password: password}, updated); err != nil {
		t.Fatal("AIOStreams acceptance update failed")
	}
	read, err = client.Get(ctx, aiostreams.Credentials{UUID: created.UUID, Password: password})
	if err != nil {
		t.Fatal("AIOStreams acceptance post-update read failed")
	}
	if err := json.Unmarshal(read, &document); err != nil || document["manifestNotice"] != "never" {
		t.Fatal("AIOStreams acceptance update was not persisted")
	}
}
