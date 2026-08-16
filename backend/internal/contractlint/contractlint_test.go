// Package contractlint pins the external REST and Kafka contracts to a golden
// baseline so the DDD refactor cannot drift them unnoticed:
//
//   - REST: every route pattern ("METHOD /path") registered across all
//     services' HTTP layers, extracted statically from HandleFunc calls.
//   - Kafka: the topic catalog (name + task-partitioned flag) and the JSON
//     shape of every event payload type (field names, omitempty behavior).
//
// Run with -update to regenerate the golden files after an INTENTIONAL
// contract change (which the spec says should not happen in this refactor).
package contractlint

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/aaks/server/internal/contracts/events"
)

var update = flag.Bool("update", false, "rewrite golden files")

// routePatternRe matches net/http 1.22 route patterns in string literals.
var routePatternRe = regexp.MustCompile(`"(GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS) [^"]+"`)

// httpLayerDirs are the directories whose handlers own the REST surface —
// today's httpapi layer and the DDD interfaces/http layer it converts into.
var httpLayerDirs = []string{"internal/httpapi", "internal/interfaces/http"}

// TestRestEndpoints pins the per-service REST route surface.
func TestRestEndpoints(t *testing.T) {
	servicesRoot := filepath.Join("..", "..", "services")
	services, err := os.ReadDir(servicesRoot)
	if err != nil {
		t.Fatalf("reading services dir: %v", err)
	}

	got := map[string][]string{}
	for _, svc := range services {
		if !svc.IsDir() {
			continue
		}
		for _, layer := range httpLayerDirs {
			layerDir := filepath.Join(servicesRoot, svc.Name(), layer)
			err := filepath.WalkDir(layerDir, func(path string, d os.DirEntry, err error) error {
				if err != nil {
					if os.IsNotExist(err) {
						return nil
					}
					return err
				}
				if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
					return nil
				}
				src, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				fset := token.NewFileSet()
				f, err := parser.ParseFile(fset, path, src, 0)
				if err != nil {
					t.Errorf("%s: parse: %v", path, err)
					return nil
				}
				// Only scan files that register routes; keeps false positives
				// (route-pattern-looking strings in comments/data) out.
				if !registersRoutes(string(src)) {
					return nil
				}
				for _, m := range routePatternRe.FindAllString(string(src), -1) {
					got[svc.Name()] = append(got[svc.Name()], strings.Trim(m, `"`))
				}
				_ = f
				return nil
			})
			if err != nil {
				t.Fatalf("walking %s: %v", layerDir, err)
			}
		}
	}

	var b strings.Builder
	names := make([]string, 0, len(got))
	for svc := range got {
		names = append(names, svc)
	}
	sort.Strings(names)
	for _, svc := range names {
		routes := got[svc]
		sort.Strings(routes)
		uniq := routes[:0]
		for i, r := range routes {
			if i == 0 || r != routes[i-1] {
				uniq = append(uniq, r)
			}
		}
		b.WriteString("# " + svc + "\n")
		for _, r := range uniq {
			b.WriteString(r + "\n")
		}
	}
	checkGolden(t, "rest_endpoints.golden", b.String())
}

func registersRoutes(src string) bool {
	return strings.Contains(src, "HandleFunc(") || strings.Contains(src, "mux.Handle(")
}

// kafkaPayloads enumerates every event payload type by zero value; the
// marshaled shape (field names + omitempty behavior) is the contract.
func kafkaPayloads() map[string]any {
	return map[string]any{
		"EventEnvelope":         events.EventEnvelope{},
		"RunRequestedData":      events.RunRequestedData{},
		"ReviewRequestedData":   events.ReviewRequestedData{},
		"StopRequestedData":     events.StopRequestedData{},
		"PrOpenRequestedData":   events.PrOpenRequestedData{},
		"StepData":              events.StepData{},
		"RunCompletedData":      events.RunCompletedData{},
		"FindingData":           events.FindingData{},
		"VerdictData":           events.VerdictData{},
		"PrOpenedData":          events.PrOpenedData{},
		"TaskStatusChangedData": events.TaskStatusChangedData{},
		"SignupRequestedData":   events.SignupRequestedData{},
		"SignupApprovedData":    events.SignupApprovedData{},
		"SignupDeclinedData":    events.SignupDeclinedData{},
		"InviteCreatedData":     events.InviteCreatedData{},
		"WorkspaceCreatedData":  events.WorkspaceCreatedData{},
		"McpCreatedData":        events.McpCreatedData{},
		"McpDeletedData":        events.McpDeletedData{},
		"SkillCreatedData":      events.SkillCreatedData{},
		"SkillDeletedData":      events.SkillDeletedData{},
		"RunStartedData":        events.RunStartedData{},
		"AuditRecordedData":     events.AuditRecordedData{},
	}
}

// TestKafkaContract pins topic names, partitioning, and payload JSON shapes.
func TestKafkaContract(t *testing.T) {
	var b strings.Builder
	b.WriteString("# topics (name task-partitioned)\n")
	topics := events.AllTopics()
	sort.Strings(topics)
	for _, tp := range topics {
		fmt.Fprintf(&b, "%s %t\n", tp, events.IsTaskPartitioned(tp))
	}
	b.WriteString("\n# payload JSON shapes (zero-value marshal)\n")
	payloads := kafkaPayloads()
	names := make([]string, 0, len(payloads))
	for n := range payloads {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		buf, err := json.MarshalIndent(payloads[n], "", "  ")
		if err != nil {
			t.Fatalf("marshaling %s: %v", n, err)
		}
		b.WriteString("## " + n + "\n" + string(buf) + "\n")
	}
	checkGolden(t, "kafka_contract.golden", b.String())
}

func checkGolden(t *testing.T, name, got string) {
	t.Helper()
	goldenPath := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatalf("writing golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("reading golden (run go test ./internal/contractlint -update to create): %v", err)
	}
	if string(want) != got {
		t.Errorf("%s drifted from the pinned contract baseline; if the change is intentional update with -update", name)
	}
}
