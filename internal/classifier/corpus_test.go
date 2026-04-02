package classifier

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/mimikos-io/mimikos/internal/parser"
	"github.com/pb33f/libopenapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// expectedCorpus represents the expected classification JSON file format.
type expectedCorpus struct {
	Spec            string            `json:"spec"`
	Source          string            `json:"source"`
	TotalOperations int               `json:"total_operations"` //nolint:tagliatelle // matches existing test corpus files
	Classifications map[string]string `json:"classifications"`
	Notes           map[string]string `json:"notes,omitempty"`
}

// testdataDir returns the absolute path to the testdata directory root.
func testdataDir(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")

	return filepath.Join(filepath.Dir(filename), "..", "..", "testdata")
}

// loadExpectedCorpus reads and parses an expected classification JSON file.
func loadExpectedCorpus(t *testing.T, path string) expectedCorpus {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var corpus expectedCorpus
	require.NoError(t, json.Unmarshal(data, &corpus))

	return corpus
}

// misclassification records a single classification disagreement.
type misclassification struct {
	key      string
	expected string
	got      string
}

func TestCorpusAccuracy(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping corpus accuracy test in short mode")
	}

	base := testdataDir(t)
	expectedDir := filepath.Join(base, "expected")
	specsDir := filepath.Join(base, "specs")

	// Discover all expected classification files.
	expectedFiles, err := filepath.Glob(filepath.Join(expectedDir, "*.json"))
	require.NoError(t, err)
	require.NotEmpty(t, expectedFiles, "no expected classification files found")

	p := parser.NewLibopenAPIParser(nil)
	c := New()

	var (
		totalCorrect int
		totalChecked int
		specsTested  int
		specsSkipped int
		allMisses    []misclassification
	)

	for _, expectedFile := range expectedFiles {
		corpus := loadExpectedCorpus(t, expectedFile)
		specName := corpus.Spec
		specPath := filepath.Join(specsDir, specName)

		// Skip specs that aren't present (large gitignored specs).
		if _, err := os.Stat(specPath); os.IsNotExist(err) {
			t.Logf("SKIP %s (spec file not present)", specName)

			specsSkipped++

			continue
		}

		specsTested++

		specData, err := os.ReadFile(specPath)
		require.NoError(t, err, "reading spec %s", specName)

		doc, docErr := libopenapi.NewDocument(specData)
		require.NoError(t, docErr, "creating document for %s", specName)

		spec, err := p.Parse(context.Background(), doc)
		require.NoError(t, err, "parsing spec %s", specName)

		var (
			correct int
			checked int
			misses  []misclassification
		)

		for _, op := range spec.Operations {
			key := op.Method + " " + op.Path
			expectedType, ok := corpus.Classifications[key]

			if !ok {
				// Operation not in expected set — skip (partial corpus).
				continue
			}

			result := c.Classify(op)
			checked++

			if result.Type.String() == expectedType {
				correct++
			} else {
				misses = append(misses, misclassification{
					key:      key,
					expected: expectedType,
					got:      result.Type.String(),
				})
			}
		}

		// Also check for expected classifications not found in parsed spec.
		parsedKeys := make(map[string]struct{}, len(spec.Operations))
		for _, op := range spec.Operations {
			parsedKeys[op.Method+" "+op.Path] = struct{}{}
		}

		for key := range corpus.Classifications {
			if _, ok := parsedKeys[key]; !ok {
				t.Logf("  WARN %s: expected key %q not found in parsed operations", specName, key)
			}
		}

		accuracy := float64(0)
		if checked > 0 {
			accuracy = float64(correct) / float64(checked) * 100
		}

		t.Logf("%-30s %3d/%3d correct (%.1f%%)", specName, correct, checked, accuracy)

		totalCorrect += correct
		totalChecked += checked

		allMisses = append(allMisses, misses...)
	}

	// Overall summary.
	overallAccuracy := float64(0)
	if totalChecked > 0 {
		overallAccuracy = float64(totalCorrect) / float64(totalChecked) * 100
	}

	t.Logf("")
	t.Logf("SPECS: %d tested, %d skipped (not present), %d total",
		specsTested, specsSkipped, len(expectedFiles))
	t.Logf("OVERALL: %d/%d correct (%.1f%%)", totalCorrect, totalChecked, overallAccuracy)
	t.Logf("MISCLASSIFICATIONS: %d", len(allMisses))

	// Log misclassifications grouped by (expected → got) for analysis.
	if len(allMisses) > 0 {
		sort.Slice(allMisses, func(i, j int) bool {
			if allMisses[i].expected != allMisses[j].expected {
				return allMisses[i].expected < allMisses[j].expected
			}

			if allMisses[i].got != allMisses[j].got {
				return allMisses[i].got < allMisses[j].got
			}

			return allMisses[i].key < allMisses[j].key
		})

		// Summary by category.
		categories := make(map[string]int)

		for _, m := range allMisses {
			cat := fmt.Sprintf("%s → %s", m.expected, m.got)
			categories[cat]++
		}

		t.Logf("")
		t.Logf("Misclassification categories:")

		catKeys := make([]string, 0, len(categories))
		for k := range categories {
			catKeys = append(catKeys, k)
		}

		sort.Strings(catKeys)

		for _, cat := range catKeys {
			t.Logf("  %-25s %d", cat, categories[cat])
		}

		// Detail (truncated for readability).
		t.Logf("")
		t.Logf("Details (first 50):")

		limit := len(allMisses)
		if limit > 50 {
			limit = 50
		}

		for _, m := range allMisses[:limit] {
			t.Logf("  %-50s expected=%-10s got=%s", m.key, m.expected, m.got)
		}

		if len(allMisses) > 50 {
			t.Logf("  ... and %d more", len(allMisses)-50)
		}
	}

	// Sanity checks: ensure meaningful corpus coverage.
	assert.GreaterOrEqual(t, specsTested, 5,
		"at least 5 specs must be present to produce meaningful accuracy numbers")
	assert.Greater(t, totalChecked, 50,
		"expected to check at least 50 operations across the corpus")

	// Log a machine-readable summary line for easy grepping.
	t.Logf("")
	t.Logf("CORPUS_ACCURACY=%.1f CHECKED=%d CORRECT=%d MISCLASSIFIED=%d SPECS=%d/%d",
		overallAccuracy, totalChecked, totalCorrect, len(allMisses),
		specsTested, len(expectedFiles))
}
