package importer

import (
	"os"
	"path/filepath"
	"sync"
	"time"
)

// OsuDB reads the set of beatmap MD5s known to osu! stable.
//
// Rather than parse every field of osu!.db (its layout changes between client
// versions), it scans for osu! strings of exactly 32 lowercase hex chars:
// 0x0b, ULEB128 length 0x20, then the hash. Those are the per-beatmap MD5s.
type OsuDB struct {
	mu     sync.Mutex
	path   string
	mod    time.Time
	hashes map[string]bool
}

func (db *OsuDB) Has(osuDir string, sums []string) bool {
	h := db.load(filepath.Join(osuDir, "osu!.db"))
	if len(h) == 0 || len(sums) == 0 {
		return false
	}
	for _, s := range sums {
		if !h[s] {
			return false
		}
	}
	return true
}

func (db *OsuDB) load(path string) map[string]bool {
	db.mu.Lock()
	defer db.mu.Unlock()
	st, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if path == db.path && st.ModTime().Equal(db.mod) {
		return db.hashes
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return db.hashes
	}
	db.path, db.mod, db.hashes = path, st.ModTime(), ScanHashes(b)
	return db.hashes
}

func ScanHashes(b []byte) map[string]bool {
	out := map[string]bool{}
	for i := 0; i+34 <= len(b); i++ {
		if b[i] != 0x0b || b[i+1] != 0x20 {
			continue
		}
		s := b[i+2 : i+34]
		ok := true
		for _, c := range s {
			if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
				ok = false
				break
			}
		}
		if ok {
			out[string(s)] = true
			i += 33
		}
	}
	return out
}
