package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/joho/godotenv"
	_ "modernc.org/sqlite"
)

// DBConnection is a singleton struct that manages the database connection.
type DBConnection struct {
	database *sql.DB

	// internal init gate
	once    syncOnce
	initErr error

	// cached path for diagnostics
	dbPath string
}

// syncOnce is a minimal wrapper we can replace or extend later (keeps imports clean).
type syncOnce struct {
	done bool
	ch   chan struct{}
}

func (o *syncOnce) Do(fn func()) {
	if o.done {
		return
	}
	if o.ch == nil {
		o.ch = make(chan struct{})
		go func() {
			defer close(o.ch)
			fn()
		}()
	}
	<-o.ch
	o.done = true
}

var instance *DBConnection

// GetInstance returns the singleton instance of DBConnection.
func GetInstance() *DBConnection {
	if instance == nil {
		instance = &DBConnection{}
	}
	return instance
}

// Config holds initialization settings for the SQLite database connection.
type Config struct {
	// Path to the SQLite database file. If empty, will be read from SFC_CLON (.env/env).
	Path string

	// Pooling settings
	MaxOpenConns    int           // default 1 (SQLite single writer)
	MaxIdleConns    int           // default 1
	ConnMaxLifetime time.Duration // default 0 (unlimited)
	ConnMaxIdleTime time.Duration // default 0 (unlimited)

	// Pragmas applied per-connection via DSN "_pragma=" so they apply to the whole pool.
	// Defaults are tuned for "write-once-per-minute, read-heavy".
	BusyTimeoutMs int // default 30000
	// Synchronous mode: FULL | NORMAL | OFF; default NORMAL for balanced durability/perf
	Synchronous string // default "NORMAL"
	// TempStore: DEFAULT | FILE | MEMORY; default MEMORY
	TempStore string // default "MEMORY"
	// Cache size as KB (negative form) so size is clear regardless of page size, e.g., -10240 for ~10MB
	CacheSizeKB int // default 10240 (~10MB). Stored as negative for pragma.
	// Mmap size bytes (0 disables); default 268435456 (256MB)
	MmapSizeBytes int64 // default 268435456
	// Enforce foreign keys
	ForeignKeys bool // default true

	// Persistent/once pragmas executed after open:
	// journal_mode=WAL persists with the database; safer concurrent reads during writes
	EnableWAL bool // default true
	// wal_autocheckpoint pages; default 1000
	WALAutoCheckpoint int // default 1000
}

// DefaultConfig returns sensible defaults for a read-heavy workload with occasional writes.
func DefaultConfig() Config {
	return Config{
		Path:              "", // load from SFC_CLON by default
		MaxOpenConns:      1,
		MaxIdleConns:      1,
		ConnMaxLifetime:   0,
		ConnMaxIdleTime:   0,
		BusyTimeoutMs:     30000,
		Synchronous:       "NORMAL",
		TempStore:         "MEMORY",
		CacheSizeKB:       10240, // ~10MB cache
		MmapSizeBytes:     0,     // disabled by default to avoid OOM on some systems
		ForeignKeys:       true,
		EnableWAL:         true,
		WALAutoCheckpoint: 1000,
	}
}

// Init initializes the SQLite database connection using the provided context and config.
// It is safe to call multiple times; initialization is performed once.
// If cfg.Path is empty, it will attempt to load from .env/env variable SFC_CLON.
// Returns an error if SFC_CLON is not provided.
func (h *DBConnection) Init(ctx context.Context, cfg Config) error {
	h.once.Do(func() {
		h.initErr = h.initInternal(ctx, cfg)
	})
	return h.initErr
}

// InitDefault loads .env (if present), reads SFC_CLON, and initializes with defaults.
// Returns error if SFC_CLON is not set or empty.
func (h *DBConnection) InitDefault(ctx context.Context) error {
	_ = godotenv.Load() // best-effort; ok if not present
	path := os.Getenv("SFC_CLON")

	fmt.Println(path)
	if path == "" {
		return fmt.Errorf("SFC_CLON is not set")
	}
	cfg := DefaultConfig()
	cfg.Path = path
	return h.Init(ctx, cfg)
}

