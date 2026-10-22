package vpk

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/pkg/errors"
)

// Archive is an archive on disk, read one file at a time: a single-file
// archive such as an addon's, or a directory such as the game's
// pak01_dir.vpk whose files live in numbered parts beside it.
type Archive struct {
	// path is the archive's directory file.
	path string
	// dataStart is where the directory file's own embedded data starts.
	dataStart int64
	// entries are the files' records by name.
	entries map[string]entry
}

// OpenArchive reads the directory of the archive at path.
func OpenArchive(path string) (*Archive, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	archive := &Archive{path: path, entries: make(map[string]entry)}
	archive.dataStart, err = walk(f, func(file entry) {
		archive.entries[file.name] = file
	})
	if err != nil {
		return nil, errors.Wrap(err, path)
	}
	return archive, nil
}

// Names returns the archive's file names in order.
func (a *Archive) Names() []string {
	names := make([]string, 0, len(a.entries))
	for name := range a.entries {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// Size returns the length of the file name, and whether the archive holds it.
func (a *Archive) Size(name string) (int64, bool) {
	file, ok := a.entries[name]
	return int64(len(file.preload)) + int64(file.length), ok
}

// ReadFile reads the file name: its preload bytes, then the rest from the
// directory file or the numbered part that holds it.
func (a *Archive) ReadFile(name string) ([]byte, error) {
	// Find the file and the part holding its data.
	file, ok := a.entries[strings.ToLower(name)]
	if !ok {
		return nil, errors.Errorf("%s holds no %s", a.path, name)
	}
	if file.length == 0 {
		return file.preload, nil
	}
	part, offset := a.path, a.dataStart+int64(file.offset)
	if file.archive != embedded {
		part = fmt.Sprintf("%s_%03d.vpk", strings.TrimSuffix(a.path, "_dir.vpk"), file.archive)
		offset = int64(file.offset)
	}

	// Read the data after the preload bytes.
	f, err := os.Open(part)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data := make([]byte, len(file.preload)+int(file.length))
	copy(data, file.preload)
	if _, err := io.ReadFull(io.NewSectionReader(f, offset, int64(file.length)), data[len(file.preload):]); err != nil {
		return nil, errors.Wrapf(err, "read %s from %s", name, part)
	}
	return data, nil
}
