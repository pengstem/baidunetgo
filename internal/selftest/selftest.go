// Package selftest exercises every feature against a signed-in account,
// in a folder of its own that it deletes when done, and reports what
// worked: `BAIDUNETGO_SELFTEST=report.json` runs it with the session the
// app saved.
package selftest

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/png"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/audio"

	"github.com/pengstem/baidunetgo/internal/baidu"
	"github.com/pengstem/baidunetgo/internal/fakepan"
	"github.com/pengstem/baidunetgo/internal/pdf"
	"github.com/pengstem/baidunetgo/internal/player"
	"github.com/pengstem/baidunetgo/internal/remote"
	"github.com/pengstem/baidunetgo/internal/stream"
	"github.com/pengstem/baidunetgo/internal/transfer"
	"github.com/pengstem/baidunetgo/internal/tune"
)

// Check is one thing tested.
type Check struct {
	Name   string        `json:"name"`
	OK     bool          `json:"ok"`
	Detail string        `json:"detail,omitempty"`
	Took   time.Duration `json:"took"`
}

// Report is what a run found.
type Report struct {
	Account map[string]any `json:"account"`
	VIP     int            `json:"vip"`
	Checks  []Check        `json:"checks"`
	Probe   *tune.Result   `json:"probe,omitempty"`
	Config  tune.Config    `json:"config"`
}

type run struct {
	ctx  context.Context
	c    *baidu.Client
	rep  *Report
	tmp  string
	box  string // the netdisk folder it works in
	logf func(string, ...any)
}

func (r *run) check(name string, fn func() (string, error)) bool {
	start := time.Now()
	detail, err := fn()
	ck := Check{Name: name, OK: err == nil, Detail: detail, Took: time.Since(start).Round(time.Millisecond)}
	if err != nil {
		ck.Detail = strings.TrimSpace(detail + " " + err.Error())
	}
	r.rep.Checks = append(r.rep.Checks, ck)
	mark := "✓"
	if !ck.OK {
		mark = "✗"
	}
	r.logf("%s %-24s %6s  %s", mark, name, ck.Took.Round(10*time.Millisecond), ck.Detail)
	return ck.OK
}

// Run tests c, logging as it goes. probe sets whether to measure download
// speeds, which reads a few hundred megabytes.
func Run(ctx context.Context, c *baidu.Client, probe bool, logf func(string, ...any)) *Report {
	tmp, _ := os.MkdirTemp("", "bng-selftest")
	defer os.RemoveAll(tmp)
	r := &run{ctx: ctx, c: c, rep: &Report{}, tmp: tmp, logf: logf,
		box: "/BaiduNetGo自检-" + time.Now().Format("150405")}

	var user *baidu.User
	r.check("账户信息", func() (string, error) {
		info, err := c.LoginInfo(ctx)
		if err != nil {
			return "", err
		}
		// Keep what tells the membership, never tokens.
		r.rep.Account = map[string]any{}
		for k, v := range info {
			lk := strings.ToLower(k)
			if strings.Contains(lk, "vip") || strings.Contains(lk, "level") || lk == "username" || lk == "uk" || strings.Contains(lk, "member") {
				r.rep.Account[k] = v
			}
		}
		user, err = c.User(ctx)
		if err != nil {
			return "", err
		}
		r.rep.VIP = user.VIP
		return fmt.Sprintf("%s uid=%d 会员=%q 字段=%v", user.Name, user.UID, user.VIPLabel(), r.rep.Account), nil
	})
	if user == nil {
		return r.rep
	}
	r.check("容量", func() (string, error) {
		q, err := c.Quota(ctx)
		return fmt.Sprintf("%d / %d GB", q.Used>>30, q.Total>>30), err
	})
	r.check("列出根目录", func() (string, error) {
		files, err := c.List(ctx, "/", baidu.Order{})
		return fmt.Sprintf("%d 项", len(files)), err
	})
	if !r.check("新建文件夹", func() (string, error) { return r.box, c.Mkdir(ctx, r.box) }) {
		return r.rep
	}
	defer r.cleanup()

	files := r.uploads()
	r.listing(files)
	r.previews(files)
	r.ops()
	r.sharing()
	r.offline()
	r.transfers(files)
	r.recycle()

	if probe {
		r.speed(user.VIP)
	} else {
		r.rep.Config = tune.ForVIP(user.VIP)
	}
	return r.rep
}

