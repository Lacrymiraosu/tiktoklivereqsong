//go:build windows

// Package platform wraps the Windows APIs the helper needs: opening files and
// URLs with their associated app, finding TikFinity, and global hotkeys.
package platform

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// ShellOpen opens a file or URL with its associated application
// (.osz -> osu!, https -> default browser).
func ShellOpen(target string) error {
	verb, _ := windows.UTF16PtrFromString("open")
	file, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, file, nil, nil, windows.SW_SHOWNORMAL)
}

// ProcessRunning reports whether a process with this exe name is running.
func ProcessRunning(exe string) bool {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if strings.EqualFold(windows.UTF16ToString(e.ExeFile[:]), exe) {
			return true
		}
	}
	return false
}

// ProcessPath returns the full path of the first running process with this exe name.
func ProcessPath(exe string) (string, bool) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return "", false
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if !strings.EqualFold(windows.UTF16ToString(e.ExeFile[:]), exe) {
			continue
		}
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, e.ProcessID)
		if err != nil {
			continue
		}
		buf := make([]uint16, windows.MAX_LONG_PATH)
		n := uint32(len(buf))
		err = windows.QueryFullProcessImageName(h, 0, &buf[0], &n)
		windows.CloseHandle(h)
		if err == nil {
			return windows.UTF16ToString(buf[:n]), true
		}
	}
	return "", false
}

// FindTikFinity looks for TikFinity.exe: the configured path, the default
// per-user install folder, then the Uninstall registry entries.
func FindTikFinity(configured string) (string, bool) {
	cands := []string{configured, filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "tikfinity", "TikFinity.exe")}
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		cands = append(cands, uninstallPaths(root)...)
	}
	for _, c := range cands {
		if c == "" {
			continue
		}
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, true
		}
	}
	return "", false
}

func uninstallPaths(root registry.Key) []string {
	const base = `Software\Microsoft\Windows\CurrentVersion\Uninstall`
	k, err := registry.OpenKey(root, base, registry.ENUMERATE_SUB_KEYS|registry.READ)
	if err != nil {
		return nil
	}
	defer k.Close()
	names, _ := k.ReadSubKeyNames(-1)
	var out []string
	for _, n := range names {
		sk, err := registry.OpenKey(k, n, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		dn, _, _ := sk.GetStringValue("DisplayName")
		if strings.HasPrefix(strings.ToLower(dn), "tikfinity") {
			if loc, _, err := sk.GetStringValue("InstallLocation"); err == nil && loc != "" {
				out = append(out, filepath.Join(loc, "TikFinity.exe"))
			}
			if un, _, err := sk.GetStringValue("UninstallString"); err == nil {
				un = strings.Trim(strings.SplitN(strings.TrimPrefix(un, `"`), `"`, 2)[0], " ")
				out = append(out, filepath.Join(filepath.Dir(un), "TikFinity.exe"))
			}
		}
		sk.Close()
	}
	return out
}

// FindOsuStable returns the osu! stable folder from the .osz / osu:// file
// associations, e.g. `"F:\osu!\osu!.exe" "%1"` -> `F:\osu!`.
func FindOsuStable() (string, bool) {
	for _, key := range []string{`osustable.File.osz\shell\open\command`, `osu\shell\open\command`, `osu!\shell\open\command`} {
		k, err := registry.OpenKey(registry.CLASSES_ROOT, key, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		cmd, _, err := k.GetStringValue("")
		k.Close()
		if err != nil {
			continue
		}
		exe := strings.TrimSpace(cmd)
		if strings.HasPrefix(exe, `"`) {
			exe = strings.SplitN(exe[1:], `"`, 2)[0]
		} else {
			exe = strings.Fields(exe)[0]
		}
		if !strings.EqualFold(filepath.Base(exe), "osu!.exe") {
			continue // lazer registers osu.exe
		}
		if st, err := os.Stat(exe); err == nil && !st.IsDir() {
			return filepath.Dir(exe), true
		}
	}
	return "", false
}

// Launch starts an executable apart from the helper. A console app (tosu)
// gets its own visible console window rather than none (DETACHED_PROCESS
// would leave it running hidden); GUI apps ignore the console flag.
func Launch(path string) error {
	cmd := exec.Command(path)
	cmd.Dir = filepath.Dir(path)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NEW_CONSOLE}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// Close stops every running process with this exe name. Visible windows are
// asked to close first (so a GUI app can save); whatever is still running
// after a few seconds, or has no window (a console app such as tosu), is
// terminated. It returns how many processes were running.
func Close(exe string) (int, error) {
	pids := processIDs(exe)
	if len(pids) == 0 {
		return 0, nil
	}
	if closeWindows(pids) > 0 {
		for end := time.Now().Add(3 * time.Second); time.Now().Before(end) && len(processIDs(exe)) > 0; {
			time.Sleep(200 * time.Millisecond)
		}
	}
	var errs []error
	for _, pid := range processIDs(exe) {
		h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid)
		if err == nil {
			err = windows.TerminateProcess(h, 1)
			windows.CloseHandle(h)
		}
		if err != nil && !errors.Is(err, windows.ERROR_INVALID_PARAMETER) { // gone already
			errs = append(errs, err)
		}
	}
	return len(pids), errors.Join(errs...)
}

// processIDs lists the running processes with this exe name.
func processIDs(exe string) []uint32 {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	var ids []uint32
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if strings.EqualFold(windows.UTF16ToString(e.ExeFile[:]), exe) {
			ids = append(ids, e.ProcessID)
		}
	}
	return ids
}

