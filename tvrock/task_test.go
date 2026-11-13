package tvrock

import (
	"strings"
	"testing"
	"time"
)

func TestSanitizeRecordFilePath(t *testing.T) {
	path := SanitizeRecordFilePath(configure.RecorderSetting.FilenameEx, "record", `a<b>c:d"e/f\g|h?i* .`, configure.RecorderSetting.RecordFileExtension, "REC01", 0, 0)

	if strings.ContainsAny(path, `<>:"\|?*`) {
		t.Fatalf("path contains invalid filename characters: %s", path)
	}
	if strings.HasSuffix(path, " .ts") || strings.HasSuffix(path, "..ts") {
		t.Fatalf("path contains trailing filename space or dot: %s", path)
	}
}

func TestSanitizeRecordFilePathLength(t *testing.T) {
	path := SanitizeRecordFilePath(configure.RecorderSetting.FilenameEx, "record", strings.Repeat("a", maxRecordFilePathLength), configure.RecorderSetting.RecordFileExtension, "REC01", 0, 0)

	if len([]rune(path)) > maxRecordFilePathLength {
		t.Fatalf("path is too long: length=%d path=%s", len([]rune(path)), path)
	}
	if !strings.HasSuffix(path, ".ts") {
		t.Fatalf("path does not keep extension: %s", path)
	}
}

func TestUpdateModifiesExistingTaskWhenMaxTasksReached(t *testing.T) {
	oldTaskSetting := configure.TaskSetting
	configure.TaskSetting.MaxTasks = 1
	configure.TaskSetting.ExecTime = 1
	defer func() {
		configure.TaskSetting = oldTaskSetting
		if refreshTaskInterval.exec != nil {
			refreshTaskInterval.exec.Stop()
			refreshTaskInterval.exec = nil
		}
	}()

	now := Now().Unix()
	oldSchedule := &RecordingSchedule{
		ID:         12345,
		DeviceID:   "REC01",
		Title:      "old title",
		Validate:   true,
		RecordOnly: true,
		StartTime:  now + 600,
		EndTime:    now + 1800,
		IdleTime:   60,
		ReadyTime:  30,
		TailTime:   10,
	}
	job := &RecordingJob{
		DeviceID: oldSchedule.DeviceID,
		ID:       oldSchedule.ID,
		Title:    oldSchedule.Title,
		BootType: GetBootType(oldSchedule),
		Validate: oldSchedule.Validate,
		Com:      make(chan ScheduleJobCom, 16),
	}
	job.SetOffsetTimer(oldSchedule)

	tasks := &TaskArray{
		Job: map[uint64]*RecordingJob{
			oldSchedule.ID: job,
		},
	}

	updatedSchedule := *oldSchedule
	updatedSchedule.Title = "updated title"
	updatedSchedule.StartTime += 900
	updatedSchedule.EndTime += 900

	if err := tasks.Update([]*RecordingSchedule{&updatedSchedule}); err != nil {
		t.Fatalf("update task: %v", err)
	}

	wantStart := time.Unix(updatedSchedule.StartTime-updatedSchedule.ReadyTime, 0)
	wantStop := time.Unix(updatedSchedule.EndTime+updatedSchedule.TailTime, 0)
	if !job.RecordStartTime.Equal(wantStart) {
		t.Fatalf("record start time was not updated: got %s want %s", job.RecordStartTime, wantStart)
	}
	if !job.RecordStopTime.Equal(wantStop) {
		t.Fatalf("record stop time was not updated: got %s want %s", job.RecordStopTime, wantStop)
	}
	if job.Title != updatedSchedule.Title {
		t.Fatalf("title was not updated: got %q want %q", job.Title, updatedSchedule.Title)
	}
}

func TestUpdateSchedulesRefreshWhenMaxTasksReachedAndPendingTaskIsDue(t *testing.T) {
	oldTaskSetting := configure.TaskSetting
	configure.TaskSetting.MaxTasks = 1
	configure.TaskSetting.ExecTime = 60
	defer func() {
		configure.TaskSetting = oldTaskSetting
		if refreshTaskInterval.exec != nil {
			refreshTaskInterval.exec.Stop()
			refreshTaskInterval.exec = nil
		}
	}()

	now := Now().Unix()
	runningSchedule := &RecordingSchedule{
		ID:         12346,
		DeviceID:   "REC01",
		Title:      "running",
		Validate:   true,
		RecordOnly: true,
		StartTime:  now - 60,
		EndTime:    now + 1800,
		IdleTime:   60,
		ReadyTime:  30,
		TailTime:   10,
	}
	runningJob := &RecordingJob{
		DeviceID: runningSchedule.DeviceID,
		ID:       runningSchedule.ID,
		Title:    runningSchedule.Title,
		BootType: GetBootType(runningSchedule),
		Validate: runningSchedule.Validate,
		Com:      make(chan ScheduleJobCom, 16),
	}
	runningJob.SetOffsetTimer(runningSchedule)

	pendingSchedule := &RecordingSchedule{
		ID:         12347,
		DeviceID:   "REC02",
		Title:      "pending",
		Validate:   true,
		RecordOnly: true,
		StartTime:  now + 10,
		EndTime:    now + 1810,
		IdleTime:   60,
		ReadyTime:  30,
		TailTime:   10,
	}
	tasks := &TaskArray{
		Job: map[uint64]*RecordingJob{
			runningSchedule.ID: runningJob,
		},
	}

	if err := tasks.Update([]*RecordingSchedule{runningSchedule, pendingSchedule}); err != nil {
		t.Fatalf("update task: %v", err)
	}

	if refreshTaskInterval.exec == nil {
		t.Fatal("refresh task timer was not scheduled")
	}
	if _, ok := tasks.Get(pendingSchedule.ID); ok {
		t.Fatal("pending task was added despite max tasks limit")
	}
}

