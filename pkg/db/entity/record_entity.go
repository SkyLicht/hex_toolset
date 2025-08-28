package entity

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"
)

// RecordEntity represents a record in the records_table
type RecordEntity struct {
	ID                 string    `json:"id" database:"id"`
	PPID               string    `json:"ppid" database:"ppid"`
	WorkOrder          string    `json:"work_order" database:"work_order"`
	CollectedTimestamp time.Time `json:"collected_timestamp" database:"collected_timestamp"`
	EmployeeName       string    `json:"employee_name" database:"employee_name"`
	GroupName          string    `json:"group_name" database:"group_name"`
	LineName           string    `json:"line_name" database:"line_name"`
	StationName        string    `json:"station_name" database:"station_name"`
	ModelName          string    `json:"model_name" database:"model_name"`
	ErrorFlag          bool      `json:"error_flag" database:"error_flag"`
	NextStation        string    `json:"next_station" database:"next_station"`
}

const (
	tableName = "records_table"
	// Index names for better organization
	idxTimestampPPID      = "idx_records_table_timestamp_ppid"
	idxCompositeLookup    = "idx_records_table_composite_lookup"
	idxDateRange          = "idx_records_table_date_range"
	idxErrorFlag          = "idx_records_table_error_flag"
	idxWorkOrder          = "idx_records_table_work_order"
	idxStationPerformance = "idx_records_table_station_performance"
	idxGroupLineTime      = "idx_records_table_line_group_time"
)

type RecordEntityManager struct {
	TableName string
	db        *sql.DB
}

// NewRecordManagerEntity creates a new RecordEntityManager instance
func NewRecordManagerEntity(db *sql.DB) *RecordEntityManager {
	if db == nil {
		log.Fatal("database connection cannot be nil")
	}

	return &RecordEntityManager{
		TableName: tableName,
		db:        db,
	}
}

// CreateTable creates the optimized records_table for 500MB daily data handling
func (rm *RecordEntityManager) CreateTable() error {
	if err := rm.createMainTable(); err != nil {
		return fmt.Errorf("failed to create main table: %v", err)
	}

	if err := rm.createIndexes(); err != nil {
		return fmt.Errorf("failed to create indexes: %v", err)
	}

	log.Printf("Table %s created successfully with %d indexes", rm.TableName, len(rm.getIndexDefinitions()))
	return nil
}

// createMainTable creates the main table structure
func (rm *RecordEntityManager) createMainTable() error {
	query := rm.buildCreateTableQuery()

	if _, err := rm.db.Exec(query); err != nil {
		return fmt.Errorf("failed to execute CREATE TABLE: %v", err)
	}

	return nil
}

// buildCreateTableQuery builds the CREATE TABLE SQL query
func (rm *RecordEntityManager) buildCreateTableQuery() string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (`, rm.TableName))
	builder.WriteString(`
			id TEXT PRIMARY KEY,
			ppid TEXT NOT NULL CHECK(length(ppid) <= 23),
			work_order TEXT NOT NULL CHECK(length(work_order) <= 12),
			collected_timestamp DATETIME NOT NULL,
			employee_name TEXT NOT NULL CHECK(length(employee_name) <= 16),
			group_name TEXT NOT NULL CHECK(length(group_name) <= 23),
			line_name TEXT NOT NULL CHECK(length(line_name) <= 3),
			station_name TEXT NOT NULL CHECK(length(station_name) <= 23),
			model_name TEXT NOT NULL CHECK(length(model_name) <= 5),
			error_flag INTEGER NOT NULL DEFAULT 0,
			next_station TEXT CHECK(length(next_station) <= 16),
			
			-- Composite unique constraint with conflict resolution
			UNIQUE(ppid, collected_timestamp, line_name, station_name, group_name) ON CONFLICT IGNORE
		) WITHOUT ROWID;`)

	return builder.String()
}

// getIndexDefinitions returns all index definitions for the table
func (rm *RecordEntityManager) getIndexDefinitions() []IndexDefinition {
	return []IndexDefinition{
		{
			Name: idxTimestampPPID,
			Query: fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s 
				ON %s (collected_timestamp DESC, ppid)`, idxTimestampPPID, rm.TableName),
		},
		{
			Name: idxCompositeLookup,
			Query: fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s 
				ON %s (ppid, line_name, station_name, group_name, collected_timestamp DESC)`,
				idxCompositeLookup, rm.TableName),
		},
		{
			Name: idxDateRange,
			Query: fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s 
				ON %s (date(collected_timestamp), line_name)`, idxDateRange, rm.TableName),
		},
		{
			Name: idxErrorFlag,
			Query: fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s 
				ON %s (error_flag, collected_timestamp DESC) WHERE error_flag = 1`,
				idxErrorFlag, rm.TableName),
		},
		{
			Name: idxWorkOrder,
			Query: fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s 
				ON %s (work_order, collected_timestamp DESC)`, idxWorkOrder, rm.TableName),
		},
		{
			Name: idxStationPerformance,
			Query: fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s 
				ON %s (station_name, line_name, collected_timestamp DESC)`,
				idxStationPerformance, rm.TableName),
		},
		{
			Name: idxGroupLineTime,
			Query: fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s ON %s (line_name, group_name, collected_timestamp DESC)`,
				idxGroupLineTime, rm.TableName),
		},
	}
}

