package platform

import (
	"fmt"
	"strings"
)

var namedKeys = map[string]uint32{
	"BACKSPACE": 0x08, "TAB": 0x09, "ENTER": 0x0D, "PAUSE": 0x13, "SPACE": 0x20,
	"PAGEUP": 0x21, "PAGEDOWN": 0x22, "END": 0x23, "HOME": 0x24,
	"LEFT": 0x25, "UP": 0x26, "RIGHT": 0x27, "DOWN": 0x28,
	"INSERT": 0x2D, "DELETE": 0x2E,
}

// ParseHotkey turns "Ctrl+Alt+N" into RegisterHotKey modifier and virtual-key codes.
func ParseHotkey(s string) (mod, key uint32, err error) {
	parts := strings.Split(strings.ToUpper(strings.ReplaceAll(s, " ", "")), "+")
	for i, p := range parts {
		last := i == len(parts)-1
		switch {
		case !last && (p == "CTRL" || p == "CONTROL"):
			mod |= 0x2
		case !last && p == "ALT":
			mod |= 0x1
		case !last && p == "SHIFT":
			mod |= 0x4
		case !last && p == "WIN":
			mod |= 0x8
		case last && len(p) == 1 && (p[0] >= 'A' && p[0] <= 'Z' || p[0] >= '0' && p[0] <= '9'):
			key = uint32(p[0])
		case last && len(p) >= 2 && len(p) <= 3 && p[0] == 'F':
			var n int
			if _, e := fmt.Sscanf(p[1:], "%d", &n); e == nil && n >= 1 && n <= 24 {
				key = 0x70 + uint32(n-1)
			}
		case last && strings.HasPrefix(p, "NUM") && len(p) == 4 && p[3] >= '0' && p[3] <= '9':
			key = 0x60 + uint32(p[3]-'0')
		case last:
			key = namedKeys[p]
		default:
			return 0, 0, fmt.Errorf("unknown modifier %q in %q", p, s)
		}
	}
	if key == 0 {
		return 0, 0, fmt.Errorf("unknown key in %q", s)
	}
	if mod == 0 && !(key >= 0x70 && key <= 0x87) {
		return 0, 0, fmt.Errorf("%q needs a modifier (Ctrl/Alt/Shift/Win) unless it is an F-key", s)
	}
	return mod, key, nil
}
