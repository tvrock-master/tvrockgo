package tvrock

import (
	"regexp"
	"testing"
)

func TestFuzzyApproximateTextMatch(t *testing.T) {
	keyword := fuzzySearchText("\u30d7\u30ea\u30ad\u30e5\u30a2")

	tests := []struct {
		name string
		text string
		want bool
	}{
		{
			name: "exact",
			text: fuzzySearchText("\u30d7\u30ea\u30ad\u30e5\u30a2"),
			want: true,
		},
		{
			name: "one-character-different",
			text: fuzzySearchText("\u30d7\u30ea\u30ad\u30a2"),
			want: true,
		},
		{
			name: "embedded",
			text: fuzzySearchText("\u540d\u63a2\u5075\u30d7\u30ea\u30ad\u30e5\u30a2"),
			want: true,
		},
		{
			name: "less-than-75-percent",
			text: fuzzySearchText("\u30ad\u30e5\u30a2"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fuzzyApproximateTextMatch(keyword, tt.text); got != tt.want {
				t.Fatalf("fuzzyApproximateTextMatch() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatchRegexpTextFuzzyApproximate(t *testing.T) {
	re := regexp.MustCompile(fuzzySearchText("\u30d7\u30ea\u30ad\u30e5\u30a2"))
	if !matchRegexpText(re, "\u30d7\u30ea\u30ad\u30a2", true) {
		t.Fatal("expected 75 percent fuzzy match")
	}
}

func TestFuzzySearchQuestionMark(t *testing.T) {
	tests := []struct {
		name    string
		keyword string
		text    string
		want    bool
	}{
		{"middle", "どうして？教えて", "どうして？教えて", true},
		{"leading", "？なぜ", "？なぜ", true},
		{"repeated", "？？", "？？", true},
		{"width", "何？", "何?", true},
		{"literal", "何？", "何", false},
		{"literal-question", "？", "別の番組", false},
		{"other-fullwidth-symbols", "（特集）＋", "（特集）＋", true},
		{"regexp-optional", "^ab?c$", "ＡＣ", true},
		{"regexp-alternative", "猫|犬", "犬の番組", true},
		{"escaped-question", `何\?`, "何？", true},
		{"mixed-regexp", "^どうして？教えて$", "どうして？教えて", true},
		{"mixed-regexp-literal", "^何？$", "何", false},
		{"escaped-fullwidth-question", `何\？`, "何？", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re, err := regexp.Compile(fuzzySearchPattern(tt.keyword))
			if err != nil {
				t.Fatal(err)
			}
			if got := matchRegexpText(re, tt.text, true); got != tt.want {
				t.Fatalf("matchRegexpText(%q, %q) = %v, want %v", tt.keyword, tt.text, got, tt.want)
			}
		})
	}
}

func TestMatchProgramTextQuestionMark(t *testing.T) {
	re := regexp.MustCompile(fuzzySearchPattern("どうして？教えて"))
	for _, prg := range []*TvProgram{
		{Title: "どうして？教えて"},
		{Subtitle: "どうして？教えて"},
		{Content: "どうして？教えて"},
	} {
		if !matchProgramText(false, true, re, re, prg) {
			t.Fatalf("expected question mark to match in program: %+v", prg)
		}
	}
	if !matchRegexpText(regexp.MustCompile("どうして？教えて"), "どうして？教えて", false) {
		t.Fatal("expected literal match with fuzzy search disabled")
	}
}
