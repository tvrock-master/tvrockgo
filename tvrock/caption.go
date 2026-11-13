package tvrock

import (
	"context"
	"encoding/json"
	"strings"

	_ "modernc.org/sqlite"
)

type CaptionHistoryItem struct {
	ID       int64                   `json:"ID"`
	Text     string                  `json:"Text"`
	Lang     uint                    `json:"Lang"`
	Pts      int64                   `json:"Pts"`
	Segments []CaptionHistorySegment `json:"Segments,omitempty"`
}

type CaptionHistorySegment struct {
	Text      string `json:"Text"`
	Color     uint   `json:"Color"`
	ColorName string `json:"ColorName"`
}

// CaptionLayoutはブロードキャスト・ピクセル座標を使用します。
// サイズ情報だけではルビやCSIの配置を表現できません。
type CaptionLayout struct {
	X           float64 `json:"X"`
	Y           float64 `json:"Y"`
	CellWidth   float64 `json:"CellWidth"`
	CellHeight  float64 `json:"CellHeight"`
	GlyphWidth  float64 `json:"GlyphWidth"`
	GlyphHeight float64 `json:"GlyphHeight"`
	Cells       uint    `json:"Cells"`
}

type PacketCaption struct {
	HistoryID int64          `json:"HistoryID,omitempty"`
	Timing    uint           `json:"Timing,omitempty"`
	DelayMs   uint           `json:"DelayMs,omitempty"`
	Layout    *CaptionLayout `json:"Layout,omitempty"`
	Pid       uint           `json:"Pid"`
	Pts       int64          `json:"Pts"`
	Lang      uint           `json:"Lang"`
	Clear     bool           `json:"Clear"`
	X         int            `json:"X"`
	Y         int            `json:"Y"`
	Color     uint           `json:"Color"`
	ColorName string         `json:"ColorName"`
	BgColor   uint           `json:"BgColor"`
	Size      string         `json:"Size"`
	Text      string         `json:"Text"`
}

type CaptionInfo struct {
	DeviceID string `json:"DeviceID"`
	Sid      uint   `json:"Sid"`
	Eid      uint   `json:"Eid"`
	Start    int64  `json:"Start"`
	End      int64  `json:"End"`
	PacketCaption
}

// IDはブロードキャストのPTSがラップまたはリセットされる場合を含め受信順序を保持します。
// 既存の (sid, eid, start) インデックスにはSQLiteのrowid (id) が含まれています。
func (p *RockDB) CaptionHistory(ctx context.Context, sid, eid uint, start, after int64, limit int, snapshot ...int64) ([]CaptionHistoryItem, error) {
	items := make([]CaptionHistoryItem, 0)
	for _, db := range p.Caption {
		if db == nil {
			continue
		}
		var exists bool
		if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM caption WHERE sid=? AND eid=? AND start=?)`, sid, eid, start).Scan(&exists); err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		until := int64(1<<63 - 1)
		if len(snapshot) > 0 {
			until = snapshot[0]
		}
		rows, err := db.QueryContext(ctx, `SELECT id,cap,lang,pts,clear,COALESCE(size, ''),COALESCE(color, 7),COALESCE(colorname, '') FROM caption WHERE sid=? AND eid=? AND start=? AND id>? AND id<=? ORDER BY id`, sid, eid, start, after, until)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var previousPTS int64
		boundary := true
		for rows.Next() {
			var item CaptionHistoryItem
			var pts int64
			var clear bool
			var size string
			var segment CaptionHistorySegment
			if err := rows.Scan(&item.ID, &item.Text, &item.Lang, &pts, &clear, &size, &segment.Color, &segment.ColorName); err != nil {
				return nil, err
			}
			item.Pts = pts
			// 「clear」は新しいステートメントの開始を示しますが履歴を削除することはありません。
			// 同じPTS（提示タイムスタンプ）と言語を持つテキストや色の断片は同一のステートメントに属します。
			boundary = boundary || clear
			if size == "small" || size == "micro" {
				// 末尾のルビを消費することで、ページネーションやライブ継続処理が
				// 最後のフラグメントIDを使用するようにする（そのテキスト自体は履歴から隠されているが）。
				if !boundary && pts == previousPTS && items[len(items)-1].Lang == item.Lang {
					items[len(items)-1].ID = item.ID
				}
				continue
			}
			if strings.TrimSpace(item.Text) == "" {
				continue
			}
			segment.Text = item.Text
			item.Segments = []CaptionHistorySegment{segment}
			if boundary || pts != previousPTS || items[len(items)-1].Lang != item.Lang {
				if len(items) >= limit {
					return items, nil
				}
				items = append(items, item)
			} else {
				last := &items[len(items)-1]
				last.Text += item.Text
				last.Segments = append(last.Segments, segment)
				last.ID = item.ID // Resume after the entire statement, not its first fragment.
			}
			previousPTS = pts
			boundary = false
		}
		return items, rows.Err()
	}
	return items, nil
}

func (p *RockDB) CaptionSnapshot(ctx context.Context, sid, eid uint, start int64) (int64, error) {
	for _, db := range p.Caption {
		if db == nil {
			continue
		}
		var id int64
		if err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(id),0) FROM caption WHERE sid=? AND eid=? AND start=?`, sid, eid, start).Scan(&id); err != nil {
			return 0, err
		}
		if id > 0 {
			return id, nil
		}
	}
	return 0, nil
}

func (p *RockDB) InsertCaption(no int, sid, eid uint, start int64, cap *PacketCaption) error {
	tx, err := p.GetCaption(no).Begin()
	if err != nil {
		return err
	}

	result, err := tx.Exec(
		`REPLACE INTO caption (sid,eid,start,pts,lang,clear,x,y,color,colorname,bgcolor,size,cap) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		sid,
		eid,
		start,
		cap.Pts,
		cap.Lang,
		cap.Clear,
		cap.X,
		cap.Y,
		cap.Color,
		cap.ColorName,
		cap.BgColor,
		cap.Size,
		cap.Text,
	)
	if err != nil {
		tx.Rollback()
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	cap.HistoryID = id

	return nil
}

func SendCaption(deviceID string, tvp *TvProgram, cap *PacketCaption) error {
	cinfo := CaptionInfo{
		DeviceID:      deviceID,
		Sid:           tvp.Sid,
		Eid:           tvp.Eid,
		Start:         tvp.Start,
		End:           tvp.End,
		PacketCaption: *cap,
	}

	js, err := json.Marshal(&cinfo)
	if err != nil {
		return err
	}

	deviceController.Notify(ControllerSendCaption, string(js))

	return nil
}
