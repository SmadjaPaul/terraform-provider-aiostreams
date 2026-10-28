package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func desiredDocument(ctx context.Context, config configurationModel, remote json.RawMessage) (json.RawMessage, error) {
	base := map[string]any{}
	if len(remote) > 0 {
		if err := json.Unmarshal(remote, &base); err != nil || base == nil {
			return nil, errors.New("remote UserData must be a JSON object")
		}
	}

	for name, value := range map[string]types.String{
		"configuration_json": config.ConfigurationJSON,
		"filters_json":       config.FiltersJSON,
		"preferences_json":   config.PreferencesJSON,
	} {
		if value.IsNull() || value.IsUnknown() || strings.TrimSpace(value.ValueString()) == "" {
			continue
		}
		overlay, err := objectString(value.ValueString(), name, true)
		if err != nil {
			return nil, err
		}
		mergeObject(base, overlay)
		if name == "configuration_json" {
			clearNativeCatalogBoundary(base, overlay)
		}
	}

	if values, present, err := stringList(ctx, config.PreferredLanguages); err != nil {
		return nil, err
	} else if present {
		base["preferredLanguages"] = values
	}
	if values, present, err := stringList(ctx, config.PreferredSubtitles); err != nil {
		return nil, err
	} else if present {
		base["preferredSubtitles"] = values
	}

	if !config.SecretsJSONWO.IsNull() && !config.SecretsJSONWO.IsUnknown() && strings.TrimSpace(config.SecretsJSONWO.ValueString()) != "" {
		overlay, err := objectString(config.SecretsJSONWO.ValueString(), "secrets_json_wo", false)
		if err != nil {
			return nil, err
		}
		mergeObject(base, overlay)
	}

	if present := !config.Services.IsNull() && !config.Services.IsUnknown(); present {
		var models []serviceModel
		if diags := config.Services.ElementsAs(ctx, &models, false); diags.HasError() {
			return nil, errors.New("invalid services")
		}
		items := []any{}
		for _, model := range models {
			item := map[string]any{"id": requiredString(model.ID, "services.id")}
			if !model.Enabled.IsNull() && !model.Enabled.IsUnknown() {
				item["enabled"] = model.Enabled.ValueBool()
			}
			credentials, err := objectStringOptional(model.Credentials, "services.credentials_json_wo")
			if err != nil {
				return nil, err
			}
			if credentials != nil {
				item["credentials"] = credentials
			} else if existing := findObjectByString(base["services"], "id", model.ID.ValueString()); existing != nil {
				item["credentials"] = existing["credentials"]
			} else {
				item["credentials"] = map[string]any{}
			}
			if existing := findObjectByString(base["services"], "id", model.ID.ValueString()); existing != nil {
				merged := cloneObject(existing)
				mergeObject(merged, item)
				item = merged
			}
			items = append(items, item)
		}
		base["services"] = items
	}

	if present := !config.Addons.IsNull() && !config.Addons.IsUnknown(); present {
		var models []addonModel
		if diags := config.Addons.ElementsAs(ctx, &models, false); diags.HasError() {
			return nil, errors.New("invalid addons")
		}
		items := []any{}
		for _, model := range models {
			instanceID := requiredString(model.InstanceID, "addons.instance_id")
			typeName := requiredString(model.Type, "addons.type")
			item := map[string]any{"type": typeName, "instanceId": instanceID, "enabled": model.Enabled.ValueBool()}
			if !model.Category.IsNull() && !model.Category.IsUnknown() {
				item["category"] = model.Category.ValueString()
			}
			if existing := findObjectByString(base["presets"], "instanceId", instanceID); existing != nil {
				item = cloneObject(existing)
				item["type"], item["instanceId"], item["enabled"] = typeName, instanceID, model.Enabled.ValueBool()
				if !model.Category.IsNull() && !model.Category.IsUnknown() {
					item["category"] = model.Category.ValueString()
				}
			}
			options := map[string]any{}
			if existing, ok := item["options"].(map[string]any); ok {
				options = cloneObject(existing)
			}
			if !model.Options.IsNull() && !model.Options.IsUnknown() && strings.TrimSpace(model.Options.ValueString()) != "" {
				overlay, err := objectString(model.Options.ValueString(), "addons.options_json", true)
				if err != nil {
					return nil, err
				}
				mergeObject(options, overlay)
			}
			if !model.OptionsWO.IsNull() && !model.OptionsWO.IsUnknown() && strings.TrimSpace(model.OptionsWO.ValueString()) != "" {
				overlay, err := objectString(model.OptionsWO.ValueString(), "addons.options_json_wo", false)
				if err != nil {
					return nil, err
				}
				mergeObject(options, overlay)
			}
			item["options"] = options
			items = append(items, item)
		}
		base["presets"] = items
	}

	if present := !config.Catalogs.IsNull() && !config.Catalogs.IsUnknown(); present {
		var models []catalogModel
		if diags := config.Catalogs.ElementsAs(ctx, &models, false); diags.HasError() {
			return nil, errors.New("invalid catalogs")
		}
		items := make([]any, 0, len(models))
		for _, model := range models {
			item := map[string]any{"id": requiredString(model.ID, "catalogs.id"), "type": requiredString(model.Type, "catalogs.type")}
			for key, value := range map[string]types.String{"name": model.Name, "addonName": model.AddonName} {
				if !value.IsNull() && !value.IsUnknown() {
					item[key] = value.ValueString()
				}
			}
			for key, value := range map[string]types.Bool{"enabled": model.Enabled, "showOnHome": model.ShowOnHome, "onlyOnDiscover": model.OnlyOnDiscover, "onlyOnSearch": model.OnlyOnSearch, "disableSearch": model.DisableSearch} {
				if !value.IsNull() && !value.IsUnknown() {
					item[key] = value.ValueBool()
				}
			}
			items = append(items, item)
		}
		base["catalogModifications"] = items
	}

	if present := !config.Templates.IsNull() && !config.Templates.IsUnknown(); present {
		var models []templateModel
		if diags := config.Templates.ElementsAs(ctx, &models, false); diags.HasError() {
			return nil, errors.New("invalid templates")
		}
		items := make([]any, 0, len(models))
		for _, model := range models {
			item := map[string]any{"id": requiredString(model.ID, "templates.id"), "version": requiredString(model.Version, "templates.version")}
			for key, value := range map[string]types.String{"url": model.URL, "dismissedVersion": model.DismissedVersion} {
				if !value.IsNull() && !value.IsUnknown() {
					item[key] = value.ValueString()
				}
			}
			if !model.Ignored.IsNull() && !model.Ignored.IsUnknown() {
				item["ignored"] = model.Ignored.ValueBool()
			}
			items = append(items, item)
		}
		base["appliedTemplates"] = items
	}

	if !config.Primary.IsNull() && !config.Primary.IsUnknown() {
		model, err := profileFromObject(config.Primary)
		if err != nil {
			return nil, err
		}
		jellyfin := ensureObject(base, "jellyfin")
		primary := ensureObject(jellyfin, "primary")
		applyProfile(primary, model, false, ctx)
		jellyfin["primary"] = primary
		base["jellyfin"] = jellyfin
	}

	if present := !config.Personas.IsNull() && !config.Personas.IsUnknown(); present {
		var models []profileModel
		if diags := config.Personas.ElementsAs(ctx, &models, false); diags.HasError() {
			return nil, errors.New("invalid personas")
		}
		jellyfin := ensureObject(base, "jellyfin")
		items := make([]any, 0, len(models))
		for _, model := range models {
			item := map[string]any{"id": requiredString(model.ID, "personas.id"), "name": requiredString(model.Name, "personas.name")}
			if existing := findObjectByString(jellyfin["personas"], "id", model.ID.ValueString()); existing != nil {
				item = cloneObject(existing)
			}
			applyProfile(item, model, true, ctx)
			items = append(items, item)
		}
		jellyfin["personas"] = items
		base["jellyfin"] = jellyfin
	}

	result, err := json.Marshal(base)
	if err != nil {
		return nil, fmt.Errorf("marshal UserData: %w", err)
	}
	return result, nil
}

