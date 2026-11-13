package tvrock

import "testing"

func TestPFDetailsAreIncludedInCurrentProgramWithoutRepeatedUpdates(t *testing.T) {
	program := &TvProgram{Sid: 65537, Eid: 2345, Start: Now().Unix() - 100, End: Now().Unix() + 1800,
		Title: "番組", Subtitle: "概要", Content: "詳細"}
	if err := tvprogram.Insert(0, program); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rockDB.Program[0].Exec("DELETE FROM program WHERE sid=? AND eid=?", program.Sid, program.Eid) })
	device := &RockDevice{}
	pf := *program
	pf.Subtitle, pf.Content = "", ""
	if _, err := device.UpdatePFRecordings(&pf, &TvProgram{}); err != nil {
		t.Fatal(err)
	}
	if device.CurrentProgram.Subtitle != "概要" || device.CurrentProgram.Content != "詳細" {
		t.Fatalf("current program is missing details: %+v", device.CurrentProgram)
	}
	repeated := *program
	repeated.Subtitle, repeated.Content = "", ""
	changed, err := device.UpdatePFRecordings(&repeated, &TvProgram{})
	if err != nil || changed {
		t.Fatalf("repeated PF causes unnecessary update: %v, %v", changed, err)
	}
	updated := *program
	updated.Content = "更新された詳細"
	if _, err := device.UpdatePFRecordings(&updated, &TvProgram{}); err != nil {
		t.Fatal(err)
	}
	stored, err := tvprogram.Get(0, program.Sid, program.Eid)
	if err != nil || len(stored) != 1 || stored[0].Content != updated.Content {
		t.Fatalf("updated PF details not stored: %v, %v", stored, err)
	}
}

func TestPFDetailsDoNotCarryOverToOtherBroadcasts(t *testing.T) {
	source := &TvProgram{Sid: 1, Eid: 2, Start: 100, End: 200, Subtitle: "概要", Content: "詳細"}
	for _, pf := range []TvProgram{
		{Sid: 2, Eid: 2, Start: 100, End: 200},
		{Sid: 1, Eid: 3, Start: 100, End: 200},
		{Sid: 1, Eid: 2, Start: 200, End: 300},
	} {
		inheritPFDetails(&pf, source)
		if pf.Subtitle != "" || pf.Content != "" {
			t.Fatalf("details carried over to another broadcast: %+v", pf)
		}
	}
}
