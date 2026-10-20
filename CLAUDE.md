# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

BaiduNetGo is a Baidu Netdisk desktop client in pure Go (no cgo). The UI uses MyGo's native `ui` package (`github.com/egoist/mygo/ui`, immediate mode, drawn on the GPU, no webview). Netdisk access uses BaiduPCS-Go's importable packages (`baidupcs`, `requester`, `pcsutil`) as a library. UI strings are Chinese, and `main.go` forces the zh-Hans locale for MyGo's own controls.

## Commands

```bash
go tool mygo dev                      # run with live rebuild (real netdisk)
BAIDUNETGO_FAKE=1 go run .            # run against the in-process fake netdisk, no account needed
go test ./...                         # all tests (app tests take ~30s)
go test ./app -run TestPreviewImage -v
go test -tags live ./internal/baidu -run Live -v   # real netdisk: QR endpoint, membership, ranges (uses BaiduPCS-Go's login)
BAIDUNETGO_SELFTEST=report.json BAIDUNETGO_PROBE=1 go run .   # self-test every feature against the real account, plus a speed probe
go vet ./... && gofmt -l .
go tool mygo build                    # package for this platform into build/
CGO_ENABLED=0 GOOS=windows go build -o /dev/null .   # cross-compile check
```

**Releases.** Every push to `main` releases (`.github/workflows/release.yml`): tests, then `go run ./tools/release` picks the version (last tag + patch; `#minor`/`#major` in a commit, or a higher version set by hand in `mygo.json`) and writes its `CHANGELOG.md` section (the `## Unreleased` notes if any, else the commits grouped 新增/修复/改进), then the workflow pushes a `Release vX.Y.Z [skip ci]` commit and tag to main, builds Linux/Windows on Ubuntu and `darwin/universal` on macOS, uploads to the release and publishes it. So `git pull --rebase` before pushing. The repo is private, so MyGo's self-updater (`updates` in `mygo.json`, which needs a public repo) is not set up.

Dev-only environment variables (see `capture.go`, `app/app.go`, `app/session.go`):
- `BAIDUNETGO_FAKE=1`: sign in to `internal/fakepan` instead of Baidu.
- `BAIDUNETGO_ROUTE=/files/照片`: router start page.
- `BAIDUNETGO_GRID=1`: start in grid view.
- `BAIDUNETGO_CAPTURE=out.png` (with `BAIDUNETGO_CAPTURE_AFTER=4s`): write a PNG of the window, then quit. This is how to look at the real window from a script.
- `BAIDUNETGO_SELFTEST=report.json` (`selftest.go`, `internal/selftest`): headless run of every feature against the saved session, else BaiduPCS-Go's login, inside a `/BaiduNetGo自检-…` folder it deletes afterwards. `BAIDUNETGO_PROBE=1` adds the download speed probe.
- `BAIDUNETGO_SNAPSHOTS=dir` with `go test ./app`: tests that call `save(t, tt, name)` write `ui.Tester.Image()` PNGs there, for visual checks without a window.

## Architecture

**UI threading model.** `app.App` holds all state. The view (`App.View`) runs on the main thread every frame and only reads state. Network work runs on goroutines and writes results back through `a.update(fn)`, which is `Window.Update` in the real app. With no window (tests), it queues `fn` and `runQueued` drains the queue at the start of the next frame. Never mutate `App` state directly from a goroutine. Toasts and clipboard writes need a `*ui.Context`, so code outside the view queues them (`a.toast`, `a.pendingToasts`, `a.copy`) and the view flushes them.

**Phases and routing.** `App.phase` is booting → login → ready. Once ready, `shell.go` draws the sidebar and title bar around a `ui.Router` with these routes: `/files/{path...}` (netdisk path, segments `url.PathEscape`d by `filesRoute`/`dirOfRoute`), `/search?q=`, `/transfers`, `/shares`, `/offline`, `/recycle`, `/settings`. The title bar's controls switch on `a.section()`. Each directory gets its own `listing` (cached in `a.listings`, keyed by dir), so a page sliding out during a router transition keeps showing its own files.

