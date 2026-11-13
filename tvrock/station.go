package tvrock

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

type TuningSpace uint

const (
	TuningSpaceNone TuningSpace = iota
	TuningSpaceTS
	TuningSpaceBS
	TuningSpaceCS
	TuningSpaceOS
	TuningSpaceReserved1
	TuningSpaceReserved2
	TuningSpaceReserved3
	SubChannnelCode = 8
	TuningSpace4K   = 16
)

const (
	TERRESTRIAL_CHANNEL_FIRST uint = 13
	TERRESTRIAL_CHANNEL_LAST  uint = 63
)

type FrequencyTSID struct {
	Freq uint `json:"freq"`
	TSID uint `json:"tsid"`
	TSP  uint `json:"tsp"`
}

type TvStation struct {
	Sid          uint   `json:"Sid"`
	Tsid         uint   `json:"Tsid"`
	Ch           uint   `json:"Ch"`
	Tsp          uint   `json:"Tsp"`
	No           uint   `json:"No"`
	Name         string `json:"Name"`
	View         uint   `json:"View"`
	Parent       uint   `json:"Parent"`
	FetchProgram bool   `json:"FetchProgram"`
	HashTag      string `json:"HashTag"`
	Update       bool   `json:"Update"`
}

// ServiceGroupは放送種別と放送波の16ビットSIDで指定する親子関係。
type ServiceGroup struct {
	Tsp      TuningSpace `yaml:"tsp"`
	Parent   uint        `yaml:"parent"`
	Children []uint      `yaml:"children"`
}

type Station struct {
	Cache map[uint]*TvStation
	db    *RockDB
	sync.RWMutex
}

var station Station

func (p *Station) Initialize(db *RockDB) error {
	p.db = db

	// 局情報
	var err error
	p.Cache, err = p.GetStations(0)
	if err != nil {
		return err
	}

	no := p.getStationFreeNo()
	for _, val := range p.Cache {
		if val.No == 0 {
			val.No = no
			val.Update = true
			no++
		}
	}

	p.updateServiceParents()
	p.UpdateStation()

	return nil
}

func (p *Station) Get(sid uint) (*TvStation, bool) {
	p.Lock()
	defer p.Unlock()
	ret, ok := p.Cache[sid]
	return ret, ok
}

func (p *Station) UpdateStation() (int, error) {
	n := 0
	p.Lock()
	defer p.Unlock()
	for _, val := range p.Cache {
		if val.Update {
			if err := p.ReplaceStation(CurrentSlot, val); err != nil {
				return n, err
			}
			n++
		}
	}
	return n, nil
}

func (p *Station) InsertStation(no int, tvs *TvStation) error {
	tx, err := p.db.GetData(no).Begin()
	if err != nil {
		return err
	}
	_, err = tx.Exec(
		`INSERT INTO station (sid,tsid,ch,tsp,no,name,view,parent,fetchprogram,hashtag) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		tvs.Sid,
		tvs.Tsid,
		tvs.Ch,
		tvs.Tsp,
		tvs.No,
		tvs.Name,
		tvs.View,
		tvs.Parent,
		tvs.FetchProgram,
		tvs.HashTag,
	)
	if err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit()
}

func (p *Station) ReplaceStation(no int, tvs *TvStation) error {

	tx, err := p.db.GetData(no).Begin()
	if err != nil {
		return err
	}
	_, err = tx.Exec(
		`REPLACE INTO station (sid,tsid,ch,tsp,no,name,view,parent,fetchprogram,hashtag) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		tvs.Sid&0xffff,
		tvs.Tsid,
		tvs.Ch,
		tvs.Tsp,
		tvs.No,
		tvs.Name,
		tvs.View,
		tvs.Parent,
		tvs.FetchProgram,
		tvs.HashTag,
	)
	if err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit()
}

