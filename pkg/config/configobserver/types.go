package configobserver

type InsightsConfiguration struct {
	DataReporting DataReporting `json:"dataReporting"`
}

type DataReporting struct {
	Interval         string `json:"interval,omitempty"`
	UploadEndpoint   string `json:"uploadEndpoint,omitempty"`
	DownloadEndpoint string `json:"downloadEndpoint,omitempty"`
}
