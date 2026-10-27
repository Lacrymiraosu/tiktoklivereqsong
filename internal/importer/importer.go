// Package importer hands verified .osz archives to osu!.
package importer

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Mode string

const (
	Open    Mode = "open"
	Copy    Mode = "copy-to-songs"
	Extract Mode = "extract-to-songs"
)

type Target struct {
	SetID  int
	Artist string
	Title  string
}

type Importer struct {
	Trusted   func(path string) bool
	ShellOpen func(target string) error
}

var ErrUntrusted = errors.New("refusing to open a file the helper did not download and verify")

// Import applies mode. Lazer has no Songs folder, so it always uses Open.
func (im *Importer) Import(file string, mode Mode, game, osuDir string, t Target) error {
	if !im.Trusted(file) {
		return ErrUntrusted
	}
	if game == "lazer" || mode == Open {
		return im.ShellOpen(file)
	}
	songs, err := songsDir(osuDir)
	if err != nil {
		return err
	}
	switch mode {
	case Copy:
		_, err = CopyToSongs(file, songs, t)
	case Extract:
		_, err = ExtractToSongs(file, songs, t)
	default:
		err = fmt.Errorf("unknown import mode %q", mode)
	}
	return err
}

func songsDir(osuDir string) (string, error) {
	songs := filepath.Join(osuDir, "Songs")
	if st, err := os.Stat(songs); err != nil || !st.IsDir() {
		return "", fmt.Errorf("osu! Songs folder not found at %s", songs)
	}
	return songs, nil
}

var reBad = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)

// SafeName strips characters Windows does not allow in file names.
func SafeName(s string) string {
	s = reBad.ReplaceAllString(s, "")
	s = strings.Trim(strings.TrimSpace(s), ". ")
	if len(s) > 120 {
		s = s[:120]
	}
	switch strings.ToUpper(s) {
	case "CON", "PRN", "AUX", "NUL", "COM1", "LPT1":
		s = "_" + s
	}
	return s
}

func (t Target) folderName() string {
	n := SafeName(fmt.Sprintf("%d %s - %s", t.SetID, t.Artist, t.Title))
	if n == "" {
		n = fmt.Sprint(t.SetID)
	}
	return n
}

// CopyToSongs copies the archive into Songs; stable imports it on song select / F5.
func CopyToSongs(file, songs string, t Target) (string, error) {
	dst := filepath.Join(songs, t.folderName()+".osz")
	src, err := os.Open(file)
	if err != nil {
		return "", err
	}
	defer src.Close()
	tmp := dst + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(out, src); err != nil {
		out.Close()
		os.Remove(tmp)
		return "", err
	}
	if err := out.Close(); err != nil {
		return "", err
	}
	return dst, os.Rename(tmp, dst)
}

// ExtractToSongs unpacks into Songs/{setId} {Artist} - {Title}/ and refuses
// any entry that would land outside that folder.
func ExtractToSongs(file, songs string, t Target) (string, error) {
	zr, err := zip.OpenReader(file)
	if err != nil {
		return "", err
	}
	defer zr.Close()
	root := filepath.Join(songs, t.folderName())
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	for _, f := range zr.File {
		dst, err := SafeJoin(root, f.Name)
		if err != nil {
			return "", err
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return "", err
			}
			continue
		}
		if f.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("archive contains a symlink: %s", f.Name)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return "", err
		}
		if err := extractOne(f, dst); err != nil {
			return "", err
		}
	}
	return root, nil
}

func extractOne(f *zip.File, dst string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, io.LimitReader(rc, 1<<30))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}

// SafeJoin joins name under root, rejecting absolute paths, drive letters and "..".
func SafeJoin(root, name string) (string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	if name == "" || strings.HasPrefix(name, "/") || filepath.VolumeName(name) != "" || strings.Contains(name, ":") {
		return "", fmt.Errorf("unsafe path in archive: %q", name)
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return "", fmt.Errorf("unsafe path in archive: %q", name)
		}
	}
	dst := filepath.Join(root, filepath.FromSlash(name))
	rel, err := filepath.Rel(root, dst)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe path in archive: %q", name)
	}
	return dst, nil
}
