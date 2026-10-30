// Package support builds the "support file": one zip with what is needed to
// fix a problem after a stream (log, raw TikFinity events, settings, state),
// so field names that were never seen on a live stream can be checked.
package support

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
)

// logTail is how much of the end of the log goes into the zip.
const logTail = 2 << 20

// Input is what goes into the zip. Config and State are marshalled as JSON.
type Input struct {
	Version  string
	LogPath  string // osureq.log; osureq.log.old is used too when the log is short
	Home     string // the user's home folder, replaced by ~ everywhere
	Config   any
	State    any
	Raw      any // TikFinity events: first of each kind and the recent ones
	Now      time.Time
	Problems []string // anything that could not be collected
}

// Write writes the zip to w. It never fails because a file is missing; that
// is noted in README.txt instead.
func Write(w io.Writer, in Input) error {
	zw := zip.NewWriter(w)
	red := redactor(in.Home)
	add := func(name string, b []byte) error {
		f, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: in.Now})
		if err != nil {
			return err
		}
		_, err = f.Write(red(b))
		return err
	}
	jsonFile := func(name string, v any) error {
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			in.Problems = append(in.Problems, fmt.Sprintf("%s: %v", name, err))
			return nil
		}
		return add(name, b)
	}
	for _, f := range []struct {
		name string
		v    any
	}{{"config.json", in.Config}, {"state.json", in.State}, {"tikfinity-events.json", in.Raw}} {
		if err := jsonFile(f.name, f.v); err != nil {
			return err
		}
	}

	logB, err := tail(in.LogPath, logTail)
	if err != nil {
		in.Problems = append(in.Problems, "osureq.log: "+err.Error())
	} else if err := add("osureq.log", logB); err != nil {
		return err
	}
	if left := logTail - len(logB); left > 0 {
		if old, err := tail(in.LogPath+".old", left); err == nil {
			if err := add("osureq.log.old", old); err != nil {
				return err
			}
		}
	}

	readme := fmt.Sprintf(`osureq %s support file, made %s

config.json             settings (your home folder is shown as ~)
state.json              queue, history, rejections, activity and TikFinity/tosu status
tikfinity-events.json   raw TikFinity events: the first of each kind, and the last 100
                        when "Log raw events" is on. They show the real field names.
osureq.log(.old)        the end of the log

It contains TikTok names and messages of viewers who chatted. Share it only
with the person helping you.
`, in.Version, in.Now.Format(time.RFC3339))
	if len(in.Problems) > 0 {
		readme += "\nNot included:\n  " + strings.Join(in.Problems, "\n  ") + "\n"
	}
	if err := add("README.txt", []byte(readme)); err != nil {
		return err
	}
	return zw.Close()
}

// tail returns at most n bytes from the end of the file, starting at a line.
func tail(path string, n int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	start := max(st.Size()-int64(n), 0)
	b := make([]byte, st.Size()-start)
	if _, err := f.ReadAt(b, start); err != nil && err != io.EOF {
		return nil, err
	}
	if start > 0 {
		if i := strings.IndexByte(string(b), '\n'); i >= 0 {
			b = b[i+1:]
		}
	}
	return b, nil
}

// redactor replaces the home folder (any slash style, any case, also JSON-
// escaped) with ~, so the Windows user name does not travel with the file.
func redactor(home string) func([]byte) []byte {
	home = strings.TrimRight(home, `\/`)
	if len(home) < 3 {
		return func(b []byte) []byte { return b }
	}
	parts := regexp.MustCompile(`[\\/]+`).Split(home, -1)
	for i, p := range parts {
		parts[i] = regexp.QuoteMeta(p)
	}
	re := regexp.MustCompile(`(?i)` + strings.Join(parts, `(?:\\\\|\\|/)+`))
	return func(b []byte) []byte { return re.ReplaceAll(b, []byte("~")) }
}
