package managers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// StoreFileManager manages saving arbitrary data to JSON files in a directory configured via MESSAGE_DIR.
type StoreFileManager struct {
	dir string
}

// Envelope used to wrap data with a massage_type.
// JSON structure: { "massage_type": "<type>", "massage": <data> }
type MassageEnvelope struct {
	MassageType string      `json:"massage_type"`
	Massage     interface{} `json:"massage"`
}

// NewStoreFileManager creates a manager using MESSAGE_DIR env var.
// Returns error if MESSAGE_DIR is unset or not writable; will attempt to create the directory if it doesn't exist.
func NewStoreFileManager() (*StoreFileManager, error) {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using system environment")
	}
	dir := os.Getenv("MESSAGE_DIR")
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("environment variable MESSAGE_DIR is not set")
	}

	if strings.HasPrefix(dir, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			dir = filepath.Join(home, strings.TrimPrefix(dir, "~"))
		}
	}
	if !filepath.IsAbs(dir) {
		if abs, err := filepath.Abs(dir); err == nil {
			dir = abs
		}
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to ensure directory %s: %w", dir, err)
	}

	// Check write permissions
	f, err := os.CreateTemp(dir, ".permcheck-*")
	if err != nil {
		return nil, fmt.Errorf("directory %s not writable: %w", dir, err)
	}
	_ = f.Close()
	_ = os.Remove(f.Name())

	return &StoreFileManager{dir: dir}, nil
}

// Save writes v as JSON to filename within MESSAGE_DIR.
// If filename has no .json extension, it will be appended.
// Returns the full path to the written file.
func (m *StoreFileManager) Save(filename string, v any) (string, error) {
	if m == nil {
		return "", errors.New("StoreFileManager is nil")
	}
	if strings.TrimSpace(filename) == "" {
		return "", errors.New("filename is required")
	}
	if !strings.HasSuffix(strings.ToLower(filename), ".json") {
		filename += ".json"
	}
	path := filepath.Join(m.dir, filename)

	// Marshal with indentation for readability
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON: %w", err)
	}

	// Write atomically: write to temp then rename
	tmp, err := os.CreateTemp(m.dir, ".tmp-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	_, werr := tmp.Write(b)
	cerr := tmp.Close()
	if werr != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("failed to write temp file: %w", werr)
	}
	if cerr != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("failed to close temp file: %w", cerr)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("failed to move temp file into place: %w", err)
	}
	return path, nil
}

// SaveWrapped writes the data wrapped in an envelope { "massage_type": ..., "massage": ... }.
func (m *StoreFileManager) SaveWrapped(filename, massageType string, data any) (string, error) {
	if strings.TrimSpace(massageType) == "" {
		return "", errors.New("massageType is required")
	}
	env := MassageEnvelope{
		MassageType: massageType,
		Massage:     data,
	}
	return m.Save(filename, env)
}

// SaveWithTimestamp saves v to a file named <base>-YYYYMMDD-HHMMSS.json within MESSAGE_DIR.
// base must be non-empty and will be sanitized for filesystem safety.
func (m *StoreFileManager) SaveWithTimestamp(base string, v any) (string, error) {
	if strings.TrimSpace(base) == "" {
		return "", errors.New("base is required")
	}
	safe := sanitizeBase(base)
	ts := time.Now().Format("20060102-150405")
	name := fmt.Sprintf("%s-%s.json", safe, ts)
	return m.Save(name, v)
}

// SaveWithTimestampWrapped saves the wrapped data using a timestamped name: <base>-YYYYMMDD-HHMMSS.json.
func (m *StoreFileManager) SaveWithTimestampWrapped(base, massageType string, data any) (string, error) {
	if strings.TrimSpace(massageType) == "" {
		return "", errors.New("massageType is required")
	}
	env := MassageEnvelope{
		MassageType: massageType,
		Massage:     data,
	}
	return m.SaveWithTimestamp(base, env)
}

// Directory returns the resolved MESSAGE_DIR directory path.
func (m *StoreFileManager) Directory() string {
	if m == nil {
		return ""
	}
	return m.dir
}

func sanitizeBase(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, string(filepath.Separator), "_")
	// allow letters, digits, dash, underscore, dot; replace others with underscore
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" {
		return "data"
	}
	return out
}
