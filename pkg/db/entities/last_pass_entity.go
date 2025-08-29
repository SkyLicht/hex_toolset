package entities

import (
	"database/sql"
	"fmt"
	skylogger "hex_toolset/pkg/logger"
)

// LatestPass represents the latest passing timestamp per (line, group)
type LatestPass struct {
	LineName           string `json:"line_name" database:"line_name"`
	GroupName          string `json:"group_name" database:"group_name"`
	CollectedTimestamp string `json:"collected_timestamp" database:"collected_timestamp"` // 'YYYY-MM-DD HH:MM:SS'
}

const latestPassTable = "latest_pass"

// LatestPassManager provides helpers to manage latest_pass table
type LatestPassManager struct {
	TableName string
	db        *sql.DB
	logger    *skylogger.Logger
}

// NewLatestPassManager creates a new manager
func NewLatestPassManager(db *sql.DB) *LatestPassManager {
	if db == nil {
		panic("database connection cannot be nil")
	}
	lgr, _ := skylogger.New(
		skylogger.WithName("entities"),
		skylogger.WithFilePattern("{name}.log"),
	)
	return &LatestPassManager{TableName: latestPassTable, db: db, logger: lgr}
}

// CreateTable creates the latest_pass table and its index
func (m *LatestPassManager) CreateTable() error {
	create := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
  line_name TEXT NOT NULL,
  group_name TEXT NOT NULL,
  collected_timestamp TEXT NOT NULL,
  PRIMARY KEY (line_name, group_name)
);`, m.TableName)
	if m.logger != nil {
		m.logger.Infof("entity operation \"%s\" \"%s\" \"%s\"", "LatestPass", "CreateTable", "start")
	}
	if _, err := m.db.Exec(create); err != nil {
		if m.logger != nil {
			m.logger.Errorf("create latest_pass table error: %v", err)
		}
		return err
	}
	idx := fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_latest_pass_line_group ON %s (line_name, group_name);`, m.TableName)
	if _, err := m.db.Exec(idx); err != nil {
		if m.logger != nil {
			m.logger.Errorf("create latest_pass index error: %v", err)
		}
		return err
	}
	if m.logger != nil {
		m.logger.Infof("entity operation \"%s\" \"%s\" \"%s\"", "LatestPass", "CreateTable", "done")
	}
	return nil
}

// UpsertIfNewer inserts or updates the latest pass only if incoming timestamp is newer or row doesn't exist.
// timestamp must be in format 'YYYY-MM-DD HH:MM:SS'
func (m *LatestPassManager) UpsertIfNewer(lineName, groupName, timestamp string) error {
	// Use INSERT ... ON CONFLICT DO UPDATE with a WHERE clause to enforce newer timestamp only
	q := fmt.Sprintf(`INSERT INTO %s (line_name, group_name, collected_timestamp)
VALUES (?, ?, ?)
ON CONFLICT(line_name, group_name) DO UPDATE SET
  collected_timestamp=excluded.collected_timestamp
WHERE excluded.collected_timestamp > %s.collected_timestamp;`, m.TableName, m.TableName)
	_, err := m.db.Exec(q, lineName, groupName, timestamp)
	return err
}

// Get returns the latest pass for a (line, group). sql.ErrNoRows if not found.
func (m *LatestPassManager) Get(lineName, groupName string) (LatestPass, error) {
	q := fmt.Sprintf(`SELECT line_name, group_name, collected_timestamp FROM %s WHERE line_name=? AND group_name=?`, m.TableName)
	var lp LatestPass
	err := m.db.QueryRow(q, lineName, groupName).Scan(&lp.LineName, &lp.GroupName, &lp.CollectedTimestamp)
	return lp, err
}

// DeleteAll removes all rows (utility/testing)
func (m *LatestPassManager) DeleteAll() error {
	q := fmt.Sprintf(`DELETE FROM %s`, m.TableName)
	_, err := m.db.Exec(q)
	return err
}
