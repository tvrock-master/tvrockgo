package tvrock

import (
	"testing"
)

func TestSearchDynamicSliceLength(t *testing.T) {
	const oneDay = 24 * 60 * 60
	if got := calcSearchSliceLength(oneDay * 3); got != oneDay {
		t.Fatalf("3 days slice = %d, want %d", got, oneDay)
	}
	if got := calcSearchSliceLength(oneDay * 14); got != 2*oneDay {
		t.Fatalf("14 days slice = %d, want %d", got, 2*oneDay)
	}
	if got := calcSearchSliceLength(oneDay * 60); got != 7*oneDay {
		t.Fatalf("60 days slice = %d, want %d", got, 7*oneDay)
	}
}

func TestVerifyProgramBatchOrder(t *testing.T) {
	task := &SearchTask{
		option: &SearchOption{
			FuzzySearch: false,
		},
	}
	var err error
	task.title, _, err = compileSearchPattern("テスト", false)
	if err != nil {
		t.Fatal(err)
	}

	batch := []*TvProgram{
		{Sid: 1, Eid: 1, Start: 100, Title: "無関係A"},
		{Sid: 1, Eid: 2, Start: 200, Title: "テスト番組1"},
		{Sid: 1, Eid: 3, Start: 300, Title: "無関係B"},
		{Sid: 1, Eid: 4, Start: 400, Title: "テスト番組2"},
		{Sid: 1, Eid: 5, Start: 500, Title: "無関係C"},
	}

	matched, err := task.verifyProgramBatch(t.Context(), batch, -1)
	if err != nil {
		t.Fatal(err)
	}
	if len(matched) != 2 {
		t.Fatalf("matched count = %d, want 2", len(matched))
	}
	if matched[0].Eid != 2 || matched[1].Eid != 4 {
		t.Fatalf("matched order unexpected: %#v", matched)
	}
}