func (p *Station) GetStations(no int) (map[uint]*TvStation, error) {
	rows, err := p.db.GetData(no).Query("SELECT sid,tsid,ch,tsp,no,name,view,parent,fetchprogram,hashtag FROM station")

	list := map[uint]*TvStation{}
	if err == sql.ErrNoRows {
		return list, nil
	} else if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		sta := TvStation{}
		err = rows.Scan(&sta.Sid, &sta.Tsid, &sta.Ch, &sta.Tsp, &sta.No, &sta.Name, &sta.View, &sta.Parent, &sta.FetchProgram, &sta.HashTag)
		if err != nil {
			return nil, err
		}
		sta.Sid = sta.Tsp<<16 | sta.Sid
		list[sta.Sid] = &sta
	}

	return list, nil
}

func (p *Station) GetStationList() map[uint]*TvStation {
	p.Lock()
	defer p.Unlock()

	list := map[uint]*TvStation{}
	for _, val := range p.Cache {
		list[val.Sid] = val
	}
	return list
}

func (p *Station) getStationFreeNo() uint {
	var no uint = 0
	for _, val := range p.Cache {
		if no < val.No {
			no = val.No
		}
	}
	return (no + 1)
}

func (p *Station) GetValidServiceID() []uint {
	p.Lock()
	defer p.Unlock()

	list := []uint{}
	for _, val := range p.Cache {
		if val.View == 0 {
			continue
		}
		list = append(list, val.Sid)
	}

	return list
}

func (p *Station) GetStationsByTSID(tsid uint) []*TvStation {
	p.Lock()
	defer p.Unlock()

	list := []*TvStation{}
	for _, val := range p.Cache {
		if val.Tsid != tsid {
			continue
		}
		list = append(list, val)
	}

	return list
}

// GetServiceIDByChannelCode はchコードから地上波のService-IDを検索します
// chが+13されている場合があることを考慮してください
func (p *Station) GetServiceIDByChannelCode(ch uint) uint {
	p.Lock()
	defer p.Unlock()

	var selected *TvStation
	for _, val := range p.Cache {
		if val.Tsp == uint(TuningSpaceTS) && val.View != 0 && val.Ch == ch {
			isParent := val.Parent == 0 || val.Parent == val.Sid
			selectedIsParent := selected != nil && (selected.Parent == 0 || selected.Parent == selected.Sid)
			if selected == nil || (isParent && !selectedIsParent) || (isParent == selectedIsParent && val.Sid < selected.Sid) {
				selected = val
			}
		}
	}
	if selected != nil {
		return selected.Sid
	}
	return 0
}

func (p *Station) ModifyStation(tvs *TvStation) {
	p.Lock()
	defer p.Unlock()

	val, ok := p.Cache[tvs.Sid]
	if ok {
		if val.Name != tvs.Name ||
			val.Tsid != tvs.Tsid ||
			val.Ch != tvs.Ch ||
			val.Tsp != tvs.Tsp ||
			val.Parent != tvs.Parent ||
			val.View != tvs.View ||
			val.FetchProgram != tvs.FetchProgram ||
			val.HashTag != tvs.HashTag {
			val.Name = tvs.Name
			val.Tsid = tvs.Tsid
			val.Ch = tvs.Ch
			val.Tsp = tvs.Tsp
			val.Parent = tvs.Parent
			val.View = tvs.View
			val.FetchProgram = tvs.FetchProgram
			val.HashTag = tvs.HashTag
			val.Sid = (val.Tsp << 16) | (val.Sid & 0xffff)
			val.Update = true
		}
	} else {
		tvs.Update = true
		tvs.No = p.getStationFreeNo()
		p.Cache[tvs.Sid] = tvs
	}
}

func (p *Station) ModifyStation2(tvs *TvStation) {
	p.Lock()
	defer p.Unlock()

	val, ok := p.Cache[tvs.Sid]
	if ok {
		if val.Name != tvs.Name ||
			val.Tsid != tvs.Tsid ||
			val.Ch != tvs.Ch ||
			val.Tsp != tvs.Tsp ||
			val.Parent != tvs.Parent {
			val.Name = tvs.Name
			val.Tsid = tvs.Tsid
			val.Ch = tvs.Ch
			val.Tsp = tvs.Tsp
			val.Parent = tvs.Parent
			val.Update = true
		}
	} else {
		tvs.Update = true
		tvs.No = p.getStationFreeNo()
		p.Cache[tvs.Sid] = tvs
	}
}

