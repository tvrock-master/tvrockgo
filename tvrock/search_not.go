package tvrock

import (
	"fmt"
	"regexp"
	"strings"
)

// 大文字の「NOT」が肯定パターンと除外パターンを区切ります。
// 空白や大文字・小文字の区別を取り除く「ファジー正規化」の前に分割を行います。
var searchNotSeparator = regexp.MustCompile(`(?:^|[\s\p{Z}]+)NOT(?:[\s\p{Z}]+|$)`)

type searchExclusions struct {
	title   *regexp.Regexp
	content *regexp.Regexp
}

func compileSearchPattern(text string, fuzzy bool) (include, exclude *regexp.Regexp, err error) {
	positive, negative := text, ""
	if loc := searchNotSeparator.FindStringIndex(text); loc != nil {
		positive = strings.TrimSpace(text[:loc[0]])
		negative = strings.TrimSpace(text[loc[1]:])
		if positive == "" || negative == "" {
			return nil, nil, fmt.Errorf("NOT の前後に検索条件を指定してください")
		}
	}
	compile := func(pattern string) (*regexp.Regexp, error) {
		if fuzzy {
			pattern = fuzzySearchPattern(pattern)
		}
		if pattern == "" {
			return nil, nil
		}
		return regexp.Compile(pattern)
	}
	include, err = compile(positive)
	if err != nil {
		return nil, nil, err
	}
	exclude, err = compile(negative)
	return
}

func (e searchExclusions) active() bool {
	return e.title != nil || e.content != nil
}

func (e searchExclusions) matchesProgram(prg *TvProgram, fuzzy bool) bool {
	return matchRegexpText(e.title, prg.Title, fuzzy) ||
		matchRegexpText(e.content, prg.Subtitle, fuzzy) ||
		matchRegexpText(e.content, prg.Content, fuzzy)
}

func (e searchExclusions) matchesCaption(text string, fuzzy bool) bool {
	return matchRegexpText(e.title, text, fuzzy) || matchRegexpText(e.content, text, fuzzy)
}
