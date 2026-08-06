package controlplane

// ConsoleMessage is one frame of the console WebSocket protocol.
// Contract: api/console.schema.json, docs/agent-developers/console-protocol.md.
type ConsoleMessage struct {
	Type string `json:"type"`
	TS   int64  `json:"ts,omitempty"`
	Line string `json:"line,omitempty"`
	ID   string `json:"id,omitempty"`
	Data string `json:"data,omitempty"`
}

// Client→server message types.
const (
	ConsoleTypeRcon = "rcon"
	ConsoleTypePing = "ping"
)

// Server→client message types.
const (
	ConsoleTypeLog          = "log"
	ConsoleTypeRconResponse = "rcon-response"
	ConsoleTypeStatus       = "status"
	ConsoleTypeError        = "error"
)
