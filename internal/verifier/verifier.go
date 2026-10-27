// Package verifier checks that a downloaded .osz is a real archive whose .osu
// files match the reference MD5s from osu!.
package verifier

import (
	"archive/zip"
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

type Level string

const (
	Strict    Level = "strict"    // every difficulty of the set must match, no unknown .osu files
	Requested Level = "requested" // only the requested difficulty (whole set when requested by set id)
)

var (
	ErrNotZip   = errors.New("response is not a ZIP archive")
	ErrNoOsu    = errors.New("archive contains no .osu files")
	ErrMismatch = errors.New("MD5 mismatch")
)

// Reference is what osu! says the set should contain.
type Reference struct {
	Checksums   map[int]string // beatmap id -> md5 (lowercase hex)
	RequestedID int            // 0 = whole set was requested
}

type Result struct {
	OK       bool     `json:"ok"`
	Matched  int      `json:"matched"`
	Expected int      `json:"expected"`
	Missing  []int    `json:"missing,omitempty"` // beatmap ids with no matching .osu
	Unknown  []string `json:"unknown,omitempty"` // .osu files that match nothing
}

func (r Result) String() string {
	if r.OK {
		return fmt.Sprintf("MD5 ok (%d/%d)", r.Matched, r.Expected)
	}
	return fmt.Sprintf("MD5 mismatch (%d/%d matched, %d unknown)", r.Matched, r.Expected, len(r.Unknown))
}

// LooksLikeZip checks the local file header magic.
func LooksLikeZip(b []byte) bool {
	return len(b) >= 4 && bytes.Equal(b[:4], []byte("PK\x03\x04"))
}

// OsuHashes opens the archive in memory and returns md5 -> file name for every .osu file.
func OsuHashes(b []byte) (map[string]string, error) {
	if !LooksLikeZip(b) {
		return nil, ErrNotZip
	}
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotZip, err)
	}
	out := map[string]string{}
	for _, f := range zr.File {
		if !strings.EqualFold(path.Ext(f.Name), ".osu") || f.FileInfo().IsDir() {
			continue
		}
		if f.UncompressedSize64 > 64<<20 {
			return nil, fmt.Errorf("%s is suspiciously large", f.Name)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		h := md5.New()
		_, err = io.Copy(h, io.LimitReader(rc, 64<<20))
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", f.Name, err)
		}
		out[hex.EncodeToString(h.Sum(nil))] = f.Name
	}
	if len(out) == 0 {
		return nil, ErrNoOsu
	}
	return out, nil
}

// Verify compares the archive against the reference at the given level.
func Verify(b []byte, ref Reference, level Level) (Result, error) {
	hashes, err := OsuHashes(b)
	if err != nil {
		return Result{}, err
	}
	return Compare(hashes, ref, level), nil
}

// Compare is Verify on precomputed hashes.
func Compare(hashes map[string]string, ref Reference, level Level) Result {
	want := ref.Checksums
	if level == Requested && ref.RequestedID != 0 {
		if sum, ok := ref.Checksums[ref.RequestedID]; ok {
			want = map[int]string{ref.RequestedID: sum}
		}
	}
	r := Result{Expected: len(want)}
	known := map[string]bool{}
	for id, sum := range want {
		sum = strings.ToLower(sum)
		known[sum] = true
		if _, ok := hashes[sum]; ok {
			r.Matched++
		} else {
			r.Missing = append(r.Missing, id)
		}
	}
	for sum, name := range hashes {
		if !known[sum] && !inRef(ref, sum) {
			r.Unknown = append(r.Unknown, name)
		}
	}
	r.OK = r.Expected > 0 && r.Matched == r.Expected
	if level == Strict && len(r.Unknown) > 0 {
		r.OK = false
	}
	return r
}

func inRef(ref Reference, sum string) bool {
	for _, s := range ref.Checksums {
		if strings.EqualFold(s, sum) {
			return true
		}
	}
	return false
}