type local struct {
	name string
	data []byte
	sum  string
}

func (r *run) uploads() []local {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	big := make([]byte, 9<<20+12345) // over two 4 MB parts
	rng.Read(big)
	var img bytes.Buffer
	{
		// A PNG of the fake netdisk's, made distinct.
		png := fakepan.SamplePNG(rng.Intn(100), 640, 400)
		img.Write(png)
	}
	files := []local{
		{name: "说明.txt", data: []byte("BaiduNetGo 自检\n你好，网盘。\n" + time.Now().String())},
		{name: "随机数据.bin", data: big},
		{name: "图片.png", data: img.Bytes()},
		{name: "文档.pdf", data: fakepan.SamplePDF(3)},
		{name: "和弦.wav", data: fakepan.SampleWAV(3)},
	}
	for i := range files {
		f := &files[i]
		sum := md5.Sum(f.data)
		f.sum = hex.EncodeToString(sum[:])
		p := filepath.Join(r.tmp, f.name)
		os.WriteFile(p, f.data, 0o644)
		r.check("上传 "+f.name, func() (string, error) {
			start := time.Now()
			err := r.c.Upload(r.ctx, p, path.Join(r.box, f.name), 4, nil, nil, nil)
			sec := time.Since(start).Seconds()
			return fmt.Sprintf("%s，%.1f KB/s", size(len(f.data)), float64(len(f.data))/1024/sec), err
		})
	}
	if f, ok := r.resumed(rng); ok {
		files = append(files, f)
	}
	return files
}

// resumed uploads a file in two goes, stopping the first once a part is
// sent, and checks what the netdisk joined.
func (r *run) resumed(rng *rand.Rand) (local, bool) {
	f := local{name: "续传.bin", data: make([]byte, 13<<20+777)}
	rng.Read(f.data)
	sum := md5.Sum(f.data)
	f.sum = hex.EncodeToString(sum[:])
	p := filepath.Join(r.tmp, f.name)
	os.WriteFile(p, f.data, 0o644)
	ok := false
	r.check("断点续传", func() (string, error) {
		remotePath := path.Join(r.box, f.name)
		ctx, cancel := context.WithCancel(r.ctx)
		defer cancel()
		var saved baidu.UploadState
		err := r.c.Upload(ctx, p, remotePath, 1, nil, nil, func(s baidu.UploadState) {
			saved = s
			if slices.ContainsFunc(s.Parts, func(x string) bool { return x != "" }) {
				cancel()
			}
		})
		if !errors.Is(err, context.Canceled) {
			return "", fmt.Errorf("第一次没有停在中途：%v", err)
		}
		before := 0
		for _, x := range saved.Parts {
			if x != "" {
				before++
			}
		}
		if err := r.c.Upload(r.ctx, p, remotePath, 4, nil, &saved, nil); err != nil {
			return "", err
		}
		got, err := remote.ReadAll(r.ctx, r.c, baidu.Expired, remotePath, int64(len(f.data)), 64<<20, nil)
		if err != nil {
			return "", err
		}
		if gs := md5.Sum(got); hex.EncodeToString(gs[:]) != f.sum {
			return "", fmt.Errorf("续传后的内容不一致")
		}
		ok = true
		return fmt.Sprintf("先传 %d/%d 块，续传后内容一致", before, len(saved.Parts)), nil
	})
	return f, ok
}

