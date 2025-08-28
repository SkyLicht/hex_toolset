package managers

import (
	"context"
	skylogger "hex_toolset/pkg/logger"
	"hex_toolset/pkg/sfc_api"
	"time"
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

func (m *SFCAPIManager) GetCurrentMinute(minute time.Time) {
	// You can use the minute argument to request the exact window you need.
	// For now, this is a placeholder where you'd call your client with the minute.
	// Example:
	// date := minute.Format("02-Jan-2006")
	// hour := minute.Hour()
	// min := minute.Minute()
	// recs, err := m.client.RequestMinuteData(m.ctx, date, hour, min)
	// handle recs/err...

	recs, err := m.client.RequestMinute(m.ctx, minute)
	if err != nil {
		m.logger.Errorf("Error requesting minute data: %v", err)
	}

	if len(recs) != 1 {
		m.logger.Errorf("Unexpected number of records: %d", len(recs))
	}

	//PrintRecordDataCollectors

	m.logger.Warnf("Heehhe %s", minute)
	m.logger.Warnf("mot %s", minute)
}
