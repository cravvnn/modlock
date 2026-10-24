package project

import (
	"crypto/sha256"
	"encoding/binary"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"

	"github.com/pkg/errors"
)

// Fingerprint hashes the built mod: every file's path and content in Output,
// leaving out logs and hidden staging files, which change without the mod
// changing. Two builds with the same fingerprint run the same mod, so a watch
// that rebuilt for a save outside the mod's inputs, such as a note in the
// project, need not reload it.
func (p *Project) Fingerprint() ([sha256.Size]byte, error) {
	var sum [sha256.Size]byte
	hash := sha256.New()
	built := os.DirFS(p.Output())
	err := fs.WalkDir(built, ".", func(name string, entry fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case name != "." && strings.HasPrefix(entry.Name(), "."):
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		case !entry.Type().IsRegular() || path.Ext(name) == ".log":
			return nil
		}

		// Length-prefix the path so no two layouts hash alike.
		file, err := built.Open(name)
		if err != nil {
			return err
		}
		defer file.Close()
		_ = binary.Write(hash, binary.LittleEndian, uint64(len(name)))
		_, _ = io.WriteString(hash, name)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		_ = binary.Write(hash, binary.LittleEndian, uint64(info.Size()))
		_, err = io.Copy(hash, file)
		return err
	})
	if err != nil {
		return sum, errors.Wrap(err, "fingerprint the built mod")
	}
	hash.Sum(sum[:0])
	return sum, nil
}