func (r *run) listing(files []local) {
	r.check("列出并核对", func() (string, error) {
		got, err := r.c.List(r.ctx, r.box, baidu.Order{})
		if err != nil {
			return "", err
		}
		var missing []string
		for _, f := range files {
			if !slices.ContainsFunc(got, func(g baidu.File) bool { return g.Name == f.name && g.Size == int64(len(f.data)) }) {
				missing = append(missing, f.name)
			}
		}
		thumbs := 0
		for _, g := range got {
			if g.Thumb != "" {
				thumbs++
			}
		}
		if len(missing) > 0 {
			return "", fmt.Errorf("缺少或大小不对：%v", missing)
		}
		return fmt.Sprintf("%d 项，%d 个有缩略图", len(got), thumbs), nil
	})
	r.check("搜索", func() (string, error) {
		got, err := r.c.Search(r.ctx, r.box, "随机数据")
		if err != nil {
			return "", err
		}
		if len(got) == 0 {
			return "搜索索引可能尚未更新", fmt.Errorf("没有结果")
		}
		return fmt.Sprintf("%d 个结果", len(got)), nil
	})
	r.check("缩略图", func() (string, error) {
		got, err := r.c.List(r.ctx, r.box, baidu.Order{})
		if err != nil {
			return "", err
		}
		for _, g := range got {
			if g.Name == "图片.png" && g.Thumb != "" {
				resp, err := r.c.Fetch(r.ctx, g.Thumb, 0, 0)
				if err != nil {
					return "", err
				}
				defer resp.Body.Close()
				_, _, err = image.Decode(resp.Body)
				return "解码成功", err
			}
		}
		return "新上传的图片还没有缩略图", fmt.Errorf("无缩略图")
	})
}

func (r *run) previews(files []local) {
	byName := map[string]local{}
	for _, f := range files {
		byName[f.name] = f
	}
	read := func(name string) ([]byte, error) {
		f := byName[name]
		return remote.ReadAll(r.ctx, r.c, baidu.Expired, path.Join(r.box, name), int64(len(f.data)), 64<<20, nil)
	}
	r.check("直链 + 内存读取", func() (string, error) {
		start := time.Now()
		u, err := r.c.DownloadURL(r.ctx, path.Join(r.box, "随机数据.bin"))
		if err != nil {
			return "", err
		}
		link := time.Since(start)
		data, err := read("随机数据.bin")
		if err != nil {
			return "", err
		}
		sum := md5.Sum(data)
		if hex.EncodeToString(sum[:]) != byName["随机数据.bin"].sum {
			return "", fmt.Errorf("内容不一致")
		}
		host := u
		if i := strings.Index(u, "://"); i >= 0 {
			host = strings.SplitN(u[i+3:], "/", 2)[0]
		}
		return fmt.Sprintf("取链接 %v，主机 %s，内容一致", link.Round(time.Millisecond), host), nil
	})
	r.check("文本预览", func() (string, error) {
		data, err := read("说明.txt")
		if err == nil && !bytes.Equal(data, byName["说明.txt"].data) {
			err = fmt.Errorf("内容不一致")
		}
		return "", err
	})
	r.check("图片预览", func() (string, error) {
		data, err := read("图片.png")
		if err != nil {
			return "", err
		}
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return "", err
		}
		return fmt.Sprint(img.Bounds().Size()), nil
	})
	r.check("PDF 预览", func() (string, error) {
		data, err := read("文档.pdf")
		if err != nil {
			return "", err
		}
		doc, err := pdf.Open(r.ctx, data)
		if err != nil {
			return "", err
		}
		defer doc.Close()
		_, err = doc.Render(0, 600)
		return fmt.Sprintf("%d 页，首页已渲染", len(doc.Pages)), err
	})
	r.check("音频边下边播", func() (string, error) {
		f := byName["和弦.wav"]
		stream := remote.Open(r.ctx, r.c, baidu.Expired, path.Join(r.box, f.name), int64(len(f.data)))
		dec, format, err := audio.NewDecoder(stream)
		if err != nil {
			return "", err
		}
		buf := make([]float32, 4096)
		_, err = dec.Read(buf)
		return fmt.Sprintf("%s，%.1f 秒", format, float64(dec.Len())/float64(dec.SampleRate())), err
	})
	srv, err := stream.Start(r.c, baidu.Expired)
	if err != nil {
		r.check("本地串流服务", func() (string, error) { return "", err })
		return
	}
	defer srv.Close()
	playMPV := func(p string, size int64, secs string) (string, error) {
		mpv, err := player.Find("")
		if err != nil {
			return "", err
		}
		args := append([]string{"--ao=null", "--vo=null", "--no-config", "--really-quiet", "--length=" + secs}, player.Args(srv.URL(p, size), "自检", nil)...)
		ctx, cancel := context.WithTimeout(r.ctx, 60*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, mpv, args...).CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	r.check("mpv 播放（经本地串流）", func() (string, error) {
		f := byName["和弦.wav"]
		return playMPV(path.Join(r.box, f.name), int64(len(f.data)), "1")
	})
	r.check("mpv 播放大文件（经本地串流）", func() (string, error) {
		big, err := r.bigVideo()
		if err != nil {
			return "", err
		}
		start := time.Now()
		out, err := playMPV(big.Path, big.Size, "3")
		return fmt.Sprintf("%s（%s），用时 %v %s", big.Name, size(int(big.Size)), time.Since(start).Round(time.Millisecond), out), err
	})
}

