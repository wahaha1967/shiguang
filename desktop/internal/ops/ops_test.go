package ops_test

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"shiguang-study/desktop/internal/ops"
	"shiguang-study/desktop/internal/store"
)

func applyOwner(t *testing.T, st *store.Store, owner string, operations []any) error {
	t.Helper()
	return st.WithTx(func(tx *sql.Tx) error {
		_, err := ops.ApplyOps(tx, owner, operations)
		return err
	})
}

func questionRow(id string) map[string]any {
	return map[string]any{
		"t": "questions", "op": "upsert",
		"row": map[string]any{"id": id, "title": "原题", "answer": "答案",
			"due": "2026-11-10", "created": "2026-11-10"},
	}
}

func TestUpsertCannotTakeAnotherOwnersRecord(t *testing.T) {
	cases := []struct {
		name string
		op   map[string]any
	}{
		{"questions", questionRow("same")},
		{"logs", map[string]any{"t": "logs", "op": "upsert", "row": map[string]any{
			"id": "same", "date": "2026-11-10", "type": "add"}}},
		{"rewards", map[string]any{"t": "rewards", "op": "upsert", "row": map[string]any{
			"id": "same", "name": "奖励", "cost": 1}}},
		{"redemptions", map[string]any{"t": "redemptions", "op": "upsert", "row": map[string]any{
			"id": "same", "name": "奖励", "cost": 1, "date": "2026-11-10"}}},
		{"aiSets", map[string]any{"t": "aiSets", "op": "upsert", "row": map[string]any{
			"id": "same", "created": "2026-11-10",
			"questions": []any{
				map[string]any{"id": "item-0"}, map[string]any{"id": "item-1"},
				map[string]any{"id": "item-2"}}}}},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			st := newStore(t)
			if err := applyOwner(t, st, "owner-a", []any{testCase.op}); err != nil {
				t.Fatalf("owner-a: %v", err)
			}
			err := applyOwner(t, st, "owner-b", []any{testCase.op})
			if err == nil || err.Error() != "记录编号已被使用" {
				t.Fatalf("owner-b error = %v", err)
			}
			if got := len(snapshotFor(t, st, "owner-b")[testCase.name].([]any)); got != 0 {
				t.Fatalf("owner-b sees %d rows", got)
			}
			if got := len(snapshotFor(t, st, "owner-a")[testCase.name].([]any)); got != 1 {
				t.Fatalf("owner-a sees %d rows", got)
			}
		})
	}
}

func snapshotFor(t *testing.T, st *store.Store, owner string) map[string]any {
	t.Helper()
	state, err := ops.ReadState(st.DB(), owner)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	decodeJSON(t, data, &decoded)
	return decoded
}

func TestOwnerCanUpdateExistingQuestion(t *testing.T) {
	st := newStore(t)
	if err := applyOwner(t, st, "owner-a", []any{questionRow("q-1")}); err != nil {
		t.Fatal(err)
	}
	update := questionRow("q-1")
	update["row"].(map[string]any)["title"] = "修改后的题目"
	if err := applyOwner(t, st, "owner-a", []any{update}); err != nil {
		t.Fatal(err)
	}
	state, err := ops.ReadState(st.DB(), "owner-a")
	if err != nil {
		t.Fatal(err)
	}
	if state.Questions[0].Title != "修改后的题目" {
		t.Fatalf("title = %q", state.Questions[0].Title)
	}
}

func TestDeleteIgnoresAnotherOwnersRow(t *testing.T) {
	st := newStore(t)
	if err := applyOwner(t, st, "owner-a", []any{questionRow("q-1")}); err != nil {
		t.Fatal(err)
	}
	err := applyOwner(t, st, "owner-b", []any{map[string]any{
		"t": "questions", "op": "delete", "id": "q-1"}})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	state, err := ops.ReadState(st.DB(), "owner-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Questions) != 1 {
		t.Fatal("owner-a lost its question")
	}
}

func TestClearingOwnerKeepsKeys(t *testing.T) {
	st := newStore(t)
	if err := st.WithTx(func(tx *sql.Tx) error {
		return ops.SetKey(tx, "owner-a", "qwen", "ciphertext-a")
	}); err != nil {
		t.Fatal(err)
	}
	if err := applyOwner(t, st, "owner-a", []any{
		map[string]any{"t": "reset", "op": "reset"},
	}); err != nil {
		t.Fatal(err)
	}
	ciphertext, err := ops.ReadKey(st.DB(), "owner-a", "qwen")
	if err != nil {
		t.Fatal(err)
	}
	if ciphertext != "ciphertext-a" {
		t.Fatalf("key lost: %q", ciphertext)
	}
	if other, _ := ops.ReadKey(st.DB(), "owner-b", "qwen"); other != "" {
		t.Fatalf("owner-b sees key %q", other)
	}
}

