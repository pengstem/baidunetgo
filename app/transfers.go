package app

import "github.com/pengstem/baidunetgo/internal/baidu"

// download queues files to download.
func (a *App) download(files []baidu.File) {
	a.toast("下载功能即将到来", "")
}

// pickUpload asks for files to upload into dir.
func (a *App) pickUpload(dir string) { a.toast("上传功能即将到来", "") }

// share asks how to share files.
func (a *App) share(files []baidu.File) { a.toast("分享功能即将到来", "") }
