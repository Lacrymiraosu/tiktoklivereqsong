package platform

import "testing"

func TestParseHotkey(t *testing.T) {
	ok := map[string][2]uint32{
		"Ctrl+Alt+N":         {0x3, 'N'},
		"ctrl + shift + f5":  {0x6, 0x74},
		"F9":                 {0, 0x78},
		"Ctrl+Alt+Backspace": {0x3, 0x08},
		"Win+Num5":           {0x8, 0x65},
		"Alt+PageDown":       {0x1, 0x22},
	}
	for in, want := range ok {
		m, k, err := ParseHotkey(in)
		if err != nil || m != want[0] || k != want[1] {
			t.Errorf("%q: got %#x %#x %v want %#x %#x", in, m, k, err, want[0], want[1])
		}
	}
	for _, bad := range []string{"", "N", "Ctrl+", "Hyper+N", "Ctrl+Alt+Foo", "Ctrl+N+Alt"} {
		if _, _, err := ParseHotkey(bad); err == nil {
			t.Errorf("%q should fail", bad)
		}
	}
}
