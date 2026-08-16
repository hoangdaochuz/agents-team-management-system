// Package archlint enforces the DDD dependency-direction contract of the
// backend-service-architecture spec as a failing test:
//
//   - domain imports nothing infrastructural — no same-service
//     application/infrastructure/interfaces package, no shared platform infra
//     (db, kafka, svcrun, http transport helpers, config), no pgx or sarama.
//   - application imports no concrete infrastructure — no same-service
//     infrastructure/interfaces package, no database driver, no Kafka client.
//
// The check walks every service's internal/{domain,application} trees with
// go/parser, so it needs no build and fails on any violation.
package archlint

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// forbiddenByLayer lists import-path substrings each layer must not reference.
// Substrings are matched against the full import path.
var forbiddenByLayer = map[string][]string{
	"domain": {
		"/internal/application",
		"/internal/infrastructure",
		"/internal/interfaces",
		"github.com/jackc/pgx",
		"github.com/IBM/sarama",
		"net/http",
		"/internal/platform/db",
		"/internal/platform/kafka",
		"/internal/platform/svcrun",
		"/internal/platform/http",
		"/internal/platform/config",
	},
	"application": {
		"/internal/infrastructure",
		"/internal/interfaces",
		"github.com/jackc/pgx",
		"github.com/IBM/sarama",
		"net/http",
		"/internal/platform/db",
		"/internal/platform/kafka",
	},
}

// TestImportDirection walks every service's domain/application packages and
// asserts the dependency rules above.
func TestImportDirection(t *testing.T) {
	servicesRoot := filepath.Join("..", "..", "services")
	services, err := os.ReadDir(servicesRoot)
	if err != nil {
		t.Fatalf("reading services dir: %v", err)
	}

	checked := 0
	for _, svc := range services {
		if !svc.IsDir() {
			continue
		}
		for _, layer := range []string{"domain", "application"} {
			layerDir := filepath.Join(servicesRoot, svc.Name(), "internal", layer)
			forbidden := forbiddenByLayer[layer]
			err := filepath.WalkDir(layerDir, func(path string, d os.DirEntry, err error) error {
				if err != nil {
					if os.IsNotExist(err) {
						return nil // layer not created yet in this service
					}
					return err
				}
				if d.IsDir() || !strings.HasSuffix(path, ".go") {
					return nil
				}
				checked++
				fset := token.NewFileSet()
				f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
				if err != nil {
					t.Errorf("%s: parse: %v", path, err)
					return nil
				}
				for _, imp := range f.Imports {
					importPath := strings.Trim(imp.Path.Value, `"`)
					for _, bad := range forbidden {
						if strings.Contains(importPath, bad) {
							t.Errorf("%s: %s layer must not import %q (imported %q)",
								path, layer, bad, importPath)
						}
					}
				}
				return nil
			})
			if err != nil {
				t.Fatalf("walking %s: %v", layerDir, err)
			}
		}
	}
	// Guard against the check silently rotting if the tree is restructured.
	if checked == 0 {
		t.Log("no domain/application packages found yet — check is vacuous for now")
	}
}
