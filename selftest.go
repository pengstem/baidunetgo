package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/egoist/mygo"

	"github.com/pengstem/baidunetgo/internal/auth"
	"github.com/pengstem/baidunetgo/internal/baidu"
	"github.com/pengstem/baidunetgo/internal/pcsimport"
	"github.com/pengstem/baidunetgo/internal/selftest"
)

// selfTest runs the self-test with the saved session when
// $BAIDUNETGO_SELFTEST names a report file, then quits. With
// $BAIDUNETGO_PROBE set it also measures download speeds.
func selfTest() bool {
	out := os.Getenv("BAIDUNETGO_SELFTEST")
	if out == "" {
		return false
	}
	mygo.App.WhenReady(func() {
		go func() {
			code := 0
			defer func() { mygo.App.Exit(code) }()
			dir, err := mygo.App.Path(mygo.PathUserData)
			if err != nil {
				fmt.Println("selftest:", err)
				code = 1
				return
			}
			var cr baidu.Credentials
			store := &auth.Store{Dir: dir, Keyring: mygo.Secrets}
			if err := store.Load(&cr); err != nil {
				// Else the account BaiduPCS-Go signed in with.
				acc, perr := pcsimport.Find()
				if perr != nil {
					fmt.Println("selftest: 没有已保存的登录:", err, perr)
					code = 1
					return
				}
				fmt.Println("selftest: 使用 BaiduPCS-Go 的登录")
				cr = acc.Creds
			}
			rep := selftest.Run(context.Background(), baidu.Open(cr), os.Getenv("BAIDUNETGO_PROBE") != "", func(f string, a ...any) {
				fmt.Printf(f+"\n", a...)
			})
			data, _ := json.MarshalIndent(rep, "", "  ")
			os.WriteFile(out, data, 0o600)
			for _, c := range rep.Checks {
				if !c.OK {
					code = 2
				}
			}
		}()
	})
	return true
}
