package tvrock

import (
	"context"
	"regexp"
	"strings"
)

type searchProgramKey struct {
	sid   uint
	eid   uint
	start int64
}

// キャプションは両方のキーワードフィールドに適用されます。「AND」条件はメタデータと
// キャプションの組み合わせ・あるいは同一プログラムに属する個別のキャプション行によって満たすことができます。
func (p *RockDB) matchProgramCaption(ctx context.Context, andSearch, fuzzySearch bool, title, content *regexp.Regexp, prg *TvProgram, exclusions ...searchExclusions) (bool, error) {
	var excluded searchExclusions
	if len(exclusions) > 0 {
		excluded = exclusions[0]
	}
	return p.matchProgramCaptionInDB(ctx, andSearch, fuzzySearch, title, content, prg, excluded, -1)
}

func (p *RockDB) matchProgramCaptionInDB(ctx context.Context, andSearch, fuzzySearch bool, title, content *regexp.Regexp, prg *TvProgram, excluded searchExclusions, targetDB int) (bool, error) {
	if excluded.matchesProgram(prg, fuzzySearch) {
		return false, nil
	}
	veto := false
	titleMatch := matchRegexpText(title, prg.Title, fuzzySearch)
	contentMatch := matchRegexpText(content, prg.Subtitle, fuzzySearch) || matchRegexpText(content, prg.Content, fuzzySearch)
	err := p.forEachCaptionStatement(ctx, prg.Sid, prg.Eid, prg.Start, func(text string) bool {
		if excluded.matchesCaption(text, fuzzySearch) {
			veto = true
			return false
		}
		if !titleMatch {
			titleMatch = matchRegexpText(title, text, fuzzySearch)
		}
		if !contentMatch {
			contentMatch = matchRegexpText(content, text, fuzzySearch)
		}
		return excluded.active() || !matchSearchTerms(andSearch, title, content, titleMatch, contentMatch)
	}, targetDB)
	return !veto && matchSearchTerms(andSearch, title, content, titleMatch, contentMatch), err
}

// CaptionTextはCaptionHistoryおよびCaptionHistoryWindowに従い、
// 表示フラグメントを連結してステートメントを構成します。
// クリア、PTSの変更、言語の変更は、ステートメントの区切りとなります。
// PTSはラップアラウンドやリセットが発生し得るため、
// 受信IDの順序を維持することが不可欠です。
func (p *RockDB) CaptionText(ctx context.Context, sid, eid uint, start int64) (string, error) {
	var text strings.Builder
	err := p.forEachCaptionStatement(ctx, sid, eid, start, func(statement string) bool {
		if text.Len() > 0 {
			text.WriteByte('\n')
		}
		text.WriteString(statement)
		return true
	})
	return text.String(), err
}

func (p *RockDB) forEachCaptionStatement(ctx context.Context, sid, eid uint, start int64, fn func(string) bool, targetDB ...int) error {
	dbs := p.Caption
	if len(targetDB) > 0 && targetDB[0] >= 0 && targetDB[0] < len(p.Caption) {
		dbs = p.Caption[targetDB[0] : targetDB[0]+1]
	}
	for _, db := range dbs {
		if db == nil {
			continue
		}
		rows, err := db.QueryContext(ctx, `SELECT COALESCE(cap, ''),pts,lang,clear FROM caption WHERE sid=? AND eid=? AND start=? ORDER BY id`, sid, eid, start)
		if err != nil {
			return err
		}
		var statement strings.Builder
		var previousPTS int64
		var previousLang uint
		boundary := true
		for rows.Next() {
			var text string
			var pts int64
			var lang uint
			var clear bool
			if err := rows.Scan(&text, &pts, &lang, &clear); err != nil {
				rows.Close()
				return err
			}
			boundary = boundary || clear
			if strings.TrimSpace(text) == "" {
				continue
			}
			if statement.Len() > 0 && (boundary || pts != previousPTS || lang != previousLang) {
				if !fn(statement.String()) {
					rows.Close()
					return nil
				}
				statement.Reset()
			}
			statement.WriteString(text)
			previousPTS, previousLang, boundary = pts, lang, false
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if statement.Len() > 0 && !fn(statement.String()) {
			return nil
		}
	}
	return nil
}