func (h *DBConnection) initInternal(ctx context.Context, cfg Config) error {
	// Fill defaults explicitly
	def := DefaultConfig()

	// If Path not provided, load from env
	if cfg.Path == "" {
		// Best-effort load .env here too, in case caller didn't call InitDefault
		_ = godotenv.Load()
		cfg.Path = os.Getenv("SFC_CLON")
		if cfg.Path == "" {
			return fmt.Errorf("SFC_CLON is not set")
		}
	}
	if cfg.MaxOpenConns == 0 {
		cfg.MaxOpenConns = def.MaxOpenConns
	}
	if cfg.MaxIdleConns == 0 {
		cfg.MaxIdleConns = def.MaxIdleConns
	}
	if cfg.ConnMaxLifetime == 0 {
		cfg.ConnMaxLifetime = def.ConnMaxLifetime
	}
	if cfg.ConnMaxIdleTime == 0 {
		cfg.ConnMaxIdleTime = def.ConnMaxIdleTime
	}
	if cfg.BusyTimeoutMs == 0 {
		cfg.BusyTimeoutMs = def.BusyTimeoutMs
	}
	if cfg.Synchronous == "" {
		cfg.Synchronous = def.Synchronous
	}
	if cfg.TempStore == "" {
		cfg.TempStore = def.TempStore
	}
	if cfg.CacheSizeKB == 0 {
		cfg.CacheSizeKB = def.CacheSizeKB
	}
	if cfg.MmapSizeBytes == 0 {
		cfg.MmapSizeBytes = def.MmapSizeBytes
	}
	if cfg.WALAutoCheckpoint == 0 {
		cfg.WALAutoCheckpoint = def.WALAutoCheckpoint
	}
	if !cfg.ForeignKeys {
		cfg.ForeignKeys = def.ForeignKeys
	}
	if !cfg.EnableWAL {
		cfg.EnableWAL = def.EnableWAL
	}

	// Resolve absolute path and ensure directory exists
	absPath, err := filepath.Abs(cfg.Path)
	if err != nil {
		return fmt.Errorf("resolve absolute path: %w", err)
	}
	info, statErr := os.Stat(absPath)
	if statErr == nil && info.IsDir() {
		return fmt.Errorf("database path points to a directory: %s", absPath)
	}
	// Ensure parent directory exists
	if dir := filepath.Dir(absPath); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create database directory %s: %w", dir, err)
		}
	}
	// Explicitly create the database file if it does not exist yet.
	if _, err := os.Stat(absPath); errors.Is(err, os.ErrNotExist) {
		f, cerr := os.OpenFile(absPath, os.O_RDWR|os.O_CREATE, 0o644)
		if cerr != nil {
			return fmt.Errorf("create database file %s: %w", absPath, cerr)
		}
		_ = f.Close()
	}
	h.dbPath = absPath

	// Open using plain absolute path to avoid Windows file URL encoding issues.
	db, err := sql.Open("sqlite", absPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}

	// Apply per-connection pragmas via Exec on this connection (MaxOpenConns=1 by default).
	{
		execCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		stmts := []string{
			fmt.Sprintf("PRAGMA busy_timeout=%d", cfg.BusyTimeoutMs),
			fmt.Sprintf("PRAGMA synchronous=%s", cfg.Synchronous),
			fmt.Sprintf("PRAGMA temp_store=%s", cfg.TempStore),
			fmt.Sprintf("PRAGMA cache_size=%d", -cfg.CacheSizeKB),
		}
		if cfg.MmapSizeBytes > 0 {
			stmts = append(stmts, fmt.Sprintf("PRAGMA mmap_size=%d", cfg.MmapSizeBytes))
		}
		if cfg.ForeignKeys {
			stmts = append(stmts, "PRAGMA foreign_keys=ON")
		} else {
			stmts = append(stmts, "PRAGMA foreign_keys=OFF")
		}
		for _, s := range stmts {
			if _, err := db.ExecContext(execCtx, s); err != nil {
				_ = db.Close()
				return fmt.Errorf("apply pragma %q: %w", s, err)
			}
		}
	}

	// Apply pool settings suitable for SQLite
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	if cfg.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	}
	if cfg.ConnMaxIdleTime > 0 {
		db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)
	}

	// Ping with timeout to ensure the DB is reachable
	{
		pctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		if err := db.PingContext(pctx); err != nil {
			_ = db.Close()
			return fmt.Errorf("ping database: %w", err)
		}
	}

	// Persistent/once pragmas (journal_mode=WAL, wal_autocheckpoint)
	{
		execCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		if cfg.EnableWAL {
			if _, err := db.ExecContext(execCtx, "PRAGMA journal_mode=WAL"); err != nil {
				_ = db.Close()
				return fmt.Errorf("set journal_mode=WAL: %w", err)
			}
		}
		if cfg.WALAutoCheckpoint > 0 {
			if _, err := db.ExecContext(execCtx, fmt.Sprintf("PRAGMA wal_autocheckpoint=%d", cfg.WALAutoCheckpoint)); err != nil {
				_ = db.Close()
				return fmt.Errorf("set wal_autocheckpoint: %w", err)
			}
		}
	}

	h.database = db
	log.Printf("Database initialized successfully at: %s", absPath)
	return nil
}

