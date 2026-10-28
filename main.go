package main

import (
	"context"
	"log"

	"github.com/SmadjaPaul/terraform-provider-aiostreams/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

var version = "dev"

func main() {
	if err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/smadjapaul/aiostreams",
	}); err != nil {
		log.Fatal(err)
	}
}
