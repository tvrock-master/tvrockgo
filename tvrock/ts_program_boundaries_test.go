package tvrock

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"testing"
	"time"
)

func testPresentSection(sid, event uint16, following bool) []byte {
	section := make([]byte, 30)
	section[0] = 0x4e
	section[1] = 0xb0
	section[2] = 27
	binary.BigEndian.PutUint16(section[3:5], sid)
	section[5] = 0xc1
	if following {
		section[6] = 1
	}
	section[7] = 1
	binary.BigEndian.PutUint16(section[8:10], 1)
	binary.BigEndian.PutUint16(section[10:12], 1)
	section[12] = 1
	section[13] = 0x4e
	binary.BigEndian.PutUint16(section[14:16], event)
	section[16] = 0xea
	section[17] = 0x60
	section[18] = byte(event % 24)
	section[22] = 1
	section[24] = 0x80
	binary.BigEndian.PutUint32(section[len(section)-4:], tsSectionCRC(section[:len(section)-4]))
	return section
}

func testSectionPacket(section []byte, pid uint16, cc byte) []byte {
	packet := bytes.Repeat([]byte{0xff}, 188)
	packet[0] = 0x47
	packet[1] = 0x40 | byte(pid>>8)
	packet[2] = byte(pid)
	packet[3] = 0x10 | cc&15
	packet[4] = 0
	copy(packet[5:], section)
	return packet
}

func TestProgramBoundaryOnlyUsesSelectedPresentEvent(t *testing.T) {
	metrics := tsAnalysisMetrics{ServiceID: 1, TransportStreamID: 1}
	scanner := newTSMetadataScanner(&metrics, uint(metrics.ServiceID))
	cc := byte(0)
	send := func(section []byte, at time.Duration, hasPCR bool) {
		scanner.observe(testSectionPacket(section, 0x12, cc), int64(cc)*188, at, 0, hasPCR)
		cc++
	}
	send(testPresentSection(1, 10, false), 0, false)         // initial present is a baseline, even before the first PCR
	send(testPresentSection(1, 11, true), time.Second, true) // f is not p
	send(testPresentSection(2, 99, false), 2*time.Second, true)
	otherTS := testPresentSection(1, 99, false)
	otherTS[9] = 2
	binary.BigEndian.PutUint32(otherTS[len(otherTS)-4:], tsSectionCRC(otherTS[:len(otherTS)-4]))
	send(otherTS, 2*time.Second, true)
	future := testPresentSection(1, 99, false)
	future[5] &= 0xfe
	binary.BigEndian.PutUint32(future[len(future)-4:], tsSectionCRC(future[:len(future)-4]))
	send(future, 2*time.Second, true)
	bad := testPresentSection(1, 99, false)
	bad[len(bad)-1] ^= 1
	send(bad, 2*time.Second, true)
	correction := testPresentSection(1, 10, false)
	correction[5] = 0xc3
	binary.BigEndian.PutUint32(correction[len(correction)-4:], tsSectionCRC(correction[:len(correction)-4]))
	send(correction, 3*time.Second, true)
	send(testPresentSection(1, 11, false), 5*time.Second, true)
	send(testPresentSection(1, 11, false), 6*time.Second, true)
	send(testPresentSection(1, 10, false), 7*time.Second, true) // stale repeated p section
	send(testPresentSection(1, 12, false), 10*time.Second, true)
	scanner.finish(20 * time.Second)
	rows := metrics.ProgramBoundaries
	if len(rows) != 2 || rows[0].TimeNS != int64(5*time.Second) || rows[0].PreviousEventID != 10 || rows[0].EventID != 11 || rows[1].EventID != 12 {
		t.Fatalf("bad p/f boundaries: %+v", rows)
	}
	if len(metrics.Candidates) != 0 || metrics.CMStatus != "not-signaled" {
		t.Fatal("program changes were classified as CM")
	}
}

func TestProgramBoundaryFragmentedSectionsAndEITPIDPreference(t *testing.T) {
	metrics := tsAnalysisMetrics{ServiceID: 1, TransportStreamID: 1}
	scanner := newTSMetadataScanner(&metrics, uint(metrics.ServiceID))
	scanner.observe(testSectionPacket(testPresentSection(1, 10, false), 0x27, 0), 0, 0, 0, true)
	scanner.observe(testSectionPacket(testPresentSection(1, 11, false), 0x12, 0), 188, time.Second, 0, true) // new PID baseline
	scanner.observe(testSectionPacket(testPresentSection(1, 10, false), 0x27, 1), 376, 2*time.Second, 0, true)
	section := testPresentSection(1, 12, false)
	section = append(append(append([]byte{}, section[:26]...), 0x80, 200), make([]byte, 204)...)
	length := len(section) - 3
	section[1] = 0xb0 | byte(length>>8)
	section[2] = byte(length)
	section[24] = 0x80
	section[25] = 202
	binary.BigEndian.PutUint32(section[len(section)-4:], tsSectionCRC(section[:len(section)-4]))
	scanner.observe(testSectionPacket(section, 0x12, 1), 564, 5*time.Second, 0, true)
	rest := bytes.Repeat([]byte{0xff}, 188)
	copy(rest, []byte{0x47, 0, 0x12, 0x12})
	copy(rest[4:], section[183:])
	scanner.observe(rest, 752, 6*time.Second, 0, true)
	scanner.finish(10 * time.Second)
	if rows := metrics.ProgramBoundaries; len(rows) != 1 || rows[0].EventID != 12 || rows[0].PreviousEventID != 11 || rows[0].TimeNS != int64(6*time.Second) {
		t.Fatalf("fragment/PID preference: %+v", rows)
	}
}

func TestProgramBoundaryIndexAndCacheRoundTrip(t *testing.T) {
	schedule := tsAnalysisTestSchedule(t)
	base := playbackTestTS(t)
	// Read the selected PAT identity instead of assuming the muxer's default TSID.
	file, _ := os.Open(schedule.FileName)
	initial, err := indexPlaybackTS(context.Background(), file, 1)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	section := func(id uint16) []byte {
		row := testPresentSection(initial.metrics.ServiceID, id, false)
		binary.BigEndian.PutUint16(row[8:10], initial.metrics.TransportStreamID)
		binary.BigEndian.PutUint32(row[len(row)-4:], tsSectionCRC(row[:len(row)-4]))
		return row
	}
	var data bytes.Buffer
	data.Write(testSectionPacket(section(10), 0x12, 0))
	inserted := false
	for at := 0; at < len(base); at += 188 {
		packet := base[at : at+188]
		data.Write(packet)
		if pcr, ok := playbackPCR(packet, initial.pcrPID); ok && pcr >= 27000000 && !inserted {
			data.Write(testSectionPacket(section(11), 0x12, 1))
			inserted = true
		}
	}
	if err = os.WriteFile(schedule.FileName, data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	file, err = os.Open(schedule.FileName)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	index, err := analyzeRecordedTS(context.Background(), file, schedule, CurrentSlot, false)
	if err != nil {
		t.Fatal(err)
	}
	identity, _ := identifyTSFile(file)
	cached, err := readTSAnalysis(context.Background(), rockDB.GetData(CurrentSlot), schedule, identity)
	if err != nil {
		t.Fatal(err)
	}
	if rows := programPlaybackBoundaries(cached); len(rows) != 1 || rows[0].PositionMS != 1000 || rows[0].EventID != 11 || len(index.metrics.Candidates) != 0 {
		t.Fatalf("program boundary lost in index/cache: %+v", rows)
	}
}
