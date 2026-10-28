package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/SmadjaPaul/terraform-provider-aiostreams/internal/aiostreams"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = (*configurationResource)(nil)
var _ resource.ResourceWithModifyPlan = (*configurationResource)(nil)
var _ resource.ResourceWithImportState = (*configurationResource)(nil)

type configurationResource struct {
	client   *aiostreams.Client
	provider *Provider
}

func NewConfigurationResource() resource.Resource {
	return &configurationResource{}
}

type configurationModel struct {
	UUID               types.String `tfsdk:"uuid"`
	PasswordWO         types.String `tfsdk:"password_wo"`
	PasswordWOVersion  types.Int64  `tfsdk:"password_wo_version"`
	ConfigurationJSON  types.String `tfsdk:"configuration_json"`
	SecretsJSONWO      types.String `tfsdk:"secrets_json_wo"`
	FiltersJSON        types.String `tfsdk:"filters_json"`
	PreferencesJSON    types.String `tfsdk:"preferences_json"`
	PreferredLanguages types.List   `tfsdk:"preferred_languages"`
	PreferredSubtitles types.List   `tfsdk:"preferred_subtitles"`
	Services           types.List   `tfsdk:"services"`
	Addons             types.List   `tfsdk:"addons"`
	Catalogs           types.List   `tfsdk:"catalogs"`
	Templates          types.List   `tfsdk:"templates"`
	Primary            types.Object `tfsdk:"primary_profile"`
	Personas           types.List   `tfsdk:"personas"`
	ManagedFingerprint types.String `tfsdk:"managed_fingerprint"`
}

type serviceModel struct {
	ID          types.String `tfsdk:"id"`
	Enabled     types.Bool   `tfsdk:"enabled"`
	Credentials types.String `tfsdk:"credentials_json_wo"`
}

type addonModel struct {
	Type       types.String `tfsdk:"type"`
	InstanceID types.String `tfsdk:"instance_id"`
	Enabled    types.Bool   `tfsdk:"enabled"`
	Options    types.String `tfsdk:"options_json"`
	OptionsWO  types.String `tfsdk:"options_json_wo"`
	Category   types.String `tfsdk:"category"`
}

type catalogModel struct {
	ID             types.String `tfsdk:"id"`
	Type           types.String `tfsdk:"type"`
	Name           types.String `tfsdk:"name"`
	Enabled        types.Bool   `tfsdk:"enabled"`
	ShowOnHome     types.Bool   `tfsdk:"show_on_home"`
	OnlyOnDiscover types.Bool   `tfsdk:"only_on_discover"`
	OnlyOnSearch   types.Bool   `tfsdk:"only_on_search"`
	DisableSearch  types.Bool   `tfsdk:"disable_search"`
	AddonName      types.String `tfsdk:"addon_name"`
}

type templateModel struct {
	ID               types.String `tfsdk:"id"`
	Version          types.String `tfsdk:"version"`
	URL              types.String `tfsdk:"url"`
	DismissedVersion types.String `tfsdk:"dismissed_version"`
	Ignored          types.Bool   `tfsdk:"ignored"`
}

type profileModel struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	Avatar   types.String `tfsdk:"avatar"`
	Variants types.List   `tfsdk:"variants"`
	Trackers types.List   `tfsdk:"trackers"`
	History  types.String `tfsdk:"history"`
	Hidden   types.Bool   `tfsdk:"hidden"`
	LockWO   types.String `tfsdk:"lock_wo"`
}

func (r *configurationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_configuration"
}

func (r *configurationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	providerData, ok := req.ProviderData.(*Provider)
	if !ok || providerData.client == nil {
		resp.Diagnostics.AddError("Invalid provider configuration", "The AIOStreams provider client was not configured.")
		return
	}
	r.client = providerData.client
	r.provider = providerData
}