// A native family document owns its preset list. When that document does not
// declare catalogModifications, an older HCL catalogs overlay must not survive
// the transition as a stale, provider-managed field. Other remote fields stay
// untouched so unknown UserData remains losslessly preserved.
func clearNativeCatalogBoundary(base, overlay map[string]any) {
	if _, hasPresets := overlay["presets"]; !hasPresets {
		return
	}
	if _, hasCatalogModifications := overlay["catalogModifications"]; !hasCatalogModifications {
		delete(base, "catalogModifications")
	}
}

func profileFromObject(value types.Object) (profileModel, error) {
	attrs := value.Attributes()
	getString := func(name string) types.String {
		if item, ok := attrs[name].(types.String); ok {
			return item
		}
		return types.StringNull()
	}
	getBool := func(name string) types.Bool {
		if item, ok := attrs[name].(types.Bool); ok {
			return item
		}
		return types.BoolNull()
	}
	getList := func(name string) types.List {
		if item, ok := attrs[name].(types.List); ok {
			return item
		}
		return types.ListNull(types.StringType)
	}
	return profileModel{
		ID:       getString("id"),
		Name:     getString("name"),
		Avatar:   getString("avatar"),
		Variants: getList("variants"),
		Trackers: getList("trackers"),
		History:  getString("history"),
		Hidden:   getBool("hidden"),
		LockWO:   getString("lock_wo"),
	}, nil
}

