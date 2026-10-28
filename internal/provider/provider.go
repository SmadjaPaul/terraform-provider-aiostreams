package provider

import (
	"context"
	"os"
	"strings"

	"github.com/SmadjaPaul/terraform-provider-aiostreams/internal/aiostreams"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	providerschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ provider.Provider = (*Provider)(nil)

type Provider struct {
	version  string
	client   *aiostreams.Client
	password string
}

type providerModel struct {
	Endpoint types.String `tfsdk:"endpoint"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider { return &Provider{version: version} }
}

func (p *Provider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "aiostreams"
	resp.Version = p.version
}

func (p *Provider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = providerschema.Schema{
		Attributes: map[string]providerschema.Attribute{
			"endpoint": providerschema.StringAttribute{
				Optional:    true,
				Description: "AIOStreams HTTPS origin. HTTP is accepted only for loopback qualification.",
			},
		},
	}
}

func (p *Provider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpoint := data.Endpoint.ValueString()
	if data.Endpoint.IsNull() || data.Endpoint.IsUnknown() || endpoint == "" {
		endpoint = os.Getenv("AIOSTREAMS_ENDPOINT")
	}
	if endpoint == "" {
		resp.Diagnostics.AddError("Missing AIOStreams endpoint", "Set provider endpoint or AIOSTREAMS_ENDPOINT.")
		return
	}
	client, err := aiostreams.New(endpoint)
	if err != nil {
		resp.Diagnostics.AddError("Invalid AIOStreams endpoint", err.Error())
		return
	}
	if operatorAuth := os.Getenv("AIOSTREAMS_OPERATOR_AUTH"); operatorAuth != "" {
		username, password, ok := strings.Cut(operatorAuth, ":")
		if !ok {
			resp.Diagnostics.AddError("Invalid AIOStreams operator credentials", "AIOSTREAMS_OPERATOR_AUTH must be a username:password pair.")
			return
		}
		if err := client.SetOperatorAuth(username, password); err != nil {
			resp.Diagnostics.AddError("Invalid AIOStreams operator credentials", err.Error())
			return
		}
	}
	p.client = client
	p.password = os.Getenv("AIOSTREAMS_PASSWORD")
	resp.DataSourceData = p
	resp.ResourceData = p
}

func (p *Provider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		func() resource.Resource { return NewConfigurationResource() },
	}
}

func (p *Provider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}
