package tvrock

import (
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"sync"
)

type StorageInfo struct {
	Using uint64 `yaml:"using"`
	Size  uint64 `yaml:"size"`
}

var storageInfo map[string]*StorageInfo
var storageInfoMutex sync.RWMutex

func InitializeStorageInfo() error {
	storageInfo = map[string]*StorageInfo{}

	// cron登録
	cronManager.Add(TaskStorageSurvey, func() {
		if err := StorageSurvey(); err != nil && serviceContext().Err() == nil {
			log.Println("error: ", err)
		}

		if err := SendStorageInfo(); err != nil {
			log.Println("error: ", err)
		}
	})

	return nil
}

func StorageSurvey() error {
	v := &configure.StorageCommand
	if len(v.ID) < 1 {
		return nil
	}

	ret, err := customCommandManager.ExecCustomCommand(v, "", "", "", 0, 0, 0, "", "")
	if err != nil {
		return err
	}

	info := parseStorageInfo(ret)
	storageInfoMutex.Lock()
	storageInfo = info
	storageInfoMutex.Unlock()
	return nil
}

func parseStorageInfo(ret [][]string) map[string]*StorageInfo {
	info := make(map[string]*StorageInfo)
	if len(ret) == 0 || len(ret[0]) != 4 {
		return info
	}
	captionColumn := storageColumnIndex("Caption", ret[0])
	volumeColumn := storageColumnIndex("VolumeName", ret[0])
	freeColumn := storageColumnIndex("FreeSpace", ret[0])
	sizeColumn := storageColumnIndex("Size", ret[0])
	if captionColumn < 0 || volumeColumn < 0 || freeColumn < 0 || sizeColumn < 0 {
		return info
	}
	for _, v := range ret[1:] {
		if len(v) < 3 || strings.HasPrefix(v[0], "---") {
			continue
		}
		// VolumeNameのみが、空白区切りのフィールドを0個または複数個占有できる。
		// ヘッダーで指定された順序を維持しつつ、それ以降の列を調整する。
		volumeWords := len(v) - 3
		columnIndex := func(column int) int {
			if column > volumeColumn {
				return column + volumeWords - 1
			}
			return column
		}
		id := v[columnIndex(captionColumn)] + strings.Join(v[volumeColumn:volumeColumn+volumeWords], " ")
		freespace := v[columnIndex(freeColumn)]
		size := v[columnIndex(sizeColumn)]

		fs, err := strconv.ParseUint(freespace, 10, 64)
		if err != nil {
			log.Println("error:", err)
			continue
		}
		sz, err := strconv.ParseUint(size, 10, 64)
		if err != nil {
			log.Println("error:", err)
			continue
		}
		if fs > sz {
			continue
		}
		info[id] = &StorageInfo{
			Using: sz - fs,
			Size:  sz,
		}
	}
	return info
}

func SendStorageInfo() error {
	storageInfoMutex.RLock()
	js, err := json.Marshal(storageInfo)
	storageInfoMutex.RUnlock()
	if err != nil {
		return err
	}
	deviceController.Notify(ControllerSendStorageInfo, string(js))
	return nil
}

func storageColumnIndex(name string, line []string) int {
	for i, v := range line {
		if v == name {
			return i
		}
	}
	return -1
}
