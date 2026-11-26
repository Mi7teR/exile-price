// Command notify-dev tries the live search notices on macOS: the same cards
// as the Windows tool, in a plain window, for the browser extension in
// extension/. A click on a card brings its tab up in the browser that sent it.
package main

import (
	"flag"
	"fmt"
	"image"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/Mi7teR/exile-price/internal/notify"
	"github.com/Mi7teR/exile-price/internal/trade"
	"github.com/Mi7teR/exile-price/internal/ui"
)

const ttl = 20 * time.Second

func main() {
	path := flag.String("stats", "testdata/recorded/data-stats.json", "trade stats, as /api/trade/data/stats returns them")
	flag.Parse()
	f, err := os.Open(*path)
	if err != nil {
		log.Fatal(err)
	}
	stats, err := trade.ParseStats(f)
	f.Close()
	if err != nil {
		log.Fatal(err)
	}

	w := new(app.Window)
	w.Option(app.Title("poe-pricecheck live"), app.Size(unit.Dp(380), unit.Dp(40)), app.TopMost(true))
	cards := ui.NewCards("новый лот")
	var mu sync.Mutex
	browsers := map[int]string{} // tab → .app bundle

	mux := http.NewServeMux()
	mux.Handle("/notify", notify.Handler(notify.ExtensionID, stats.Text, func(s notify.Search, tab int, from uint16) {
		b := browserApp(from)
		log.Printf("новый лот: %s (вкладка %d, %s)", s, tab, b)
		mu.Lock()
		browsers[tab] = b
		mu.Unlock()
		id := cards.Add(tab, s.Name, s.Filters, time.Now())
		w.Invalidate()
		go exec.Command("afplay", "/System/Library/Sounds/Glass.aiff").Run()
		time.AfterFunc(ttl, func() { cards.Remove(id); w.Invalidate() })
	}))
	mux.HandleFunc("/focus", notify.FocusPage)
	addr := fmt.Sprintf("127.0.0.1:%d", notify.Port)
	go func() { log.Fatal(http.ListenAndServe(addr, mux)) }()
	log.Print("жду расширение на ", addr)

	go func() {
		idle := ui.New()
		idle.Set("жду лоты · "+addr, ui.Info)
		var ops op.Ops
		var size image.Point
		for {
			switch e := w.Event().(type) {
			case app.FrameEvent:
				gtx := app.NewContext(&ops, e)
				need, tab := cards.Layout(gtx)
				if cards.Len() == 0 {
					need = idle.Layout(gtx)
				}
				e.Frame(gtx.Ops)
				if tab != 0 {
					mu.Lock()
					b := browsers[tab]
					mu.Unlock()
					go open(b, notify.FocusURL(tab))
				}
				if need != size && need.Y > 0 {
					size = need
					dp := func(px int) unit.Dp { return unit.Dp(float32(px) / e.Metric.PxPerDp) }
					w.Option(app.Size(dp(need.X), dp(need.Y)))
				}
			case app.DestroyEvent:
				os.Exit(0)
			}
		}
	}()
	app.Main()
}

// browserApp is the .app bundle of the process on the other end of our
// connection from port from, found with lsof.
func browserApp(from uint16) string {
	out, err := exec.Command("lsof", "-nP", "-t", "-iTCP:"+strconv.Itoa(int(from))).Output()
	if err != nil {
		return ""
	}
	for _, pid := range strings.Fields(string(out)) {
		if pid == strconv.Itoa(os.Getpid()) {
			continue
		}
		comm, err := exec.Command("ps", "-o", "comm=", "-p", pid).Output()
		if err != nil {
			continue
		}
		p := strings.TrimSpace(string(comm))
		if i := strings.Index(p, ".app/"); i >= 0 {
			return p[:i+len(".app")] // a helper's path starts with the browser's bundle
		}
	}
	return ""
}

func open(bundle, url string) {
	args := []string{url}
	if bundle != "" {
		args = []string{"-a", bundle, url}
	}
	if err := exec.Command("open", args...).Run(); err != nil {
		log.Print("open: ", err)
	}
	log.Printf("клик: %s в %s", url, bundle)
}
