package logger

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// helper to read entire file content as string
func readFileString(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	return string(b)
}

// helper to read last non-empty line from file
func readLastLine(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open file: %v", err)
	}
	defer f.Close()
	var last string
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimRight(s.Text(), "\r\n")
		if line != "" {
			last = line
		}
	}
	if err := s.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	return last
}

func TestDefaultConfigAndOptions(t *testing.T) {
	c := DefaultConfig()
	if c.Name != "app" || c.MinLevel != Info || c.Dir != "logs" || c.Console != true || c.JSON != false {
		t.Fatalf("unexpected DefaultConfig: %+v", c)
	}

	dir := t.TempDir()
	l, err := New(
		WithName("my app"),
		WithLevel(Debug),
		WithDir(dir),
		WithConsole(false),
		WithJSON(true),
		WithTimeFormat("2006"),
		WithStaticFields(map[string]any{"k": "v"}),
		WithFilePattern("{name}-{pid}.log"),
	)
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })

	// ensure fields are set on logger
	if l.cfg.Name != "my app" || l.cfg.MinLevel != Debug || l.cfg.Dir != dir || l.cfg.Console != false || l.cfg.JSON != true || l.cfg.TimeFormat != "2006" {
		t.Fatalf("options not applied: %+v", l.cfg)
	}

	// filename should match pattern
	// find the single file created in dir
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 file, got %d", len(entries))
	}
	name := entries[0].Name()
	if !strings.Contains(name, "my-app") {
		t.Fatalf("filename should contain sanitized name 'my-app', got %q", name)
	}
	if !strings.HasSuffix(name, ".log") {
		t.Fatalf("filename should end with .log, got %q", name)
	}
}

func TestNew_UsesEnvDirUnlessWithDir(t *testing.T) {
	tempEnvDir := t.TempDir()
	// ensure env is set before first New in this test
	t.Setenv("LOG_DIR", tempEnvDir)

	// without WithDir, should use env dir
	l1, err := New(WithConsole(false))
	if err != nil {
		t.Fatalf("New err: %v", err)
	}
	defer l1.Close()
	// expect a file exists in tempEnvDir
	found := false
	entries, _ := os.ReadDir(tempEnvDir)
	if len(entries) > 0 {
		found = true
	}
	if !found {
		t.Fatalf("expected a log file in env dir %s", tempEnvDir)
	}

	// WithDir should override env
	customDir := t.TempDir()
	l2, err := New(WithDir(customDir), WithConsole(false))
	if err != nil {
		t.Fatalf("New err: %v", err)
	}
	defer l2.Close()

	entries2, _ := os.ReadDir(customDir)
	if len(entries2) == 0 {
		t.Fatalf("expected a log file in custom dir %s", customDir)
	}
}

