package migrate

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/sha1"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/pengstem/baidunetgo/internal/baidu"
)

// NetdiskPath returns the cookie store of Baidu's own Linux client (an
// Electron app), "" when there is none.
func NetdiskPath() string {
	var dirs []string
	if d, err := os.UserConfigDir(); err == nil {
		dirs = append(dirs, filepath.Join(d, "baidunetdisk"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".var", "app", "com.baidu.NetDisk", "config", "baidunetdisk"))
	}
	for _, d := range dirs {
		for _, p := range []string{filepath.Join(d, "Network", "Cookies"), filepath.Join(d, "Cookies")} {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return ""
}

// ReadNetdisk returns the account of the client's cookie store at path.
func ReadNetdisk(path string) (*Account, error) {
	db, err := openSQLite(path)
	if err != nil {
		return nil, err
	}
	cols, rows, err := db.table("cookies")
	if err != nil {
		return nil, err
	}
	col := map[string]int{}
	for i, c := range cols {
		col[c] = i
	}
	version := 0
	if _, meta, err := db.table("meta"); err == nil {
		for _, r := range meta {
			if len(r) >= 2 && r[0] == "version" {
				if s, ok := r[1].(string); ok {
					version, _ = strconv.Atoi(s)
				}
			}
		}
	}
	get := func(r []any, name string) any {
		if i, ok := col[name]; ok && i < len(r) {
			return r[i]
		}
		return nil
	}

	// The cookies a browser sends to pan.baidu.com, the netdisk's own
	// over those of all of baidu.com.
	now := time.Now()
	general, own := map[string]string{}, map[string]string{}
	var order []string
	for _, r := range rows {
		host, _ := get(r, "host_key").(string)
		name, _ := get(r, "name").(string)
		var m map[string]string
		switch host {
		case ".baidu.com", "baidu.com":
			m = general
		case ".pan.baidu.com", "pan.baidu.com":
			m = own
		default:
			continue
		}
		if exp, ok := get(r, "expires_utc").(int64); ok && exp != 0 && chromeTime(exp).Before(now) {
			continue
		}
		value, _ := get(r, "value").(string)
		if value == "" {
			enc, _ := get(r, "encrypted_value").([]byte)
			value = decrypt(enc, version)
		}
		if name == "" || value == "" {
			continue
		}
		if _, seen := general[name]; !seen {
			if _, seen := own[name]; !seen {
				order = append(order, name)
			}
		}
		m[name] = value
	}
	value := func(name string) string {
		if v, ok := own[name]; ok {
			return v
		}
		return general[name]
	}
	cr := baidu.Credentials{BDUSS: value("BDUSS"), STOKEN: value("STOKEN")}
	if cr.BDUSS == "" {
		return nil, errors.New("百度网盘客户端没有登录")
	}
	var parts []string
	for _, name := range order {
		parts = append(parts, name+"="+value(name))
	}
	cr.Cookies = strings.Join(parts, "; ")
	return &Account{From: "百度网盘客户端", Creds: cr}, nil
}

// chromeTime converts Chromium's microseconds since 1601.
func chromeTime(us int64) time.Time {
	const epochDelta = 11644473600 // seconds from 1601 to 1970
	return time.Unix(us/1e6-epochDelta, us%1e6*1000)
}

// decrypt opens a value Chromium encrypted with its fixed Linux key
// ("v10"); values under a keyring key ("v11") stay closed.
func decrypt(enc []byte, version int) string {
	if runtime.GOOS != "linux" || !bytes.HasPrefix(enc, []byte("v10")) {
		return ""
	}
	enc = enc[3:]
	if len(enc) == 0 || len(enc)%aes.BlockSize != 0 {
		return ""
	}
	key, err := pbkdf2.Key(sha1.New, "peanuts", []byte("saltysalt"), 1, 16)
	if err != nil {
		return ""
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return ""
	}
	out := make([]byte, len(enc))
	cipher.NewCBCDecrypter(block, bytes.Repeat([]byte(" "), aes.BlockSize)).CryptBlocks(out, enc)
	pad := int(out[len(out)-1])
	if pad == 0 || pad > aes.BlockSize || pad > len(out) {
		return ""
	}
	out = out[:len(out)-pad]
	// Since version 24 the value starts with the SHA-256 of its host.
	if version >= 24 && len(out) >= 32 {
		out = out[32:]
	}
	return string(out)
}
