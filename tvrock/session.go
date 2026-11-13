package tvrock

import (
	"crypto/md5"
	"encoding/gob"
	"encoding/hex"

	"reflect"
)

type MySessionData struct {
	IsOK         bool `json:"isok"`
	Hours        int  `json:"hours"`
	Broadcast    uint `json:"broadcast"`
	ViewStations int  `json:"viewstations"`
	SubtitleMin  int  `json:"subtitlemin"`
	ContentMin   int  `json:"contentmin"`
	GuideType    int  `json:"guidetype"`
}

var ssRevision string

func InitializeSession() error {

	ssRevision = sessionSchemaHash(&MySessionData{})

	// セッションで使用する構造体
	gob.Register(&UserSession{})

	return nil
}

func sessionSchemaHash(p interface{}) string {
	d := reflect.Indirect(reflect.ValueOf(p)).Interface()
	t := reflect.TypeOf(d)
	str := t.Name()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tn := ""
		if f.Type.Kind() == reflect.Ptr {
			tn = "*" + f.Type.Elem().Name()
		} else {
			tn = f.Type.Name()
		}
		str = "+" + str + f.Name + "," + tn + "," + f.Tag.Get("json")
	}

	md5 := md5.Sum([]byte(str))

	return hex.EncodeToString(md5[:])
}

func newSessionData() *MySessionData {
	return &MySessionData{
		IsOK:         false,
		Broadcast:    uint(TuningSpaceTS),
		SubtitleMin:  60,
		ContentMin:   120,
		Hours:        8,
		ViewStations: 8,
		GuideType:    0,
	}
}
