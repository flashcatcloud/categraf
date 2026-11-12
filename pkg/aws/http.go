package aws

import (
	"net"

	"flashcat.cloud/categraf/pkg/netx"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	configV2 "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/endpointcreds"
)

// HTTPClientOptionsFromEnv configures the root before credentials are created.
// The SDK retains a BuildableClient, so custom CAs, smart defaults and metadata
// timeouts still apply. Its finalizer survives every SDK client clone.
func HTTPClientOptionsFromEnv() []func(*configV2.LoadOptions) error {
	if !netx.PreferIPv4() {
		return nil
	}
	client := awshttp.NewBuildableClient().WithTransportConfigurer(netx.HTTPTransportConfigurerFromEnv())
	return []func(*configV2.LoadOptions) error{
		configV2.WithHTTPClient(client),
		// Endpoint credentials construct an independent HTTP provider; neither its
		// preflight hostname lookup nor its HTTP client inherits cfg.HTTPClient.
		configV2.WithEndpointCredentialLookupHost(netx.LookupHostIPv4(net.DefaultResolver)),
		configV2.WithEndpointCredentialOptions(func(options *endpointcreds.Options) { options.HTTPClient = client }),
	}
}
