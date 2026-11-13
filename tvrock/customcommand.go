package tvrock

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type CustomCommandExecutionMode uint8

const (
	CustomCommandExecutionModeRecordStop CustomCommandExecutionMode = iota
	CustomCommandExecutionModeRecordStart
	CustomCommandExecutionModeIdleStart
)

type CustomCommand struct {
	ID           string                     `yaml:"id"`
	Mode         CustomCommandExecutionMode `yaml:"mode"`
	Dependencies string                     `yaml:"dependencies"`
	Type         string                     `yaml:"type"`
	AppPath      string                     `yaml:"apppath"`
	Options      []string                   `yaml:"options"`
}

type CustomCommandManager struct {
	BusyList map[string]*exec.Cmd
	sync.RWMutex
}

var customCommandManager CustomCommandManager

func InitializeCustomCommandManager() error {
	customCommandManager.BusyList = make(map[string]*exec.Cmd, 10)
	return nil
}

func (p *CustomCommandManager) AddCustomCommand(cmd *exec.Cmd) string {
	id := uuid.New().String()
	p.Lock()
	p.BusyList[id] = cmd
	p.Unlock()
	return id
}

func (p *CustomCommandManager) RemoveCustomCommand(id string) {
	p.Lock()
	delete(p.BusyList, id)
	p.Unlock()
}

func (p *CustomCommandManager) GetBusyCount() int {
	p.RLock()
	n := len(p.BusyList)
	p.RUnlock()
	return n
}

func (p *CustomCommandManager) ExecCustomCommand(cc *CustomCommand, did, title, recpath string, sid, eid, errCount uint, hashtagk, hashtags string) ([][]string, error) {
	rows, err := p.execCustomCommandContext(serviceContext(), cc, did, title, recpath, sid, eid, errCount, hashtagk, hashtags)
	if err != nil {
		log.Printf("error: custom command %s: %v", cc.ID, err)
	}
	return rows, err
}

func (p *CustomCommandManager) execCustomCommandContext(ctx context.Context, cc *CustomCommand, did, title, recpath string, sid, eid, errCount uint, hashtagk, hashtags string) ([][]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var ret [][]string

	if err := cc.validate(); err != nil {
		return nil, err
	}
	exe, err := newCustomCommand(ctx, cc)
	if err != nil {
		return nil, err
	}
	data, err := buildCustomCommandContext(title, recpath, sid, eid, errCount, hashtagk, hashtags)
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "tvrock-command-")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			log.Printf("error: custom command context cleanup: %v", err)
		}
	}()
	contextPath := filepath.Join(dir, "context.json")
	if err := writeCustomCommandContext(contextPath, data); err != nil {
		return nil, err
	}
	// 親から同名変数を継承しても、この実行専用のJSONパスで置き換える。
	env := exe.Environ()
	exe.Env = make([]string, 0, len(env)+1)
	for _, value := range env {
		key, _, _ := strings.Cut(value, "=")
		if !strings.EqualFold(key, "TVROCK_CONTEXT_FILE") {
			exe.Env = append(exe.Env, value)
		}
	}
	exe.Env = append(exe.Env, "TVROCK_CONTEXT_FILE="+contextPath)
	configureBackgroundCommand(exe)
	exe.WaitDelay = 2 * time.Second

	id := p.AddCustomCommand(exe)
	defer p.RemoveCustomCommand(id)

	out, err := exe.CombinedOutput()
	if ctx.Err() != nil {
		return ret, ctx.Err()
	}
	if err != nil {
		log.Println("error:", err, string(out))
		return ret, err
	}

	result := string(out)
	lines := strings.Split(result, "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) > 0 {
			ret = append(ret, fields)
		}
	}

	return ret, nil
}

func (cc *CustomCommand) commandType() string {
	if cc.Type == "" {
		return "executable"
	}
	return cc.Type
}

func (cc *CustomCommand) validate() error {
	if strings.TrimSpace(cc.AppPath) == "" {
		return fmt.Errorf("apppath is required")
	}
	for _, value := range append([]string{cc.AppPath}, cc.Options...) {
		for _, token := range FilenameExCommands {
			if strings.Contains(value, token) {
				return fmt.Errorf("legacy placeholder %s is not allowed; use TVROCK_CONTEXT_FILE (docs/custom-commands.md)", token)
			}
		}
	}
	ext := strings.ToLower(filepath.Ext(cc.AppPath))
	switch cc.commandType() {
	case "executable":
		if ext == ".bat" || ext == ".cmd" || ext == ".ps1" {
			return fmt.Errorf("script requires type: batch or powershell")
		}
	case "batch":
		if ext != ".bat" && ext != ".cmd" {
			return fmt.Errorf("type: batch requires .bat or .cmd")
		}
		// cmd.exeの再展開・引用符の脱出を許さない。引数は固定値だけを扱う。
		if strings.ContainsAny(cc.AppPath, "\"%!\r\n") {
			return fmt.Errorf("batch path contains unsupported shell characters")
		}
		for _, arg := range cc.Options {
			if strings.ContainsAny(arg, "\"%!&|<>^\r\n") {
				return fmt.Errorf("batch option contains unsupported shell characters")
			}
		}
	case "powershell":
		if ext != ".ps1" {
			return fmt.Errorf("type: powershell requires .ps1")
		}
	default:
		return fmt.Errorf("unsupported custom command type: %s", cc.Type)
	}
	return nil
}