func (r *configurationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages one complete AIOStreams UserData document. Unknown remote fields are preserved on updates.",
		Attributes: map[string]schema.Attribute{
			"uuid":                schema.StringAttribute{Computed: true, Description: "AIOStreams-assigned configuration UUID."},
			"password_wo":         schema.StringAttribute{Required: true, WriteOnly: true, Sensitive: true, Description: "Configuration password. Supplied on every operation and never persisted."},
			"password_wo_version": schema.Int64Attribute{Required: true, Description: "Change this value when rotating the write-only password input."},
			"configuration_json":  schema.StringAttribute{Optional: true, Description: "Non-secret UserData JSON overlay. Secret-shaped fields are rejected; use write-only secret inputs."},
			"secrets_json_wo":     schema.StringAttribute{Optional: true, WriteOnly: true, Sensitive: true, Description: "Write-only top-level UserData secret overlay, for example API keys."},
			"filters_json":        schema.StringAttribute{Optional: true, Description: "Non-secret UserData filter fields as a JSON object."},
			"preferences_json":    schema.StringAttribute{Optional: true, Description: "Non-secret UserData preference fields as a JSON object."},
			"preferred_languages": schema.ListAttribute{Optional: true, ElementType: types.StringType, Description: "Preferred audio languages, e.g. fr then en."},
			"preferred_subtitles": schema.ListAttribute{Optional: true, ElementType: types.StringType, Description: "Preferred subtitle languages, e.g. fr then en."},
			"services": schema.ListNestedAttribute{Optional: true, NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
				"id":                  schema.StringAttribute{Required: true},
				"enabled":             schema.BoolAttribute{Optional: true},
				"credentials_json_wo": schema.StringAttribute{Optional: true, WriteOnly: true, Sensitive: true, Description: "Write-only service credential map."},
			}}},
			"addons": schema.ListNestedAttribute{Optional: true, NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
				"type":            schema.StringAttribute{Required: true},
				"instance_id":     schema.StringAttribute{Required: true},
				"enabled":         schema.BoolAttribute{Required: true},
				"options_json":    schema.StringAttribute{Optional: true, Description: "Non-secret native preset options."},
				"options_json_wo": schema.StringAttribute{Optional: true, WriteOnly: true, Sensitive: true, Description: "Write-only native preset options containing secrets."},
				"category":        schema.StringAttribute{Optional: true},
			}}},
			"catalogs": schema.ListNestedAttribute{Optional: true, NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
				"id":               schema.StringAttribute{Required: true},
				"type":             schema.StringAttribute{Required: true},
				"name":             schema.StringAttribute{Optional: true},
				"enabled":          schema.BoolAttribute{Optional: true},
				"show_on_home":     schema.BoolAttribute{Optional: true},
				"only_on_discover": schema.BoolAttribute{Optional: true},
				"only_on_search":   schema.BoolAttribute{Optional: true},
				"disable_search":   schema.BoolAttribute{Optional: true},
				"addon_name":       schema.StringAttribute{Optional: true},
			}}},
			"templates": schema.ListNestedAttribute{Optional: true, NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
				"id":                schema.StringAttribute{Required: true},
				"version":           schema.StringAttribute{Required: true},
				"url":               schema.StringAttribute{Optional: true},
				"dismissed_version": schema.StringAttribute{Optional: true},
				"ignored":           schema.BoolAttribute{Optional: true},
			}}},
			"primary_profile":     schema.SingleNestedAttribute{Optional: true, Attributes: profileAttributes(false)},
			"personas":            schema.ListNestedAttribute{Optional: true, NestedObject: schema.NestedAttributeObject{Attributes: profileAttributes(true)}},
			"managed_fingerprint": schema.StringAttribute{Computed: true, Description: "Hash of provider-managed non-secret fields, used for drift detection."},
		},
	}
}

func profileAttributes(persona bool) map[string]schema.Attribute {
	attrs := map[string]schema.Attribute{
		"name":     schema.StringAttribute{Optional: true},
		"avatar":   schema.StringAttribute{Optional: true},
		"variants": schema.ListAttribute{Optional: true, ElementType: types.StringType},
		"trackers": schema.ListAttribute{Optional: true, ElementType: types.StringType},
		"hidden":   schema.BoolAttribute{Optional: true},
		"lock_wo":  schema.StringAttribute{Optional: true, WriteOnly: true, Sensitive: true},
	}
	if persona {
		attrs["id"] = schema.StringAttribute{Required: true}
		attrs["history"] = schema.StringAttribute{Optional: true}
	}
	return attrs
}

