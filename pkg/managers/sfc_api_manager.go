package managers

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	pkgcfg "hex_toolset/pkg"
	"hex_toolset/pkg/db"
	"hex_toolset/pkg/db/entities"
	skylogger "hex_toolset/pkg/logger"
	"hex_toolset/pkg/sfc_api"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

type SFCAPIManager struct {
	ctx          context.Context
	client       *sfc_api.APIClient
	logger       *skylogger.Logger
	recordEntity *entities.RecordEntityManager
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

	record := entities.NewRecordManagerEntity(db.GetDB())

	return &SFCAPIManager{
		client:       sfc_api.NewAPIClient(),
		ctx:          *ctx,
		logger:       lgr,
		recordEntity: record,
	}
}

func (m *SFCAPIManager) UpdateLostMinutes() {
	cfg := pkgcfg.GetConfig()
	statusDir := strings.TrimSpace(cfg.SFC_DB_STATUS)
	if statusDir == "" {
		m.logger.Warnf("SFC_DB_STATUS not set; skipping UpdateLostMinutes")
		return
	}
	statusFile := filepath.Join(statusDir, "erro_minute_sync")
	f, err := os.Open(statusFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return
		}
		m.logger.Errorf("failed to open status file: %v", err)
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var remaining []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// parse time (support new "2006-01-02 15:04:05 -0700 MST" and legacy RFC3339)
		const layoutNew = "2006-01-02 15:04:05 -0700 MST"
		var min time.Time
		if t, err := time.Parse(layoutNew, line); err == nil {
			min = t
		} else if t2, err2 := time.Parse(time.RFC3339, line); err2 == nil {
			min = t2
		} else {
			m.logger.Warnf("invalid time format in status file: %s", line)
			remaining = append(remaining, line)
			continue
		}
		recs, rerr := m.client.RequestMinute(m.ctx, min)
		if rerr != nil {
			m.logger.Errorf("retry minute failed %s: %v", min, rerr)
			remaining = append(remaining, line)
			continue
		}
		// Action not defined yet; per log. We consider successful if no error returned.
		m.logger.Infof("retry minute succeeded %s, records: %d", min.Format(layoutNew), len(recs))
	}
	if serr := scanner.Err(); serr != nil {
		m.logger.Errorf("scanner error reading status file: %v", serr)
		return
	}

	// If remaining is empty, delete the file; else write back remaining
	if len(remaining) == 0 {
		if derr := os.Remove(statusFile); derr != nil && !errors.Is(derr, os.ErrNotExist) {
			m.logger.Errorf("failed to delete status file: %v", derr)
		}
		return
	}

	tmpFile := statusFile + ".tmp"
	wf, werr := os.OpenFile(tmpFile, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if werr != nil {
		m.logger.Errorf("failed to open temp file: %v", werr)
		return
	}
	for _, ln := range remaining {
		_, _ = wf.WriteString(ln + "\n")
	}
	_ = wf.Close()
	if rerr := os.Rename(tmpFile, statusFile); rerr != nil {
		m.logger.Errorf("failed to replace status file: %v", rerr)
	}
}

func (m *SFCAPIManager) persistFailedMinute(minute time.Time) {
	cfg := pkgcfg.GetConfig()
	statusDir := strings.TrimSpace(cfg.SFC_DB_STATUS)
	if statusDir == "" {
		m.logger.Errorf("SFC_DB_STATUS not set; cannot persist failed minute")
		return
	}
	if err := os.MkdirAll(statusDir, 0755); err != nil {
		m.logger.Errorf("failed to ensure status directory: %v", err)
		return
	}
	statusFile := filepath.Join(statusDir, "erro_minute_sync")
	f, err := os.OpenFile(statusFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		m.logger.Errorf("failed to open status file for append: %v", err)
		return
	}
	defer f.Close()
	_, werr := f.WriteString(minute.In(time.Local).Format("2006-01-02 15:04:05 -0700 MST") + "\n")
	if werr != nil {
		m.logger.Errorf("failed to write to status file: %v", werr)
	}
}

