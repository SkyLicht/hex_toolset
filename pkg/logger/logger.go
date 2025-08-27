package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var envOnce sync.Once

// Level represents the severity of a log entry.
// Order: Debug < Info < Warn < Error
//
// Use WithLevel(...) option to configure the minimum level that will be written.
// Messages below the level are ignored.
type Level int

const (
	Debug Level = iota
	Info
	Warn
	Error
)

func (l Level) String() string {
	switch l {
	case Debug:
		return "DEBUG"
	case Info:
		return "INFO"
	case Warn:
		return "WARN"
	case Error:
		return "ERROR"
	default:
		return fmt.Sprintf("LEVEL(%d)", int(l))
	}
}

// Option defines a functional option for configuring the Logger.
type Option func(*Config)

// Config holds configuration for the Logger.
type Config struct {
	Name         string
	MinLevel     Level
	Dir          string
	DirSet       bool   // true if set via WithDir
	FilePattern  string // e.g., "{name}_{timestamp}_{rand}.log"
	Console      bool   // also write to stdout
	JSON         bool   // JSON output; otherwise text
	TimeFormat   string // time format for text output
	StaticFields map[string]any
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		Name:         "app",
		MinLevel:     Info,
		Dir:          "logs",
		FilePattern:  "{name}_{timestamp}_{rand}.log",
		Console:      true,
		JSON:         false,
		TimeFormat:   time.RFC3339,
		StaticFields: map[string]any{},
	}
}

// WithName sets the logical logger name, used in file naming and output.
func WithName(name string) Option { return func(c *Config) { c.Name = name } }

// WithLevel sets the minimum level to write.
func WithLevel(level Level) Option { return func(c *Config) { c.MinLevel = level } }

// WithDir sets the directory where log files are written.
func WithDir(dir string) Option { return func(c *Config) { c.Dir = dir; c.DirSet = true } }

// WithFilePattern sets the filename pattern. Supported tokens: {name}, {timestamp}, {rand}, {pid}
func WithFilePattern(pattern string) Option { return func(c *Config) { c.FilePattern = pattern } }

// WithConsole enables/disables console output.
func WithConsole(enabled bool) Option { return func(c *Config) { c.Console = enabled } }

// WithJSON enables/disables JSON output.
func WithJSON(enabled bool) Option { return func(c *Config) { c.JSON = enabled } }

// WithTimeFormat sets the time format for text output.
func WithTimeFormat(format string) Option { return func(c *Config) { c.TimeFormat = format } }

// WithStaticFields attaches constant fields to every log entry.
func WithStaticFields(fields map[string]any) Option {
	return func(c *Config) { c.StaticFields = cloneMap(fields) }
}

// Logger is a flexible, leveled, structured logger with per-instance file.
type Logger struct {
	cfg    Config
	mu     sync.Mutex
	out    io.Writer
	std    *log.Logger    // standard logger adapter
	file   *os.File       // owned file (per instance)
	fields map[string]any // contextual fields
	closed bool
}

// New creates a new Logger instance with its own file.
// It guarantees a unique log file per instance using timestamp and random suffix.
func New(opts ...Option) (*Logger, error) {
	cfg := DefaultConfig()
	for _, o := range opts {
		o(&cfg)
	}

	// load .env once and prefer OS env over .env. Apply only if Dir was not explicitly set.
	loadEnvOnce()
	if !cfg.DirSet {
		if v := strings.TrimSpace(os.Getenv("LOG_DIR")); v != "" {
			cfg.Dir = v
		}
	}
	if strings.TrimSpace(cfg.Dir) == "" {
		cfg.Dir = "logs"
	}

	if err := os.MkdirAll(cfg.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("logger: create dir: %w", err)
	}

	fileName := buildFileName(cfg)
	filePath := filepath.Join(cfg.Dir, fileName)
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("logger: open file: %w", err)
	}

	var w io.Writer = f
	if cfg.Console {
		w = io.MultiWriter(f, os.Stdout)
	}

	l := &Logger{
		cfg:    cfg,
		out:    w,
		std:    log.New(io.Discard, "", 0), // replaced by adapter below
		file:   f,
		fields: cloneMap(cfg.StaticFields),
	}
	// std logger will write via Info level formatting through the adapter writer
	l.std = log.New(&adapterWriter{l: l}, "", 0)
	return l, nil
}

// Close closes the underlying file of this logger. Safe to call multiple times.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// StdLogger returns a *log.Logger adapter that writes using Info level formatting.
func (l *Logger) StdLogger() *log.Logger { return l.std }

// With returns a child logger that will include the given fields on every entry.
func (l *Logger) With(fields map[string]any) *Logger {
	child := *l // shallow copy
	child.fields = mergeMaps(l.fields, fields)
	return &child
}