func (r *configurationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var config configurationModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Provider not configured", "The AIOStreams client is unavailable.")
		return
	}
	password, ok := writeOnlyString(config.PasswordWO, "password_wo", &resp.Diagnostics)
	if !ok {
		return
	}
	desired, err := desiredDocument(ctx, config, nil)
	if err != nil {
		resp.Diagnostics.AddError("Invalid AIOStreams configuration", err.Error())
		return
	}
	created, err := r.client.Create(ctx, password, desired)
	if err != nil {
		resp.Diagnostics.AddError("AIOStreams configuration creation failed", safeAPIError(err))
		return
	}
	config.UUID = types.StringValue(created.UUID)
	config.ManagedFingerprint = types.StringValue(managedFingerprintForConfig(desired, config))
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func (r *configurationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state configurationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	password, ok := r.readPassword(&resp.Diagnostics)
	if !ok {
		return
	}
	remote, err := r.client.Get(ctx, aiostreams.Credentials{UUID: state.UUID.ValueString(), Password: password})
	if err != nil {
		if aiostreams.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("AIOStreams configuration read failed", safeAPIError(err))
		return
	}
	if err := validateJSONObject(remote); err != nil {
		resp.Diagnostics.AddError("Invalid UserData returned by AIOStreams", err.Error())
		return
	}
	state.ManagedFingerprint = types.StringValue(managedFingerprintForConfig(remote, state))
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *configurationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config configurationModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state configurationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	password, ok := writeOnlyString(config.PasswordWO, "password_wo", &resp.Diagnostics)
	if !ok {
		return
	}
	remote, err := r.client.Get(ctx, aiostreams.Credentials{UUID: state.UUID.ValueString(), Password: password})
	if err != nil {
		resp.Diagnostics.AddError("AIOStreams configuration read before update failed", safeAPIError(err))
		return
	}
	desired, err := desiredDocument(ctx, config, remote)
	if err != nil {
		resp.Diagnostics.AddError("Invalid AIOStreams configuration", err.Error())
		return
	}
	if err := r.client.Update(ctx, aiostreams.Credentials{UUID: state.UUID.ValueString(), Password: password}, desired); err != nil {
		resp.Diagnostics.AddError("AIOStreams configuration update failed", safeAPIError(err))
		return
	}
	config.UUID = state.UUID
	config.ManagedFingerprint = types.StringValue(managedFingerprintForConfig(desired, config))
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func (r *configurationResource) Delete(_ context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.Diagnostics.AddError("Implicit AIOStreams deletion is disabled", "The provider never calls DELETE. Remove the resource only through a separately reviewed state transition and retain the remote configuration.")
}

func (r *configurationResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() || req.Plan.Raw.IsKnown() == false {
		return
	}
	var config configurationModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	desired, err := desiredDocument(ctx, config, nil)
	if err != nil {
		return
	}
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("managed_fingerprint"), types.StringValue(managedFingerprintForConfig(desired, config)))...)
}

func (r *configurationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("uuid"), req, resp)
	resp.Diagnostics.AddWarning("Imported configuration requires write-only credentials", "Set password_wo and password_wo_version before refresh or plan.")
}

func (r *configurationResource) readPassword(diags *diag.Diagnostics) (string, bool) {
	if r.provider == nil || r.provider.password == "" {
		diags.AddError("Missing AIOStreams read credential", "Set AIOSTREAMS_PASSWORD through the operator secret broker. Read cannot retrieve a write-only value from state.")
		return "", false
	}
	return r.provider.password, true
}

func writeOnlyString(value types.String, name string, diags *diag.Diagnostics) (string, bool) {
	if value.IsNull() || value.IsUnknown() || value.ValueString() == "" {
		diags.AddError("Missing write-only secret", fmt.Sprintf("%s must be configured for every operation.", name))
		return "", false
	}
	return value.ValueString(), true
}

func configOrStateUUID(config, state types.String) types.String {
	if !config.IsNull() && !config.IsUnknown() && config.ValueString() != "" {
		return config
	}
	return state
}

func safeAPIError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func validateJSONObject(raw json.RawMessage) error {
	var value map[string]json.RawMessage
	if !json.Valid(raw) || json.Unmarshal(raw, &value) != nil || value == nil {
		return errors.New("expected a JSON object")
	}
	return nil
}

