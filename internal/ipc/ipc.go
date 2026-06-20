package ipc

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"
)

// dialTimeout bounds connect+request round-trips from the CLI.
const dialTimeout = 5 * time.Second

// handleTimeout bounds how long the server waits for a request on a connection.
const handleTimeout = 10 * time.Second

// WriteMessage encodes v as a single NDJSON line.
func WriteMessage(w io.Writer, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = w.Write(data)
	return err
}

// ReadMessage decodes one NDJSON line into v.
func ReadMessage(r *bufio.Reader, v any) error {
	line, err := r.ReadBytes('\n')
	if err != nil && len(bytes.TrimSpace(line)) == 0 {
		return err
	}
	return json.Unmarshal(bytes.TrimSpace(line), v)
}

// Client is a short-lived connection to the daemon. One request per connection.
type Client struct {
	Socket string
}

func (c *Client) call(req Request) (Response, error) {
	conn, err := net.DialTimeout("unix", c.Socket, dialTimeout)
	if err != nil {
		return Response{}, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(dialTimeout))
	if err := WriteMessage(conn, req); err != nil {
		return Response{}, err
	}
	var resp Response
	if err := ReadMessage(bufio.NewReader(conn), &resp); err != nil {
		return Response{}, err
	}
	return resp, nil
}

// Ping checks daemon liveness.
func (c *Client) Ping() (Response, error) { return c.call(Request{Op: "ping"}) }

// Lease resolves this instance's ports, registering it on first use.
func (c *Client) Lease(instance, project, label string, services []string) (Response, error) {
	return c.call(Request{Op: "lease", Instance: instance, Project: project, Label: label, Services: services})
}

// Get returns the instance's current lease without allocating or probing. The
// reply's Found reports whether a lease exists. This is the read-only path behind
// `hm ports`. See DECISIONS.md D10.
func (c *Client) Get(instance string) (Response, error) {
	return c.call(Request{Op: "get", Instance: instance})
}

// List returns every leased instance.
func (c *Client) List() (Response, error) { return c.call(Request{Op: "list"}) }

// Release frees one instance's lease.
func (c *Client) Release(instance string) (Response, error) {
	return c.call(Request{Op: "release", Instance: instance})
}

// Prune reclaims leases whose worktree directory is gone.
func (c *Client) Prune() (Response, error) { return c.call(Request{Op: "prune"}) }

// Doctor reports daemon/pool health.
func (c *Client) Doctor() (Response, error) { return c.call(Request{Op: "doctor"}) }

// Shutdown asks the daemon to exit gracefully. Unknown to pre-D11 daemons (they
// reply with an error), so callers fall back to a pidfile signal.
func (c *Client) Shutdown() (Response, error) { return c.call(Request{Op: "shutdown"}) }

// IsLive reports whether a daemon is answering on socket.
func IsLive(socket string) bool {
	c := &Client{Socket: socket}
	resp, err := c.Ping()
	return err == nil && resp.OK
}

// Handler turns a request into a response. The daemon implements it.
type Handler interface {
	Handle(Request) Response
}

// Server accepts NDJSON requests on a Unix socket and replies via Handler.
type Server struct {
	Socket  string
	Handler Handler

	ln net.Listener
}

// Listen binds the Unix socket, creating its parent directory and clearing a
// stale socket file. Call this before Serve; callers must ensure no live daemon
// already owns the socket (see IsLive).
func (s *Server) Listen() error {
	if err := os.MkdirAll(filepath.Dir(s.Socket), 0o755); err != nil {
		return err
	}
	// A leftover socket file from a crashed daemon blocks bind; clear it.
	if _, err := os.Stat(s.Socket); err == nil {
		_ = os.Remove(s.Socket)
	}
	ln, err := net.Listen("unix", s.Socket)
	if err != nil {
		return err
	}
	s.ln = ln
	return nil
}

// Serve accepts connections until the listener is closed.
func (s *Server) Serve() error {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go s.serveConn(conn)
	}
}

func (s *Server) serveConn(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(handleTimeout))
	var req Request
	if err := ReadMessage(bufio.NewReader(conn), &req); err != nil {
		_ = WriteMessage(conn, Err("bad request: %v", err))
		return
	}
	_ = WriteMessage(conn, s.Handler.Handle(req))
}

// Close stops accepting and removes the socket file.
func (s *Server) Close() error {
	if s.ln == nil {
		return nil
	}
	err := s.ln.Close()
	_ = os.Remove(s.Socket)
	return err
}
