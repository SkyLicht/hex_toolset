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
