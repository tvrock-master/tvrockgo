package tvrock

import (
	"testing"
	"time"
)

func TestIsInFetchProgramScheduleHours(t *testing.T) {
	loc := time.Local
	tests := []struct {
		name  string
		hour  int
		hours []int
		want  bool
	}{
		{name: "inside configured hour", hour: 7, hours: []int{7, 8}, want: true},
		{name: "at configured hour start", hour: 7, hours: []int{7}, want: true},
		{name: "at next hour after range", hour: 9, hours: []int{7, 8}, want: false},
		{name: "unconfigured hour", hour: 8, hours: []int{7}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := time.Date(2026, 8, 10, tt.hour, 0, 0, 0, loc)
			if got := isInFetchProgramScheduleHours(now, tt.hours); got != tt.want {
				t.Fatalf("isInFetchProgramScheduleHours(%v, %v) = %t, want %t", now, tt.hours, got, tt.want)
			}
		})
	}
}

func TestHasFetchProgramScheduleInRange(t *testing.T) {
	now := time.Date(2026, 8, 10, 7, 3, 0, 0, time.Local)
	rangeStart := time.Date(2026, 8, 10, 7, 0, 0, 0, time.Local)
	rangeEnd := time.Date(2026, 8, 10, 8, 0, 0, 0, time.Local)

	schedules := []*RecordingSchedule{
		{
			DeviceID:    "REC08",
			CustomComID: "FetchProgramBS",
			Validate:    true,
			Complete:    false,
			StartTime:   time.Date(2026, 8, 10, 7, 0, 0, 0, time.Local).Unix(),
			EndTime:     time.Date(2026, 8, 10, 7, 30, 0, 0, time.Local).Unix(),
		},
	}

	if !hasFetchProgramScheduleInRange("FetchProgramBS", schedules, rangeStart, rangeEnd, now.Unix()) {
		t.Fatal("active fetch schedule in the same range was not detected")
	}

	if hasFetchProgramScheduleInRange("FetchProgramCS", schedules, rangeStart, rangeEnd, now.Unix()) {
		t.Fatal("different fetch schedule type was detected")
	}
}

func TestHasFetchProgramScheduleInRangeIgnoresCompletedAndExpired(t *testing.T) {
	now := time.Date(2026, 8, 10, 8, 1, 0, 0, time.Local)
	rangeStart := time.Date(2026, 8, 10, 7, 0, 0, 0, time.Local)
	rangeEnd := time.Date(2026, 8, 10, 8, 0, 0, 0, time.Local)

	schedules := []*RecordingSchedule{
		{
			DeviceID:    "REC08",
			CustomComID: "FetchProgramBS",
			Validate:    true,
			Complete:    false,
			StartTime:   time.Date(2026, 8, 10, 7, 0, 0, 0, time.Local).Unix(),
			EndTime:     time.Date(2026, 8, 10, 7, 30, 0, 0, time.Local).Unix(),
		},
		{
			DeviceID:    "REC07",
			CustomComID: "FetchProgramBS",
			Validate:    true,
			Complete:    true,
			StartTime:   time.Date(2026, 8, 10, 7, 30, 0, 0, time.Local).Unix(),
			EndTime:     time.Date(2026, 8, 10, 8, 0, 0, 0, time.Local).Unix(),
		},
	}

	if hasFetchProgramScheduleInRange("FetchProgramBS", schedules, rangeStart, rangeEnd, now.Unix()) {
		t.Fatal("completed or expired fetch schedules should be ignored")
	}
}

func TestShouldKeepFetchProgramScheduleRequiresActiveTaskOrCurrentRecording(t *testing.T) {
	oldJobs := tasks.Job
	defer func() {
		tasks.Job = oldJobs
	}()

	now := time.Date(2026, 8, 10, 7, 3, 0, 0, time.Local).Unix()
	sch := &RecordingSchedule{
		ID:        123,
		DeviceID:  "REC08",
		Validate:  true,
		Complete:  false,
		StartTime: time.Date(2026, 8, 10, 7, 0, 0, 0, time.Local).Unix(),
		EndTime:   time.Date(2026, 8, 10, 7, 30, 0, 0, time.Local).Unix(),
		IdleTime:  30,
		ReadyTime: 15,
		TailTime:  0,
	}
	s := &Scheduler{CurrentRecording: map[string]*RecordingSchedule{}}

	tasks.Job = map[uint64]*RecordingJob{}
	if s.shouldKeepFetchProgramSchedule(sch, now) {
		t.Fatal("stale active-time fetch schedule without a task should not be kept")
	}

	tasks.Job[sch.ID] = &RecordingJob{ID: sch.ID, DeviceID: sch.DeviceID}
	if !s.shouldKeepFetchProgramSchedule(sch, now) {
		t.Fatal("fetch schedule with an active task should be kept")
	}

	tasks.Job = map[uint64]*RecordingJob{}
	s.CurrentRecording[sch.DeviceID] = sch
	if !s.shouldKeepFetchProgramSchedule(sch, now) {
		t.Fatal("fetch schedule with current recording ownership should be kept")
	}
}

func TestShouldKeepFetchProgramScheduleKeepsFutureTask(t *testing.T) {
	oldJobs := tasks.Job
	defer func() {
		tasks.Job = oldJobs
	}()

	now := time.Date(2026, 8, 10, 18, 59, 0, 0, time.Local).Unix()
	sch := &RecordingSchedule{
		ID:        456,
		DeviceID:  "REC04",
		Validate:  true,
		Complete:  false,
		StartTime: time.Date(2026, 8, 11, 7, 0, 0, 0, time.Local).Unix(),
		EndTime:   time.Date(2026, 8, 11, 7, 30, 0, 0, time.Local).Unix(),
		IdleTime:  30,
		TailTime:  0,
	}
	s := &Scheduler{CurrentRecording: map[string]*RecordingSchedule{}}

	tasks.Job = map[uint64]*RecordingJob{
		sch.ID: {ID: sch.ID, DeviceID: sch.DeviceID},
	}

	if !s.shouldKeepFetchProgramSchedule(sch, now) {
		t.Fatal("future fetch schedule with an active task should be kept")
	}
}
