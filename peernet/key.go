package peernet

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
)

// LoadKey reads the peer key at path, creating it on first use. Only one
// process may call it for a path at a time, such as the daemon that holds its
// state directory.
func LoadKey(path string) (crypto.PrivKey, error) {
	// Keep an existing key, so the PC's peer ID stays the same.
	data, err := os.ReadFile(path)
	if err == nil {
		key, err := keypem.ParsePrivKeyPem(data)
		return key, errors.Wrapf(err, "read %s", filepath.Base(path))
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	// Create one, writing it whole before it takes the key's name.
	key, _, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		return nil, err
	}
	data, err = keypem.MarshalPrivKeyPem(key)
	if err != nil {
		return nil, err
	}
	temp := path + ".new"
	if err := os.WriteFile(temp, data, 0o600); err != nil {
		return nil, err
	}
	return key, os.Rename(temp, path)
}
