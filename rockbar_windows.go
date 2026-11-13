//go:build windows

package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// electron-builderのデフォルトのアンインストール名は「${productName} ${version}」です
var rockbarInstallName = regexp.MustCompile(`(?i)^Rockbar(?:\s+v?\d+\.\d+\.\d+(?:-[0-9a-z.-]+)?(?:\+[0-9a-z.-]+)?)?$`)

func openRockbar(explicitPath, browserURL string) error {
	paths := []string{explicitPath}
	paths = append(paths, installedRockbarPaths()...)
	if exe, err := os.Executable(); err == nil {
		paths = append(paths, filepath.Join(filepath.Dir(exe), "rockbar", "Rockbar.exe"), filepath.Join(filepath.Dir(exe), "Rockbar.exe"))
	}
	for _, location := range []struct{ variable, suffix string }{
		{"LOCALAPPDATA", "Programs/Rockbar/Rockbar.exe"},
		{"ProgramFiles", "Rockbar/Rockbar.exe"},
		{"ProgramFiles(x86)", "Rockbar/Rockbar.exe"},
	} {
		if root := os.Getenv(location.variable); root != "" {
			paths = append(paths, filepath.Join(root, filepath.FromSlash(location.suffix)))
		}
	}
	return launchRockbarOrBrowser(paths, browserURL, func(path string) error {
		cmd := exec.Command(path)
		if err := cmd.Start(); err != nil {
			return err
		}
		go func() { _ = cmd.Wait() }()
		return nil
	}, openExternal)
}

func launchRockbarOrBrowser(paths []string, browserURL string, launch, browse func(string) error) error {
	seen := make(map[string]bool)
	for _, path := range paths {
		if path == "" {
			continue
		}
		key := strings.ToLower(filepath.Clean(path))
		if seen[key] {
			continue
		}
		seen[key] = true
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if err := launch(path); err == nil {
			return nil
		} else {
			log.Printf("warn: Rockbarアプリの起動に失敗: %v", err)
		}
	}
	return browse(browserURL)
}

// NSISは、選択されたインストールディレクトリを Software\<app-guid> の下に記録します。
// カスタムインストールパスにも対応できるよう、それに対応するアンインストール用エントリを参照してください。
func installedRockbarPaths() []string {
	const uninstall = `Software\Microsoft\Windows\CurrentVersion\Uninstall`
	var paths []string
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		for _, view := range []uint32{registry.WOW64_64KEY, registry.WOW64_32KEY} {
			parent, err := registry.OpenKey(root, uninstall, registry.ENUMERATE_SUB_KEYS|view)
			if err != nil {
				continue
			}
			names, _ := parent.ReadSubKeyNames(-1)
			parent.Close()
			for _, name := range names {
				key, err := registry.OpenKey(root, uninstall+`\`+name, registry.QUERY_VALUE|view)
				if err != nil {
					continue
				}
				displayName, _, _ := key.GetStringValue("DisplayName")
				if !rockbarInstallName.MatchString(strings.TrimSpace(displayName)) {
					key.Close()
					continue
				}
				location, _, _ := key.GetStringValue("InstallLocation")
				icon, _, _ := key.GetStringValue("DisplayIcon")
				key.Close()
				if location != "" {
					paths = append(paths, filepath.Join(expandRegistryPath(location), "Rockbar.exe"))
				}
				if install, err := registry.OpenKey(root, `Software\`+name, registry.QUERY_VALUE|view); err == nil {
					location, _, _ := install.GetStringValue("InstallLocation")
					install.Close()
					if location != "" {
						paths = append(paths, filepath.Join(expandRegistryPath(location), "Rockbar.exe"))
					}
				}
				if path := rockbarDisplayIconPath(icon); path != "" {
					paths = append(paths, path)
				}
			}
		}
	}
	return paths
}

func expandRegistryPath(path string) string {
	if expanded, err := registry.ExpandString(path); err == nil {
		return expanded
	}
	return path
}

func rockbarDisplayIconPath(value string) string {
	value = strings.TrimSpace(value)
	if comma := strings.LastIndexByte(value, ','); comma >= 0 {
		if _, err := strconv.Atoi(strings.TrimSpace(value[comma+1:])); err == nil {
			value = value[:comma]
		}
	}
	path := expandRegistryPath(strings.Trim(strings.TrimSpace(value), `"`))
	if !strings.EqualFold(filepath.Base(path), "Rockbar.exe") {
		return ""
	}
	return path
}
