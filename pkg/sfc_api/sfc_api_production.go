package sfc_api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	sflogger "hex_toolset/pkg/logger"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"time"
)

// Configuration constants
const (
	HTTPTimeout = 30 * time.Second
	MaxRetries  = 3
	RetryDelay  = 5 * time.Second
)

// RecordDataCollector represents the API response structure (for reference)

// APIClient handles HTTP requests to the external API
type APIClient struct {
	httpClient *http.Client
	baseURL    string
	logger     *log.Logger
}

// NewAPIClient creates a new API client with timeout configuration
func NewAPIClient() *APIClient {
	baseURL := strings.TrimSpace(os.Getenv("SFC_API"))
	if baseURL == "" {
		baseURL = "https://emdii-webtool.foxconn-na.com"
	}

	// Initialize custom logger named "sfc_api_production" and use a stable file name
	var stdLogger *log.Logger
	if lgr, err := sflogger.New(
		sflogger.WithName("sfc_api_production"),
		sflogger.WithFilePattern("{name}.log"),
	); err == nil {
		stdLogger = lgr.StdLogger()
	} else {
		// Fallback to default logger if custom logger fails
		stdLogger = log.Default()
	}

	return &APIClient{
		httpClient: &http.Client{Timeout: HTTPTimeout},
		baseURL:    baseURL,
		logger:     stdLogger,
	}
}

// Optional configuration setters (non-breaking)
func (api *APIClient) SetBaseURL(u string) { api.baseURL = u }

func (api *APIClient) SetHTTPClient(h *http.Client) {
	if h != nil {
		api.httpClient = h
	}
}
func (api *APIClient) SetLogger(l *log.Logger) {
	if l != nil {
		api.logger = l
	}
}

// buildURL constructs API URLs with proper encoding and stable order
func (api *APIClient) buildURL(endpoint string, params map[string]interface{}) string {
	u, _ := url.Parse(api.baseURL)
	u.Path = path.Join(u.Path, endpoint)

	if len(params) > 0 {
		vs := url.Values{}
		// ensure deterministic order
		keys := make([]string, 0, len(params))
		for k := range params {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			vs.Set(k, fmt.Sprint(params[k]))
		}
		u.RawQuery = vs.Encode()
	}
	return u.String()
}

func (api *APIClient) makeRequest(ctx context.Context, url string) ([]byte, error) {
	start := time.Now()
	//api.logger.Printf("HTTP GET start url=%s", url)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		api.logger.Printf("HTTP GET error url=%s err=%v duration=%s", url, err, time.Since(start))
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "hex-toolset/1.0")

	resp, err := api.httpClient.Do(req)
	if err != nil {
		// context canceled or deadline exceeded should return ctx.Err()
		api.logger.Printf("HTTP GET error url=%s err=%v duration=%s", url, err, time.Since(start))
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// read a limited error body for context
		const maxErr = 4 << 10 // 4KB
		b, _ := io.ReadAll(io.LimitReader(resp.Body, maxErr))
		api.logger.Printf("HTTP GET non-200 url=%s status=%d duration=%s body_preview=%q", url, resp.StatusCode, time.Since(start), strings.TrimSpace(string(b)))
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		api.logger.Printf("HTTP GET read error url=%s err=%v duration=%s", url, err, time.Since(start))
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	api.logger.Printf("HTTP GET done url=%s status=%d duration=%s bytes=%d", url, resp.StatusCode, time.Since(start), len(body))
	return body, nil
}

// RequestMinuteData fetches minute-level data from the API
func (api *APIClient) RequestMinuteData(ctx context.Context, date string, hour, minute int) ([]RecordDataCollector, error) {
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return nil, fmt.Errorf("invalid time: hour=%d minute=%d", hour, minute)
	}
	if date == "" {
		return nil, fmt.Errorf("date must not be empty")
	}

	params := map[string]interface{}{
		"date":   date,
		"hour":   fmt.Sprintf("%02d", hour),
		"minute": fmt.Sprintf("%02d", minute),
	}

	_url := api.buildURL("api/getPPIDRecords", params)
	//api.logger.Printf("Requesting: %s", _url)

	body, err := api.makeRequest(ctx, _url)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}

	var data []RecordDataCollector
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	// Normalize LineName to extracted J-line code for all records
	for i := range data {
		data[i].LineName = ExtractJLineCode(data[i].LineName)
		data[i].GroupName = strings.ReplaceAll(data[i].NextStations, " ", "_")
		data[i].NextStations = strings.ReplaceAll(data[i].NextStations, " ", "_")
	}

	api.logger.Printf("Successfully fetched %d records for %s %02d:%02d", len(data), date, hour, minute)
	return data, nil
}

