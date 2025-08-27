package sfc_api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type fixtureRecord struct {
	ContainerNo   string `json:"CONTAINER_NO"`
	EmpNo         string `json:"EMP_NO"`
	GroupName     string `json:"GROUP_NAME"`
	InLineTime    string `json:"IN_LINE_TIME"`
	InStationTime string `json:"IN_STATION_TIME"`
	LineName      string `json:"LINE_NAME"`
	ModelName     string `json:"MODEL_NAME"`
	MoNumber      string `json:"MO_NUMBER"`
	PalletNo      string `json:"PALLET_NO"`
	SectionName   string `json:"SECTION_NAME"`
	SerialNumber  string `json:"SERIAL_NUMBER"`
	StationName   string `json:"STATION_NAME"`
	VersionCode   string `json:"VERSION_CODE"`
	ErrorFlag     string `json:"ERROR_FLAG"`
	NextStations  string `json:"NEXT_STATION"`
}

func makeServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/getPPIDRecords", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := []fixtureRecord{
			{
				ContainerNo:   "C1",
				EmpNo:         "E1",
				GroupName:     "G1",
				InLineTime:    time.Now().Format(time.RFC3339),
				InStationTime: time.Now().Format(time.RFC3339),
				LineName:      "LINE J01",
				ModelName:     "MODELX",
				MoNumber:      "MO123",
				PalletNo:      "P1",
				SectionName:   "SEC1",
				SerialNumber:  "SN1",
				StationName:   "ST1",
				VersionCode:   "V1",
				ErrorFlag:     "0",
				NextStations:  "NS1",
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})
	return httptest.NewServer(mux)
}

func TestRequestMinuteData_Success(t *testing.T) {
	ts := makeServer()
	defer ts.Close()

	client := NewAPIClient()
	client.SetBaseURL(ts.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	date := time.Now().Format("02-Jan-2006")
	recs, err := client.RequestMinuteData(ctx, date, 12, 34)
	if err != nil {
		t.Fatalf("RequestMinuteData error: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	if recs[0].ModelName == "" || recs[0].SerialNumber == "" {
		t.Fatalf("unexpected empty fields in response: %+v", recs[0])
	}
}

func TestRequestHourData_Success(t *testing.T) {
	ts := makeServer()
	defer ts.Close()

	client := NewAPIClient()
	client.SetBaseURL(ts.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	recs, err := client.RequestHourData(ctx, time.Now().Format("02-Jan-2006"), 8)
	if err != nil {
		t.Fatalf("RequestHourData error: %v", err)
	}
	if len(recs) == 0 {
		t.Fatalf("expected non-empty records")
	}
}

func TestRequestPreviousMinute_Success(t *testing.T) {
	ts := makeServer()
	defer ts.Close()

	client := NewAPIClient()
	client.SetBaseURL(ts.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	recs, err := client.RequestPreviousMinute(ctx)
	if err != nil {
		t.Fatalf("RequestPreviousMinute error: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
}