func TestLevelsAndJSONOutput(t *testing.T) {
	dir := t.TempDir()
	l, err := New(WithDir(dir), WithConsole(false), WithJSON(true), WithLevel(Info), WithName("svc"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer l.Close()

	l.Debugf("ignore %d", 1)
	l.Infof("hello %s", "world")

	// read file
	files, _ := os.ReadDir(dir)
	if len(files) != 1 {
		t.Fatalf("expected 1 log file, got %d", len(files))
	}
	path := filepath.Join(dir, files[0].Name())
	content := readFileString(t, path)

	if strings.Contains(content, "ignore") {
		t.Fatalf("debug message should have been filtered out")
	}

	// last line should be JSON and contain fields
	last := readLastLine(t, path)
	var m map[string]any
	if err := json.Unmarshal([]byte(last), &m); err != nil {
		t.Fatalf("expected JSON line, got %q, err=%v", last, err)
	}
	if m["level"] != "INFO" || m["name"] != "svc" || m["msg"] != "hello world" {
		t.Fatalf("unexpected JSON fields: %#v", m)
	}
	if _, ok := m["ts"].(string); !ok {
		t.Fatalf("expected timestamp field")
	}
}

func TestTextOutputWithTimeFormat(t *testing.T) {
	dir := t.TempDir()
	l, err := New(WithDir(dir), WithConsole(false), WithJSON(false), WithTimeFormat("2006"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer l.Close()

	l.Infof("x")

	files, _ := os.ReadDir(dir)
	path := filepath.Join(dir, files[0].Name())
	last := readLastLine(t, path)

	year := time.Now().Format("2006")
	if !strings.HasPrefix(last, year+" ") {
		t.Fatalf("expected line to start with year %s, got %q", year, last)
	}
	if !strings.Contains(last, "[INFO]") || !strings.Contains(last, "| x") {
		t.Fatalf("unexpected text format: %q", last)
	}
}

func TestStaticAndContextFieldsMerge(t *testing.T) {
	dir := t.TempDir()
	l, err := New(
		WithDir(dir),
		WithConsole(false),
		WithJSON(true),
		WithStaticFields(map[string]any{"a": 1, "b": "x"}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer l.Close()

	child := l.With(map[string]any{"b": "y", "c": 3})
	child.Infof("test")

	files, _ := os.ReadDir(dir)
	path := filepath.Join(dir, files[0].Name())
	last := readLastLine(t, path)

	var m map[string]any
	if err := json.Unmarshal([]byte(last), &m); err != nil {
		t.Fatalf("json: %v", err)
	}
	if m["a"] != float64(1) { // json numbers become float64
		t.Fatalf("expected a=1, got %#v", m["a"])
	}
	if m["b"] != "y" || m["c"] != float64(3) {
		t.Fatalf("expected merged fields b=y,c=3, got %#v", m)
	}
}

func TestStdLoggerAdapter(t *testing.T) {
	dir := t.TempDir()
	l, err := New(WithDir(dir), WithConsole(false), WithJSON(true))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer l.Close()

	std := l.StdLogger()
	std.Print("hello via std")

	files, _ := os.ReadDir(dir)
	path := filepath.Join(dir, files[0].Name())
	last := readLastLine(t, path)
	var m map[string]any
	if err := json.Unmarshal([]byte(last), &m); err != nil {
		t.Fatalf("json: %v", err)
	}
	if m["level"] != "INFO" || m["msg"] != "hello via std" {
		t.Fatalf("unexpected std adapter output: %#v", m)
	}
}

func TestCloseIdempotentAndNoWriteAfterClose(t *testing.T) {
	dir := t.TempDir()
	l, err := New(WithDir(dir), WithConsole(false), WithJSON(true))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	files, _ := os.ReadDir(dir)
	if len(files) != 1 {
		t.Fatalf("expected one file")
	}
	path := filepath.Join(dir, files[0].Name())

	// write one line
	l.Infof("before close")
	// close twice
	if err := l.Close(); err != nil {
		t.Fatalf("close err: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("second close err: %v", err)
	}

	// size after close
	fi1, _ := os.Stat(path)

	// attempt write after close
	l.Infof("after close")

	fi2, _ := os.Stat(path)
	if fi2.Size() != fi1.Size() {
		t.Fatalf("file grew after close: %d -> %d", fi1.Size(), fi2.Size())
	}
}

func TestSafeSprintfDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("safeSprintf panicked: %v", r)
		}
	}()
	_ = safeSprintf("%s", panicStringer{})
}

func TestTextOutputWithFieldsFormatting(t *testing.T) {
	dir := t.TempDir()
	l, err := New(WithDir(dir), WithConsole(false), WithJSON(false), WithStaticFields(map[string]any{"k1": "v1", "k2": 2}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer l.Close()

	l.Infof("msg")

	files, _ := os.ReadDir(dir)
	path := filepath.Join(dir, files[0].Name())
	last := readLastLine(t, path)
	if !strings.Contains(last, "k1=v1") || !strings.Contains(last, "k2=2") || !strings.Contains(last, "| msg") {
		t.Fatalf("unexpected text with fields: %q", last)
	}
}

// Basic sanity for sanitize and map helpers
func TestHelpers(t *testing.T) {
	sep := string(os.PathSeparator)
	in := " a" + sep + "b "
	if sanitize(in) != "a-b" {
		t.Fatalf("sanitize failed for sep %q: got %q", sep, sanitize(in))
	}
	a := map[string]any{"x": 1}
	b := cloneMap(a)
	if &a == &b || len(b) != 1 || b["x"].(int) != 1 {
		t.Fatalf("cloneMap failed")
	}
	m := mergeMaps(map[string]any{"x": 1}, map[string]any{"x": 2, "y": 3})
	if m["x"].(int) != 2 || m["y"].(int) != 3 {
		t.Fatalf("mergeMaps failed")
	}
}

// Ensure loadDotEnv respects existing env and parses .env when needed.
func TestLoadDotEnvRespectsEnv(t *testing.T) {
	// set env and ensure loadDotEnv doesn't change it even if .env exists
	t.Setenv("LOG_DIR", "already-set")
	// create .env with a different value
	content := []byte("LOG_DIR=from-env-file\n")
	if err := os.WriteFile(".env", content, 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	defer os.Remove(".env")

	loadDotEnv()
	if os.Getenv("LOG_DIR") != "already-set" {
		t.Fatalf("loadDotEnv should respect existing env")
	}
}

func TestLoadDotEnvParsesFile(t *testing.T) {
	// clear env for the duration of this test
	t.Setenv("LOG_DIR", "")
	if err := os.WriteFile(".env", []byte("# comment\n LOG_DIR = \"from-file\"\n"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	defer os.Remove(".env")

	loadDotEnv()
	if os.Getenv("LOG_DIR") != "from-file" {
		t.Fatalf("expected LOG_DIR from .env, got %q", os.Getenv("LOG_DIR"))
	}
}

// Guard against write adapter returning errors
func TestAdapterWriterWriteReturnsLen(t *testing.T) {
	dir := t.TempDir()
	l, err := New(WithDir(dir), WithConsole(false))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer l.Close()

	aw := &adapterWriter{l: l}
	n, err := aw.Write([]byte("line\n"))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if n != len([]byte("line\n")) {
		t.Fatalf("unexpected n: %d", n)
	}
}

// Ensure buildFileName uses pattern tokens
func TestBuildFileNamePatternTokens(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Name = "X Y"
	cfg.FilePattern = "{name}_{pid}.log"
	name := buildFileName(cfg)
	if !strings.HasPrefix(name, "X-Y_") || !strings.HasSuffix(name, ".log") || !strings.Contains(name, "_") {
		t.Fatalf("unexpected file name: %q", name)
	}
}

// minimal interface checks to avoid unused imports
var _ io.Writer = (*adapterWriter)(nil)

// protect against unused imported errors
var _ = errors.New
