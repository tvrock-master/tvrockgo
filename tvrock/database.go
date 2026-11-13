package tvrock

import (
	"context"
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// スキーマ記述にgorm等あるが使用しない
type RockDB struct {
	Data    []*sql.DB
	Program []*sql.DB
	Caption []*sql.DB
}

var rockDB RockDB

func sqliteDSN(dbpath string) string {
	return sqliteDSNWithSettings(dbpath, configure.SqliteSetting)
}

func sqliteDSNWithSettings(dbpath string, settings []string) string {
	var sb strings.Builder
	for i, v := range settings {
		if i == 0 {
			sb.WriteString("?")
		} else {
			sb.WriteString("&")
		}
		sb.WriteString(v)
	}
	return dbpath + sb.String()
}

func (p *RockDB) Initialize(dbpath1, pgpath1, cppath1, dbpath2, pgpath2, cppath2 string) (initErr error) {
	var err error
	defer func() {
		if initErr != nil {
			p.CloseDB()
		}
	}()

	p.Data = []*sql.DB{nil, nil}
	p.Program = []*sql.DB{nil, nil}
	p.Caption = []*sql.DB{nil, nil}

	if p.Data[0], err = p.initDataDB(dbpath1); err != nil {
		return err
	}

	if p.Program[0], err = p.initProgramDB(pgpath1); err != nil {
		return err
	}

	if p.Caption[0], err = p.initCaptionDB(cppath1); err != nil {
		return err
	}

	if p.Data[1], err = p.initDataDB(dbpath2); err != nil {
		return err
	}

	if p.Program[1], err = p.initProgramDB(pgpath2); err != nil {
		return err
	}

	if p.Caption[1], err = p.initCaptionDB(cppath2); err != nil {
		return err
	}

	return nil
}

func (p *RockDB) initDataDB(dbpath string) (*sql.DB, error) {
	var err error
	var db *sql.DB

	dbdir := filepath.Dir(dbpath)
	if err := os.MkdirAll(dbdir, 0755); err != nil {
		return nil, err
	}

	db, err = sql.Open("sqlite", sqliteDSN(dbpath))
	if err != nil {
		return nil, err
	}
	/*
		for _, pragma := range configure.SqliteSetting {
			_, err = db.Exec(pragma)
			if err != nil {
				return nil, err
			}
		}
	*/
	err = CreateTable(db)
	if err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

func (p *RockDB) initProgramDB(dbpath string) (*sql.DB, error) {
	var err error
	var db *sql.DB

	dbdir := filepath.Dir(dbpath)
	if err := os.MkdirAll(dbdir, 0755); err != nil {
		return nil, err
	}

	db, err = sql.Open("sqlite", sqliteDSN(dbpath))
	if err != nil {
		return nil, err
	}

	err = CreateProgramTable(db)
	if err != nil {
		db.Close()
		return nil, err
	}

	err = Optimize(db)
	if err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

func (p *RockDB) initCaptionDB(dbpath string) (*sql.DB, error) {
	var err error
	var db *sql.DB

	dbdir := filepath.Dir(dbpath)
	if err := os.MkdirAll(dbdir, 0755); err != nil {
		return nil, err
	}

	db, err = sql.Open("sqlite", sqliteDSN(dbpath))
	if err != nil {
		return nil, err
	}

	err = CreateCaptionTable(db)
	if err != nil {
		db.Close()
		return nil, err
	}

	err = Optimize(db)
	if err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

func (p *RockDB) CloseDB() {
	for _, group := range []struct {
		name string
		dbs  []*sql.DB
	}{
		{"data", p.Data}, {"program", p.Program}, {"caption", p.Caption},
	} {
		for i, db := range group.dbs {
			if db != nil {
				if err := db.Close(); err != nil {
					log.Printf("error: DB close %s[%d]: %v", group.name, i, err)
				}
			}
		}
	}
}

func (p *RockDB) GetData(no int) *sql.DB {
	return p.Data[no]
}

func (p *RockDB) GetProgram(no int) *sql.DB {
	return p.Program[no]
}

func (p *RockDB) GetCaption(no int) *sql.DB {
	return p.Caption[no]
}

func CreateTable(p *sql.DB) error {
	var err error

	_, err = p.Exec(
		"CREATE TABLE IF NOT EXISTS migration(" +
			"version TEXT NOT NULL DEFAULT ''" +
			")",
	)
	if err != nil {
		return err
	}

	_, err = p.Exec("CREATE INDEX IF NOT EXISTS migration_1 ON migration(version)")
	if err != nil {
		return err
	}

	_, err = p.Exec(
		"CREATE TABLE IF NOT EXISTS station(" +
			"sid INTEGER UNSIGNED NOT NULL," +
			"tsid INTEGER UNSIGNED NOT NULL," +
			"ch INTEGER UNSIGNED NOT NULL," +
			"tsp INTEGER UNSIGNED NOT NULL," +
			"no INTEGER UNSIGNED NOT NULL," +
			"name TEXT," +
			"view TINYINT UNSIGNED NOT NULL," +
			"parent INTEGER UNSIGNED NOT NULL DEFAULT 0," +
			"fetchprogram TINYINT UNSIGNED NOT NULL DEFAULT 0," +
			"hashtag TEXT NOT NULL DEFAULT ''," +
			"PRIMARY KEY(tsp,sid)" +
			")",
	)
	if err != nil {
		return err
	}
	_, err = p.Exec("CREATE INDEX IF NOT EXISTS station_1 ON station(tsp)")
	if err != nil {
		return err
	}

	_, err = p.Exec("CREATE INDEX IF NOT EXISTS station_2 ON station(no)")
	if err != nil {
		return err
	}

	_, err = p.Exec(
		"CREATE TABLE IF NOT EXISTS schedule(" +
			"id INTEGER PRIMARY KEY AUTOINCREMENT," +

			"deviceid TEXT NOT NULL," +
			"title TEXT NOT NULL," +
			"customcomid TEXT NOT NULL," +
			"filename TEXT," +

			"validate TINYINT UNSIGNED NOT NULL," +
			"watchonly TINYINT UNSIGNED NOT NULL," +
			"recordonly TINYINT UNSIGNED NOT NULL," +
			"shutdown TINYINT UNSIGNED NOT NULL," +
			"complete TINYINT UNSIGNED NOT NULL," +
			"epgfollow TINYINT UNSIGNED NOT NULL," +
			"autoselect TINYINT UNSIGNED NOT NULL," +

			"starttime BIGINT NOT NULL," +
			"endtime BIGINT NOT NULL," +
			"idletime BIGINT NOT NULL," +
			"readytime BIGINT NOT NULL," +
			"tailtime BIGINT NOT NULL," +
			"lastupdate BIGINT NOT NULL," +

			"sid INTEGER UNSIGNED NOT NULL," +
			"eid INTEGER UNSIGNED NOT NULL," +
			"searchid INTEGER UNSIGNED NOT NULL," +
			"priority INTEGER UNSIGNED NOT NULL," +
			"exitmode INTEGER UNSIGNED NOT NULL," +
			"no INTEGER UNSIGNED NOT NULL," +
			"errors INTEGER UNSIGNED NOT NULL" +
			")",
	)
	if err != nil {
		return err
	}

	_, err = p.Exec("CREATE INDEX IF NOT EXISTS schedule_1 ON schedule(starttime)")
	if err != nil {
		return err
	}

	_, err = p.Exec("CREATE INDEX IF NOT EXISTS schedule_2 ON schedule(searchid)")
	if err != nil {
		return err
	}

	_, err = p.Exec(
		"CREATE TABLE IF NOT EXISTS keywordreserve(" +
			"id INTEGER PRIMARY KEY AUTOINCREMENT," +
			"validate TINYINT UNSIGNED NOT NULL," +
			"deviceid TEXT NOT NULL," +
			"starttime BIGINT NOT NULL," +
			"endtime BIGINT NOT NULL," +
			"starthours TINYINT UNSIGNED NOT NULL," +
			"hours TINYINT UNSIGNED NOT NULL," +
			"idletime BIGINT NOT NULL," +
			"readytime BIGINT NOT NULL," +
			"tailtime BIGINT NOT NULL," +
			"title TEXT NOT NULL," +
			"content TEXT NOT NULL," +
			"days BIGINT NOT NULL," +
			"sid TEXT NOT NULL," +
			"category INTEGER UNSIGNED NOT NULL," +
			"week TINYINT UNSIGNED NOT NULL DEFAULT 0," +
			"andsearch TINYINT UNSIGNED NOT NULL," +
			"fuzzysearch TINYINT UNSIGNED NOT NULL," +
			"recordonly TINYINT UNSIGNED NOT NULL," +
			"watchonly TINYINT UNSIGNED NOT NULL," +
			"shutdown TINYINT UNSIGNED NOT NULL," +
			"epgfollow TINYINT UNSIGNED NOT NULL," +
			"customcomid TEXT NOT NULL," +
			"hashtag TEXT NOT NULL," +
			"createdat BIGINT NOT NULL," +
			"updatedat BIGINT NOT NULL" +
			")",
	)
	if err != nil {
		return err
	}
	_, err = p.Exec("CREATE INDEX IF NOT EXISTS keywordreserve_1 ON keywordreserve(updatedat)")
	if err != nil {
		return err
	}

	_, err = p.Exec("CREATE INDEX IF NOT EXISTS keywordreserve_2 ON keywordreserve(starttime)")
	if err != nil {
		return err
	}

	_, err = p.Exec("CREATE INDEX IF NOT EXISTS keywordreserve_3 ON keywordreserve(endtime)")
	if err != nil {
		return err
	}

	return createTSAnalysisTable(p)
}

func CreateProgramTable(p *sql.DB) error {
	var err error

	_, err = p.Exec(
		"CREATE TABLE IF NOT EXISTS migration(" +
			"version TEXT NOT NULL DEFAULT ''" +
			")",
	)
	if err != nil {
		return err
	}

	_, err = p.Exec("CREATE INDEX IF NOT EXISTS migration_1 ON migration(version)")
	if err != nil {
		return err
	}

	_, err = p.Exec(
		"CREATE TABLE IF NOT EXISTS program(" +
			"sid INTEGER UNSIGNED NOT NULL," +
			"eid INTEGER UNSIGNED NOT NULL," +
			"start BIGINT NOT NULL," +
			"end BIGINT NOT NULL," +
			"title TEXT," +
			"subtitle TEXT," +
			"content TEXT," +
			"cnib_a INTEGER UNSIGNED NOT NULL," +
			"cnib_b INTEGER UNSIGNED NOT NULL," +
			"common_sid INTEGER UNSIGNED," +
			"common_eid INTEGER UNSIGNED," +
			"PRIMARY KEY(sid, start)" +
			")",
	)
	if err != nil {
		return err
	}

	_, err = p.Exec("CREATE INDEX IF NOT EXISTS program_1 ON program(start,end)")
	if err != nil {
		return err
	}

	_, err = p.Exec("CREATE INDEX IF NOT EXISTS program_2 ON program(title)")
	if err != nil {
		return err
	}

	_, err = p.Exec("CREATE INDEX IF NOT EXISTS program_3 ON program(subtitle)")
	if err != nil {
		return err
	}

	_, err = p.Exec("CREATE INDEX IF NOT EXISTS program_4 ON program(content)")
	if err != nil {
		return err
	}

	_, err = p.Exec("CREATE INDEX IF NOT EXISTS program_5 ON program(sid,start,end)")
	if err != nil {
		return err
	}

	_, err = p.Exec("CREATE INDEX IF NOT EXISTS program_6 ON program(sid,eid)")
	if err != nil {
		return err
	}

	_, err = p.Exec("CREATE INDEX IF NOT EXISTS program_7 ON program(start)")
	if err != nil {
		return err
	}

	_, err = p.Exec("CREATE INDEX IF NOT EXISTS program_8 ON program(end)")
	if err != nil {
		return err
	}

	_, err = p.Exec("CREATE INDEX IF NOT EXISTS program_9 ON program(sid,end,start)")
	if err != nil {
		return err
	}

	_, err = p.Exec("CREATE INDEX IF NOT EXISTS program_10 ON program(sid,eid,start DESC)")
	if err != nil {
		return err
	}

	_, err = p.Exec("CREATE INDEX IF NOT EXISTS program_11 ON program(end,start)")
	if err != nil {
		return err
	}

	_, err = p.Exec("CREATE INDEX IF NOT EXISTS program_12 ON program(start,sid,eid)")
	if err != nil {
		return err
	}

	return err
}

func CreateCaptionTable(p *sql.DB) error {
	var err error

	_, err = p.Exec(
		"CREATE TABLE IF NOT EXISTS caption(" +
			"id INTEGER PRIMARY KEY AUTOINCREMENT," +
			"sid INTEGER UNSIGNED NOT NULL," +
			"eid INTEGER UNSIGNED NOT NULL," +
			"start BIGINT NOT NULL," +
			"pts BIGINT NOT NULL," +
			"lang TINYINT UNSIGNED NOT NULL," +
			"clear TINYINT UNSIGNED NOT NULL," +
			"x INTEGER NOT NULL," +
			"y INTEGER NOT NULL," +
			"color TINYINT UNSIGNED NOT NULL," +
			"colorname TEXT NOT NULL," +
			"bgcolor TINYINT UNSIGNED NOT NULL," +
			"size TEXT NOT NULL," +
			"cap TEXT NOT NULL" +
			")",
	)
	if err != nil {
		return err
	}

	_, err = p.Exec("CREATE INDEX IF NOT EXISTS caption_1 ON caption(sid,eid,start)")
	if err != nil {
		return err
	}

	_, err = p.Exec("CREATE INDEX IF NOT EXISTS caption_2 ON caption(start)")
	if err != nil {
		return err
	}

	return nil
}

func Optimize(d *sql.DB) error {
	_, err := d.Exec("PRAGMA optimize")
	if err != nil {
		return err
	}
	return nil
}

func Vacuum(d *sql.DB) error {
	return vacuumContext(serviceContext(), d)
}

func vacuumContext(ctx context.Context, d *sql.DB) error {
	_, err := d.ExecContext(ctx, "VACUUM")
	if err != nil {
		return err
	}
	return nil
}