// .ch2ファイルを1行読み込み、次の行で使用する放送種別を返す。
func (p *Station) ReadChFile(chtxt string, tsp TuningSpace) (TuningSpace, error) {
	header := strings.TrimSpace(chtxt)
	if strings.HasPrefix(strings.ToUpper(header), ";#SPACE(") && strings.HasSuffix(header, ")") {
		index, name, ok := strings.Cut(header[len(";#SPACE("):len(header)-1], ",")
		space, err := strconv.Atoi(strings.TrimSpace(index))
		if !ok || err != nil || space < 0 || space >= 100 {
			return TuningSpaceNone, fmt.Errorf("invalid .ch2 tuning space: %s", header)
		}
		// 番号はBonDriverの空間インデックス。放送種別は視聴アプリと同じ優先順で名前から判定する。
		name = strings.ToUpper(strings.TrimSpace(name))
		switch {
		case strings.Contains(name, "地"), strings.Contains(name, "VHF"), strings.Contains(name, "UHF"), strings.Contains(name, "CATV"):
			return TuningSpaceTS, nil
		case strings.Contains(name, "BS"):
			return TuningSpaceBS, nil
		case strings.Contains(name, "CS"):
			return TuningSpaceCS, nil
		default:
			return TuningSpaceNone, nil
		}
	}
	if len(chtxt) == 0 {
		return tsp, nil
	}
	if strings.HasPrefix(header, ";") || strings.HasPrefix(header, "#") || tsp == TuningSpaceNone {
		return tsp, nil
	}

	chtxt = strings.Replace(chtxt, "&#010;", "", -1)
	chtxt = strings.Replace(chtxt, "&#013;", "", -1)
	chdat := strings.Split(chtxt, ",")
	if len(chdat) < 9 {
		return tsp, nil
	}

	// ヘッダー行を無視する
	if chdat[2] == "チャンネル" || chdat[5] == "サービスID" {
		return tsp, nil
	}

	// 無効
	valid, err := strconv.ParseUint(chdat[8], 10, 32)
	if err == nil && valid == 0 {
		return tsp, err
	}

	ch, err := strconv.ParseUint(chdat[2], 10, 32)
	if err != nil {
		return tsp, err
	}

	sid, err := strconv.ParseUint(chdat[5], 10, 32)
	if err != nil {
		return tsp, err
	}

	tsid, err := strconv.ParseUint(chdat[7], 10, 32)
	if err != nil {
		return tsp, err
	}

	localSID := uint(tsp)<<16 | uint(sid)
	par := uint(0)
	// 登録済みの関係を保持する。全行読み込み後にupdateServiceParentsで再判定する。
	if sta, ok := p.Get(localSID); ok && sta.Parent != localSID {
		par = sta.Parent
	}

	tvs := TvStation{
		Sid:     localSID,
		Tsid:    uint(tsid),
		Ch:      uint(ch),
		Tsp:     uint(tsp),
		Parent:  par,
		Name:    chdat[0],
		HashTag: "",
		View:    1,
	}

	p.ModifyStation2(&tvs)

	return tsp, nil
}

func (p *Station) GetDeviceIDOrderList(sid uint) ([]string, error) {
	p.Lock()
	defer p.Unlock()

	val, ok := p.Cache[sid]
	if ok {
		switch TuningSpace(val.Tsp) {
		case TuningSpaceTS:
			return configure.RecordingDevice.OrderTS, nil
		case TuningSpaceBS:
			return configure.RecordingDevice.OrderBS, nil
		case TuningSpaceCS:
			return configure.RecordingDevice.OrderCS, nil
		case TuningSpaceOS:
			return configure.RecordingDevice.OrderOS, nil
		default:
			return nil, errors.New("unknown tuning space")
		}
	}
	return nil, fmt.Errorf("unknown service %d", sid)
}

func (p *Station) GetFetchProgramDeviceIDOrderList(sid uint) ([]string, error) {
	p.Lock()
	defer p.Unlock()

	val, ok := p.Cache[sid]
	if ok {
		switch TuningSpace(val.Tsp) {
		case TuningSpaceTS:
			return configure.FetchProgram.DeviceOrderTS, nil
		case TuningSpaceBS:
			return configure.FetchProgram.DeviceOrderBS, nil
		case TuningSpaceCS:
			return configure.FetchProgram.DeviceOrderCS, nil
		case TuningSpaceOS:
			return configure.FetchProgram.DeviceOrderOS, nil
		default:
			return nil, errors.New("unknown tuning space")
		}
	}
	return nil, fmt.Errorf("unknown service %d", sid)
}

