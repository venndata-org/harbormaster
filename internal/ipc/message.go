// Package ipc is the transport between the CLI and the daemon: NDJSON messages
// over a Unix domain socket. It is pure transport — it imports no other internal
// package — so both the client (CLI) and the server (daemon) share one wire
// definition. Field names follow docs/socket-protocol.md.
package ipc

import "fmt"

// Request is a single CLI->daemon command. One JSON object per line.
type Request struct {
	Op       string   `json:"op"`
	Instance string   `json:"instance,omitempty"`
	Project  string   `json:"project,omitempty"`
	Label    string   `json:"label,omitempty"`
	Services []string `json:"services,omitempty"`
}

// InstanceInfo is one row in a `list` reply.
type InstanceInfo struct {
	Instance   string         `json:"instance"`
	Project    string         `json:"project"`
	Label      string         `json:"label"`
	Block      [2]int         `json:"block"`
	Berths     map[string]int `json:"berths"`
	CreatedAt  string         `json:"createdAt"`
	LastSeenAt string         `json:"lastSeenAt"`
}

// Squatter is a leased port currently bound by some process (a `doctor` finding).
type Squatter struct {
	Instance string `json:"instance"`
	Service  string `json:"service"`
	Port     int    `json:"port"`
}

// Response is the daemon's single-line reply. Every reply sets OK; op-specific
// fields are populated as documented in docs/socket-protocol.md.
type Response struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`

	// ping
	Version string `json:"version,omitempty"`

	// lease / get
	Tilt     int            `json:"tilt,omitempty"`
	Ports    map[string]int `json:"ports,omitempty"`
	Block    *[2]int        `json:"block,omitempty"`
	Warnings []string       `json:"warnings,omitempty"`

	// get: whether the instance currently holds a lease
	Found bool `json:"found,omitempty"`

	// list
	Instances []InstanceInfo `json:"instances,omitempty"`

	// release
	Released bool `json:"released,omitempty"`

	// prune
	Reclaimed []string `json:"reclaimed,omitempty"`

	// doctor
	Leases    int        `json:"leases,omitempty"`
	Headroom  int        `json:"headroom,omitempty"`
	Squatters []Squatter `json:"squatters,omitempty"`
}

// Err builds a failed response.
func Err(format string, args ...any) Response {
	if len(args) == 0 {
		return Response{OK: false, Error: format}
	}
	return Response{OK: false, Error: fmt.Sprintf(format, args...)}
}
