package cache

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/veecue/pacman-smartmirror/packet"
)

// checks if the given repository is already in the repository cache and downloads
// it asynchronously. If the function returns no immediate error (nil), it will write
// the final error to the channel if the channel is not nil.
func (c *Cache) addRepo(repo string) error {
	c.repoMu.Lock()
	_, ok := c.repos[repo]
	c.repoMu.Unlock()
	if ok {
		return errors.New("repo already available")
	}

	log.Println("Downloading repo", repo)
	err := c.downloadRepo(repo)
	if err == nil {
		log.Println("Repo", repo, "now available")
	} else {
		log.Println("Error downloading", repo, err)
	}

	return err
}

// downloadRepo will download the database file of the given repository and add
// it to the repository cache. If no immediate error occurs (nil is returned),
// the final error will be pushed to the given channel if the channel is not nil.
func (c *Cache) downloadRepo(dirpath string) error {
	match := c.r.MatchPath(dirpath)
	if match == nil {
		return fmt.Errorf("no match found for repo %s", dirpath)
	}

	file := match.Impl.GetDB(match.MatchedPath)
	match = c.r.MatchPath(file)
	if match == nil {
		return fmt.Errorf("no match found for repo %s", dirpath)
	}

	result := make(chan error)
	rd, async, err := c.downloadManager.GetFile(filepath.Join(c.directory, filepath.FromSlash(file)), match.UpstreamURLs, result, true)
	if err != nil {
		return fmt.Errorf("error downloading database: %w", err)
	}
	rd.Close()

	if async {
		err := <-result
		if err != nil {
			return fmt.Errorf("error downloading database: %w", err)
		}
	}

	return nil
}

// updatePackets will update all locally cached packages that are part of the given repository
func (c *Cache) updatePackets(repodir string) {
	match := c.r.MatchPath(repodir)
	if match == nil {
		return
	}

	file := filepath.Join(c.directory, filepath.FromSlash(match.Impl.GetDB(repodir)))
	f, err := os.Open(file)
	if err != nil {
		return
	}

	// List of packages that are out of date
	toDownload := make([]string, 0)
	err = match.Impl.ParseDB(f, func(p packet.Packet) {
		c.mu.Lock()
		for _, other := range c.packets[match.MatchedPath].FindOtherVersions(p) {
			if packet.CompareVersions(p.Version(), other.Version()) > 0 {
				// Version in the repository is later than the local one
				toDownload = append(toDownload, path.Join(match.MatchedPath, p.Filename()))
				break
			}
		}
		c.mu.Unlock()
	})

	if err != nil {
		log.Println("Error parsing db file:", err)
		return
	}

	// Update all outdated packages
	for _, p := range toDownload {
		match := c.r.MatchPath(p)
		if match == nil {
			continue
		}
		if c.downloadManager.BackgroundDownload(filepath.Join(c.directory, filepath.FromSlash(p)), match.UpstreamURLs) == nil {
			if err != nil {
				log.Println(fmt.Errorf("error downloading %s: %w", p, err))
			}
		}
	}

	log.Println("All cached packages for", repodir, "up to date")
}

// GetDBFile returns the latest cached version of a given database together with
// the time it was updated.
func (c *Cache) GetDBFile(repopath string) (ReadSeekCloser, time.Time, error) {
	if _, ok := c.repos[repopath]; ok {
		c.repoMu.Lock()
		defer c.repoMu.Unlock()

		match := c.r.MatchPath(repopath)
		if match == nil {
			return nil, time.Time{}, fmt.Errorf("repo not found")
		}
		path := match.Impl.GetDB(match.MatchedPath)
		file, err := os.Open(path)
		if err != nil {
			return nil, time.Time{}, fmt.Errorf("error opening repository file: %w", err)
		}

		modtime := time.Time{}
		if stat, err := os.Stat(path); err == nil {
			modtime = stat.ModTime()
		}

		return file, modtime, nil
	}

	return nil, time.Time{}, errors.New("database file not found")
}

// ProxyRepo will proxy the given repository database file from a mirror
// with out downloading it to the cache.
func (c *Cache) ProxyRepo(w http.ResponseWriter, r *http.Request) {
	match := c.r.MatchPath(r.URL.Path)
	if match == nil {
		http.NotFound(w, r)
		return
	}

	for _, upstream := range match.UpstreamURLs {
		req, _ := http.NewRequest("GET", upstream, nil)
		req.Header = r.Header
		req.Header.Set("User-Agent", "pacman-smartmirror/0.0")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			continue
		}

		if resp.StatusCode != 200 && resp.StatusCode != 304 {
			continue
		}

		// seems to work, use this mirror
		for key := range resp.Header {
			w.Header().Add(key, resp.Header.Get(key))
		}
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
		return
	}

	http.NotFound(w, r)
}

// UpdateDatabases will update all cached database files in the background
// The error (may be nil) WILL be sent to the channel EXACTLY ONCE
//
// For each updated database, updating their accoring cached packages
// will be started in the background.
func (c *Cache) UpdateDatabases(result chan<- error) {
	// Gather list of all databases
	toUpdate := make([]string, 0)
	c.repoMu.Lock()
	for repo := range c.repos {
		toUpdate = append(toUpdate, repo)
	}
	c.repoMu.Unlock()

	go func() {
		var lastErr error
		for _, repo := range toUpdate {
			log.Println("Updating", repo)
			err := c.downloadRepo(repo)
			if err != nil {
				lastErr = fmt.Errorf("error updating databases: %w", err)
				log.Println(lastErr)
				continue
			}

			go c.updatePackets(repo)
		}
		if lastErr == nil {
			log.Println("All databases updated successfully")
		} else {
			log.Println("Error(s) during database updates")
		}

		if result != nil {
			result <- lastErr
		}
	}()
}