func TestUpdateInvalidatesExistingTaskBeforeDelete(t *testing.T) {
	now := Now().Unix()
	schedule := &RecordingSchedule{
		ID:         32346,
		DeviceID:   "REC01",
		Title:      "disabled",
		Validate:   true,
		RecordOnly: true,
		StartTime:  now + 600,
		EndTime:    now + 1800,
		IdleTime:   60,
		ReadyTime:  30,
		TailTime:   10,
	}
	job := &RecordingJob{
		DeviceID: schedule.DeviceID,
		ID:       schedule.ID,
		Title:    schedule.Title,
		BootType: GetBootType(schedule),
		Validate: true,
		Com:      make(chan ScheduleJobCom, 16),
	}
	job.SetOffsetTimer(schedule)

	tasks := &TaskArray{
		Job: map[uint64]*RecordingJob{
			schedule.ID: job,
		},
	}

	disabledSchedule := *schedule
	disabledSchedule.Validate = false

	if err := tasks.Update([]*RecordingSchedule{&disabledSchedule}); err != nil {
		t.Fatalf("update task: %v", err)
	}

	if job.Validate {
		t.Fatal("existing task validate flag was not updated before delete")
	}
}

func TestRecordingJobShouldCompleteOnlyAfterExitTime(t *testing.T) {
	now := Now()
	job := &RecordingJob{
		Validate: true,
		ExitTime: now.Add(time.Hour),
	}

	if job.shouldComplete(now) {
		t.Fatal("future job should not be marked complete")
	}

	if !job.shouldComplete(job.ExitTime) {
		t.Fatal("job should be marked complete at exit time")
	}

	job.Validate = false
	if job.shouldComplete(job.ExitTime.Add(time.Second)) {
		t.Fatal("invalid job should not be marked complete")
	}
}

func TestUpdatePrioritizesEarlierPendingTaskOverFutureTask(t *testing.T) {
	oldTaskSetting := configure.TaskSetting
	configure.TaskSetting.MaxTasks = 1
	configure.TaskSetting.ExecTime = 60
	defer func() {
		configure.TaskSetting = oldTaskSetting
		if refreshTaskInterval.exec != nil {
			refreshTaskInterval.exec.Stop()
			refreshTaskInterval.exec = nil
		}
	}()

	now := Now().Unix()
	farSchedule := &RecordingSchedule{
		ID:         22346,
		DeviceID:   "REC01",
		Title:      "far future",
		Validate:   true,
		RecordOnly: true,
		StartTime:  now + 7200,
		EndTime:    now + 9000,
		IdleTime:   60,
		ReadyTime:  30,
		TailTime:   10,
	}
	farJob := &RecordingJob{
		DeviceID: farSchedule.DeviceID,
		ID:       farSchedule.ID,
		Title:    farSchedule.Title,
		BootType: GetBootType(farSchedule),
		Validate: farSchedule.Validate,
		Com:      make(chan ScheduleJobCom, 16),
	}
	farJob.SetOffsetTimer(farSchedule)

	nearSchedule := &RecordingSchedule{
		ID:         22347,
		DeviceID:   "REC02",
		Title:      "near future",
		Validate:   true,
		RecordOnly: true,
		StartTime:  now + 600,
		EndTime:    now + 1800,
		IdleTime:   60,
		ReadyTime:  30,
		TailTime:   10,
	}
	tasks := &TaskArray{
		Job: map[uint64]*RecordingJob{
			farSchedule.ID: farJob,
		},
	}
	defer tasks.DeleteAll()

	if err := tasks.Update([]*RecordingSchedule{nearSchedule, farSchedule}); err != nil {
		t.Fatalf("update task: %v", err)
	}

	if _, ok := tasks.Get(nearSchedule.ID); !ok {
		t.Fatal("earlier pending task was not added")
	}
	if _, ok := tasks.Get(farSchedule.ID); ok {
		t.Fatal("far future task was kept instead of earlier pending task")
	}
}