func (r *run) ops() {
	p := func(s string) string { return path.Join(r.box, s) }
	r.check("重命名", func() (string, error) { return "", r.c.Rename(r.ctx, p("说明.txt"), "说明-改名.txt") })
	r.check("新建子文件夹", func() (string, error) { return "", r.c.Mkdir(r.ctx, p("子文件夹")) })
	r.check("复制", func() (string, error) {
		return "", r.c.Copy(r.ctx, []string{p("说明-改名.txt")}, p("子文件夹"))
	})
	r.check("移动", func() (string, error) { return "", r.c.Move(r.ctx, []string{p("图片.png")}, p("子文件夹")) })
	r.check("核对操作结果", func() (string, error) {
		sub, err := r.c.List(r.ctx, p("子文件夹"), baidu.Order{})
		if err != nil {
			return "", err
		}
		var names []string
		for _, f := range sub {
			names = append(names, f.Name)
		}
		if !slices.Contains(names, "说明-改名.txt") || !slices.Contains(names, "图片.png") {
			return "", fmt.Errorf("子文件夹里是 %v", names)
		}
		return strings.Join(names, "、"), nil
	})
}

func (r *run) sharing() {
	var link, pwd string
	r.check("创建分享", func() (string, error) {
		pwd = "bng1"
		var err error
		link, err = r.c.CreateShare(r.ctx, []string{path.Join(r.box, "子文件夹")}, pwd, 1)
		return link, err
	})
	if link == "" {
		return
	}
	var id int64
	r.check("分享列表与提取码", func() (string, error) {
		shares, err := r.c.Shares(r.ctx)
		if err != nil {
			return "", err
		}
		for _, s := range shares {
			if strings.HasPrefix(s.Path, r.box) {
				id = s.ID
			}
		}
		if id == 0 {
			return "", fmt.Errorf("列表里没有刚建的分享（共 %d 个）", len(shares))
		}
		got, err := r.c.SharePassword(r.ctx, id)
		if err == nil && got != pwd {
			err = fmt.Errorf("提取码是 %q", got)
		}
		return "", err
	})
	r.check("转存分享链接", func() (string, error) {
		dir := path.Join(r.box, "转存")
		if err := r.c.Mkdir(r.ctx, dir); err != nil {
			return "", err
		}
		text := "链接: " + link + "?pwd=" + pwd + " 提取码: " + pwd
		l, p, ok := baidu.ParseShareLink(text)
		if !ok {
			return "", fmt.Errorf("解析不出 %q", text)
		}
		n, err := r.c.SaveShare(r.ctx, l, p, dir)
		return fmt.Sprintf("%d 项（转存自己的分享，网盘可能拒绝）", n), err
	})
	if id != 0 {
		r.check("取消分享", func() (string, error) { return "", r.c.CancelShares(r.ctx, []int64{id}) })
	}
}

