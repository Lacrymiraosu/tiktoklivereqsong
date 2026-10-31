// Command winres writes rsrc_windows_amd64.syso: the Windows resources that
// go build links into osureq.exe (icon, version info, manifest). An exe with
// a description, publisher name, icon and an "asInvoker" manifest looks far
// less suspicious to Windows Defender and SmartScreen than a bare Go binary.
//
//	go run ./tools/winres -version 1.4.6
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/tc-hib/winres"
	"github.com/tc-hib/winres/version"
)

func main() {
	ver := flag.String("version", "1.0.0", "file and product version (x.y.z)")
	out := flag.String("out", "rsrc_windows_amd64.syso", "output .syso next to main.go")
	flag.Parse()

	rs := winres.ResourceSet{}

	icon, err := winres.NewIconFromResizedImage(logo(256), []int{16, 20, 24, 32, 40, 48, 64, 128, 256})
	if err != nil {
		log.Fatal(err)
	}
	if err := rs.SetIcon(winres.ID(1), icon); err != nil {
		log.Fatal(err)
	}

	const en = 0x0409
	vi := version.Info{}
	vi.SetFileVersion(*ver)
	vi.SetProductVersion(*ver)
	for k, v := range map[string]string{
		version.ProductName:      "osureq",
		version.FileDescription:  "osureq - osu! beatmap requests for TikTok LIVE",
		version.CompanyName:      "osureq",
		version.InternalName:     "osureq",
		version.OriginalFilename: "osureq.exe",
		version.LegalCopyright:   "osureq contributors",
		version.Comments:         "Local helper: reads TikTok chat from TikFinity, downloads requested osu! beatmaps and opens them in osu!.",
	} {
		if err := vi.Set(en, k, v); err != nil {
			log.Fatal(err)
		}
	}
	rs.SetVersionInfo(vi)

	rs.SetManifest(winres.AppManifest{
		Identity:       winres.AssemblyIdentity{Name: "osureq", Version: versionArray(*ver)},
		Description:    "osureq - osu! beatmap requests for TikTok LIVE",
		Compatibility:  winres.Win10AndAbove,
		ExecutionLevel: winres.AsInvoker, // never asks for administrator rights
		DPIAwareness:   winres.DPIPerMonitorV2,
		LongPathAware:  true,
	})

	f, err := os.Create(*out)
	if err != nil {
		log.Fatal(err)
	}
	if err := rs.WriteObject(f, winres.ArchAMD64); err != nil {
		log.Fatal(err)
	}
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %s (version %s)\n", *out, *ver)
}

func versionArray(v string) [4]uint16 {
	var out [4]uint16
	for i, p := range strings.SplitN(v, ".", 4) {
		n, _ := strconv.Atoi(p)
		out[i] = uint16(n)
	}
	return out
}

// logo draws the app logo (same as the dashboard's): a pink ball shaded from
// the top-left, a white ring and a white play triangle.
func logo(n int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	s := float64(n) / 32
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			fx, fy := (float64(x)+0.5)/s, (float64(y)+0.5)/s // in 32-unit logo space
			d := math.Hypot(fx-16, fy-16)
			cover := clamp((15-d)*s + 0.5) // anti-aliased edge at radius 15
			if cover <= 0 {
				continue
			}
			t := clamp(math.Hypot(fx-11, fy-10) / 20)
			r, g, b := lerp(0xff, 0xd8, t), lerp(0xa6, 0x38, t), lerp(0xd0, 0x7d, t)
			white := clamp((d-12.2)*s + 0.5)
			if fx >= 13 && fx <= 23 && math.Abs(fy-16) <= 6*(23-fx)/10 {
				white = 1
			}
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(lerp(r, 255, white)), G: uint8(lerp(g, 255, white)), B: uint8(lerp(b, 255, white)),
				A: uint8(255 * cover),
			})
		}
	}
	return img
}

func lerp(a, b, t float64) float64 { return a + (b-a)*t }

func clamp(v float64) float64 { return math.Max(0, math.Min(1, v)) }
