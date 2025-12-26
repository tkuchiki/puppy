package cli

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
)

type MonitorCmd struct {
	PageSize int32 `short:"p" help:"Maximum number of results to return"`
}

func (m *MonitorCmd) Run(cli *CLI) error {
	pageSize := m.PageSize

	ctx := newContext(cli.ApiKey, cli.AppKey)

	monitorsApi := datadogV1.NewMonitorsApi(cli.apiClient)

	resp, _ := monitorsApi.ListMonitorsWithPagination(ctx, *datadogV1.NewListMonitorsOptionalParameters().WithPageSize(pageSize))

	for paginationResult := range resp {
		if paginationResult.Error != nil {
			return fmt.Errorf("Error when calling `MonitorsApi.ListMonitors`: %v\n", paginationResult.Error)
		}
		responseContent, _ := json.MarshalIndent(paginationResult.Item, "", "  ")
		fmt.Println(string(responseContent))
	}

	return nil
}