func (r *run) offline() {
	src := "https://www.baidu.com/img/flexible/logo/pc/result.png"
	before, _ := r.c.Offlines(r.ctx)
	if !r.check("添加离线任务", func() (string, error) { return src, r.c.AddOffline(r.ctx, src, r.box) }) {
		return
	}
	var task baidu.Offline
	r.check("离线任务进度", func() (string, error) {
		for range 15 {
			tasks, err := r.c.Offlines(r.ctx)
			if err != nil {
				return "", err
			}
			for _, t := range tasks {
				if t.Source == src && !slices.ContainsFunc(before, func(b baidu.Offline) bool { return b.ID == t.ID }) {
					task = t
				}
			}
			if task.ID != 0 && task.Status != 1 {
				break
			}
			time.Sleep(2 * time.Second)
		}
		if task.ID == 0 {
			return "", fmt.Errorf("列表里没有新任务")
		}
		return task.StatusText() + "，" + size(int(task.Size)), nil
	})
	if task.ID != 0 {
		r.check("删除离线任务", func() (string, error) { return "", r.c.DeleteOffline(r.ctx, task.ID) })
	}
}

func (r *run) transfers(files []local) {
	r.check("多连接下载", func() (string, error) {
		f := files[1]
		m := transfer.New(r.c, baidu.Expired, "")
		m.Threads = 4
		defer m.Close()
		dir := filepath.Join(r.tmp, "dl")
		m.AddDownloads([]baidu.File{{Path: path.Join(r.box, f.name), Name: f.name, Size: int64(len(f.data))}}, dir)
		start := time.Now()
		for time.Since(start) < 3*time.Minute {
			time.Sleep(300 * time.Millisecond)
			ts := m.Tasks()
			if len(ts) == 0 {
				continue
			}
			switch ts[0].State {
			case transfer.Done:
				data, err := os.ReadFile(ts[0].Local)
				if err != nil {
					return "", err
				}
				sum := md5.Sum(data)
				if hex.EncodeToString(sum[:]) != f.sum {
					return "", fmt.Errorf("内容不一致")
				}
				sec := time.Since(start).Seconds()
				return fmt.Sprintf("%s，%.1f MB/s，内容一致", size(len(data)), float64(len(data))/sec/(1<<20)), nil
			case transfer.Failed:
				return "", fmt.Errorf("%s", ts[0].Err)
			}
		}
		return "", fmt.Errorf("超时")
	})
}

func (r *run) recycle() {
	target := path.Join(r.box, "文档.pdf")
	if !r.check("删除到回收站", func() (string, error) { return "", r.c.Delete(r.ctx, []string{target}) }) {
		return
	}
	var item baidu.Recycled
	r.check("回收站与还原", func() (string, error) {
		items, err := r.c.RecycleList(r.ctx)
		if err != nil {
			return "", err
		}
		for _, it := range items {
			if it.Path == target {
				item = it
			}
		}
		if item.FsID == 0 {
			return "", fmt.Errorf("回收站里没有（共 %d 项）", len(items))
		}
		if err := r.c.Restore(r.ctx, []int64{item.FsID}); err != nil {
			return "", err
		}
		time.Sleep(time.Second)
		got, err := r.c.List(r.ctx, r.box, baidu.Order{})
		if err == nil && !slices.ContainsFunc(got, func(f baidu.File) bool { return f.Name == "文档.pdf" }) {
			err = fmt.Errorf("还原后没有出现")
		}
		return fmt.Sprintf("保留 %d 天", item.LeftDays), err
	})
}

