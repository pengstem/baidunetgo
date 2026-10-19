package pcsimport

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRead(t *testing.T) {
	p := filepath.Join(t.TempDir(), "pcs_config.json")
	os.WriteFile(p, []byte(`{"baidu_active_uid":7,"baidu_user_list":[
		{"uid":7,"name":"小明","bduss":"","stoken":"","cookies":"BAIDUID=a; BDUSS=b; STOKEN=c; PANPSC=d"}]}`), 0o600)
	acc, err := Read(p)
	if err != nil {
		t.Fatal(err)
	}
	if acc.Name != "小明" || acc.Creds.BDUSS != "b" || acc.Creds.STOKEN != "c" || acc.Creds.UID != 7 || acc.Creds.Cookies == "" {
		t.Errorf("%+v", acc)
	}
}
