package tvrock

import "testing"

func TestRecordingStatusKeepsActualBindingAndReturnsIndependentSnapshots(t *testing.T) {
	rs := &RecordingSchedule{ID: 7, DeviceID: "assigned-elsewhere", Sid: 1, Eid: 2,
		Title: "recording", StartTime: 1000, EndTime: 2000, IdleTime: 60, ReadyTime: 30, TailTime: 10}
	s := &Scheduler{CurrentRecording: map[string]*RecordingSchedule{"REC01": rs}}
	first, err := s.GetRrecordingStatus()
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Recording) != 1 || first.Recording[0] != 7 || first.CurrentMilliseconds <= 0 {
		t.Fatalf("invalid status: %+v", first)
	}
	current := first.Current["REC01"]
	if current.DeviceID != "REC01" || current.StartTime != 1000 || current.EndTime != 2000 || current.IdleTime != 60 || current.ReadyTime != 30 || current.TailTime != 10 {
		t.Fatalf("actual binding or timing lost: %+v", current)
	}
	rs.EndTime = 3000
	delete(s.CurrentRecording, "REC01")
	second, err := s.GetRrecordingStatus()
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Recording) != 0 || len(second.Current) != 0 {
		t.Fatalf("ended recording still reported: %+v", second)
	}
	if len(first.Recording) != 1 || first.Current["REC01"].EndTime != 2000 {
		t.Fatal("later status or scheduler mutation changed an in-flight response")
	}
}
