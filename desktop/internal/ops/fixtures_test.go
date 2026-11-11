package ops_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"shiguang-study/desktop/internal/ops"
	"shiguang-study/desktop/internal/store"
)

const fixtureOwner = "fixture-owner"

type opsCase struct {
	Name     string         `json:"name"`
	Setup    []any          `json:"setup"`
	Ops      []any          `json:"ops"`
	Expected map[string]any `json:"expected"`
	Error    string         `json:"error"`
}

type importCase struct {
	Name     string         `json:"name"`
	Import   map[string]any `json:"import"`
	Expected map[string]any `json:"expected"`
	Error    string         `json:"error"`
}

func decodeJSON(t *testing.T, data []byte, target any) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
}

func readFixture(t *testing.T, name string, target any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "fixtures", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	decodeJSON(t, data, target)
}

func newStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "study.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func apply(t *testing.T, st *store.Store, operations []any) error {
	t.Helper()
	return st.WithTx(func(tx *sql.Tx) error {
		_, err := ops.ApplyOps(tx, fixtureOwner, operations)
		return err
	})
}

func snapshot(t *testing.T, st *store.Store) map[string]any {
	t.Helper()
	state, err := ops.ReadState(st.DB(), fixtureOwner)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	var decoded map[string]any
	decodeJSON(t, data, &decoded)
	return decoded
}

func normalize(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			result[key] = normalize(item)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = normalize(item)
		}
		return result
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil {
			return typed.String()
		}
		return parsed
	default:
		return value
	}
}

func assertState(t *testing.T, actual map[string]any, expected map[string]any) {
	t.Helper()
	if !reflect.DeepEqual(normalize(actual), normalize(expected)) {
		actualJSON, _ := json.MarshalIndent(normalize(actual), "", "  ")
		expectedJSON, _ := json.MarshalIndent(normalize(expected), "", "  ")
		t.Fatalf("state mismatch\nactual:\n%s\nexpected:\n%s", actualJSON, expectedJSON)
	}
}

func TestOpsFixtures(t *testing.T) {
	var fixture struct {
		Cases []opsCase `json:"cases"`
	}
	readFixture(t, "ops.json", &fixture)
	if len(fixture.Cases) == 0 {
		t.Fatal("no ops fixtures")
	}
	for _, testCase := range fixture.Cases {
		testCase := testCase
		t.Run(testCase.Name, func(t *testing.T) {
			st := newStore(t)
			if len(testCase.Setup) > 0 {
				if err := apply(t, st, testCase.Setup); err != nil {
					t.Fatalf("setup: %v", err)
				}
			}
			before := snapshot(t, st)
			err := apply(t, st, testCase.Ops)
			if testCase.Error != "" {
				if err == nil {
					t.Fatalf("expected error %q, got none", testCase.Error)
				}
				if err.Error() != testCase.Error {
					t.Fatalf("error = %q, want %q", err.Error(), testCase.Error)
				}
				assertState(t, snapshot(t, st), before)
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertState(t, snapshot(t, st), testCase.Expected)
		})
	}
}

func TestImportFixtures(t *testing.T) {
	var fixture struct {
		Cases []importCase `json:"cases"`
	}
	readFixture(t, "import.json", &fixture)
	if len(fixture.Cases) == 0 {
		t.Fatal("no import fixtures")
	}
	for _, testCase := range fixture.Cases {
		testCase := testCase
		t.Run(testCase.Name, func(t *testing.T) {
			st := newStore(t)
			existing := []any{map[string]any{
				"t": "questions", "op": "upsert",
				"row": map[string]any{"id": "existing", "title": "原题", "answer": "答案"},
			}}
			if err := apply(t, st, existing); err != nil {
				t.Fatalf("seed: %v", err)
			}
			err := st.WithTx(func(tx *sql.Tx) error {
				return ops.ReplaceState(tx, fixtureOwner, testCase.Import)
			})
			if testCase.Error != "" {
				if err == nil {
					t.Fatalf("expected error %q, got none", testCase.Error)
				}
				if err.Error() != testCase.Error {
					t.Fatalf("error = %q, want %q", err.Error(), testCase.Error)
				}
				state := snapshot(t, st)
				questions, _ := state["questions"].([]any)
				if len(questions) != 1 {
					t.Fatalf("rollback lost the existing question: %v", state)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertState(t, snapshot(t, st), testCase.Expected)
		})
	}
}
