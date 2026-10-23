# 更新日志

## Unreleased

<!-- 写在这里的内容会成为下一个版本的更新说明；留空则按提交记录自动生成。 -->

## [0.4.20] - 2026-10-23

### 改进

- Show the speed test's details as a card for each setting, the value chosen large with a column for each value tried and a line saying why it was chosen, in place of rows of bars and raw numbers

## [0.4.19] - 2026-10-22

### 改进

- Have the self-test download its file as listed, so the download is checked against the netdisk's MD5 as the app's are, and fail when the listed MD5 is not the content's

## [0.4.18] - 2026-10-22

### 新增

- Add Finder's shortcuts: Space looks at the file chosen and closes the preview again, Cmd-Down opens and Cmd-Up goes up, Cmd-R reads the folder again, Cmd-1 and Cmd-2 switch list and grid, Cmd-F finds and Cmd-comma opens the settings; the preview takes the focus itself, where its zoom-out button had it and Space pressed it

### 改进

- Let the settings cap the speed of all downloads and of all uploads, so a transfer need not take the whole connection, and ask before quitting or closing the window while transfers go on, which paused them without a word

## [0.4.17] - 2026-10-22

### 改进

- Search reads every page of results, up to 3,000, where it read only the first 500 and said nothing of the rest; searched from a folder, a switch keeps the search to that folder; and searching the same words again searches afresh

## [0.4.16] - 2026-10-22

### 改进

- Read the folder showing again, quietly, when the window comes to the front and every two minutes while it shows, and other folders as they show again, so changes made from the web or a phone appear without pressing refresh
- Fetch thumbnails at the size they show, about 140×90 for list icons and 360×270 for grid tiles where every one was 850×580 (2 MB of memory each), the ones asked for last first, skipping rows scrolled past, and keep up to 256 MB of them on disk across launches, deleted on sign-out

## [0.4.15] - 2026-10-22

### 改进

- Queue the files of a folder a few hundred at a time and tell listeners of changes at most every 50 ms, where each file went through the queue and its listeners alone: queueing 10,000 files took 10,000 notices and 50 million task copies, now 3 and 25,000
- Check a finished download against the content MD5 the listing and the download host agree on, downloading it afresh when it differs, and check the disk has room before a download starts; the listing's scrambled MD5s are decoded, which the info panel showed scrambled

## [0.4.14] - 2026-10-22

### 改进

- Try a failed transfer again by itself, five times over a few minutes, from where it stopped, unless trying again cannot help (session expired, disk or netdisk full, file gone); list a folder to download four subfolders at a time, say so on the transfers page while it does, and tell which folders could not be read instead of leaving their files out silently

## [0.4.13] - 2026-10-22

### 改进

- Go on with an upload paused, failed or left when the app quit from the parts the netdisk already has, keeping its checksums and upload id in transfers.json, where it started over and read the whole file again; an upload the netdisk let go of starts afresh, and the self-test checks a resumed upload against the real netdisk

## [0.4.12] - 2026-10-22

### 改进

- Ask before an upload, move or copy takes a name its folder has, offering to skip, replace or keep both, where uploads overwrote the file there silently and moves and copies failed; replacing puts the old one in the recycle bin, and a copy into its own folder takes a free name

## [0.4.11] - 2026-10-22

### 改进

- Pinch to zoom the image preview about the fingers and pan it with two fingers, as a native viewer does, where two fingers zoomed it before; a mouse wheel still zooms, now about the pointer
- Update MyGo to v0.6.2, which hovers the row that takes a removed one's place under a still pointer, shows the right cursor on scroll bars, brings the window forward on a second launch on Windows and Wayland, and reports touchpad pinches

## [0.4.10] - 2026-10-21

### 改进

- Tell the whole path when the pointer rests on a breadcrumb, the folders folded into "…" on it, and the whole summary on the line under them, which a narrow window cuts short

## [0.4.9] - 2026-10-21

### 改进

- Keep the selection bar on one line in a narrow window: under 660 px of page its actions drop their labels for icons with tooltips, and the count no longer shrinks, which squeezed "已选 1 项" into a column

## [0.4.8] - 2026-10-21

### 改进

- Use MyGo 0.6's loading.dev indicators for the waits that fill a page or stand for a task: a Pulse in the accent on the boot screen, a Ring while a folder loads, a Radar on the speed test's button; the small waits in buttons, rows and dialogs keep AppKit's spinner, which suits the native look

## [0.4.7] - 2026-10-21

### 新增

- Add the software engineering course materials under docs/course: requirements, design, project management, how the app was built with AI and what we learned, and the talk's script, with the code size, tests and history taken from the repository

### 改进

- Choose files by dragging over their rows, as in Finder, with MyGo 0.6's DragSelect (a right-click on a row not chosen now chooses it first, so its menu acts on it), line the speed card's unit up with its figure by baseline instead of a hand-tuned margin, and test dropping a file from Finder with Tester.DropFiles
- Upgrade MyGo to v0.6.0: it redraws only what changed, lays out again only what changed, folds translations out of the general GPU path, and adds the drag select, right-click choosing, row double clicks and Tester.DropFiles the next commits use
- Bring the README up to date: screenshots taken again in the macOS look (the old ones showed the first dark design), and the features added since the release workflow, sign-in migration, auto tuning, the system integration, file details and copying pictures, the specks, the debug variables and the newer packages

## [0.4.6] - 2026-10-20

### 改进

- Keep thumbnails through a folder read again: the cache was keyed by the thumbnail URL, which the netdisk signs afresh with every listing, so deleting one picture refetched and redrew all the others; it is now keyed by the file's fs_id, mtime and size, fakepan signs its thumbnail URLs per listing the same way, and a test deletes a picture and checks the rest keep theirs

## [0.4.5] - 2026-10-20

### 改进

- Give the app an icon, a white cloud with a download arrow on a blue squircle in the accent's colors, drawn by tools/appicon into resources/icon.png, which mygo build turns into the macOS .icns, the Windows executable's icon and the Linux desktop entry's; builds had none and showed the generic app icon

## [0.4.4] - 2026-10-20

### 改进

- Copy pictures: the image preview copies its picture as PNG (a button, Cmd+C, and a right-click menu that also copies the path and zooms, with Cmd+= / Cmd+- / Cmd+0), turned upright as a JPEG's EXIF orientation says; text previews copy their whole text; the file list's and search's menus copy a picture without opening it; tests for all of it (the code landed in 28c0fd0)
- Bring a folder's files in one after another from the top as it first shows, each popping up from a little below and a little smaller, rows 28 ms apart and tiles 45 ms, in place of the placeholders: a folder loading shows nothing, then a spinner if it takes a while, thumbnails on their way show the file's icon again, and the other pages' skeleton rows pulse as they did

## [0.4.3] - 2026-10-20

### 改进

- Sign macOS builds with entitlements that let wazero run its compiled code under the hardened runtime, which a fixed signing identity (-sign, so the keychain's Always Allow survives rebuilds) turns on and which otherwise kills the app as PDFium renders; note how to build signed

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