// createIndexes creates optimized indexes for high-volume daily data
func (rm *RecordEntityManager) createIndexes() error {
	indexes := rm.getIndexDefinitions()

	for i, index := range indexes {
		if err := rm.createSingleIndex(index); err != nil {
			return fmt.Errorf("failed to create index %d (%s): %v", i+1, index.Name, err)
		}
	}

	return nil
}

// createSingleIndex creates a single index
func (rm *RecordEntityManager) createSingleIndex(index IndexDefinition) error {
	if _, err := rm.db.Exec(index.Query); err != nil {
		return fmt.Errorf("failed to execute index query for %s: %v", index.Name, err)
	}
	return nil
}

// DropTable drops the table and all its indexes (useful for testing/cleanup)
func (rm *RecordEntityManager) DropTable() error {
	query := fmt.Sprintf(`DROP TABLE IF EXISTS %s`, rm.TableName)

	if _, err := rm.db.Exec(query); err != nil {
		return fmt.Errorf("failed to drop table %s: %v", rm.TableName, err)
	}

	log.Printf("Table %s dropped successfully", rm.TableName)
	return nil
}

// TableExists checks if the table exists
func (rm *RecordEntityManager) TableExists() (bool, error) {
	query := `SELECT name FROM sqlite_master WHERE type='table' AND name=?`

	var name string
	err := rm.db.QueryRow(query, rm.TableName).Scan(&name)

	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check table existence: %v", err)
	}

	return true, nil
}

// GetTableInfo returns information about the table structure
func (rm *RecordEntityManager) GetTableInfo() ([]map[string]interface{}, error) {
	query := fmt.Sprintf(`PRAGMA table_info(%s)`, rm.TableName)

	rows, err := rm.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get table info: %v", err)
	}
	defer rows.Close()

	var tableInfo []map[string]interface{}

	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, pk int
		var defaultValue sql.NullString

		err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk)
		if err != nil {
			return nil, fmt.Errorf("failed to scan table info: %v", err)
		}

		column := map[string]interface{}{
			"cid":           cid,
			"name":          name,
			"type":          dataType,
			"notnull":       notNull == 1,
			"default_value": defaultValue.String,
			"pk":            pk == 1,
		}

		tableInfo = append(tableInfo, column)
	}

	return tableInfo, nil
}

// InsertBatch inserts multiple records in a single transaction for better performance
func (rm *RecordEntityManager) InsertBatch(records []RecordEntity) error {
	if len(records) == 0 {
		return nil
	}

	// Start transaction for batch insert
	tx, err := rm.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	// Prepare the INSERT statement
	query := fmt.Sprintf(`
		INSERT INTO %s (
			id, ppid, work_order, collected_timestamp, employee_name, 
			group_name, line_name, station_name, model_name, error_flag, next_station
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, rm.TableName)

	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %v", err)
	}
	defer stmt.Close()

	// Execute batch insert
	insertedCount := 0
	for i, record := range records {
		_, err := stmt.Exec(
			record.ID,
			record.PPID,
			record.WorkOrder,
			record.CollectedTimestamp.Format("2006-01-02 15:04:05"),
			record.EmployeeName,
			record.GroupName,
			record.LineName,
			record.StationName,
			record.ModelName,
			record.ErrorFlag,
			record.NextStation,
		)

		if err != nil {
			return fmt.Errorf("failed to insert record %d (ID: %s): %v", i+1, record.ID, err)
		}
		insertedCount++
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %v", err)
	}

	log.Printf("Successfully inserted %d records into %s", insertedCount, rm.TableName)
	return nil
}
