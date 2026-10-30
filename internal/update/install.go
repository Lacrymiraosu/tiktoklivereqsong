package update

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	maxZip = 64 << 20  // the release zip is about 7 MB
	maxExe = 128 << 20 // osureq.exe inside it
)

// ZipName is the release file with the Windows build.
func ZipName(version string) string { return "osureq-" + version + "-windows-x64.zip" }

// StagedName is where a downloaded update waits, next to osureq.exe, until
// it is installed.
func StagedName(version string) string { return "osureq.update-" + version + ".exe" }

// Download fetches rel's zip, checks it against the release's SHA-256 file
// and puts the osureq.exe from it in dir as StagedName. It returns that path.
func Download(ctx context.Context, hc *http.Client, rel Release, dir string) (string, error) {
	if rel.Zip == "" || rel.Sum == "" {
		return "", fmt.Errorf("release %s has no Windows zip", rel.Version)
	}
	for _, u := range []string{rel.Zip, rel.Sum} {
		if !strings.HasPrefix(u, DownloadPrefix) {
			return "", fmt.Errorf("refusing to download %s: not an osureq release file", u)
		}
	}
	sumFile, err := get(ctx, hc, rel.Sum, 4<<10)
	if err != nil {
		return "", err
	}
	// "<hex>  osureq-x.y.z-windows-x64.zip"
	want := strings.ToLower(strings.TrimPrefix(firstField(string(sumFile)), string(rune(0xFEFF))))
	if len(want) != 64 {
		return "", fmt.Errorf("the SHA-256 file of %s is not a hash", rel.Version)
	}
	data, err := get(ctx, hc, rel.Zip, maxZip)
	if err != nil {
		return "", err
	}
	if got := sha256.Sum256(data); hex.EncodeToString(got[:]) != want {
		return "", fmt.Errorf("the downloaded zip of %s does not match its SHA-256", rel.Version)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("zip of %s: %w", rel.Version, err)
	}
	var exe *zip.File
	for _, f := range zr.File {
		if f.Name == "osureq.exe" {
			exe = f
		}
	}
	if exe == nil {
		return "", fmt.Errorf("the zip of %s has no osureq.exe", rel.Version)
	}
	dst := filepath.Join(dir, StagedName(rel.Version))
	if err := extract(exe, dst+".part"); err != nil {
		os.Remove(dst + ".part")
		return "", err
	}
	if err := os.Rename(dst+".part", dst); err != nil {
		os.Remove(dst + ".part")
		return "", err
	}
	return dst, nil
}

func get(ctx context.Context, hc *http.Client, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "osureq (update)")
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub answered %s for %s", resp.Status, filepath.Base(url))
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("%s is larger than %d MB", filepath.Base(url), limit>>20)
	}
	return b, nil
}

func firstField(s string) string {
	if f := strings.Fields(s); len(f) > 0 {
		return f[0]
	}
	return ""
}

func extract(f *zip.File, dst string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	n, err := io.Copy(out, io.LimitReader(rc, maxExe+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > maxExe {
		err = errors.New("osureq.exe in the zip is too large")
	}
	return err
}

// Staged returns the newest update waiting in dir, or "" when there is none.
func Staged(dir string) (version, path string) {
	files, _ := filepath.Glob(filepath.Join(dir, StagedName("*")))
	for _, f := range files {
		v := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(f), "osureq.update-"), ".exe")
		if _, ok := Parse(v); ok && (version == "" || Newer(v, version)) {
			version, path = v, f
		}
	}
	return version, path
}

// Install puts staged in place of exe. The old exe is kept as exe+".old"
// (Windows lets a running program be renamed but not deleted) and put back
// if the swap fails.
func Install(exe, staged string) error {
	old := exe + ".old"
	os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		return fmt.Errorf("moving the old osureq.exe aside: %w", err)
	}
	if err := os.Rename(staged, exe); err != nil {
		if rerr := os.Rename(old, exe); rerr != nil {
			return fmt.Errorf("installing the update: %w (and putting the old osureq.exe back: %v)", err, rerr)
		}
		return fmt.Errorf("installing the update: %w", err)
	}
	return nil
}

// Cleanup removes the exe left by the last install and updates that are not
// newer than current. The old exe cannot be removed while the copy that
// installed the update is still closing; a later call gets it.
func Cleanup(exe, current string) {
	os.Remove(exe + ".old")
	files, _ := filepath.Glob(filepath.Join(filepath.Dir(exe), StagedName("*")))
	parts, _ := filepath.Glob(filepath.Join(filepath.Dir(exe), StagedName("*")+".part"))
	for _, f := range append(files, parts...) {
		v := strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(filepath.Base(f), "osureq.update-"), ".part"), ".exe")
		if strings.HasSuffix(f, ".part") || !Newer(v, current) {
			os.Remove(f)
		}
	}
}

// Relaunch starts exe again with args in the same console window.
func Relaunch(exe string, args []string) error {
	cmd := exec.Command(exe, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Start()
}
