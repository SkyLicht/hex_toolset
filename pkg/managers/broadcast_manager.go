package managers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	pkg "hex_toolset/pkg"
	"hex_toolset/pkg/logger"
	ws "hex_toolset/pkg/websocket"

	"github.com/fsnotify/fsnotify"
)

// BroadcastManager encapsulates the websocket server, hub and file watcher.
// It exposes a simple API to run and gracefully shutdown the broadcast service.
type BroadcastManager struct {
	cfg *pkg.Config
	log *logger.Logger

	// runtime
	hub    *ws.Hub
	server *http.Server

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewBroadcastManager constructs a new BroadcastManager using application config and logger.
func NewBroadcastManager(cfg *pkg.Config, logg *logger.Logger) *BroadcastManager {
	return &BroadcastManager{cfg: cfg, log: logg}
}

// Run starts the websocket server and directory watcher, and blocks until ctx is cancelled.
// If addr is empty, it falls back to cfg.BROADCAST_WS_ADDR or ":8081".
func (m *BroadcastManager) Run(ctx context.Context) error {
	if m == nil {
		return errors.New("nil BroadcastManager")
	}
	// derive internal cancelable context
	m.ctx, m.cancel = context.WithCancel(ctx)

	dir := m.cfg.BROADCAST_MESSAGE_DIR
	addr := m.cfg.BROADCAST_WS_ADDR
	if strings.TrimSpace(addr) == "" {
		addr = ":8081"
	}
	if err := ensureDir(dir); err != nil {
		return fmt.Errorf("ensure dir %s: %w", dir, err)
	}

	// hub
	m.hub = ws.NewHub()
	go m.hub.Run(m.log)

	// http server
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("/ws", ws.WSHandler(m.hub, m.log))
	m.server = &http.Server{
		Addr:         addr,
		Handler:      ws.RecoverMiddleware(mux, m.log),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// watcher
	if err := m.startWatcher(dir); err != nil {
		return fmt.Errorf("start watcher: %w", err)
	}

	// initial files.json broadcast
	filesJSON := filepath.Join(dir, "files.json")
	if b, err := os.ReadFile(filesJSON); err == nil {
		m.log.Infof("broadcasting initial files.json (%d bytes)", len(b))
		m.hub.Broadcast(b)
	}

	// start HTTP server
	go func() {
		defer func() {
			if r := recover(); r != nil {
				m.log.Errorf("http server panic recovered: %v", r)
			}
		}()
		m.log.Infof("websocket server listening on %s (endpoint /ws)", addr)
		if err := m.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			m.log.Errorf("server error: %v", err)
		}
	}()

	// Block until context is done, then shutdown
	<-m.ctx.Done()
	return m.shutdown()
}

// Stop requests the manager to shutdown (non-blocking). Safe to call multiple times.
func (m *BroadcastManager) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
}

// internal shutdown sequence
func (m *BroadcastManager) shutdown() error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if m.server != nil {
		if err := m.server.Shutdown(shutdownCtx); err != nil {
			m.log.Errorf("server shutdown error: %v", err)
		}
	}
	if m.hub != nil {
		m.hub.Shutdown()
	}
	m.wg.Wait()
	m.log.Infof("broadcast service stopped")
	return nil
}

func ensureDir(dir string) error {
	if dir == "" {
		return errors.New("empty directory path")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return nil
}

func (m *BroadcastManager) startWatcher(dir string) error {
	if err := ensureDir(dir); err != nil {
		return err
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	m.wg.Add(1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				m.log.Errorf("watcher panic recovered: %v", r)
			}
			watcher.Close()
			m.wg.Done()
		}()

		for {
			select {
			case <-m.ctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					m.log.Warnf("watcher events channel closed")
					return
				}
				if event.Op&fsnotify.Create == fsnotify.Create {
					path := event.Name
					// Skip directories
					if fi, err := os.Stat(path); err == nil && fi.IsDir() {
						continue
					}
					// small delay to allow writers to finish
					time.Sleep(100 * time.Millisecond)
					content, err := os.ReadFile(path)
					if err != nil {
						m.log.Errorf("failed reading created file %s: %v", path, err)
						continue
					}
					m.log.Infof("broadcasting created file: %s (%d bytes)", filepath.Base(path), len(content))
					m.hub.Broadcast(content)
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					m.log.Warnf("watcher errors channel closed")
					return
				}
				m.log.Errorf("watcher error: %v", err)
			}
		}
	}()

	if err := watcher.Add(dir); err != nil {
		return err
	}
	m.log.Infof("watching directory for new files: %s", dir)
	return nil
}
