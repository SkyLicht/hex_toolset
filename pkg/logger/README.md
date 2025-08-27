# Logger

A lightweight, concurrent-safe, structured logger for Go with:
- Per-instance log files
- Console + file output
- Log levels (Debug, Info, Warn, Error)
- Text or JSON formats
- Contextual fields via With(...)
- Stdlib `*log.Logger` adapter

Works with Go 1.24+.

## Installation

Add the module to your project and import the package:



Replace `your/module/path` with your actual module path.

## Quick Start


Defaults:
- name: `app`
- min level: `Info`
- dir: `logs` (overridden by LOG_DIR env or .env)
- output: file and stdout (console)
- format: text (RFC3339 timestamps)

## Configuration

Use functional options with `logger.New(...)`:

- `WithName(name string)` — logical logger name (printed and used in filenames)
- `WithLevel(level Level)` — minimum level to write (`Debug`, `Info`, `Warn`, `Error`)
- `WithDir(dir string)` — log directory (overrides env/.env)
- `WithFilePattern(pattern string)` — filename template; tokens:
    - `{name}`, `{timestamp}`, `{rand}`, `{pid}`
- `WithConsole(enabled bool)` — mirror output to stdout
- `WithJSON(enabled bool)` — JSON lines instead of text
- `WithTimeFormat(format string)` — time format for text output (default `time.RFC3339`)
- `WithStaticFields(fields map[string]any)` — fields included on every entry



## Log Levels

- `Debugf`, `Infof`, `Warnf`, `Errorf`
- `Printf` is an alias for `Infof` (drop-in compatibility)
- Messages below `MinLevel` are ignored

## Contextual Fields

Attach fields to a logger to include them on every entry:

## Stdlib Compatibility

Use the adapter for components that accept `*log.Logger`:


## Output Formats

- Text (default)
    - Without fields:
        - `2006-01-02T15:04:05Z07:00 [INFO] app | message`
    - With fields:
        - `2006-01-02T15:04:05Z07:00 [INFO] app | k1=v1 k2=v2 | message`

- JSON (`WithJSON(true)`)
    - One compact JSON object per line:
    - Fields: `ts`, `level`, `name`, `msg`, plus your static/context fields

## Files, Names, and Environment

Directory selection priority:
1. `WithDir(...)`
2. `LOG_DIR` environment variable
3. `.env` file at working directory containing `LOG_DIR=...`
4. Default: `logs`

Filename:
- Uses `WithFilePattern` (supports `{name}`, `{timestamp}`, `{rand}`, `{pid}`)
- Ensures uniqueness per instance via timestamp and random suffix (if used)

Permissions:
- Creates directories with `0755`
- Opens files with `0644` (append mode)

## Concurrency and Lifecycle

- Safe for concurrent use
- Call `Close()` when done (safe to call multiple times)
- Each logger instance owns its file handle

## Best Practices

- Use `WithStaticFields` for deployment-wide fields: service, env, version
- Use `With(...)` for per-request/job correlation: request_id, user, trace_id
- Prefer JSON in production for log aggregation and indexing
- Keep `MinLevel` at `Info` or higher in production; switch to `Debug` locally

## Troubleshooting

- No logs in file:
    - Ensure the process has write permissions to the configured directory
    - Confirm `LOG_DIR` or `.env` is set as expected
- Missing stdout logs:
    - Ensure `WithConsole(true)` (default is true)
- Fields missing in output:
    - Verify you are logging via a logger returned by `With(...)`, not the base logger
- Too many logs:
    - Raise `WithLevel(...)` to `Info`, `Warn`, or `Error`

## License

MIT (or your preferred license)