// closing is the request enumCloseCb works on; closeMu keeps one at a time
// (the callback cannot be given a Go pointer through EnumWindows' lParam).
var (
	closeMu sync.Mutex
	closing struct {
		pids map[uint32]bool
		n    int
	}
	enumCloseCb = syscall.NewCallback(func(hwnd windows.HWND, _ uintptr) uintptr {
		var pid uint32
		windows.GetWindowThreadProcessId(hwnd, &pid)
		if closing.pids[pid] && windows.IsWindowVisible(hwnd) {
			pPostMessage.Call(uintptr(hwnd), wmClose, 0, 0)
			closing.n++
		}
		return 1
	})
)

// closeWindows posts WM_CLOSE to the visible top-level windows of pids and
// returns how many it asked.
func closeWindows(pids []uint32) int {
	closeMu.Lock()
	defer closeMu.Unlock()
	closing.pids, closing.n = map[uint32]bool{}, 0
	for _, p := range pids {
		closing.pids[p] = true
	}
	windows.EnumWindows(enumCloseCb, nil)
	return closing.n
}

var (
	user32            = windows.NewLazySystemDLL("user32.dll")
	pRegisterHotKey   = user32.NewProc("RegisterHotKey")
	pUnregisterHotKey = user32.NewProc("UnregisterHotKey")
	pGetMessage       = user32.NewProc("GetMessageW")
	pPostThreadMsg    = user32.NewProc("PostThreadMessageW")
	pPostMessage      = user32.NewProc("PostMessageW")
)

const (
	wmHotkey = 0x0312
	wmQuit   = 0x0012
	wmClose  = 0x0010
)

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
}

// Hotkeys owns one OS thread with a message loop for RegisterHotKey.
type Hotkeys struct {
	mu     sync.Mutex
	thread uint32
	done   chan struct{}
}

// Start registers the bindings (name -> "Ctrl+Alt+N") and calls fire(name)
// when one is pressed. It replaces any previous registration and returns the
// bindings that failed (bad syntax or taken by another app).
func (h *Hotkeys) Start(bind map[string]string, fire func(string)) map[string]error {
	h.Stop()
	type reg struct {
		name     string
		mod, key uint32
	}
	var regs []reg
	failed := map[string]error{}
	for name, s := range bind {
		if strings.TrimSpace(s) == "" {
			continue
		}
		mod, key, err := ParseHotkey(s)
		if err != nil {
			failed[name] = err
			continue
		}
		regs = append(regs, reg{name, mod, key})
	}
	if len(regs) == 0 {
		return failed
	}
	ready := make(chan uint32)
	done := make(chan struct{})
	var mu sync.Mutex
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(done)
		ids := map[uintptr]string{}
		for i, r := range regs {
			id := uintptr(i + 1)
			ok, _, err := pRegisterHotKey.Call(0, id, uintptr(r.mod|0x4000), uintptr(r.key))
			if ok == 0 {
				mu.Lock()
				failed[r.name] = errors.New("already in use by another app: " + err.Error())
				mu.Unlock()
				continue
			}
			ids[id] = r.name
		}
		ready <- windows.GetCurrentThreadId()
		var m msg
		for {
			r, _, _ := pGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if int32(r) <= 0 {
				break
			}
			if m.message == wmHotkey {
				if name, ok := ids[m.wParam]; ok {
					go fire(name)
				}
			}
		}
		for id := range ids {
			pUnregisterHotKey.Call(0, id)
		}
	}()
	tid := <-ready
	h.mu.Lock()
	h.thread, h.done = tid, done
	h.mu.Unlock()
	mu.Lock()
	defer mu.Unlock()
	out := map[string]error{}
	for k, v := range failed {
		out[k] = v
	}
	return out
}

func (h *Hotkeys) Stop() {
	h.mu.Lock()
	tid, done := h.thread, h.done
	h.thread, h.done = 0, nil
	h.mu.Unlock()
	if tid != 0 {
		pPostThreadMsg.Call(uintptr(tid), wmQuit, 0, 0)
		<-done
	}
}

// SetConsoleTitle names the CMD window, so it is easy to find among others.
func SetConsoleTitle(title string) {
	t, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return
	}
	windows.NewLazySystemDLL("kernel32.dll").NewProc("SetConsoleTitleW").Call(uintptr(unsafe.Pointer(t)))
}
