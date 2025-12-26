package cli

import (
	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/alecthomas/kong"
)

type CLI struct {
	ApiKey      string `help:"Datadog API key" env:"DD_CLIENT_API_KEY"`
	AppKey      string `help:"Datadog Application key" env:"DD_CLIENT_APP_KEY"`
	EnableRetry bool   `help:"Enable retry mode" default:"true"`
	MaxRetries  int    `help:"Maximum number of retries to perform" default:"3"`

	Monitor MonitorCmd `cmd:"monitor" help:"Monitor commands"`
	Service ServiceCmd `cmd:"service" help:"Service commands"`

	apiClient *datadog.APIClient
}

func (c *CLI) SetAPIClient(apiClient *datadog.APIClient) {
	c.apiClient = apiClient
}

func NewCLI() (*CLI, *kong.Context) {
	cli := CLI{}

	ctx := kong.Parse(&cli,
		kong.Name("puppy"),
		kong.Description("Lightweight Datadog CLI client"),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
			Summary: true,
		}),
	)

	configuration := datadog.NewConfiguration()
	configuration.RetryConfiguration.EnableRetry = cli.EnableRetry
	configuration.RetryConfiguration.MaxRetries = cli.MaxRetries

	cli.SetAPIClient(datadog.NewAPIClient(configuration))

	return &cli, ctx
}
