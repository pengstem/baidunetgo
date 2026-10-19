// Package pcsimport reads the account BaiduPCS-Go signed in with from its
// configuration file, so that a user of the CLI signs in here with one
// click.
package pcsimport

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"

	"github.com/pengstem/baidunetgo/internal/baidu"
)

// Account is a signed-in account of BaiduPCS-Go.
type Account struct {
	Name  string
	Creds baidu.Credentials
}

// ConfigPath returns where BaiduPCS-Go keeps its configuration, as it
// finds it, "" when there is none.
func ConfigPath() string {
	var dirs []string
	if d := os.Getenv("BAIDUPCS_GO_CONFIG_DIR"); d != "" && filepath.IsAbs(d) {
		dirs = append(dirs, d)
	}
	if runtime.GOOS == "windows" {
		if d := os.Getenv("APPDATA"); d != "" {
			dirs = append(dirs, filepath.Join(d, "BaiduPCS-Go"))
		}
	} else if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".config", "BaiduPCS-Go"))
	}
	for _, d := range dirs {
		p := filepath.Join(d, "pcs_config.json")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

type config struct {
	ActiveUID uint64 `json:"baidu_active_uid"`
	Users     []struct {
		UID     uint64 `json:"uid"`
		Name    string `json:"name"`
		BDUSS   string `json:"bduss"`
		STOKEN  string `json:"stoken"`
		Cookies string `json:"cookies"`
	} `json:"baidu_user_list"`
}

// Read returns the active account of the configuration at path.
func Read(path string) (*Account, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	for _, u := range cfg.Users {
		if u.UID != cfg.ActiveUID && len(cfg.Users) > 1 {
			continue
		}
		cr := baidu.Credentials{BDUSS: u.BDUSS, STOKEN: u.STOKEN, Cookies: u.Cookies, UID: u.UID}
		if cr.BDUSS == "" && u.Cookies != "" {
			if parsed, err := baidu.ParseCredentials(u.Cookies); err == nil {
				cr.BDUSS, cr.STOKEN = parsed.BDUSS, parsed.STOKEN
			}
		}
		if cr.STOKEN == "" && u.Cookies != "" {
			if parsed, err := baidu.ParseCredentials(u.Cookies); err == nil {
				cr.STOKEN = parsed.STOKEN
			}
		}
		if cr.BDUSS == "" {
			return nil, errors.New("BaiduPCS-Go 的账号没有 BDUSS")
		}
		return &Account{Name: u.Name, Creds: cr}, nil
	}
	return nil, errors.New("BaiduPCS-Go 没有登录的账号")
}

// Find returns BaiduPCS-Go's active account, if it has one.
func Find() (*Account, error) {
	p := ConfigPath()
	if p == "" {
		return nil, errors.New("没有找到 BaiduPCS-Go 的配置")
	}
	return Read(p)
}
