package migrate

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
)

// A minimal read-only SQLite reader, enough to read the rows of one table
// of a Chromium cookie store without cgo or a whole SQL engine.

var errFormat = errors.New("sqlite: bad file")

type sqliteDB struct {
	data     []byte
	pageSize int
	usable   int
}

func openSQLite(path string) (*sqliteDB, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) < 100 || string(data[:16]) != "SQLite format 3\x00" {
		return nil, errFormat
	}
	ps := int(binary.BigEndian.Uint16(data[16:]))
	if ps == 1 {
		ps = 65536
	}
	if ps < 512 || len(data) < ps {
		return nil, errFormat
	}
	if enc := binary.BigEndian.Uint32(data[56:]); enc > 1 {
		return nil, fmt.Errorf("sqlite: text encoding %d", enc)
	}
	return &sqliteDB{data: data, pageSize: ps, usable: ps - int(data[20])}, nil
}

func (db *sqliteDB) page(n uint32) ([]byte, error) {
	off := int(n-1) * db.pageSize
	if n == 0 || off+db.pageSize > len(db.data) {
		return nil, errFormat
	}
	return db.data[off : off+db.pageSize], nil
}

// table returns the column names of a table, from its CREATE TABLE
// statement, and its rows.
func (db *sqliteDB) table(name string) ([]string, [][]any, error) {
	var root uint32
	var sql string
	err := db.walk(1, 0, func(rec []any) {
		if len(rec) >= 5 && rec[0] == "table" && rec[1] == name {
			if r, ok := rec[3].(int64); ok {
				root = uint32(r)
			}
			sql, _ = rec[4].(string)
		}
	})
	if err != nil {
		return nil, nil, err
	}
	if root == 0 {
		return nil, nil, fmt.Errorf("sqlite: no table %s", name)
	}
	var rows [][]any
	if err := db.walk(root, 0, func(rec []any) { rows = append(rows, rec) }); err != nil {
		return nil, nil, err
	}
	return columns(sql), rows, nil
}

// columns reads the column names out of a CREATE TABLE statement.
func columns(sql string) []string {
	i, j := strings.Index(sql, "("), strings.LastIndex(sql, ")")
	if i < 0 || j < i {
		return nil
	}
	var cols []string
	for _, def := range strings.Split(sql[i+1:j], ",") {
		f := strings.Fields(def)
		if len(f) == 0 {
			continue
		}
		switch strings.ToUpper(f[0]) {
		case "UNIQUE", "PRIMARY", "CONSTRAINT", "CHECK", "FOREIGN":
			return cols
		}
		cols = append(cols, strings.Trim(f[0], "`\"[]"))
	}
	return cols
}

// walk calls fn with each record of the table b-tree rooted at page n.
func (db *sqliteDB) walk(n uint32, depth int, fn func([]any)) error {
	if depth > 32 {
		return errFormat
	}
	p, err := db.page(n)
	if err != nil {
		return err
	}
	h := 0
	if n == 1 {
		h = 100
	}
	if len(p) < h+12 {
		return errFormat
	}
	kind := p[h]
	count := int(binary.BigEndian.Uint16(p[h+3:]))
	cells := h + 8
	if kind == 0x05 {
		cells = h + 12
	}
	if cells+2*count > len(p) {
		return errFormat
	}
	for i := 0; i < count; i++ {
		off := int(binary.BigEndian.Uint16(p[cells+2*i:]))
		if off+4 > len(p) {
			return errFormat
		}
		switch kind {
		case 0x05: // interior: a child, then the key
			if err := db.walk(binary.BigEndian.Uint32(p[off:]), depth+1, fn); err != nil {
				return err
			}
		case 0x0D: // leaf: the payload's size, the rowid, the payload
			size, k := varint(p[off:])
			_, k2 := varint(p[off+k:])
			payload, err := db.payload(p, off+k+k2, int(size))
			if err != nil {
				return err
			}
			rec, err := record(payload)
			if err != nil {
				return err
			}
			fn(rec)
		default:
			return errFormat
		}
	}
	if kind == 0x05 {
		return db.walk(binary.BigEndian.Uint32(p[h+8:]), depth+1, fn)
	}
	return nil
}

// payload reads a leaf cell's payload of size bytes starting at off,
// following overflow pages.
func (db *sqliteDB) payload(p []byte, off, size int) ([]byte, error) {
	u := db.usable
	x := u - 35
	local := size
	if size > x {
		m := (u-12)*32/255 - 23
		local = m + (size-m)%(u-4)
		if local > x {
			local = m
		}
	}
	if off+local > len(p) {
		return nil, errFormat
	}
	out := append([]byte(nil), p[off:off+local]...)
	if local == size {
		return out, nil
	}
	if off+local+4 > len(p) {
		return nil, errFormat
	}
	next := binary.BigEndian.Uint32(p[off+local:])
	for len(out) < size {
		op, err := db.page(next)
		if err != nil {
			return nil, err
		}
		next = binary.BigEndian.Uint32(op)
		out = append(out, op[4:min(u, 4+size-len(out))]...)
	}
	return out, nil
}

// record decodes a record into int64, float64, string, []byte and nil
// values.
func record(b []byte) ([]any, error) {
	hsize, n := varint(b)
	if n == 0 || int(hsize) > len(b) {
		return nil, errFormat
	}
	var types []uint64
	for i := n; i < int(hsize); {
		t, k := varint(b[i:])
		if k == 0 {
			return nil, errFormat
		}
		types = append(types, t)
		i += k
	}
	body := b[hsize:]
	out := make([]any, 0, len(types))
	for _, t := range types {
		var size int
		switch {
		case t >= 12:
			size = int(t-12) / 2
		case t <= 4:
			size = int(t)
		case t == 5:
			size = 6
		case t == 6, t == 7:
			size = 8
		}
		if size > len(body) {
			return nil, errFormat
		}
		v := body[:size]
		body = body[size:]
		switch {
		case t == 0:
			out = append(out, nil)
		case t <= 6:
			var x int64
			for _, c := range v {
				x = x<<8 | int64(c)
			}
			if size > 0 && size < 8 && v[0]&0x80 != 0 {
				x -= 1 << (8 * size)
			}
			out = append(out, x)
		case t == 7:
			out = append(out, math.Float64frombits(binary.BigEndian.Uint64(v)))
		case t == 8:
			out = append(out, int64(0))
		case t == 9:
			out = append(out, int64(1))
		case t >= 12 && t%2 == 0:
			out = append(out, append([]byte(nil), v...))
		case t >= 13:
			out = append(out, string(v))
		default:
			return nil, errFormat
		}
	}
	return out, nil
}

// varint reads SQLite's big-endian varint, returning its length, 0 when
// b is too short.
func varint(b []byte) (uint64, int) {
	var x uint64
	for i := 0; i < 9; i++ {
		if i >= len(b) {
			return 0, 0
		}
		if i == 8 {
			return x<<8 | uint64(b[i]), 9
		}
		x = x<<7 | uint64(b[i]&0x7f)
		if b[i]&0x80 == 0 {
			return x, i + 1
		}
	}
	return x, 9
}