// RequestHourData fetches hour-level data from the API
func (api *APIClient) RequestHourData(ctx context.Context, date string, hour int) ([]RecordDataCollector, error) {
	if hour < 0 || hour > 23 {
		return nil, fmt.Errorf("invalid hour: %d", hour)
	}
	if date == "" {
		return nil, fmt.Errorf("date must not be empty")
	}

	params := map[string]interface{}{
		"date": date,
		"hour": fmt.Sprintf("%02d", hour),
	}

	_url := api.buildURL("api/getPPIDRecords", params)
	api.logger.Printf("Requesting: %s", _url)

	body, err := api.makeRequest(ctx, _url)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}

	var data []RecordDataCollector
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	// Normalize LineName to extracted J-line code for all records
	for i := range data {
		data[i].LineName = ExtractJLineCode(data[i].LineName)
		data[i].GroupName = strings.ReplaceAll(data[i].NextStations, " ", "_")
		data[i].NextStations = strings.ReplaceAll(data[i].NextStations, " ", "_")
	}

	api.logger.Printf("Successfully fetched data for %s %02d", date, hour)
	return data, nil
}

// RequestPreviousMinute fetches current minute data with automatic retry and jittered backoff
func (api *APIClient) RequestPreviousMinute(ctx context.Context) ([]RecordDataCollector, error) {
	date, hour, minute := CalculatePreviousMinute()

	//api.logger.Printf("Fetching minute data at %s for %s %02d:%02d",
	//	time.Now().Format("15:04:05"), date, hour, minute)

	var result []RecordDataCollector
	var lastErr error

	err := doWithRetry(ctx, MaxRetries, RetryDelay, func() error {
		data, err := api.RequestMinuteData(ctx, date, hour, minute)
		if err != nil {
			lastErr = err
			api.logger.Printf("Attempt failed: %v", err)
			return err
		}
		result = data
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("all %d attempts failed, last error: %w", MaxRetries, lastErr)
	}

	return result, nil
}

// RequestMinute RequestPreviousMinute fetches current minute data with automatic retry and jittered backoff
func (api *APIClient) RequestMinute(ctx context.Context, time time.Time) ([]RecordDataCollector, error) {
	date, hour, minute := CalculateMinute(1, time)

	//api.logger.Printf("Fetching minute data at %s for %s %02d:%02d",
	//	time.Now().Format("15:04:05"), date, hour, minute)

	var result []RecordDataCollector
	var lastErr error

	err := doWithRetry(ctx, MaxRetries, RetryDelay, func() error {
		data, err := api.RequestMinuteData(ctx, date, hour, minute)
		if err != nil {
			lastErr = err
			api.logger.Printf("Attempt failed: %v", err)
			return err
		}
		result = data
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("all %d attempts failed, last error: %w", MaxRetries, lastErr)
	}

	return result, nil
}

// doWithRetry executes fn with retry using jittered backoff.
// It stops early if the context is done.
func doWithRetry(ctx context.Context, attempts int, baseDelay time.Duration, fn func() error) error {
	if attempts <= 0 {
		attempts = 1
	}
	delay := baseDelay
	if delay <= 0 {
		delay = 500 * time.Millisecond
	}

	for i := 1; i <= attempts; i++ {
		err := fn()
		if err == nil {
			return nil
		}
		if i == attempts {
			return err
		}

		// jitter: wait in [delay/2, delay)
		j := time.Duration(rand.Int63n(int64(delay / 2)))
		wait := delay/2 + j

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
			// exponential backoff with cap
			delay = time.Duration(float64(delay) * 1.5)
			if delay > 30*time.Second {
				delay = 30 * time.Second
			}
		}
	}
	return nil
}

//More update