// GetDB returns the database connection instance.
// It panics if Init has not been called successfully.
func (h *DBConnection) GetDB() *sql.DB {
	if h.database == nil {
		log.Fatal("Database not initialized. Call Init(...) first.")
	}
	return h.database
}

// CloseDB closes the database connection gracefully.
// It performs a passive WAL checkpoint to avoid heavy blocking.
func (h *DBConnection) CloseDB() error {
	db := h.database
	if db == nil {
		return nil
	}

	// Best-effort passive checkpoint with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, "PRAGMA wal_checkpoint(PASSIVE)"); err != nil {
		// Non-fatal; log and continue with close
		log.Printf("Warning: WAL checkpoint failed: %v", err)
	}

	if err := db.Close(); err != nil {
		return fmt.Errorf("close database: %w", err)
	}
	h.database = nil
	log.Println("Database connection closed successfully")
	return nil
}

// HealthCheck performs a simple health check on the database with timeout.
func (h *DBConnection) HealthCheck(ctx context.Context) error {
	if h.database == nil {
		return errors.New("database not initialized")
	}
	if err := h.database.PingContext(ctx); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	// Collect some lightweight SQLite metrics
	var (
		sqliteVersion string
		pageSize      int64
		pageCount     int64
		freeList      int64
		journalMode   string
		foreignKeys   int64
		cacheSize     int64
	)

	// SQLite version
	_ = h.database.QueryRowContext(ctx, "select sqlite_version()").Scan(&sqliteVersion)
	// Basic PRAGMAs (ignore individual scan errors; best-effort reporting)
	_ = h.database.QueryRowContext(ctx, "PRAGMA page_size").Scan(&pageSize)
	_ = h.database.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pageCount)
	_ = h.database.QueryRowContext(ctx, "PRAGMA freelist_count").Scan(&freeList)
	_ = h.database.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode)
	_ = h.database.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys)
	_ = h.database.QueryRowContext(ctx, "PRAGMA cache_size").Scan(&cacheSize)

	// If using WAL, get current WAL stats via passive checkpoint query (does not block)
	var walBusy, walLog, walCheckpointed int64
	_ = h.database.QueryRowContext(ctx, "PRAGMA wal_checkpoint(PASSIVE)").Scan(&walBusy, &walLog, &walCheckpointed)

	// Emit metrics
	log.Printf("DB Health: path=%s sqlite_version=%s page_size=%d page_count=%d freelist=%d journal_mode=%s foreign_keys=%d cache_kb=%d wal_busy=%d wal_log=%d wal_ckpt=%d",
		h.dbPath,
		sqliteVersion,
		pageSize,
		pageCount,
		freeList,
		journalMode,
		foreignKeys,
		-cacheSize, // negative cache_size means KB; value is negative when set as KB
		walBusy,
		walLog,
		walCheckpointed,
	)

	return nil
}

// DBPath returns the absolute path to the database file, if initialized.
func (h *DBConnection) DBPath() string {
	return h.dbPath
}

// Package-level helpers requested: Init and GetDB returning the singleton.
// Init reads .env, uses SFC_CLON path, creates DB if missing, and initializes once.
func Init(ctx context.Context) error {
	return GetInstance().InitDefault(ctx)
}

// GetDB returns the singleton *sql.DB instance.
func GetDB() *sql.DB {
	return GetInstance().GetDB()
}
