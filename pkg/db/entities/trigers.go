package entities

import (
	"database/sql"
	"fmt"
	skylogger "hex_toolset/pkg/logger"
)

// TriggersManager encapsulates creation of DB triggers
type TriggersManager struct {
	db     *sql.DB
	logger *skylogger.Logger
}

func NewTriggersManager(db *sql.DB) *TriggersManager {
	if db == nil {
		panic("database connection cannot be nil")
	}
	lgr, _ := skylogger.New(
		skylogger.WithName("entities"),
		skylogger.WithFilePattern("{name}.log"),
	)
	return &TriggersManager{db: db, logger: lgr}
}

// CreateRecordsPassUpsertTrigger creates the trigger that maintains latest_pass on new passing records.
// It assumes records_table and latest_pass exist. Adjust the WHEN condition if the pass criteria differs.
func (t *TriggersManager) CreateRecordsPassUpsertTrigger() error {
	// Using DATETIME strings lexicographically comparable in SQLite ('YYYY-MM-DD HH:MM:SS').
	query := `CREATE TRIGGER IF NOT EXISTS trg_records_pass_upsert
AFTER INSERT ON records_table
WHEN NEW.error_flag = 0
BEGIN
  INSERT INTO latest_pass (line_name, group_name, collected_timestamp)
  VALUES (NEW.line_name, NEW.group_name, NEW.collected_timestamp)
  ON CONFLICT(line_name, group_name) DO UPDATE SET
    collected_timestamp = excluded.collected_timestamp
  WHERE excluded.collected_timestamp > latest_pass.collected_timestamp;
END;`
	if t.logger != nil {
		t.logger.Infof("entity operation \"%s\" \"%s\" \"%s\"", "Triggers", "CreateRecordsPassUpsertTrigger", "start")
	}
	if _, err := t.db.Exec(query); err != nil {
		if t.logger != nil {
			t.logger.Errorf("create trigger trg_records_pass_upsert error: %v", err)
		}
		return fmt.Errorf("create trigger trg_records_pass_upsert: %w", err)
	}
	if t.logger != nil {
		t.logger.Infof("entity operation \"%s\" \"%s\" \"%s\"", "Triggers", "CreateRecordsPassUpsertTrigger", "done")
	}
	return nil
}

// CreateRecordsGroupUpsertTrigger creates the trigger that maintains latest_group.
// Behavior:
// - If NEW.group_name = 'IN_STORE' => DELETE ppid from latest_group.
// - Else UPSERT only if NEW.collected_timestamp is newer than the stored one.
func (t *TriggersManager) CreateRecordsGroupUpsertTrigger() error {
	query := `
CREATE TRIGGER IF NOT EXISTS trg_records_group_upsert
AFTER INSERT ON records_table
BEGIN
  -- Exit from process → remove from latest_group
  DELETE FROM latest_group
  WHERE NEW.group_name = 'IN_STORE'
    AND latest_group.ppid = NEW.ppid;

  -- In-process → upsert only if newer timestamp
  INSERT INTO latest_group (
    ppid, work_order, collected_timestamp, line_name, group_name, station_name, model_name, error_flag
  )
  SELECT
    NEW.ppid, NEW.work_order, NEW.collected_timestamp,
    NEW.line_name, NEW.group_name, NEW.station_name, NEW.model_name, NEW.error_flag
  WHERE NEW.group_name <> 'IN_STORE'
  ON CONFLICT(ppid) DO UPDATE SET
    work_order          = excluded.work_order,
    collected_timestamp = excluded.collected_timestamp,
    line_name           = excluded.line_name,
    group_name          = excluded.group_name,
    station_name        = excluded.station_name,
    model_name          = excluded.model_name,
    error_flag          = excluded.error_flag
  WHERE excluded.collected_timestamp > latest_group.collected_timestamp;
END;`

	if t.logger != nil {
		t.logger.Infof(`entity operation "%s" "%s" "%s"`, "Triggers", "CreateRecordsGroupUpsertTrigger", "start")
	}
	if _, err := t.db.Exec(query); err != nil {
		if t.logger != nil {
			t.logger.Errorf("create trigger trg_records_group_upsert error: %v", err)
		}
		return fmt.Errorf("create trigger trg_records_group_upsert: %w", err)
	}
	if t.logger != nil {
		t.logger.Infof(`entity operation "%s" "%s" "%s"`, "Triggers", "CreateRecordsGroupUpsertTrigger", "done")
	}
	return nil
}
