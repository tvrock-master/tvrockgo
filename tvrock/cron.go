package tvrock

import (
	"fmt"
	"sync"

	"github.com/robfig/cron/v3"
)

const (
	TaskStorageSurvey         = "storagesurvey"
	TaskArchiver              = "archiver"
	TaskIndexingArchive       = "indexingarchive"
	TaskFetchProgramScheduler = "fetchprogramscheduler"
)

type CronManager struct {
	Cron   *cron.Cron
	CronID map[cron.EntryID]string
	sync.RWMutex
}

var cronManager *CronManager

func InitializeCron() error {

	cronManager = &CronManager{
		Cron:   cron.New(),
		CronID: make(map[cron.EntryID]string),
	}

	cronManager.Start()

	return nil
}

func (p *CronManager) Close() error {
	p.RemoveAll()
	<-p.Cron.Stop().Done()
	return nil
}

func (p *CronManager) RemoveAll() error {
	p.Lock()
	defer p.Unlock()

	for v := range p.CronID {
		p.Cron.Remove(v)
		delete(p.CronID, v)
	}
	return nil
}

func (p *CronManager) Start() error {
	p.Lock()
	p.Cron.Start()
	p.Unlock()
	return nil
}

func (p *CronManager) Stop() error {
	p.Lock()
	p.Cron.Stop()
	p.Unlock()
	return nil
}

func (p *CronManager) Add(id string, fn func()) error {
	timer, b := configure.CronTask[id]
	if !b || len(timer) < 1 {
		return fmt.Errorf("cron id not found %s", id)
	}

	p.Lock()
	defer p.Unlock()

	// 毎分実行するタスクを登録
	job := cron.FuncJob(func() {
		if serviceContext().Err() != nil || !serviceWorkers.enter() {
			return
		}
		defer serviceWorkers.wg.Done()
		fn()
	})
	cid, err := p.Cron.AddJob(timer, cron.SkipIfStillRunning(cron.DefaultLogger)(job))
	if err != nil {
		return err
	}

	p.CronID[cid] = id

	return nil
}
