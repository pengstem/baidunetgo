package tune

import (
	"context"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/pengstem/baidunetgo/internal/baidu"
	"github.com/pengstem/baidunetgo/internal/fakepan"
)

func TestRecommend(t *testing.T) {
	r := &Result{
		Version: Version,
		Hosts:   []Sample{{Host: "a", Speed: 3e6}, {Host: "b", Speed: 9e6}, {Host: "c"}},
		// 8 is within 90% of 16; 24 is refused at times.
		Threads: []Sample{{Value: 4, Speed: 5e6}, {Value: 8, Speed: 9.5e6}, {Value: 16, Speed: 10e6}, {Value: 24, Speed: 11e6, Errors: 3}},
		// All within 5%: the default size wins.
		Blocks: []Sample{{Value: 1 << 20, Speed: 9.6e6}, {Value: 4 << 20, Speed: 9.5e6}, {Value: 16 << 20, Speed: 9.9e6}},
		// A second file adds; a third little.
		Files:  []Sample{{Value: 1, Speed: 9.5e6}, {Value: 2, Speed: 16e6}, {Value: 3, Speed: 17e6}},
		Upload: []Sample{{Value: 2, Speed: 2e6}, {Value: 4, Speed: 3.9e6}, {Value: 8, Speed: 4e6}},
	}
	cfg := Recommend(2, r)
	want := Config{Threads: 8, Parallel: 2, Upload: 4, Block: 4 << 20, Ahead: 8, Hosts: []string{"b", "a"}}
	if cfg.Threads != want.Threads || cfg.Parallel != want.Parallel || cfg.Upload != want.Upload ||
		cfg.Block != want.Block || cfg.Ahead != want.Ahead || !slices.Equal(cfg.Hosts, want.Hosts) {
		t.Errorf("got %+v, want %+v", cfg, want)
	}

	// Small requests far faster: they are chosen.
	r.Blocks = []Sample{{Value: 1 << 20, Speed: 20e6}, {Value: 4 << 20, Speed: 9.5e6}}
	if b := Recommend(2, r).Block; b != 1<<20 {
		t.Errorf("block %d, want 1 MB", b)
	}
	// A second file refused: one at a time.
	r.Files = []Sample{{Value: 1, Speed: 9.5e6}, {Value: 2, Speed: 3e6, Errors: 5}}
	if p := Recommend(2, r).Parallel; p != 1 {
		t.Errorf("parallel %d, want 1", p)
	}
	// A second file no faster: still two, so one file does not hold up
	// the queue.
	r.Files = []Sample{{Value: 1, Speed: 9.5e6}, {Value: 2, Speed: 9.4e6}}
	if p := Recommend(2, r).Parallel; p != 2 {
		t.Errorf("parallel %d, want 2", p)
	}
	// Two files together much slower: one at a time.
	r.Files = []Sample{{Value: 1, Speed: 9.5e6}, {Value: 2, Speed: 4e6}}
	if p := Recommend(2, r).Parallel; p != 1 {
		t.Errorf("parallel %d, want 1", p)
	}
	// Slow connections: players read with as many as downloads do.
	slow := &Result{Threads: []Sample{{Value: 1, Speed: 1e5}, {Value: 2, Speed: 2e5}, {Value: 4, Speed: 2.1e5}}}
	if a := Recommend(0, slow).Ahead; a != 2 {
		t.Errorf("ahead %d, want 2", a)
	}
	if cfg := Recommend(0, nil); !reflect.DeepEqual(cfg, ForVIP(0)) {
		t.Errorf("unmeasured %+v", cfg)
	}
}

func TestProbe(t *testing.T) {
	srv := fakepan.New()
	t.Cleanup(srv.Close)
	c := baidu.OpenAt(baidu.Credentials{BDUSS: "x", UID: 1}, srv.URL)
	ctx := context.Background()
	files, err := FindFiles(ctx, c, 100<<20, 2)
	if err != nil || len(files) != 2 || files[0].Size < files[1].Size {
		t.Fatalf("files %v, %v", files, err)
	}
	var stages []string
	r, err := Probe(ctx, c, files, Options{VIP: 1, Per: 150 * time.Millisecond, Budget: 400 << 20, Upload: true,
		Stage: func(s string) { stages = append(stages, s) }})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Hosts) != 2 || len(r.Threads) != len(Candidates(1)) || len(r.Blocks) != len(blockCandidates) || len(r.Files) < 2 || len(r.Upload) == 0 {
		t.Fatalf("measured hosts %d threads %d blocks %d files %d upload %d (%s)",
			len(r.Hosts), len(r.Threads), len(r.Blocks), len(r.Files), len(r.Upload), r.UploadErr)
	}
	for _, g := range [][]Sample{r.Hosts, r.Threads, r.Blocks, r.Files, r.Upload} {
		for _, s := range g {
			if s.Speed <= 0 || s.Errors > 0 {
				t.Errorf("sample %+v", s)
			}
		}
	}
	if len(stages) != 6 {
		t.Errorf("stages %q", stages)
	}
	cfg := Recommend(1, r)
	if len(cfg.Hosts) != 2 || cfg.Threads == 0 || cfg.Block == 0 || cfg.Upload == 0 {
		t.Errorf("config %+v", cfg)
	}
}