**Overlays.** One `a.dlg` (`dialog` with a `dialogKind`) drives every modal through `a.dialogs`. The file preview is a separate overlay (`a.pv`, `previewOverlay`). The image preview and the grid/list thumbnails share `ui.ElementTransition{Match: thumbMatch{fsid}}`. The source thumbnail is skipped while `a.previewing(f)`, so the image grows out of it and shrinks back.

**Netdisk client (`internal/baidu`).** `baidu.Open(creds)` builds a `baidupcs.BaiduPCS` from cookies and never touches BaiduPCS-Go's global config. It also turns TLS verification back on (the library disables it). Two code paths:
- **Library calls** for `LocateDownload` (download links, which need the uid, fetched once from Tieba), upload (`internal/pcsport`, ported from BaiduPCS-Go's `internal/` under Apache-2.0), and share-link saving.
- **The pan.baidu.com web API**, called directly through `getJSON`/`postForm` against `c.base`, for everything else: list (with `web=1` for thumbnails), file manager ops, recycle bin, shares, `cloud_dl`. Writes carry a `bdstoken`, fetched from `/api/loginStatus`.

`baidu.OpenAt(creds, base)` points the client at a fake server. `c.fake()` swaps in fake-friendly behavior in the few library-bound paths (download URL, upload, save share). Errno -6/-4/111 maps to `ErrNotLoggedIn`, which signs the user out.

**Download hosts only serve bounded ranges.** `bytes=a-b` works. No Range header, or an open-ended `bytes=a-`, returns 403 (small files sometimes slip through). Every content read must therefore be bounded: `remote` (chunked, `remote.Parallel` at once), `transfer`, `tune`. mpv, whose ffmpeg sends open-ended ranges, reads through `internal/stream`, a loopback HTTP server with a random token per file. It splits requests into bounded chunks fetched `Ahead` at a time and re-requests the link when it expires. fakepan's `/file` enforces the same rule.

**Membership and auto-tuning.** loginStatus's `vip_type` uses its own scheme (22 for an annual SVIP). The membership level comes from `/rest/2.0/xpan/nas?method=uinfo` (0/1/2), and the expiry from `/rest/2.0/membership/user`. `internal/tune` probes one setting after another, each with the best of those before it: the download host (`DownloadURLs` returns every host LocateDownload offers, `Client.SetHosts` ranks them for `DownloadURL`), connections per file, request (block) size, files at once (a second large file alongside), and upload connections (`ProbeUpload`: random parts to a precreated upload that is never joined, so nothing is kept). `Recommend` picks the fewest within 90% of the fastest (never-refused samples only), the block size nearest 4 MB within 5%, and derives how many connections players and previews read with (`Ahead`, enough for 8 MB/s). Defaults by level come from `ForVIP`. The app measures once per membership level per week (`app/tuning.go`, stored in `settings.Tune`; a result whose `Version` is older than `tune.Version` is measured again). `settings.Manual` overrides only the counts (threads, files, upload); host, block and read-ahead stay measured. `useSettings` pushes the result into the transfer manager (`Threads`, `Upload`, `Parallel`, `Block`; a resumed download keeps the block size in its `.bngdl.json`), the client's host ranking, `remote.Parallel` and the stream server. fakepan serves downloads on two hosts (a mirror port, listed by `/locatedownload`) and discards `/upload?probe`.

**Sign-in sources.** The saved session (`internal/auth`) comes first. If there is none, or it has expired, the app migrates the login of another client on this computer (`internal/migrate`): Baidu's own Linux client first (its Electron cookie store, read with a small pure-Go SQLite reader), then BaiduPCS-Go's active account (`internal/pcsimport`, reading `pcs_config.json`). It skips this when the user signed out on purpose (`settings.SkipImport`). The login screen also offers it as a link. Every sign-in is saved at once, and saved again when `Client.Credentials` changes (the uid found, cookies the netdisk rotated; `Client.OnChange`) and on quit.

**One keyring key for every build.** `mygo.Secrets` names its service after the build (`<identifier>.dev` under `mygo dev`, the app name under `go run`, the identifier when packaged), so `OpenStore` pins the key to `com.pengstem.baidunetgo`. `auth.Store.Older` lists the old services: `Load` tries their keys and saves the session again under the pinned one.

**Fake netdisk (`internal/fakepan`).** An `httptest` server that emulates every web endpoint the client uses, over an in-memory tree with generated images, a PDF and a WAV. App UI tests (`app/*_test.go`, helper `signedIn` in `app/app_test.go`) drive real views with `ui.Tester` against it. When adding a new API call, add its endpoint to fakepan too.

**No-disk previews.** `internal/remote` reads files into memory via Range requests (`ReadAll` with size limits, or a seekable `File` with a chunk cache for streaming audio) and re-requests the link on 403. PDFs render through `internal/pdf`: PDFium compiled to WebAssembly, run in wazero, pool started lazily by `pdf.Warm`. Video, and audio formats MyGo's `audio` package can't decode, go to mpv via `internal/player` with `--user-agent` and `--http-header-fields-append=Cookie: …`.

**Transfers (`internal/transfer`).** `Manager` is a queue run by `Parallel`. Downloads fetch 4 MB blocks over `Threads` connections into `<file>.bngdl`, with a `.bngdl.json` sidecar of finished blocks for resume. Task state persists to `transfers.json` in the user data dir. `OnChange`/`OnFinish` are called outside the manager's lock and read tasks back through `Tasks()`. Don't call them while holding `m.mu`: that deadlocked once.

**Session storage (`internal/auth`).** Credentials are AES-GCM encrypted in `session.enc` under the user data dir. The key lives in `mygo.Secrets` (the system keyring), with a 0600 key-file fallback. Settings are plain JSON (`app/settings.go`).

## MyGo UI gotchas found here

- Absolutely positioned elements paint above their in-flow siblings. To draw something *under* content (segmented thumbs), paint it in the parent's `Draw` callback. To layer content over absolute decorations, make the content absolute too.
- `ui.List` row selection paints the accent color itself (`Selected()`). The browser covers it with an opaque `selectionFill`.
- Dialogs focus their first focusable element. A focused button keeps Enter, so main buttons use `AutoFocus` (only in dialogs without a text field).
- Virtualized `List` rows get no enter/exit transitions. Staggered entrances use `Keyframes` inside the row (`entering` in `browser.go`).
- The grid view is not `ui.GridView`: GridView builds each line as an element, so a tile that a resize moves to another line is built anew and jumps. `app/grid.go` positions tiles itself, all under one parent keyed by fsid (percent `Left`/`Width`, so the width never lags), and on a cell change `Jump`s the slot's offset to where it showed and `AnimateTo`s it back to 0. Element `Transition`s don't help here: MyGo skips layout transitions on frames where the window resized.
- Theme: the look is a native macOS app's, modeled on Pulse (egoist's MyGo app): the sidebar draws no background so the window's `VibrancySidebar` material shows (`main.go`: `TitleBarHiddenInset`, 52 px title bar), pages marked by white glyphs on colored tiles, the chosen one on a solid accent backdrop, a value at each row's end (`navValue`); the pane is white with a title over a subtitle (`pageTitle`), round gray tool buttons (`toolButton`), capsule segmented controls (`segmented`/`segmentedOf`), striped 28 px list rows (`stripe`), rounded gray `group` panels for settings and summaries, and the system typeface (no embedded fonts; `figure` sets tabular digits). `applyTheme` swaps in the app's light/dark `ui.Theme` each frame. Extra colors are in `palette`. Motion is one curve and three lengths (`motionHover` 120 ms, `motionFade` 160 ms, `motionPanel` 180 ms), plus `popIn` and `rowEnter`, all in `app/theme.go`; no springs or bounce.
- Sidebar icons are drawn in code (`app/navicons.go`) so their parts can move; `navGlyph` takes the time into the gesture. A page's icon plays once when the page opens (`a.navRun`).
