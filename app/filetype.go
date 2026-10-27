package app

import (
	"path"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/baidu"
)

// kind is what a file is, for its icon and how it previews.
type kind int

const (
	kindOther kind = iota
	kindFolder
	kindImage
	kindVideo
	kindAudio
	kindText
	kindCode
	kindMarkdown
	kindPDF
	kindArchive
	kindDoc
	kindSheet
)

var extKinds = map[string]kind{}

func init() {
	add := func(k kind, exts string) {
		for _, e := range strings.Fields(exts) {
			extKinds[e] = k
		}
	}
	add(kindImage, "jpg jpeg png gif webp bmp heic heif tif tiff svg ico avif")
	add(kindVideo, "mp4 mkv avi mov wmv flv webm m4v ts m2ts rmvb rm 3gp mpg mpeg vob ogv")
	add(kindAudio, "mp3 flac wav ogg oga opus m4a aac ape wma alac aiff")
	add(kindText, "txt log csv tsv ini conf cfg srt ass ssa vtt nfo env properties")
	add(kindCode, "go js mjs cjs ts tsx jsx json jsonc yaml yml toml xml html htm css scss less py rb rs c h cc cpp hpp cs java kt kts swift m mm php sh bash zsh fish ps1 bat lua sql r dart scala sc sbt graphql gql graphqls zig nim vue svelte proto gradle makefile dockerfile diff patch tex")
	add(kindMarkdown, "md markdown mdx")
	add(kindPDF, "pdf")
	add(kindArchive, "zip rar 7z tar gz tgz bz2 xz zst iso dmg")
	add(kindDoc, "doc docx ppt pptx key pages rtf odt odp epub mobi azw3")
	add(kindSheet, "xls xlsx numbers ods")
}

// kindOf returns what a file is, by its extension and else the netdisk's
// category.
func kindOf(f *baidu.File) kind {
	if f.IsDir {
		return kindFolder
	}
	if strings.EqualFold(f.Name, ".Rprofile") {
		return kindCode
	}
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(f.Name), "."))
	if ext == "" {
		switch strings.ToLower(f.Name) {
		case "makefile", "dockerfile", "license", "readme":
			return kindText
		}
	}
	if k, ok := extKinds[ext]; ok {
		return k
	}
	switch f.Category {
	case baidu.CategoryVideo:
		return kindVideo
	case baidu.CategoryAudio:
		return kindAudio
	case baidu.CategoryImage:
		return kindImage
	}
	return kindOther
}

// look is how a kind of file shows.
type look struct {
	icon  string
	color ui.Color
	label string
}

func (k kind) look(p *palette) look {
	switch k {
	case kindFolder:
		return look{"folder", p.Folder, "文件夹"}
	case kindImage:
		return look{"file-image", ui.Oklch(0.68, 0.16, 330), "图片"}
	case kindVideo:
		return look{"file-video", ui.Oklch(0.64, 0.19, 25), "视频"}
	case kindAudio:
		return look{"file-music", ui.Oklch(0.66, 0.16, 300), "音频"}
	case kindText:
		return look{"file-text", ui.Oklch(0.62, 0.05, 260), "文本"}
	case kindCode:
		return look{"file-code", ui.Oklch(0.66, 0.14, 160), "代码"}
	case kindMarkdown:
		return look{"file-text", ui.Oklch(0.62, 0.13, 230), "Markdown"}
	case kindPDF:
		return look{"file-type", ui.Oklch(0.6, 0.2, 27), "PDF"}
	case kindArchive:
		return look{"file-archive", ui.Oklch(0.7, 0.13, 70), "压缩包"}
	case kindDoc:
		return look{"file-text", ui.Oklch(0.58, 0.16, 255), "文档"}
	case kindSheet:
		return look{"file-spreadsheet", ui.Oklch(0.62, 0.15, 150), "表格"}
	}
	return look{"file", ui.Oklch(0.62, 0.02, 260), "文件"}
}

// hasThumb says whether the netdisk makes thumbnails of a kind.
func (k kind) hasThumb() bool { return k == kindImage || k == kindVideo }