func managedFingerprint(raw json.RawMessage) string {
	return managedFingerprintWithExtraKeys(raw, nil)
}

func managedFingerprintForConfig(raw json.RawMessage, config configurationModel) string {
	extraKeys := map[string]struct{}{}
	for _, source := range []types.String{config.ConfigurationJSON, config.FiltersJSON, config.PreferencesJSON} {
		if source.IsNull() || source.IsUnknown() || strings.TrimSpace(source.ValueString()) == "" {
			continue
		}
		var overlay map[string]json.RawMessage
		if json.Unmarshal([]byte(source.ValueString()), &overlay) == nil {
			for key := range overlay {
				if !secretKey(key) {
					extraKeys[key] = struct{}{}
				}
			}
		}
	}
	return managedFingerprintWithExtraKeys(raw, extraKeys)
}

func managedFingerprintWithExtraKeys(raw json.RawMessage, extraKeys map[string]struct{}) string {
	var value map[string]any
	if json.Unmarshal(raw, &value) != nil || value == nil {
		return "invalid"
	}
	value = managedProjection(value)
	if len(extraKeys) > 0 {
		var original map[string]any
		if json.Unmarshal(raw, &original) == nil {
			for key := range extraKeys {
				if child, ok := original[key]; ok {
					value[key] = child
				}
			}
		}
	}
	// These fields are injected or request-scoped in the AIOStreams API and are
	// not part of the provider-owned desired configuration.
	for _, key := range []string{"uuid", "encryptedPassword", "trusted", "ip", "activeVariants", "autoVariants", "healthResults", "variantSelectorLocation", "checkOwned", "proxy"} {
		delete(value, key)
	}
	// AIOStreams v2.37.0 applies this instance default at validation time and
	// omits it from the persisted document. Treat the explicit true value as
	// equivalent so a successful apply is followed by a no-op refresh.
	if jellyfin, ok := value["jellyfin"].(map[string]any); ok && jellyfin["enabled"] == true {
		delete(jellyfin, "enabled")
	}
	redacted := redactSecrets(value)
	canonical, _ := json.Marshal(redacted)
	hash := sha256.Sum256(canonical)
	return hex.EncodeToString(hash[:])
}

// managedProjection excludes fields introduced by newer AIOStreams versions
// that this provider does not own. They remain in the document sent during an
// update, but must not create a perpetual diff merely because the provider has
// not learned their schema yet.
func managedProjection(value map[string]any) map[string]any {
	const managedKeys = "formatter sortCriteria services presets catalogModifications appliedTemplates preferredLanguages preferredSubtitles excludedResolutions includedResolutions requiredResolutions preferredResolutions excludedQualities includedQualities requiredQualities preferredQualities excludedLanguages includedLanguages requiredLanguages excludedSubtitles includedSubtitles requiredSubtitles excludedVisualTags includedVisualTags requiredVisualTags excludedAudioTags includedAudioTags requiredAudioTags excludedAudioChannels includedAudioChannels requiredAudioChannels excludedStreamTypes includedStreamTypes requiredStreamTypes excludedStreamTypes addonCategoryColors newCatalogsDisabled upstreamCatalogOrder mergedCatalogs externalDownloads cacheAndPlay autoRemoveDownloads failover serviceWrap remuxDb"
	projected := make(map[string]any)
	for _, key := range strings.Fields(managedKeys) {
		if child, ok := value[key]; ok {
			projected[key] = child
		}
	}
	if jellyfin, ok := value["jellyfin"].(map[string]any); ok {
		profiled := make(map[string]any)
		for _, key := range []string{"primary", "personas"} {
			if child, ok := jellyfin[key]; ok {
				profiled[key] = child
			}
		}
		if len(profiled) > 0 {
			projected["jellyfin"] = profiled
		}
	}
	return projected
}

func redactSecrets(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if secretKey(key) {
				out[key] = "<redacted>"
				continue
			}
			out[key] = redactSecrets(typed[key])
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i := range typed {
			out[i] = redactSecrets(typed[i])
		}
		return out
	default:
		return value
	}
}

func secretKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", ""))
	for _, fragment := range []string{"password", "apikey", "token", "secret", "credential", "auth", "privatekey", "lock"} {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return normalized == "credentials" || normalized == "headers"
}
