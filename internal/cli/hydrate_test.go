package cli

import "testing"

func TestHydrationWorkloads(t *testing.T) {
	tests := []struct {
		name       string
		vision     bool
		mcp        bool
		want       int
		wantImages int
	}{
		{name: "text only without MCP", want: 3},
		{name: "vision without MCP", vision: true, want: 4, wantImages: 1},
		{name: "text only with MCP", mcp: true, want: 4},
		{name: "vision with MCP", vision: true, mcp: true, want: 5, wantImages: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workloads := hydrationWorkloads("model", "config.yaml", tt.vision, tt.mcp)
			if len(workloads) != tt.want {
				t.Fatalf("workload count = %d, want %d", len(workloads), tt.want)
			}
			images := 0
			for _, workload := range workloads {
				if workload.flags.image != "" {
					images++
				}
				if workload.flags.config != "config.yaml" || workload.flags.model != "model" || !workload.flags.autosubmit || !workload.flags.autoexit {
					t.Fatalf("workload flags not configured for unattended inference: %+v", workload.flags)
				}
			}
			if images != tt.wantImages {
				t.Fatalf("image workload count = %d, want %d", images, tt.wantImages)
			}
		})
	}
}

func TestHydrationSnapshotCounts(t *testing.T) {
	artifacts, vision := 17, 8
	withoutMCP := artifacts*len(hydrationWorkloads("model", "config.yaml", false, false)) + vision
	withMCP := artifacts*len(hydrationWorkloads("model", "config.yaml", false, true)) + vision
	if withoutMCP != 59 || withMCP != 76 {
		t.Fatalf("snapshot counts = %d without MCP, %d with MCP; want 59 and 76", withoutMCP, withMCP)
	}
}

func TestHydrationWorkloadArgs(t *testing.T) {
	workloads := hydrationWorkloads("model", "config.yaml", true, true)
	if got := workloads[0].args(); len(got) == 0 || got[len(got)-1] != "--autoexit" {
		t.Fatalf("text args = %#v", got)
	}
	if got := workloads[1].args(); !containsArg(got, "--document") || !containsArg(got, "--nomcp") {
		t.Fatalf("document args = %#v", got)
	}
	if got := workloads[2].args(); !containsArg(got, "--nomcp") {
		t.Fatalf("application-tool args = %#v", got)
	}
	if got := workloads[3].args(); containsArg(got, "--nomcp") {
		t.Fatalf("MCP args unexpectedly disable MCP: %#v", got)
	}
	if got := workloads[4].args(); !containsArg(got, "--image") || !containsArg(got, "--nomcp") {
		t.Fatalf("image args = %#v", got)
	}
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}
