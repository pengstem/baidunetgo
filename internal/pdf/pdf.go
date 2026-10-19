// Package pdf renders PDF pages to pictures in memory with PDFium, compiled
// to WebAssembly and run by wazero, so it needs no cgo and no library of
// the system on any platform.
package pdf

import (
	"context"
	"errors"
	"image"
	"image/draw"
	"sync"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/webassembly"
)

var (
	poolOnce sync.Once
	pool     pdfium.Pool
	poolErr  error
)

// Warm compiles PDFium ahead of the first document, which otherwise waits
// for it.
func Warm() { go instancePool() }

func instancePool() (pdfium.Pool, error) {
	poolOnce.Do(func() {
		pool, poolErr = webassembly.Init(webassembly.Config{MinIdle: 0, MaxIdle: 1, MaxTotal: 2})
	})
	return pool, poolErr
}

// Page is the size of a page in points.
type Page struct{ Width, Height float64 }

// Doc is an open PDF. Its methods are safe from any goroutine; they take
// turns with PDFium.
type Doc struct {
	mu    sync.Mutex
	inst  pdfium.Pdfium
	doc   references.FPDF_DOCUMENT
	data  []byte
	Pages []Page
}

// Open reads a PDF from memory. The bytes must not change while it is
// open.
func Open(ctx context.Context, data []byte) (*Doc, error) {
	p, err := instancePool()
	if err != nil {
		return nil, err
	}
	inst, err := p.GetInstanceWithContext(ctx)
	if err != nil {
		return nil, err
	}
	d := &Doc{inst: inst, data: data}
	od, err := inst.OpenDocument(&requests.OpenDocument{File: &d.data})
	if err != nil {
		inst.Close()
		return nil, errors.New("无法打开 PDF：" + err.Error())
	}
	d.doc = od.Document
	pc, err := inst.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: d.doc})
	if err != nil {
		d.Close()
		return nil, err
	}
	for i := range pc.PageCount {
		sz, err := inst.GetPageSize(&requests.GetPageSize{Page: d.page(i)})
		if err != nil {
			d.Close()
			return nil, err
		}
		d.Pages = append(d.Pages, Page{sz.Width, sz.Height})
	}
	return d, nil
}

func (d *Doc) page(i int) requests.Page {
	return requests.Page{ByIndex: &requests.PageByIndex{Document: d.doc, Index: i}}
}

// Render draws page i (from 0) width pixels wide.
func (d *Doc) Render(i, width int) (*image.RGBA, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.inst == nil {
		return nil, errors.New("pdf: closed")
	}
	if i < 0 || i >= len(d.Pages) {
		return nil, errors.New("pdf: no such page")
	}
	pg := d.Pages[i]
	height := int(float64(width) * pg.Height / pg.Width)
	r, err := d.inst.RenderPageInPixels(&requests.RenderPageInPixels{Page: d.page(i), Width: width, Height: height})
	if err != nil {
		return nil, err
	}
	defer r.Cleanup()
	// The picture lives in PDFium's memory until Cleanup: copy it out.
	src := r.Result.Image
	out := image.NewRGBA(src.Bounds())
	draw.Draw(out, out.Bounds(), src, src.Bounds().Min, draw.Src)
	return out, nil
}

// Close frees the document.
func (d *Doc) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.inst == nil {
		return
	}
	d.inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: d.doc})
	d.inst.Close()
	d.inst = nil
}
