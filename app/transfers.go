package app

import "github.com/pengstem/baidunetgo/internal/baidu"

// download queues files to download.
func (a *App) download(files []baidu.File) {
	a.toast("下载功能即将到来", "")
}
