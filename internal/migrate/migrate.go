// Package migrate finds the accounts other netdisk clients on this
// computer are signed in with, so that the app signs in with one of them
// without asking: Baidu's own client, then BaiduPCS-Go.
package migrate

import (
	"github.com/pengstem/baidunetgo/internal/baidu"
	"github.com/pengstem/baidunetgo/internal/pcsimport"
)

// Account is an account another client is signed in with.
type Account struct {
	From  string // the client, for the user
	Name  string // the account's name, if the client keeps it
	Creds baidu.Credentials
}

// Label names the account for the user.
func (a Account) Label() string {
	if a.Name == "" {
		return a.From
	}
	return a.From + "（" + a.Name + "）"
}

// Find returns the accounts found, freshest source first, each BDUSS once.
func Find() []Account {
	var out []Account
	seen := map[string]bool{}
	add := func(a *Account, err error) {
		if err != nil || a == nil || seen[a.Creds.BDUSS] {
			return
		}
		seen[a.Creds.BDUSS] = true
		out = append(out, *a)
	}
	// Baidu's client keeps its cookies fresh, so it goes first.
	if p := NetdiskPath(); p != "" {
		add(ReadNetdisk(p))
	}
	if acc, err := pcsimport.Find(); err == nil {
		add(&Account{From: "BaiduPCS-Go", Name: acc.Name, Creds: acc.Creds}, nil)
	}
	return out
}
