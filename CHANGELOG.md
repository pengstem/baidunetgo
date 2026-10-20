# 更新日志

## Unreleased

<!-- 写在这里的内容会成为下一个版本的更新说明；留空则按提交记录自动生成。 -->

## [0.4.2] - 2026-10-20

### 改进

- Delete at once, with no question first, as Finder moves to the trash: the dialog hid the rows dissolving, and the toast still offers to undo it, from the recycle bin; delete apart from the dialogs, which it no longer closes, and keep toasts shorter, 4 s for undoing and 2.5 s for a transfer begun
- Show placeholder tiles while a folder loads in the grid, laid out as its tiles are and shimmering, instead of the list's striped rows

## [0.4.1] - 2026-10-20

### 改进

- Make the specks move every frame and finish: frames that MyGo only paints again do not build the view, so a dissolve's progress, set as the view built, stood still between builds (jumping when the pointer moved) and a deleted row stayed half dissolved until something rebuilt it; the effect now reads its progress as it paints, the view is built again while rows dissolve or uploads catch up, and a dissolve starts from the first frame that shows it, not the last one built, which could be seconds old

## [0.4.0] - 2026-10-20

### 改进

- Files come and go through specks drawn with MyGo's shaders: a deleted row breaks into specks of itself that drift up and away before the rows close up, an upload shows in its folder at once as a cloud of specks that settles into its row as its bytes go up, files dragged over the browser light a ring of flowing color that flares as they drop, and thumbnails on their way shimmer as one wave across the window; the specks search only the cells that can reach a pixel and the shimmer is a plain fill apiece, so a 20-row delete draws in about 2 ms a frame

## [0.3.0] - 2026-10-20

### 改进

- Tie the app to the system: a second launch brings the running window forward instead of running the same transfers into the same files, the Dock icon counts the transfers going and shows their progress, the computer stays awake while they run, a finished download's notification shows the file in its folder and an upload's opens its netdisk folder, and transfers wait queued while the computer is offline or asleep, going on where they were once it is back

## [0.2.0] - 2026-10-20

### 改进

- Show a file's or folder's details, from its context menu, the selection bar or Cmd-I: kind, size in bytes, place, times and MD5, and for a folder what it holds
- Sort the grid view from a menu in the title bar, by name, size or date and either way, as the list sorts by its columns; and in a narrow window fold the title bar's segmented choices into pop-up buttons, as Pulse does, with a narrower search field
- Settings drop the icon tile on every row, show the account's profile photo (fetched all along but never drawn) there and in the sidebar, and center the digits of the number fields, which the input left low: it centers a line only in a box taller than it

## [0.1.1] - 2026-10-20

### 改进

- Ask in CLAUDE.md for each finished task to be committed and pushed without being asked, the push releasing it with the next version
- Expect the netdisk client's v10 STOKEN opened only on Linux, as ReadNetdisk does, so the tests pass on macOS too
- Lay out the grid view in the app instead of ui.GridView, which builds each line as an element: a tile that a resize moved to another line was built anew and jumped there; now every tile is keyed by its file under one parent and glides from where it showed to its new cell, while resizing too
- Restyle as a native macOS app modeled on Pulse: the window's sidebar material, pages on colored tiles, the system typeface instead of IBM Plex, gray groups and striped rows; redo settings with an icon tile on every row, compact number fields with steppers, the speed measurement folded behind 详情 and a storage bar under the account
- Read a file once for every checksum a rapid upload needs, the whole-file MD5 alongside the CRC32 and block MD5s while the next chunk is read, instead of BaiduPCS-Go's checksum package
- Give the netdisk session its own HTTP transport: check certificates, dial with the request's context and timeouts, and spread connections over a host's addresses instead of the library's transport that skipped all three

## [0.1.0] - 2026-10-20

首个版本。

### 新增

- 扫码登录，或粘贴 Cookie / BDUSS+STOKEN；自动沿用本机百度网盘客户端或 BaiduPCS-Go 的登录
- 列表 / 网格浏览、缩略图、拼音排序、面包屑、全盘搜索
- 不落盘预览：图片、文本与代码、Markdown、PDF、音频边下边播，视频交给 mpv
- 文件操作：新建文件夹、重命名、移动 / 复制、删除可撤销、回收站
- 传输：多连接分块下载、断点续传、秒传上传、拖文件进窗口即上传
- 按会员等级测速，自动选择下载线路、连接数、分块大小和同时传输的文件数
- 分享：带提取码与有效期的链接、我的分享、粘贴分享文字一键转存
- 离线下载：HTTP / 磁力链接
- 刻度设计风格，跟随系统深浅色
