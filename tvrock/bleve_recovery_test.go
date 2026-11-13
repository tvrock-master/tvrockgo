package tvrock

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/blevesearch/bleve/v2"
)

type recoveryTestIndex = bleve.Index
type panicSearchIndex struct{ recoveryTestIndex }

func TestBleveInitializeReturnsOpenError(t *testing.T) {
	previousEnabled, previousPath := configure.Bleve, configure.BleveFile
	configure.Bleve = true
	// An existing empty directory is not a valid Bleve index.
	configure.BleveFile = t.TempDir()
	t.Cleanup(func() {
		configure.Bleve, configure.BleveFile = previousEnabled, previousPath
	})

	store := &BleveProgramStore{}
	if err := store.Initialize(); err == nil {
		t.Fatal("Initialize succeeded with an invalid existing index")
	}
	if store.index != nil {
		t.Fatal("Initialize retained an index after an open failure")
	}
}

func (panicSearchIndex) SearchInContext(context.Context, *bleve.SearchRequest) (*bleve.SearchResult, error) {
	values := []byte{1}
	position := 2
	_ = values[position]
	return nil, nil
}

func TestBleveSearchPanicEndsTaskWithWarning(t *testing.T) {
	previousIndex := bleveProgram.index
	previousPath := configure.BleveFile
	bleveProgram.index = panicSearchIndex{}
	configure.BleveFile = "test/bleve-program"
	t.Cleanup(func() {
		bleveProgram.index = previousIndex
		configure.BleveFile = previousPath
	})
	task := &SearchTask{option: &SearchOption{Title: "test", EndTime: 100}, com: make(chan SearchJobCom)}
	var last *SearchProgress
	finished := false
	task.progress = func(p *SearchProgress) { last = p }
	task.finish = func(p *SearchTask) { finished = true }
	task.execBleve()
	if !finished || task.state != SearchJobBreak || last == nil || last.Running {
		t.Fatalf("task did not stop: finished=%v, state=%v, progress=%+v", finished, task.state, last)
	}
	if !strings.Contains(last.Warning, configure.BleveFile) || !strings.Contains(last.Warning, "削除") {
		t.Fatalf("missing recovery instructions: %q", last.Warning)
	}
	// Search must release the store lock even when the library panics.
	if !bleveProgram.mu.TryLock() {
		t.Fatal("store lock remained held")
	}
	bleveProgram.mu.Unlock()
}

func TestBleveSearchPanicReturnsError(t *testing.T) {
	result, err := searchBleveSafely(context.Background(), panicSearchIndex{}, bleve.NewSearchRequest(bleve.NewMatchAllQuery()))
	if result != nil || !errors.Is(err, errBleveSearchPanic) {
		t.Fatalf("result=%v, error=%v", result, err)
	}
}
