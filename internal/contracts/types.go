package contracts

const Version = 1

// A separate value is linked into test executables to isolate them from the installed service.
var PipeName = `\\.\pipe\SpeedLimitFree.v1`

const ServiceName = "SpeedLimitFree"

type ServiceStatus struct {
	Installed       bool   `json:"installed"`
	State           string `json:"state"`
	Startup         string `json:"startup"`
	Binary          string `json:"binary"`
	PID             uint32 `json:"pid"`
	ExitCode        uint32 `json:"exitCode"`
	ServiceExitCode uint32 `json:"serviceExitCode"`
	LogPath         string `json:"logPath"`
	ServiceLog      string `json:"serviceLog"`
	SetupLog        string `json:"setupLog"`
}

type ServiceResult struct {
	OperationID string `json:"operationId"`
	Action      string `json:"action"`
	Success     bool   `json:"success"`
	Message     string `json:"message"`
}

type Process struct {
	PID        uint32  `json:"pid"`
	Started    string  `json:"started"`
	Name       string  `json:"name"`
	Path       string  `json:"path"`
	Download   float64 `json:"download"`
	Upload     float64 `json:"upload"`
	Downloaded uint64  `json:"downloaded"`
	Uploaded   uint64  `json:"uploaded"`
	RuleID     string  `json:"ruleId"`
}

type Rule struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Scope    string `json:"scope"`
	Path     string `json:"path"`
	PID      uint32 `json:"pid"`
	Started  string `json:"started"`
	Download *int64 `json:"download"`
	Upload   *int64 `json:"upload"`
	Enabled  bool   `json:"enabled"`
}

type Settings struct {
	Version int    `json:"version"`
	Paused  bool   `json:"paused"`
	Rules   []Rule `json:"rules"`
}

type Snapshot struct {
	Version      int       `json:"version"`
	Connected    bool      `json:"connected"`
	Engine       string    `json:"engine"`
	Message      string    `json:"message"`
	Paused       bool      `json:"paused"`
	Processes    []Process `json:"processes"`
	Rules        []Rule    `json:"rules"`
	Download     float64   `json:"download"`
	Upload       float64   `json:"upload"`
	UnknownBytes uint64    `json:"unknownBytes"`
	Dropped      uint64    `json:"dropped"`
	QueueBytes   int64     `json:"queueBytes"`
	Uptime       int64     `json:"uptime"`
}

type Request struct {
	Version int    `json:"version"`
	Method  string `json:"method"`
	Rule    *Rule  `json:"rule,omitempty"`
	ID      string `json:"id,omitempty"`
	Paused  bool   `json:"paused,omitempty"`
}
type Response struct {
	Version  int       `json:"version"`
	Error    string    `json:"error,omitempty"`
	Snapshot *Snapshot `json:"snapshot,omitempty"`
}