func (m *SFCAPIManager) RequestMinute(time time.Time) {
	// You can use the minute argument to request the exact window you need.
	// For now, this is a placeholder where you'd call your client with the minute.
	// Example:
	// date := minute.Format("02-Jan-2006")
	// hour := minute.Hour()
	// min := minute.Minute()
	// recs, err := m.client.RequestMinuteData(m.ctx, date, hour, min)
	// handle recs/err...
	fmt.Printf("Requesting minute %s\n", time)

	recs, err := m.client.RequestMinute(m.ctx, time)
	if err != nil {
		m.logger.Errorf("Error requesting minute data: %v", err)
		// error requesting minute data
		m.persistFailedMinute(time)
		return
	}

	if len(recs) == 0 {
		m.logger.Warnf("No records found for minute %s", time)
		return
	}

	// Insert records into the minute

	mapRecords, err := recordModelToEntity(recs)

	if err != nil {
		m.logger.Errorf("Error converting records to entities: %v", err)
		return
	}
	err = m.recordEntity.InsertBatch(mapRecords)
	if err != nil {
		m.logger.Errorf("Error inserting records: %v", err)
		// error inserting records
		m.persistFailedMinute(time)
		return
	}

	// Create a Broadcast file for the minute data

	// successfully got records

}

func (m *SFCAPIManager) RequestHour(t time.Time) {

	// time gets the previous hour
	previousHour := t.Add(-1 * time.Hour)

	fmt.Printf("Requesting hour %s\n", previousHour)

	recs, err := m.client.RequestHour(m.ctx, previousHour)
	if err != nil {
		m.logger.Errorf("")
		return
	}

	if len(recs) == 0 {
		m.logger.Warnf("No records found for hour %s", previousHour)
		return
	}

	// Delete records from the hour

	previousHourDB := previousHour.Format("02-Jan-2006 15:04:05")
	currentHourDB := t.Format("02-Jan-2006 15:04:05")

	err = m.recordEntity.DeleteRecordRange(previousHourDB, currentHourDB)
	if err != nil {

		m.logger.Errorf("Error deleting records: %v", err)
		return
	}

	hour, err := m.client.RequestHour(m.ctx, previousHour)
	if err != nil {
		return
	}

	mapRecords, err := recordModelToEntity(hour)

	if err != nil {
		m.logger.Errorf("Error converting records to entities: %v", err)
		return
	}
	err = m.recordEntity.InsertBatch(mapRecords)

	// successfully got records

}

func recordModelToEntity(data []sfc_api.RecordDataCollector) ([]entities.RecordEntity, error) {
	var result []entities.RecordEntity
	for _, r := range data {
		entity := entities.RecordEntity{
			ID:           uuid.New().String(),
			PPID:         r.SerialNumber,
			WorkOrder:    r.MoNumber,
			EmployeeName: r.EmpNo,
			GroupName:    r.GroupName,
			LineName:     r.LineName,
			StationName:  r.StationName,
			ModelName:    r.ModelName,
			ErrorFlag:    parseErrorFlag(r.ErrorFlag),
			NextStation:  r.NextStations,
		}

		// Try InStationTime then InLineTime; fallback to current time if all fail
		ts, err := sfc_api.ParseAPITimestamp(r.InStationTime)
		if err != nil && r.InLineTime != "" {
			ts, err = sfc_api.ParseAPITimestamp(r.InLineTime)
		}
		if err != nil {
			entity.CollectedTimestamp = time.Now()
		} else {
			entity.CollectedTimestamp = ts
		}

		result = append(result, entity)
	}
	return result, nil
}

func (m *SFCAPIManager) LoadDay(ctx context.Context, date string) error {
	// Parse input date as local time zone, hour-beginning will be 00:00 .. 23:00
	day, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(date), time.Local)
	if err != nil {
		return fmt.Errorf("invalid date %q, expected YYYY-MM-DD: %w", date, err)
	}

	startOfDay := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.Local)
	var failed int
	for h := 0; h < 24; h++ {
		select {
		case <-ctx.Done():
			m.logger.Warnf("LoadDay canceled for %s: %v", date, ctx.Err())
			if failed > 0 {
				return fmt.Errorf("canceled after %d hour(s) failed: %w", failed, ctx.Err())
			}
			return ctx.Err()
		default:
		}

		hourStart := startOfDay.Add(time.Duration(h) * time.Hour)

		// 1) Fetch hour data
		recs, rerr := m.client.RequestHour(ctx, hourStart)
		if rerr != nil {
			m.logger.Errorf("RequestHour failed for %s %02d:00: %v", date, h, rerr)
			failed++
			continue
		}
		if len(recs) == 0 {
			m.logger.Warnf("No records for %s %02d:00", date, h)
			continue
		}

		// Delete records from the hour
		hourStartDB := hourStart.Format("2006-01-02 15:04:05")
		hourEndDB := hourStart.Add(time.Hour).Format("2006-01-02 15:04:05")

		err = m.recordEntity.DeleteRecordRange(hourStartDB, hourEndDB)
		if err != nil {
			m.logger.Errorf("DeleteRecordRange failed for %s %02d:00: %v", date, h, err)
			failed++
			continue
		}
		// 2) Map to entities
		mapRecords, merr := recordModelToEntity(recs)
		if merr != nil {
			m.logger.Errorf("Mapping records failed for %s %02d:00: %v", date, h, merr)
			failed++
			continue
		}

		// 3) Persist
		if ierr := m.recordEntity.InsertBatch(mapRecords); ierr != nil {
			m.logger.Errorf("InsertBatch failed for %s %02d:00: %v", date, h, ierr)
			failed++
			continue
		}

		m.logger.Infof("Loaded %d records for %s %02d:00", len(mapRecords), date, h)
	}

	if failed > 0 {
		return fmt.Errorf("completed with %d hour(s) failed for %s", failed, date)
	}
	return nil
}

