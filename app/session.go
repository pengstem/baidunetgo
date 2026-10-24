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
	"github.com/pengstem/baidunetgo/internal/migrate"
	"github.com/pengstem/baidunetgo/internal/stream"
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

// keyService is the keyring service the session's key is kept under, the
// same for `go run`, `mygo dev` and the packaged app, which each have a
// service of their own in mygo.Secrets.
const keyService = "com.pengstem.baidunetgo"

// OpenStore finds where the session is kept, nil when it cannot.
func OpenStore() *auth.Store {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		log.Printf("session: %v", err)
		return nil
	}
	return &auth.Store{
		Dir:     dir,
		Keyring: mygo.Secrets.Service(keyService),
		// Where earlier builds kept the key.
		Older: []auth.Keyring{mygo.Secrets, mygo.Secrets.Service(keyService + ".dev"), mygo.Secrets.Service(mygo.App.Name())},
	}
}

// boot signs in with the saved session, else with an account another
// client on this computer is signed in with, else asks to sign in.
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
	skip := a.settings.SkipImport
	go func() {
		var cr baidu.Credentials
		var expired error
		if err := a.store.Load(&cr); err == nil {
			c := baidu.Open(cr)
			u, err := c.User(context.Background())
			if err == nil {
				a.signedIn(c, u, false)
				return
			}
			if !errors.Is(err, baidu.ErrNotLoggedIn) {
				// Offline, say: keep the session for the next try.
				a.signInFailed(err, false)
				return
			}
			expired = err
		} else if !errors.Is(err, auth.ErrNoSession) {
			log.Printf("session: %v", err)
		}
		if !skip && a.migrate(migrate.Find(), cr.BDUSS) {
			return
		}
		if expired != nil {
			a.signInFailed(expired, false)
			return
		}
		a.update(func() { a.phase = phaseLogin })
	}()
}

// migrate signs in with the first of the accounts that still signs in,
// skipping the BDUSS known not to. It reports whether one did.
func (a *App) migrate(accs []migrate.Account, skip string) bool {
	for _, acc := range accs {
		if acc.Creds.BDUSS == skip {
			continue
		}
		c := baidu.Open(acc.Creds)
		u, err := c.User(context.Background())
		if err != nil {
			log.Printf("session: %s: %v", acc.From, err)
			continue
		}
		a.signedIn(c, u, true)
		a.update(func() { a.toast("已迁移"+acc.From+"的登录", "success") })
		return true
	}
	return false
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
		a.signInFailed(err, fresh)
		return
	}
	a.signedIn(c, u, fresh)
}

// signInFailed shows the sign-in screen with why.
func (a *App) signInFailed(err error, fresh bool) {
	a.update(func() {
		a.phase = phaseLogin
		a.login.busy = false
		if fresh || !errors.Is(err, baidu.ErrNotLoggedIn) {
			a.login.err = err.Error()
		} else {
			a.login.err = "登录已过期，请重新登录"
		}
	})
}

// signedIn keeps the session, now and whenever it changes, and shows the
// netdisk. fresh is a sign-in the user (or a migration) just made.
func (a *App) signedIn(c *baidu.Client, u *baidu.User, fresh bool) {
	if a.store != nil && !a.fake {
		a.saveMu.Lock()
		a.saving, a.saved = c, nil
		a.saveMu.Unlock()
		c.OnChange(func() { a.persist(c) })
		a.persist(c)
	}
	a.update(func() {
		if fresh && a.settings.SkipImport {
			a.settings.SkipImport = false
			a.settings.save()
		}
		a.sess = &session{client: c, user: u}
		a.login = loginState{}
		a.phase = phaseReady
		a.onSignedIn()
	})
	go a.loadProfile(c, u)
}

// persist saves what c, the signed-in client, signs in with, when it
// changed since the last save. It runs on any goroutine.
func (a *App) persist(c *baidu.Client) {
	if a.store == nil || a.fake {
		return
	}
	cr := c.Credentials()
	a.saveMu.Lock()
	defer a.saveMu.Unlock()
	if c != a.saving || (a.saved != nil && *a.saved == cr) {
		return
	}
	if err := a.store.Save(cr); err != nil {
		log.Printf("session: save: %v", err)
		return
	}
	a.saved = &cr
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
		a.saveMu.Lock()
		a.store.Clear()
		a.saving, a.saved = nil, nil
		a.saveMu.Unlock()
		// Signed out on purpose: do not sign in with another client's
		// account again by itself.
		a.settings.SkipImport = true
		a.settings.save()
	}
	a.onSignedOut()
	if !a.fake {
		a.forgetThumbs() // another account may sign in
	}
	a.sess = nil
	a.login = loginState{}
	a.phase = phaseLogin
	a.router.Replace("/files")
}

// onSignedIn starts what the signed-in app shows.
func (a *App) onSignedIn() {
	a.clipCheck = true
	a.startThumbs()
	a.startTransfers()
	if srv, err := stream.Start(a.sess.client, baidu.Expired); err == nil {
		a.stream = srv
	} else {
		log.Printf("stream: %v", err)
	}
	a.useSettings()
	a.maybeTune()
}

// onSignedOut stops the work of the session.
func (a *App) onSignedOut() {
	a.login.stopQR()
	a.stopThumbs()
	if a.stream != nil {
		a.stream.Close()
		a.stream = nil
	}
	a.stopTransfers()
	a.shares, a.offline, a.recycle = sharesState{}, offlineState{}, recycleState{}
	a.listings = nil
}
