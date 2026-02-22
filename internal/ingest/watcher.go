package ingest

import (
	"context"
	"io/fs"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

func (s *Service) StartWatcher(ctx context.Context, relPath string) error {
	absPath := filepath.Join(s.cfg.VaultRoot, relPath)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	if err := watchRecursive(watcher, absPath); err != nil {
		watcher.Close()
		return err
	}

	log.Printf("watcher started on: %s", absPath)

	go func() {
		defer watcher.Close()

		debounce := make(map[string]*time.Timer)

		for {
			select {
			case <-ctx.Done():
				log.Println("watcher stopped")
				return

			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				if !strings.HasSuffix(strings.ToLower(event.Name), ".md") {
					continue
				}
				if strings.Contains(event.Name, string(filepath.Separator)+".") {
					continue
				}
				if event.Op == fsnotify.Chmod {
					continue
				}

				name := event.Name
				if t, exists := debounce[name]; exists {
					t.Stop()
				}
				debounce[name] = time.AfterFunc(500*time.Millisecond, func() {
					log.Printf("watcher reindexing: %s", name)

					rel, err := filepath.Rel(s.cfg.VaultRoot, name)
					if err != nil {
						log.Printf("watcher rel path error: %v", err)
						return
					}
					rel = filepath.ToSlash(rel)

					dir := filepath.Dir(rel)
					_, err = s.IngestPath(context.Background(), dir)
					if err != nil {
						log.Printf("watcher ingest error: %v", err)
					}
				})

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("watcher error: %v", err)
			}
		}
	}()

	return nil
}

func watchRecursive(watcher *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if strings.HasPrefix(filepath.Base(path), ".") {
				return filepath.SkipDir
			}
			return watcher.Add(path)
		}
		return nil
	})
}
