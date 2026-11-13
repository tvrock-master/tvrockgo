package tvrock

import "testing"

func TestReadChFileRecognizesTuningSpaceNames(t *testing.T) {
	for _, tc := range []struct {
		header string
		want   TuningSpace
	}{
		{";#SPACE(0,UHF)", TuningSpaceTS},
		{";#SPACE(3,地デジ(UHF))", TuningSpaceTS},
		{";#SPACE(2,地上D)", TuningSpaceTS},
		{";#SPACE(0,VHF)", TuningSpaceTS},
		{";#SPACE(1,catv)", TuningSpaceTS},
		{" ;#space( 4, uhf ) \r", TuningSpaceTS},
		{";#SPACE(4,BS)", TuningSpaceBS},
		{";#SPACE(2,BonDriver bs)", TuningSpaceBS},
		{";#SPACE(0,CS110)", TuningSpaceCS},
		{";#SPACE(7,cs)", TuningSpaceCS},
		{";#SPACE(0,UHF BS)", TuningSpaceTS},
		{";#SPACE(1,BS CS)", TuningSpaceBS},
		{";#SPACE(9,Unknown)", TuningSpaceNone},
		{";#SPACE(9,)", TuningSpaceNone},
	} {
		t.Run(tc.header, func(t *testing.T) {
			local := Station{Cache: map[uint]*TvStation{}}
			got, err := local.ReadChFile(tc.header, TuningSpaceBS)
			if err != nil || got != tc.want {
				t.Fatalf("ReadChFile = %d, %v; want %d", got, err, tc.want)
			}
			// 名前から確定した種別で局を登録し、未知なら直前のBSへ混入させない。
			next, err := local.ReadChFile("テスト局,0,14,1,1,1024,32736,32736,1", got)
			if err != nil || next != got {
				t.Fatalf("service row = %d, %v", next, err)
			}
			if tc.want == TuningSpaceNone {
				if len(local.Cache) != 0 {
					t.Fatal("station registered without a known tuning space")
				}
			} else if st, ok := local.Get(uint(tc.want)<<16 | 1024); !ok || st.Ch != 14 {
				t.Fatal("station was not registered under its tuning space")
			}
		})
	}
}

func TestReadChFilePreservesTuningSpaceOnCommentsAndDisabledRows(t *testing.T) {
	local := Station{Cache: map[uint]*TvStation{}}
	for _, line := range []string{"", "; comment", "# comment", "局,0,14,1,1,1024,32736,32736,0"} {
		if got, err := local.ReadChFile(line, TuningSpaceCS); err != nil || got != TuningSpaceCS {
			t.Fatalf("line %q: got %d, %v", line, got, err)
		}
	}
	if len(local.Cache) != 0 {
		t.Fatal("non-service row registered a station")
	}
}

func TestReadChFileRejectsInvalidSpaceIndex(t *testing.T) {
	local := Station{Cache: map[uint]*TvStation{}}
	for _, header := range []string{";#SPACE(x,UHF)", ";#SPACE(-1,BS)", ";#SPACE(100,CS)", ";#SPACE(0)"} {
		if _, err := local.ReadChFile(header, TuningSpaceTS); err == nil {
			t.Errorf("invalid header accepted: %s", header)
		}
	}
}
