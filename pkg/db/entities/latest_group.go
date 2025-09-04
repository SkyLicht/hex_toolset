package entities

import (
	"database/sql"
	"fmt"
	skylogger "hex_toolset/pkg/logger"
)

type LatestGroup struct {
	PPID               string `json:"ppid"                 database:"ppid"`
	WorkOrder          string `json:"work_order"           database:"work_order"`
	CollectedTimestamp string `json:"collected_timestamp"  database:"collected_timestamp"`
	LineName           string `json:"line_name"            database:"line_name"`
	GroupName          string `json:"group_name"           database:"group_name"`
	StationName        string `json:"station_name"         database:"station_name"`
	ModelName          string `json:"model_name"           database:"model_name"` // <-- added
	ErrorFlag          int    `json:"error_flag"           database:"error_flag"`
}

const latestGroupTable = "latest_group"

type LatestGroupManager struct {
	TableName string
	db        *sql.DB
	logger    *skylogger.Logger
}

func NewLatestGroupManager(db *sql.DB) *LatestGroupManager {
	if db == nil {
		panic("database connection cannot be nil")
	}
	lgr, _ := skylogger.New(
		skylogger.WithName("entities"),
		skylogger.WithFilePattern("{name}.log"),
	)
	return &LatestGroupManager{TableName: latestGroupTable, db: db, logger: lgr}
}

// CreateTable creates latest_group and all supporting indexes.
func (m *LatestGroupManager) CreateTable() error {
	if m.logger != nil {
		m.logger.Infof(`entity operation "%s" "%s" "%s"`, "LatestGroup", "CreateTable", "start")
	}
	create := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
  ppid                TEXT PRIMARY KEY,
  work_order          TEXT NOT NULL,
  collected_timestamp DATETIME NOT NULL,
  line_name           TEXT NOT NULL,
  group_name          TEXT NOT NULL,
  station_name        TEXT NOT NULL,
  model_name          TEXT NOT NULL,         -- <-- added
  error_flag          INTEGER NOT NULL DEFAULT 0
) WITHOUT ROWID;`, m.TableName)

	if _, err := m.db.Exec(create); err != nil {
		if m.logger != nil {
			m.logger.Errorf("create latest_group table error: %v", err)
		}
		return err
	}

	// --- Backward-compatible migration: add column if table existed already ---
	// This will no-op if the column already exists.
	if _, err := m.db.Exec(fmt.Sprintf(
		`ALTER TABLE %s ADD COLUMN model_name TEXT NOT NULL DEFAULT ''`, m.TableName)); err != nil {
		// ignore "duplicate column name: model_name"
		if m.logger != nil {
			m.logger.Debugf("alter latest_group add model_name (ignore if exists): %v", err)
		}
	}

	// Indexes tuned for WIP dashboards & stale detection
	idx1 := fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_latest_group_line_group ON %s (line_name, group_name);`, m.TableName)
	if _, err := m.db.Exec(idx1); err != nil {
		if m.logger != nil {
			m.logger.Errorf("create idx_latest_group_line_group error: %v", err)
		}
		return err
	}

	idx2 := fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_latest_group_ts ON %s (collected_timestamp);`, m.TableName)
	if _, err := m.db.Exec(idx2); err != nil {
		if m.logger != nil {
			m.logger.Errorf("create idx_latest_group_ts error: %v", err)
		}
		return err
	}

	// Helpful for model-specific dashboards/queries
	idx3 := fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_latest_group_line_model_group ON %s (line_name, model_name, group_name);`, m.TableName)
	if _, err := m.db.Exec(idx3); err != nil {
		if m.logger != nil {
			m.logger.Errorf("create idx_latest_group_line_model_group error: %v", err)
		}
		return err
	}

	if m.logger != nil {
		m.logger.Infof(`entity operation "%s" "%s" "%s"`, "LatestGroup", "CreateTable", "done")
	}
	return nil
}

// UpsertIfNewer lets you update programmatically (bypassing the trigger if needed).
func (m *LatestGroupManager) UpsertIfNewer(
	ppid, workOrder, timestamp, lineName, groupName, stationName string, errorFlag int,
) error {
	q := fmt.Sprintf(`INSERT INTO %s
(ppid, work_order, collected_timestamp, line_name, group_name, station_name, error_flag)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(ppid) DO UPDATE SET
  work_order          = excluded.work_order,
  collected_timestamp = excluded.collected_timestamp,
  line_name           = excluded.line_name,
  group_name          = excluded.group_name,
  station_name        = excluded.station_name,
  error_flag          = excluded.error_flag
WHERE excluded.collected_timestamp > %s.collected_timestamp;`, m.TableName, m.TableName)

	_, err := m.db.Exec(q, ppid, workOrder, timestamp, lineName, groupName, stationName, errorFlag)
	return err
}

// DeleteOnInStore mirrors trigger behavior (useful for replays or repairs).
func (m *LatestGroupManager) DeleteOnInStore(ppid string) error {
	q := fmt.Sprintf(`DELETE FROM %s WHERE ppid = ?;`, m.TableName)
	_, err := m.db.Exec(q, ppid)
	return err
}

func (m *LatestGroupManager) GetByPPID(ppid string) (LatestGroup, error) {
	q := fmt.Sprintf(`SELECT ppid, work_order, collected_timestamp, line_name, group_name, station_name, error_flag
FROM %s WHERE ppid = ?;`, m.TableName)
	var lg LatestGroup
	err := m.db.QueryRow(q, ppid).
		Scan(&lg.PPID, &lg.WorkOrder, &lg.CollectedTimestamp, &lg.LineName, &lg.GroupName, &lg.StationName, &lg.ErrorFlag)
	return lg, err
}

// Map like "J06_PACKING" -> "YYYY-MM-DD HH:MM:SS" (aggregated from latest_group)
func (m *LatestGroupManager) GetLineGroupMap() (map[string]string, error) {
	q := fmt.Sprintf(`SELECT line_name || '_' || group_name AS line_group,
       MAX(collected_timestamp) AS ts
FROM %s
GROUP BY line_group;`, m.TableName)

	rows, err := m.db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// Utility
func (m *LatestGroupManager) DeleteAll() error {
	q := fmt.Sprintf(`DELETE FROM %s;`, m.TableName)
	_, err := m.db.Exec(q)
	return err
}
