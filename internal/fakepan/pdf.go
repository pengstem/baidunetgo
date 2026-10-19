package fakepan

import (
	"bytes"
	"fmt"
)

// SamplePDF makes a small PDF of a few pages, each with a title and colored
// bars, for previews to render.
func SamplePDF(pages int) []byte {
	var objs []string
	font := 3 + 2*pages
	kids := ""
	for i := range pages {
		kids += fmt.Sprintf("%d 0 R ", 3+2*i)
	}
	objs = append(objs, "<< /Type /Catalog /Pages 2 0 R >>")
	objs = append(objs, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", kids, pages))
	for i := range pages {
		content := fmt.Sprintf("BT /F1 34 Tf 72 740 Td (BaiduNetGo) Tj ET\nBT /F1 16 Tf 72 704 Td (Page %d of %d - rendered in memory) Tj ET\n", i+1, pages)
		for b := range 6 {
			r, g, bl := 0.25+0.1*float64(b), 0.4+0.05*float64(i%4), 0.9-0.1*float64(b)
			content += fmt.Sprintf("%.2f %.2f %.2f rg 72 %d %d 34 re f\n", r, g, bl, 620-b*60, 140+b*50)
		}
		objs = append(objs, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Contents %d 0 R /Resources << /Font << /F1 %d 0 R >> >> >>", 4+2*i, font))
		objs = append(objs, fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(content), content))
	}
	objs = append(objs, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")

	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return b.Bytes()
}