// Printf is provided for compatibility with existing code and logs at Info level.
func (l *Logger) Printf(format string, args ...any) { l.logf(Info, format, args...) }

func (l *Logger) Debugf(format string, args ...any) { l.logf(Debug, format, args...) }
func (l *Logger) Infof(format string, args ...any)  { l.logf(Info, format, args...) }
func (l *Logger) Warnf(format string, args ...any)  { l.logf(Warn, format, args...) }
func (l *Logger) Errorf(format string, args ...any) { l.logf(Error, format, args...) }

func (l *Logger) logf(level Level, format string, args ...any) {
	if level < l.cfg.MinLevel {
		return
	}
	msg := safeSprintf(format, args...)
	entryTime := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return
	}

	if l.cfg.JSON {
		// JSON structured line
		payload := map[string]any{
			"ts":    entryTime.Format(time.RFC3339Nano),
			"level": level.String(),
			"name":  l.cfg.Name,
			"msg":   msg,
		}
		for k, v := range l.fields {
			payload[k] = v
		}
		b, err := json.Marshal(payload)
		if err != nil {
			// fallback to text formatting if JSON fails
			fmt.Fprintf(l.out, "%s [%s] %s | %s\n", entryTime.Format(l.cfg.TimeFormat), level.String(), l.cfg.Name, msg)
			return
		}
		fmt.Fprintln(l.out, string(b))
		return
	}

	// Text line
	if len(l.fields) == 0 {
		fmt.Fprintf(l.out, "%s [%s] %s | %s\n", entryTime.Format(l.cfg.TimeFormat), level.String(), l.cfg.Name, msg)
		return
	}
	// include fields as key=value
	var b strings.Builder
	first := true
	for k, v := range l.fields {
		if !first {
			b.WriteString(" ")
		}
		first = false
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(fmt.Sprint(v))
	}
	fmt.Fprintf(l.out, "%s [%s] %s | %s | %s\n", entryTime.Format(l.cfg.TimeFormat), level.String(), l.cfg.Name, b.String(), msg)
}

// adapterWriter allows using the logger as io.Writer for the std logger adapter.
// It writes lines through Info level formatting, trimming trailing newlines.
type adapterWriter struct{ l *Logger }

func (aw *adapterWriter) Write(p []byte) (int, error) {
	msg := strings.TrimRight(string(p), "\n\r")
	aw.l.Infof("%s", msg)
	return len(p), nil
}

func buildFileName(cfg Config) string {
	ts := time.Now().Format("20060102_150405.000")
	randSuffix := fmt.Sprintf("%04d", rand.Intn(10000))
	pid := os.Getpid()
	name := cfg.FilePattern
	name = strings.ReplaceAll(name, "{name}", sanitize(cfg.Name))
	name = strings.ReplaceAll(name, "{timestamp}", ts)
	name = strings.ReplaceAll(name, "{rand}", randSuffix)
	name = strings.ReplaceAll(name, "{pid}", fmt.Sprint(pid))
	if name == "" {
		name = fmt.Sprintf("%s_%s_%s.log", sanitize(cfg.Name), ts, randSuffix)
	}
	return name
}

func sanitize(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, string(os.PathSeparator), "-")
	return s
}

func safeSprintf(format string, args ...any) string {
	defer func() { _ = recover() }()
	return fmt.Sprintf(format, args...)
}

func cloneMap(m map[string]any) map[string]any {
	if len(m) == 0 {
		return map[string]any{}
	}
	cp := make(map[string]any, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}

func mergeMaps(a, b map[string]any) map[string]any {
	res := cloneMap(a)
	for k, v := range b {
		res[k] = v
	}
	return res
}

// loadEnvOnce ensures .env is loaded at most once for LOG_DIR.
func loadEnvOnce() {
	envOnce.Do(func() {
		loadDotEnv()
	})
}

// loadDotEnv loads LOG_DIR from a .env file in the current working directory
// if it's not already set in the environment.
func loadDotEnv() {
	if strings.TrimSpace(os.Getenv("LOG_DIR")) != "" {
		return
	}
	data, err := os.ReadFile(".env")
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		s := strings.TrimSpace(line)
		if s == "" || strings.HasPrefix(s, "#") || strings.HasPrefix(s, ";") {
			continue
		}
		idx := strings.IndexByte(s, '=')
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(s[:idx])
		val := strings.TrimSpace(s[idx+1:])
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		if strings.EqualFold(key, "LOG_DIR") && val != "" {
			_ = os.Setenv("LOG_DIR", val)
			break
		}
	}
}