func TestKeysAreNotSharedBetweenOwners(t *testing.T) {
	st := newStore(t)
	if err := st.WithTx(func(tx *sql.Tx) error {
		return ops.SetKey(tx, "owner-a", "qwen", "ciphertext-a")
	}); err != nil {
		t.Fatal(err)
	}
	keys, err := ops.ReadKeys(st.DB(), "owner-b")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 0 {
		t.Fatalf("owner-b keys = %v", keys)
	}
}

func TestFailedBatchRollsBack(t *testing.T) {
	st := newStore(t)
	err := applyOwner(t, st, "owner-a", []any{
		questionRow("new"),
		map[string]any{"t": "unknown", "op": "upsert"},
	})
	if err == nil || err.Error() != "未知的数据表 unknown" {
		t.Fatalf("error = %v", err)
	}
	state, err := ops.ReadState(st.DB(), "owner-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Questions) != 0 {
		t.Fatal("failed batch left a row behind")
	}
}

func TestTooManyOperationsAreRejected(t *testing.T) {
	st := newStore(t)
	operations := make([]any, 5001)
	for index := range operations {
		operations[index] = map[string]any{"t": "reset", "op": "reset"}
	}
	err := applyOwner(t, st, "owner-a", operations)
	if err == nil || err.Error() != "单次操作过多" {
		t.Fatalf("error = %v", err)
	}
}

func TestImageLengthLimit(t *testing.T) {
	st := newStore(t)
	row := questionRow("q-1")
	row["row"].(map[string]any)["image"] = strings.Repeat("x", ops.MaxImage)
	if err := applyOwner(t, st, "owner-a", []any{row}); err != nil {
		t.Fatalf("max image rejected: %v", err)
	}
	row["row"].(map[string]any)["image"] = strings.Repeat("x", ops.MaxImage+1)
	err := applyOwner(t, st, "owner-a", []any{row})
	if err == nil || !strings.Contains(err.Error(), "image 超过长度上限") {
		t.Fatalf("error = %v", err)
	}
}

func TestReadStateGroupsQuestionsAcrossSets(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "study.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	const owner = "owner-multi"

	setOp := func(id string, ord int, qids ...string) map[string]any {
		questions := []any{}
		for _, qid := range qids {
			questions = append(questions, map[string]any{
				"id": qid, "title": "题" + qid, "answer": "答" + qid,
			})
		}
		return map[string]any{
			"t": "aiSets", "op": "upsert", "ord": ord,
			"row": map[string]any{
				"id": id, "sourceId": "src-" + id, "sourceTitle": "源" + id,
				"subject": "数学", "created": "2026-11-10", "aiReviewed": false,
				"questions": questions,
			},
		}
	}

	// Insert the higher-ord set first so neither set order nor question order
	// can be inferred from insertion order.
	if err := applyOwner(t, st, owner, []any{
		setOp("set-b", 1, "b1", "b2", "b3"),
		setOp("set-a", 0, "a1", "a2", "a3"),
	}); err != nil {
		t.Fatal(err)
	}

	state, err := ops.ReadState(st.DB(), owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.AISets) != 2 {
		t.Fatalf("sets = %d, want 2", len(state.AISets))
	}
	if state.AISets[0].ID != "set-a" || state.AISets[1].ID != "set-b" {
		t.Fatalf("set order = %s,%s, want set-a,set-b", state.AISets[0].ID, state.AISets[1].ID)
	}
	want := map[string][]string{"set-a": {"a1", "a2", "a3"}, "set-b": {"b1", "b2", "b3"}}
	for _, set := range state.AISets {
		expected := want[set.ID]
		if len(set.Questions) != len(expected) {
			t.Fatalf("%s: %d questions, want %d", set.ID, len(set.Questions), len(expected))
		}
		for i, question := range set.Questions {
			if question.ID != expected[i] {
				t.Errorf("%s question %d = %s, want %s", set.ID, i, question.ID, expected[i])
			}
		}
	}
}
