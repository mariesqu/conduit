//go:build windows

package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"fyne.io/systray"
	"github.com/skip2/go-qrcode"
	"golang.org/x/sys/windows/registry"
)

const (
	autostartKey  = `Software\Microsoft\Windows\CurrentVersion\Run`
	autostartName = "Conduit"
)

// runApp on Windows defaults to the tray; --console forces the legacy
// foreground console loop for debugging.
func runApp(app *appRuntime) {
	if app.console {
		runConsole(app)
		return
	}
	runTray(app)
}

// runTray runs the system-tray menu loop. systray.Run takes over the main
// goroutine and blocks until Quit; the HTTP server is already running in a
// background goroutine started by main().
func runTray(app *appRuntime) {
	// Translate OS termination signals into a tray quit so shutdown stays
	// graceful even when the process is stopped externally (e.g. logoff).
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		systray.Quit()
	}()

	onReady := func() {
		systray.SetIcon(trayIconICO())
		systray.SetTitle("Conduit")
		systray.SetTooltip("Conduit — browser terminal")

		mOpen := systray.AddMenuItem("Open in browser", "Open the terminal in your default browser")
		mCopyURL := systray.AddMenuItem("Copy access URL", "Copy the tokenized sign-in URL")
		mCopyTok := systray.AddMenuItem("Copy token", "Copy the raw access token")
		systray.AddSeparator()
		mQR := systray.AddMenuItem("Show QR code", "Open a QR code for phone sign-in")
		mRotate := systray.AddMenuItem("Rotate token", "Issue a new token (signs out other devices)")
		systray.AddSeparator()
		mAuto := systray.AddMenuItemCheckbox("Start with Windows", "Launch Conduit at login", autostartEnabled())
		mLog := systray.AddMenuItem("Open log file", "Open the Conduit log")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("Quit Conduit", "Stop the server and exit")

		go func() {
			for {
				select {
				case <-mOpen.ClickedCh:
					openPath(app.accessURL())
				case <-mCopyURL.ClickedCh:
					copyToClipboard(app.accessURL())
				case <-mCopyTok.ClickedCh:
					copyToClipboard(app.cfg.CurrentToken())
				case <-mQR.ClickedCh:
					showQR(app)
				case <-mRotate.ClickedCh:
					if _, err := app.cfg.RotateToken(); err != nil {
						log.Printf("tray: rotate token: %v", err)
					} else {
						log.Printf("tray: token rotated; other clients signed out")
						copyToClipboard(app.accessURL())
					}
				case <-mAuto.ClickedCh:
					toggleAutostart(mAuto)
				case <-mLog.ClickedCh:
					if app.logPath != "" {
						openWith("notepad.exe", app.logPath)
					}
				case <-mQuit.ClickedCh:
					systray.Quit()
					return
				}
			}
		}()
	}

	systray.Run(onReady, app.shutdown)
}

// openPath opens a URL or file with the Windows shell default handler.
func openPath(target string) {
	openWith("rundll32.exe", "url.dll,FileProtocolHandler", target)
}

// openWith launches a program detached; failures are logged, not fatal.
func openWith(name string, args ...string) {
	if err := exec.Command(name, args...).Start(); err != nil {
		log.Printf("tray: launch %s: %v", name, err)
	}
}

// copyToClipboard pipes text to the built-in clip.exe. Best-effort.
func copyToClipboard(text string) {
	cmd := exec.Command("cmd", "/c", "clip")
	cmd.Stdin = strings.NewReader(text)
	if err := cmd.Run(); err != nil {
		log.Printf("tray: clipboard: %v", err)
	}
}

// showQR writes a PNG QR of the access URL and opens it with the default
// image viewer — the tray equivalent of the console QR code.
func showQR(app *appRuntime) {
	dir := conduitDataDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("tray: qr dir: %v", err)
		return
	}
	path := filepath.Join(dir, "conduit-qr.png")
	if err := qrcode.WriteFile(app.accessURL(), qrcode.Medium, 320, path); err != nil {
		log.Printf("tray: qr: %v", err)
		return
	}
	openPath(path)
}

// autostartEnabled reports whether the HKCU Run-key entry exists.
func autostartEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, autostartKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(autostartName)
	return err == nil
}

func toggleAutostart(item *systray.MenuItem) {
	if autostartEnabled() {
		if err := removeAutostart(); err != nil {
			log.Printf("tray: disable autostart: %v", err)
			return
		}
		item.Uncheck()
		return
	}
	if err := addAutostart(); err != nil {
		log.Printf("tray: enable autostart: %v", err)
		return
	}
	item.Check()
}

func addAutostart() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, autostartKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(autostartName, `"`+exe+`"`)
}

func removeAutostart() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, autostartKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.DeleteValue(autostartName)
}

// trayIconICO builds a 32×32 ICO (PNG-encoded entry, supported by the
// Windows tray since Vista) at runtime — a filled circle in the Conduit
// accent color, so we don't have to carry a binary icon asset in the repo.
func trayIconICO() []byte {
	const size = 32
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	accent := color.RGBA{R: 0x00, G: 0xd4, B: 0xaa, A: 0xff}
	cx, cy, r := 15.5, 15.5, 15.0
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := float64(x)-cx, float64(y)-cy
			if dx*dx+dy*dy <= r*r {
				img.SetRGBA(x, y, accent)
			}
		}
	}
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		return nil
	}
	return wrapPNGAsICO(pngBuf.Bytes(), size)
}

// wrapPNGAsICO packs a PNG into a single-image .ico container.
func wrapPNGAsICO(pngBytes []byte, size int) []byte {
	var buf bytes.Buffer
	// ICONDIR
	_ = binary.Write(&buf, binary.LittleEndian, uint16(0)) // reserved
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1)) // type: icon
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1)) // image count
	// ICONDIRENTRY
	buf.WriteByte(byte(size % 256))                                    // width (256 encodes as 0)
	buf.WriteByte(byte(size % 256))                                    // height
	buf.WriteByte(0)                                                   // palette size
	buf.WriteByte(0)                                                   // reserved
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))             // color planes
	_ = binary.Write(&buf, binary.LittleEndian, uint16(32))            // bits per pixel
	_ = binary.Write(&buf, binary.LittleEndian, uint32(len(pngBytes))) // image size
	_ = binary.Write(&buf, binary.LittleEndian, uint32(6+16))          // offset to image
	buf.Write(pngBytes)
	return buf.Bytes()
}
