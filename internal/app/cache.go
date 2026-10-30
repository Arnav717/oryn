package app

import (
	"encoding/json"
	"os"
	"path/filepath"
)

var cachePath string

// SetCacheDir enables saving each refresh, so the next start and agents asking
// for context get data without waiting on the network.
func SetCacheDir(dir string) { cachePath = filepath.Join(dir, "snapshot.json") }

// SaveCache writes the snapshot. It holds private repo details, so owner only.
func SaveCache(s *Snapshot) error {
	if cachePath == "" {
		return nil
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	// A unique name: the daemon and the TUI can both be saving.
	tmp, err := os.CreateTemp(filepath.Dir(cachePath), "snapshot.*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), cachePath)
}

// LoadCache returns the last saved snapshot, or nil. A snapshot that doesn't
// decode, say from an older version, counts as none.
func LoadCache() *Snapshot {
	if cachePath == "" {
		return nil
	}
	b, err := os.ReadFile(cachePath)
	if err != nil {
		return nil
	}
	s := &Snapshot{}
	if json.Unmarshal(b, s) != nil {
		return nil
	}
	return s
}
