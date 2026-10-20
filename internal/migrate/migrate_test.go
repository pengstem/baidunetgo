package migrate

import (
	"strings"
	"testing"
)

// testdata/Cookies is a Chromium cookie store made with sqlite3: 400
// unrelated rows (so the table has interior pages), a 5000-byte value
// (overflow pages), a v10-encrypted STOKEN and an expired cookie.
func TestReadNetdisk(t *testing.T) {
	acc, err := ReadNetdisk("testdata/Cookies")
	if err != nil {
		t.Fatal(err)
	}
	cr := acc.Creds
	if cr.BDUSS != "pan-bduss" || cr.STOKEN != "pan-stoken" {
		t.Errorf("BDUSS %q, STOKEN %q", cr.BDUSS, cr.STOKEN)
	}
	if !strings.Contains(cr.Cookies, "PANPSC="+strings.Repeat("p", 5000)) || !strings.Contains(cr.Cookies, "BAIDUID=id") {
		t.Errorf("cookies miss PANPSC or BAIDUID: %d bytes", len(cr.Cookies))
	}
	if strings.Contains(cr.Cookies, "OLD=") || strings.Contains(cr.Cookies, "noise") || strings.Contains(cr.Cookies, "passport") {
		t.Errorf("cookies hold expired or foreign cookies")
	}
}

func TestColumns(t *testing.T) {
	got := columns("CREATE TABLE c(a INTEGER NOT NULL,\"b\" TEXT, c BLOB,UNIQUE (a, b))")
	if strings.Join(got, ",") != "a,b,c" {
		t.Errorf("%v", got)
	}
}
