package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/venndata-org/harbormaster/internal/gitident"
	"github.com/venndata-org/harbormaster/internal/ipc"
)

// sortedServices returns service names ordered by port (then name) — i.e. block
// offset order, matching how they were declared.
func sortedServices(ports map[string]int) []string {
	svcs := make([]string, 0, len(ports))
	for s := range ports {
		svcs = append(svcs, s)
	}
	sort.Slice(svcs, func(i, j int) bool {
		if ports[svcs[i]] != ports[svcs[j]] {
			return ports[svcs[i]] < ports[svcs[j]]
		}
		return svcs[i] < svcs[j]
	})
	return svcs
}

// hmEnvLines renders the HM_* environment for a lease, Tilt port first.
func hmEnvLines(resp ipc.Response) []string {
	lines := []string{fmt.Sprintf("HM_TILT_PORT=%d", resp.Tilt)}
	for _, s := range sortedServices(resp.Ports) {
		lines = append(lines, fmt.Sprintf("HM_PORT_%s=%d", strings.ToUpper(s), resp.Ports[s]))
	}
	return lines
}

func printPortsHuman(w io.Writer, id gitident.Identity, resp ipc.Response) {
	block := ""
	if resp.Block != nil {
		block = fmt.Sprintf("  (block %d-%d)", resp.Block[0], resp.Block[1])
	}
	fmt.Fprintf(w, "%s @ %s%s\n", id.Project, id.Label, block)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "  tilt\t%d\n", resp.Tilt)
	for _, s := range sortedServices(resp.Ports) {
		fmt.Fprintf(tw, "  %s\t%d\n", s, resp.Ports[s])
	}
	_ = tw.Flush()
}

// portsDoc is the `hm ports --json` shape.
type portsDoc struct {
	Instance string         `json:"instance"`
	Project  string         `json:"project"`
	Label    string         `json:"label"`
	Tilt     int            `json:"tilt"`
	Block    *[2]int        `json:"block,omitempty"`
	Ports    map[string]int `json:"ports"`
}

func printPortsJSON(w io.Writer, id gitident.Identity, resp ipc.Response) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(portsDoc{
		Instance: id.Instance,
		Project:  id.Project,
		Label:    id.Label,
		Tilt:     resp.Tilt,
		Block:    resp.Block,
		Ports:    resp.Ports,
	})
}

func printList(w io.Writer, instances []ipc.InstanceInfo) {
	if len(instances) == 0 {
		fmt.Fprintln(w, "no active leases")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "PROJECT\tLABEL\tBLOCK\tTILT\tSERVICES\tPATH")
	for _, in := range instances {
		var parts []string
		for _, s := range sortedServices(in.Berths) {
			if s == "tilt" {
				continue
			}
			parts = append(parts, fmt.Sprintf("%s:%d", s, in.Berths[s]))
		}
		fmt.Fprintf(tw, "%s\t%s\t%d-%d\t%d\t%s\t%s\n",
			in.Project, in.Label, in.Block[0], in.Block[1], in.Berths["tilt"], strings.Join(parts, " "), in.Instance)
	}
	_ = tw.Flush()
}

// renderProjectTOML produces a starter harbormaster.toml body.
func renderProjectTOML(name string, svcs serviceList) string {
	var b strings.Builder
	fmt.Fprintf(&b, "name = %q\n\n", name)
	if len(svcs) == 0 {
		b.WriteString(`# Declare the services that consume host ports. offset is the port's slot within
# this checkout's block (offset 0 is the Tilt UI). default is the legacy port used
# when harbormaster is not installed, so the Tiltfile still works standalone.
# [services]
# web = { offset = 1, default = 3000 }
# api = { offset = 2, default = 4000 }
`)
		return b.String()
	}
	b.WriteString("[services]\n")
	for i, s := range svcs {
		fmt.Fprintf(&b, "%s = { offset = %d, default = %d }\n", s.Name, i+1, s.Default)
	}
	return b.String()
}
