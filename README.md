# BaiduNetGo

A native, cross-platform Baidu Netdisk desktop client in pure Go.

- UI: [MyGo](https://github.com/egoist/mygo)'s native `ui` package, drawn on the GPU, no webview, no cgo
- API: [BaiduPCS-Go](https://github.com/qjfoidnh/BaiduPCS-Go) used as a library
- Previews: images, text and code, Markdown, audio and PDF load into memory and never touch disk; video streams straight into [mpv](https://mpv.io)

## Development

```bash
go tool mygo dev     # run with live rebuild
go test ./...
go tool mygo build   # package for this platform
```

On Linux the app needs GTK 3; mpv must be on `PATH` (or set in Settings) to play video.

## Credits

See [NOTICE](NOTICE).
