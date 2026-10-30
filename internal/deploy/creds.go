package deploy

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Tokens linked through the UI live in credentials.toml next to config.toml,
// mode 0600, separate from the config so sharing a config never leaks them.
// Environment variables and platform CLI logins always win over it.
var credsDir string

// SetCredsDir points credential storage at dir (normally the config dir).
func SetCredsDir(dir string) { credsDir = dir }

func credsPath() string {
	dir := credsDir
	if dir == "" {
		d, err := os.UserConfigDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(d, "oryn")
	}
	return filepath.Join(dir, "credentials.toml")
}

func loadCreds() map[string]string {
	m := map[string]string{}
	if p := credsPath(); p != "" {
		toml.DecodeFile(p, &m)
	}
	return m
}

func saveCreds(m map[string]string) error {
	p := credsPath()
	if p == "" {
		return errors.New("no config directory")
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".credentials-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := toml.NewEncoder(tmp).Encode(m); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), p)
}

func stored(key string) string { return loadCreds()[key] }

func storeValues(kv map[string]string) error {
	m := loadCreds()
	for k, v := range kv {
		m[k] = v
	}
	return saveCreds(m)
}

// Disconnect forgets the token oryn stored for a platform. Env vars and CLI
// logins are not touched.
func Disconnect(platform string) error {
	m := loadCreds()
	delete(m, platform+"_token")
	if platform == "cloudflare" {
		delete(m, "cloudflare_account")
	}
	if _, err := os.Stat(credsPath()); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return saveCreds(m)
}