func (m *SFCAPIManager) LoadRangeOfDays(ctx context.Context, start string, finish string) error {
	startDay, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(start), time.Local)
	if err != nil {
		return fmt.Errorf("invalid start date %q, expected YYYY-MM-DD: %w", start, err)
	}
	endDay, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(finish), time.Local)
	if err != nil {
		return fmt.Errorf("invalid finish date %q, expected YYYY-MM-DD: %w", finish, err)
	}
	if endDay.Before(startDay) {
		return fmt.Errorf("finish date %s is before start date %s", finish, start)
	}

	var failed int
	for d := startDay; !d.After(endDay); d = d.AddDate(0, 0, 1) {
		if err := m.LoadDay(ctx, d.Format("2006-01-02")); err != nil {
			m.logger.Errorf("LoadDay error for %s: %v", d.Format("2006-01-02"), err)
			failed++
			// continue to next day, aggregating failures
		}
	}
	if failed > 0 {
		return fmt.Errorf("range load completed with %d day(s) failed", failed)
	}
	return nil
}

// LoadHour loads a single hour given "YYYY-MM-DD HH" (e.g., "2025-08-29 15").
func (m *SFCAPIManager) LoadHour(dateHour string) error {
	s := strings.TrimSpace(dateHour)
	if s == "" {
		return fmt.Errorf("dateHour is required in format YYYY-MM-DD HH")
	}
	t, err := time.ParseInLocation("2006-01-02 15", s, time.Local)
	if err != nil {
		return fmt.Errorf("invalid dateHour %q, expected YYYY-MM-DD HH: %w", s, err)
	}
	hourStart := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, time.Local)

	// Fetch hour data
	recs, rerr := m.client.RequestHour(m.ctx, hourStart)
	if rerr != nil {
		m.logger.Errorf("RequestHour failed for %s: %v", s, rerr)
		return rerr
	}
	if len(recs) == 0 {
		m.logger.Warnf("No records for %s", s)
		// still clear DB range to avoid stale data
	}

	// Delete records for that hour using "YYYY-MM-DD HH:MM:SS"
	startStr := hourStart.Format("2006-01-02 15:04:05")
	endStr := hourStart.Add(time.Hour).Format("2006-01-02 15:04:05")
	if derr := m.recordEntity.DeleteRecordRange(startStr, endStr); derr != nil {
		m.logger.Errorf("DeleteRecordRange failed for %s: %v", s, derr)
		return derr
	}

	if len(recs) == 0 {
		// nothing to insert
		m.logger.Infof("Cleared range for empty hour %s", s)
		return nil
	}

	// Map to entities
	mapRecords, merr := recordModelToEntity(recs)
	if merr != nil {
		m.logger.Errorf("Mapping records failed for %s: %v", s, merr)
		return merr
	}

	// Persist
	if ierr := m.recordEntity.InsertBatch(mapRecords); ierr != nil {
		m.logger.Errorf("InsertBatch failed for %s: %v", s, ierr)
		return ierr
	}

	m.logger.Infof("Loaded %d records for hour %s", len(mapRecords), hourStart.Format("2006-01-02 15:00:00"))
	return nil
}

func parseErrorFlag(flag string) bool {
	flag = strings.ToLower(strings.TrimSpace(flag))
	return flag == "1" || flag == "true" || flag == "yes" || flag == "y"
}
