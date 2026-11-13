package tvrock

import "testing"

func TestRecordingOwnershipChangesNotifySchedule(t *testing.T) {
	queue := &DevConQueue{Com: make(chan string, 16), Done: make(chan struct{})}
	const uid = "test-recording-status-push"
	deviceController.Lock()
	if deviceController.Handler == nil {
		deviceController.Handler = map[string]*DevConQueue{}
	}
	deviceController.Handler[uid] = queue
	deviceController.Unlock()
	t.Cleanup(func() {
		deviceController.Lock()
		delete(deviceController.Handler, uid)
		deviceController.Unlock()
		close(queue.Done)
	})
	s := &Scheduler{CurrentRecording: map[string]*RecordingSchedule{}}
	first := &RecordingSchedule{ID: 1}
	if err := s.SetCurrentRecording("REC01", first); err != nil {
		t.Fatal(err)
	}
	if message := <-queue.Com; message != ControllerUpdateSchedule {
		t.Fatal(message)
	}
	if err := s.SetCurrentRecording("REC01", first); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteCurrentRecording("REC01", 2); err != nil {
		t.Fatal(err)
	}
	select {
	case message := <-queue.Com:
		t.Fatalf("unchanged ownership notified: %s", message)
	default:
	}
	previous, err := s.ReplaceCurrentRecording("REC01", &RecordingSchedule{ID: 2})
	if err != nil || previous != first {
		t.Fatalf("replacement failed: %v", err)
	}
	if message := <-queue.Com; message != ControllerUpdateSchedule {
		t.Fatal(message)
	}
	if err := s.DeleteCurrentRecording("REC01", 2); err != nil {
		t.Fatal(err)
	}
	if message := <-queue.Com; message != ControllerUpdateSchedule {
		t.Fatal(message)
	}
}
