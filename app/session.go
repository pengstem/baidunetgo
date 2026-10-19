package app

import (
	"context"
	"errors"
	"log"
	"os"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/auth"
	"github.com/pengstem/baidunetgo/internal/baidu"
	"github.com/pengstem/baidunetgo/internal/fakepan"
)

// phase is where the app is in signing in.
type phase int

const (
	phaseBooting phase = iota // reading the saved session
	phaseLogin                // asking the user to sign in
	phaseReady                // signed in
)

// session is the signed-in account.
type session struct {
	client *baidu.Client
	user   *baidu.User
	avatar *ui.Bitmap
	quota  baidu.Quota
}

// openStore finds where the session is kept.
func openStore() *auth.Store {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		log.Printf("session: %v", err)
		return nil
	}
	return &auth.Store{Dir: dir, Keyring: mygo.Secrets}
}

// boot signs in with the saved session, if any, else asks to sign in.
func (a *App) boot() {
	if os.Getenv("BAIDUNETGO_FAKE") != "" {
		srv := fakepan.New()
		a.fake = true
		go a.signInWith(baidu.OpenAt(baidu.Credentials{BDUSS: "fake", UID: 1}, srv.URL), false)
		return
	}
	if a.store == nil {
		a.phase = phaseLogin
		return
	}
	go func() {
		var cr baidu.Credentials
		if err := a.store.Load(&cr); err != nil {
			if !errors.Is(err, auth.ErrNoSession) {
				log.Printf("session: %v", err)
			}
			a.update(func() { a.phase = phaseLogin })
			return
		}
		a.signIn(cr, false)
	}()
}

// signIn checks the credentials and, if they sign in, saves them and shows
// the netdisk. It runs on any goroutine.
func (a *App) signIn(cr baidu.Credentials, fresh bool) {
	a.signInWith(baidu.Open(cr), fresh)
}

// signInWith signs in with a client.
func (a *App) signInWith(c *baidu.Client, fresh bool) {
	u, err := c.User(context.Background())
	if err != nil {
		a.update(func() {
			a.phase = phaseLogin
			a.login.busy = false
			if fresh || !errors.Is(err, baidu.ErrNotLoggedIn) {
				a.login.err = err.Error()
			} else {
				a.login.err = "登录已过期，请重新登录"
			}
		})
		return
	}
	if a.store != nil && !a.fake {
		if err := a.store.Save(c.Credentials()); err != nil {
			log.Printf("session: save: %v", err)
		}
	}
	a.update(func() {
		a.sess = &session{client: c, user: u}
		a.login = loginState{}
		a.phase = phaseReady
		a.onSignedIn()
	})
	go a.loadProfile(c, u)
}

// loadProfile fetches the avatar and the quota.
func (a *App) loadProfile(c *baidu.Client, u *baidu.User) {
	ctx := context.Background()
	if q, err := c.Quota(ctx); err == nil {
		a.update(func() {
			if a.sess != nil && a.sess.client == c {
				a.sess.quota = q
			}
		})
	}
	if u.Avatar == "" {
		return
	}
	resp, err := c.Fetch(ctx, u.Avatar, 0, 0)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	data, err := readAll(resp.Body, 4<<20)
	if err != nil {
		return
	}
	a.update(func() {
		if a.sess == nil || a.sess.client != c {
			return
		}
		if bmp, err := ui.DecodeBitmap(data); err == nil {
			a.sess.avatar = bmp
		}
	})
}

// signOut forgets the session.
func (a *App) signOut() {
	if a.store != nil && !a.fake {
		a.store.Clear()
	}
	a.onSignedOut()
	a.sess = nil
	a.login = loginState{}
	a.phase = phaseLogin
	a.router.Replace("/files")
}

// onSignedIn starts what the signed-in app shows.
func (a *App) onSignedIn() {
	a.startThumbs()
}

// onSignedOut stops the work of the session.
func (a *App) onSignedOut() {
	a.login.stopQR()
	a.stopThumbs()
	a.listings = nil
}
