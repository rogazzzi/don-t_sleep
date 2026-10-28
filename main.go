package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	webview2 "github.com/jchv/go-webview2"
)

const appVersion = "0.1.0"

//go:embed web
var webAssets embed.FS

func main() {
	// Все окна (наши и WebView2) обязаны жить на одном потоке.
	runtime.LockOSThread()

	noWindow := flag.Bool("no-window", false, "только сервер, без окна (для проверки)")
	trayStart := flag.Bool("tray", false, "запуск свёрнутым в трей")
	debug := flag.Bool("debug", false, "отладочный режим окна")
	port := flag.Int("port", 0, "порт HTTP-сервера (0 — свободный)")
	flag.Parse()

	dir, err := dataDir()
	if err != nil {
		log.Fatalf("не удалось определить каталог данных: %v", err)
	}

	logFile, err := os.OpenFile(filepath.Join(dir, "dontsleep.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err == nil {
		defer logFile.Close()
		log.SetOutput(logFile)
	}
	log.Printf("don't sleep %s запускается, каталог данных: %s", appVersion, dir)

	cfg, err := loadConfig(filepath.Join(dir, "config.json"))
	if err != nil {
		log.Fatalf("конфиг: %v", err)
	}

	store, err := OpenStore(filepath.Join(dir, "dontsleep.db"))
	if err != nil {
		log.Fatalf("база: %v", err)
	}
	defer store.Close()

	web, err := fs.Sub(webAssets, "web")
	if err != nil {
		log.Fatalf("веб-ресурсы: %v", err)
	}

	hub := NewHub()
	api := &API{store: store, cfg: cfg, hub: hub, web: web}

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		log.Fatalf("порт: %v", err)
	}
	server := &http.Server{Handler: api.Handler()}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("сервер остановлен: %v", err)
		}
	}()

	url := "http://" + listener.Addr().String() + "/"
	log.Printf("сервер доступен на %s", url)

	if err := syncAutostart(cfg.Get().Autostart); err != nil {
		log.Printf("автозапуск: %v", err)
	}

	startScheduler(store, hub)

	if *noWindow {
		fmt.Println(url)
		select {}
	}

	setupNative(url, store, cfg, hub)

	view := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     *debug,
		AutoFocus: true,
		DataPath:  filepath.Join(dir, "webview"),
		WindowOptions: webview2.WindowOptions{
			Title:  "don't sleep — заметки и напоминания",
			Width:  1080,
			Height: 760,
			Center: true,
		},
	})

	if view != nil {
		view.Navigate(url)
		subclassMainWindow(uintptr(view.Window()))
		if *trayStart {
			hideMainWindow()
		}
	} else {
		log.Println("WebView2 недоступен — открываю в браузере")
		openBrowser(url)
	}

	runNativeLoop()

	user32UnregisterHotKey.Call(hiddenHwnd, hotkeyID)
	removeTrayIcon()
	_ = server.Close()
	log.Println("don't sleep завершил работу")
}

func openBrowser(url string) {
	_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}
