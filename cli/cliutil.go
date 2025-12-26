package cli

import (
	"context"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
)

func newContext(apiKey, appKey string) context.Context {
	return context.WithValue(
		context.Background(),
		datadog.ContextAPIKeys,
		map[string]datadog.APIKey{
			"apiKeyAuth": {
				Key: apiKey,
			},
			"appKeyAuth": {
				Key: appKey,
			},
		},
	)
}
