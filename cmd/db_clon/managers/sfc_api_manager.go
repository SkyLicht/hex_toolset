package managers

import (
	"context"
	skylogger "hex_toolset/pkg/logger"
	"hex_toolset/pkg/sfc_api"
)

type SFCAPIManager struct {
	ctx    context.Context
	client *sfc_api.APIClient
	logger *skylogger.Logger
}

func NewSFCAPIManager(
	ctx *context.Context,
) *SFCAPIManager {

	// Initialize custom logger named "loop_manager" and use a stable file name
	lgr, err := skylogger.New(
		skylogger.WithName("loop_manager"),
		skylogger.WithFilePattern("{name}.log"),
	)

	if err != nil {
		return nil
	}

	return &SFCAPIManager{
		client: sfc_api.NewAPIClient(),
		ctx:    *ctx,
		logger: lgr,
	}
}

func (m *SFCAPIManager) GetCurrentMinuteData() {

	recs, err := m.client.RequestPreviousMinute(m.ctx)
	if err != nil {

	}
	if len(recs) != 1 {

	}

	m.logger.Warnf("Heehhe %s", "dddd")
	m.logger.Warnf("Heehhe %s", "dddd")

}
