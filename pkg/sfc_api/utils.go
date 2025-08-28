package sfc_api

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// CalculatePreviousMinute calculates the previous minute with proper wraparound
func CalculatePreviousMinute() (string, int, int) {
	now := time.Now()
	prevMinute := now.Add(-time.Minute)

	return prevMinute.Format("02-Jan-2006"), prevMinute.Hour(), prevMinute.Minute()
}

// minutes can be negative to move forward in time.
func CalculateMinute(minutes int, at time.Time) (string, int, int) {
	target := at.Add(-time.Duration(minutes) * time.Minute)
	return target.Format("02-Jan-2006"), target.Hour(), target.Minute()
}

func ParseAPITimestamp(timestampStr string) (time.Time, error) {
	// Common timestamp formats from APIs
	formats := []string{
		// GMT format from your API
		"Mon, 02 Jan 2006 15:04:05 MST",
		"Mon, 02 Jan 2006 15:04:05 GMT",
		// Standard formats
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05.000Z",
		"02-Jan-2006 15:04:05",
		"2006/01/02 15:04:05",
		// Additional common formats
		"2006-01-02 15:04:05.000",
		"Jan 02, 2006 15:04:05",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, timestampStr); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse timestamp: %s", timestampStr)
}

// Precompiled regex for J-line codes
var jLineRe = regexp.MustCompile(`(?i)j\d{2}`)

// ExtractJLineCode extracts J-line codes using regex pattern (keeps original signature)
func ExtractJLineCode(lineName string) string {
	if match := jLineRe.FindString(lineName); match != "" {
		return strings.ToUpper(match)
	}
	return lineName
}

// PrintRecordDataCollectors prints the slice of RecordDataCollector in a formatted way
func PrintRecordDataCollectors(records []RecordDataCollector) {
	if len(records) == 0 {
		fmt.Println("No records.")
		return
	}
	for i, r := range records {
		fmt.Printf("#%d %s | Line=%s Station=%s Model=%s Serial=%s Error=%s Next=%s\n",
			i+1, r.InStationTime, r.LineName, r.StationName, r.ModelName, r.SerialNumber, r.ErrorFlag, r.NextStations)
	}
}
