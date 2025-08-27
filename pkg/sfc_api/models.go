package sfc_api

type RecordDataCollector struct {
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
