// Package app wires the data layer, the loopback server and the desktop window.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"shiguang-study/desktop/internal/config"
	"shiguang-study/desktop/internal/secret"
	"shiguang-study/desktop/internal/server"
	"shiguang-study/desktop/internal/store"
)

type Options struct {
	Serve bool
	Port  int
	Open  bool
}

// Run starts the HTTP service and, unless Serve is set, the desktop window.
func Run(options Options) error {
	release, alreadyRunning, err := acquireSingleInstance()
	if err != nil {
		return err
	}
	if alreadyRunning {
		notifyAlreadyRunning()
		return nil
	}
	defer release()

	dataDir, err := config.BaseDataDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return fmt.Errorf("创建数据目录失败: %w", err)
	}
	secrets := secret.NewManager(dataDir)
	secrets.LoadEnvFile()
	cfg, err := config.Load(dataDir)
	if err != nil {
		return err
	}
	logger := newLogger(dataDir, options.Serve)
	slog.SetDefault(logger)
	logger.Info("starting", "dataDir", dataDir, "database", cfg.DBPath)

	database, err := store.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("打开数据库失败: %w", err)
	}
	dev := os.Getenv("SHIGUANG_DEV") == "1"
	handler, err := server.New(cfg, database, secrets, dev)
	if err != nil {
		database.Close()
		return fmt.Errorf("加载应用页面失败: %w", err)
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", options.Port))
	if err != nil {
		database.Close()
		return fmt.Errorf("启动本地服务失败: %w", err)
	}
	httpServer := &http.Server{
		Handler: handler.Handler(),
		// ReadHeaderTimeout and IdleTimeout stop a slow or stalled client from
		// holding a connection open. WriteTimeout must exceed the 100s chat
		// upstream call, so the whole request gets a little longer.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       120 * time.Second,
		WriteTimeout:      150 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	go func() {
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			logger.Error("http server stopped", "error", err)
		}
	}()

	url := "http://" + listener.Addr().String() + "/"
	var shutdownOnce sync.Once
	shutdown := func() {
		shutdownOnce.Do(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = httpServer.Shutdown(ctx)
			_ = database.Close()
			logger.Info("stopped")
		})
	}
	defer shutdown()

	if options.Serve {
		fmt.Printf("拾光自习室本地服务: %s\n", url)
		fmt.Printf("数据目录: %s\n", dataDir)
		fmt.Println("按 Ctrl+C 停止。")
		if options.Open {
			openBrowser(url)
		}
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, os.Interrupt)
		<-signals
		return nil
	}

	wailsApp := application.New(application.Options{
		Name:        "拾光自习室",
		Description: "错题本 · 间隔复习 · 拍照录题",
		OnShutdown:  shutdown,
	})
	wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "main",
		Title:            "拾光自习室",
		Width:            1280,
		Height:           860,
		MinWidth:         960,
		MinHeight:        640,
		URL:              url,
		BackgroundColour: application.NewRGBA(245, 242, 235, 255),
	})
	if err := wailsApp.Run(); err != nil {
		return err
	}
	return nil
}

// ReportError makes a startup failure visible even without a console.
func ReportError(err error) {
	showError("拾光自习室启动失败", err.Error())
}

func newLogger(dataDir string, serve bool) *slog.Logger {
	if serve {
		return slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	path := filepath.Join(dataDir, "study.log")
	handle, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	return slog.New(slog.NewTextHandler(handle, nil))
}

func openBrowser(url string) {
	_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}