func applyProfile(target map[string]any, model profileModel, persona bool, ctx context.Context) {
	for key, value := range map[string]types.String{"name": model.Name, "avatar": model.Avatar, "history": model.History} {
		if !value.IsNull() && !value.IsUnknown() && (!persona || key != "history" || value.ValueString() != "") {
			target[key] = value.ValueString()
		}
	}
	for key, value := range map[string]types.Bool{"hidden": model.Hidden} {
		if !value.IsNull() && !value.IsUnknown() {
			target[key] = value.ValueBool()
		}
	}
	for key, value := range map[string]types.List{"variants": model.Variants, "trackers": model.Trackers} {
		if values, present, err := stringList(ctx, value); err == nil && present {
			target[key] = values
		}
	}
	if !model.LockWO.IsNull() && !model.LockWO.IsUnknown() && model.LockWO.ValueString() != "" {
		target["lock"] = model.LockWO.ValueString()
	}
}

func objectString(value, name string, rejectSecrets bool) (map[string]any, error) {
	var object map[string]any
	if err := json.Unmarshal([]byte(value), &object); err != nil || object == nil {
		return nil, fmt.Errorf("%s must be a JSON object", name)
	}
	if rejectSecrets && containsSecretKey(object) {
		return nil, fmt.Errorf("%s contains a secret-shaped field; use a write-only attribute", name)
	}
	return object, nil
}

func objectStringOptional(value types.String, name string) (map[string]any, error) {
	if value.IsNull() || value.IsUnknown() || strings.TrimSpace(value.ValueString()) == "" {
		return nil, nil
	}
	return objectString(value.ValueString(), name, false)
}

func containsSecretKey(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if secretKey(key) || containsSecretKey(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsSecretKey(child) {
				return true
			}
		}
	}
	return false
}

func mergeObject(target, overlay map[string]any) {
	for key, value := range overlay {
		if nested, ok := value.(map[string]any); ok {
			if existing, ok := target[key].(map[string]any); ok {
				mergeObject(existing, nested)
				target[key] = existing
			} else {
				target[key] = cloneObject(nested)
			}
			continue
		}
		target[key] = value
	}
}

func cloneObject(value map[string]any) map[string]any {
	out := make(map[string]any, len(value))
	for key, child := range value {
		if nested, ok := child.(map[string]any); ok {
			out[key] = cloneObject(nested)
		} else if list, ok := child.([]any); ok {
			out[key] = append([]any(nil), list...)
		} else {
			out[key] = child
		}
	}
	return out
}

func ensureObject(parent map[string]any, key string) map[string]any {
	if value, ok := parent[key].(map[string]any); ok {
		return value
	}
	value := map[string]any{}
	parent[key] = value
	return value
}

func findObjectByString(value any, key, expected string) map[string]any {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	for _, item := range items {
		object, ok := item.(map[string]any)
		if ok && fmt.Sprint(object[key]) == expected {
			return object
		}
	}
	return nil
}

func requiredString(value types.String, name string) string {
	if value.IsNull() || value.IsUnknown() || strings.TrimSpace(value.ValueString()) == "" {
		return "<missing>"
	}
	return value.ValueString()
}

func stringList(ctx context.Context, value types.List) ([]string, bool, error) {
	if value.IsNull() || value.IsUnknown() {
		return nil, false, nil
	}
	var values []types.String
	if diags := value.ElementsAs(ctx, &values, false); diags.HasError() {
		return nil, false, errors.New("string list contains an invalid value")
	}
	result := make([]string, 0, len(values))
	for _, item := range values {
		if item.IsNull() || item.IsUnknown() {
			return nil, false, errors.New("string lists cannot contain null or unknown values")
		}
		result = append(result, item.ValueString())
	}
	return result, true, nil
}