func validateServiceGroups(groups []ServiceGroup) error {
	parents, children := make(map[uint]bool), make(map[uint]bool)
	for i, group := range groups {
		switch group.Tsp {
		case TuningSpaceTS, TuningSpaceBS, TuningSpaceCS, TuningSpaceOS, TuningSpace4K:
		default:
			return fmt.Errorf("servicegroups[%d]: unsupported tsp %d", i, group.Tsp)
		}
		parent := uint(group.Tsp)<<16 | group.Parent
		if group.Parent == 0 || group.Parent > 0xffff || len(group.Children) == 0 || parents[parent] {
			return fmt.Errorf("servicegroups[%d]: invalid or duplicate parent %d, or empty children", i, group.Parent)
		}
		parents[parent] = true
		for _, sid := range group.Children {
			child := uint(group.Tsp)<<16 | sid
			if sid == 0 || sid > 0xffff || children[child] {
				return fmt.Errorf("servicegroups[%d]: invalid or duplicate child %d", i, sid)
			}
			children[child] = true
		}
	}
	for parent := range parents {
		if children[parent] {
			return fmt.Errorf("servicegroups: parent %d must not also be a child", parent)
		}
	}
	return nil
}

// 明示された親自身の0も記録し、地上波の自動判定より設定を優先する。
func (p *TvRockConf) serviceParents() map[uint]uint {
	p.mux.RLock()
	defer p.mux.RUnlock()
	parents := make(map[uint]uint)
	for _, group := range p.ServiceGroups {
		root := uint(group.Tsp)<<16 | group.Parent
		parents[root] = 0
		for _, child := range group.Children {
			parents[uint(group.Tsp)<<16|child] = root
		}
	}
	return parents
}

// 起動時・.ch2・SDTの共通判定。未指定の地上波は同じTSIDの最小SIDを親にする。
// 明示設定は常に優先し、番組情報の共有判定には使わない。
func (p *Station) updateServiceParents() {
	configured := configure.serviceParents()
	p.Lock()
	defer p.Unlock()
	roots := make(map[uint]uint)
	for sid, sta := range p.Cache {
		if sta.Tsp != uint(TuningSpaceTS) || sta.Tsid == 0 || configured[sid] != 0 {
			continue
		}
		if root, ok := roots[sta.Tsid]; !ok || sid < root {
			roots[sta.Tsid] = sid
		}
	}
	for _, sta := range p.Cache {
		candidate, explicit := configured[sta.Sid]
		if !explicit {
			switch TuningSpace(sta.Tsp) {
			case TuningSpaceTS:
				candidate = roots[sta.Tsid]
			case TuningSpaceBS, TuningSpaceCS, TuningSpace4K:
			case TuningSpaceOS:
				// 地上波のワンセグも同じTSIDの代表へ対応付ける。
				if root := p.Cache[sta.Parent]; root != nil && (root.Tsp == uint(TuningSpaceTS) || root.Tsp == uint(TuningSpaceBS)) {
					candidate = root.Sid
					if root.Tsp == uint(TuningSpaceTS) {
						candidate = roots[sta.Tsid]
					}
				}
			default:
				continue
			}
		}
		parent := uint(0)
		if candidate != 0 && candidate != sta.Sid {
			root := p.Cache[candidate]
			// 明示的な子を別の局の親にしない。自動判定との組み合わせでも多段・循環を防ぐ。
			if configured[candidate] == 0 && root != nil && sta.Tsid != 0 && root.Tsid == sta.Tsid && (root.Tsp == sta.Tsp || (!explicit && sta.Tsp == uint(TuningSpaceOS) && (root.Tsp == uint(TuningSpaceTS) || root.Tsp == uint(TuningSpaceBS)))) {
				parent = candidate
			}
		}
		if sta.Parent != parent {
			sta.Parent = parent
			sta.Update = true
		}
	}
}