// speed measures transfers: hosts, connections, request sizes, files
// at once and upload connections.
func (r *run) speed(vip int) {
	var files []baidu.File
	if !r.check("寻找测速文件", func() (string, error) {
		var err error
		files, err = tune.FindFiles(r.ctx, r.c, 64<<20, 2)
		var names []string
		for _, f := range files {
			names = append(names, fmt.Sprintf("%s（%s）", f.Path, size(int(f.Size))))
		}
		return strings.Join(names, "、"), err
	}) {
		r.rep.Config = tune.ForVIP(vip)
		return
	}
	r.check("测速", func() (string, error) {
		res, err := tune.Probe(r.ctx, r.c, files, tune.Options{
			VIP: vip, Threads: []int{1, 2, 4, 8, 16}, Per: 8 * time.Second, Budget: 1200 << 20, Upload: true,
			Stage: func(name string) { r.logf("测速：%s", name) },
		})
		if err != nil {
			return "", err
		}
		r.rep.Probe = res
		line := func(title string, samples []tune.Sample, label func(tune.Sample) string) string {
			var parts []string
			for _, s := range samples {
				p := fmt.Sprintf("%s %.2f MB/s", label(s), s.Speed/(1<<20))
				if s.Errors > 0 {
					p += fmt.Sprintf("（%d 次出错：%s）", s.Errors, s.Err)
				}
				parts = append(parts, p)
			}
			return title + "：" + strings.Join(parts, "，")
		}
		count := func(unit string) func(tune.Sample) string {
			return func(s tune.Sample) string { return fmt.Sprintf("%d %s", s.Value, unit) }
		}
		out := []string{fmt.Sprintf("取链接 %v，首字节 %v，%s", res.Link.Round(time.Millisecond), res.FirstByte.Round(time.Millisecond), res.Proto)}
		out = append(out,
			line("线路", res.Hosts, func(s tune.Sample) string { return s.Host }),
			line("连接", res.Threads, count("连接")),
			line("块", res.Blocks, func(s tune.Sample) string { return size(int(s.Value)) }),
			line("同时", res.Files, count("文件")),
			line("上传", res.Upload, count("连接")))
		if res.UploadErr != "" {
			out = append(out, "上传测速："+res.UploadErr)
		}
		return strings.Join(out, "；"), nil
	})
	r.rep.Config = tune.Recommend(vip, r.rep.Probe)
	cfg := r.rep.Config
	r.logf("推荐配置：每个文件 %d 连接，同时 %d 个文件，块 %s，上传 %d 连接，预读 %d，线路 %v",
		cfg.Threads, cfg.Parallel, size(int(cfg.Block)), cfg.Upload, cfg.Ahead, cfg.Hosts)
}

// cleanup deletes the folder and purges it from the recycle bin.
func (r *run) cleanup() {
	r.check("清理", func() (string, error) {
		if err := r.c.Delete(r.ctx, []string{r.box}); err != nil {
			return "", err
		}
		time.Sleep(time.Second)
		items, err := r.c.RecycleList(r.ctx)
		if err != nil {
			return "", err
		}
		var ids []int64
		for _, it := range items {
			if it.Path == r.box || strings.HasPrefix(it.Path, r.box+"/") {
				ids = append(ids, it.FsID)
			}
		}
		if len(ids) == 0 {
			return "已删除（回收站里没找到，未彻底删除）", nil
		}
		return "已删除并从回收站清除", r.c.Purge(r.ctx, ids)
	})
}

func size(n int) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

// bigVideo finds a large video of the account, else any large file, to
// play from far into it.
func (r *run) bigVideo() (baidu.File, error) {
	queue := []string{"/"}
	var any baidu.File
	for visited := 0; len(queue) > 0 && visited < 40; visited++ {
		files, err := r.c.List(r.ctx, queue[0], baidu.Order{By: "size", Desc: true})
		queue = queue[1:]
		if err != nil {
			continue
		}
		for _, f := range files {
			switch {
			case f.IsDir:
				queue = append(queue, f.Path)
			case f.Category == baidu.CategoryVideo && f.Size > 64<<20:
				return f, nil
			case f.Size > any.Size:
				any = f
			}
		}
	}
	if any.Size < 64<<20 {
		return baidu.File{}, fmt.Errorf("没有找到大文件")
	}
	return any, nil
}
