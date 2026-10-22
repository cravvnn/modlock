package vpk

import (
	"io/fs"
	"path"
	"strings"
	"sync"
	"testing/fstest"
	"time"
)

// Unpacked returns tree with each single-file archive named *.vpk shown as a
// directory of the same name holding the archive's files. Importing the
// result stores each file on its own, so a file a new version leaves unchanged
// stores as the same blocks. A file that is not a single-file archive, such as
// a split archive's directory, stays a file. Unpacked reads each archive whole
// the first time it is asked about and keeps its files for the returned
// tree's lifetime; an archive nested in another stays a file.
func Unpacked(tree fs.FS) fs.FS {
	return &unpacked{tree: tree, archives: make(map[string]fs.FS)}
}

// unpacked is the tree Unpacked returns.
type unpacked struct {
	// tree is the tree holding the archives.
	tree fs.FS

	// mtx guards archives.
	mtx sync.Mutex
	// archives maps each path read as an archive to its files, or to nil when
	// the path holds no single-file archive.
	archives map[string]fs.FS
}

// Open opens name, inside an archive when a parent of name is one.
func (u *unpacked) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	tree, inner, _ := u.resolve(name)
	return tree.Open(inner)
}

// Stat describes name, showing an archive as a directory.
func (u *unpacked) Stat(name string) (fs.FileInfo, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrInvalid}
	}
	tree, inner, inArchive := u.resolve(name)
	if inArchive && inner == "." {
		return archiveDir(path.Base(name)), nil
	}
	return fs.Stat(tree, inner)
}

// ReadFile reads the file name, inside an archive when a parent of name is
// one.
func (u *unpacked) ReadFile(name string) ([]byte, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readfile", Path: name, Err: fs.ErrInvalid}
	}
	tree, inner, _ := u.resolve(name)
	return fs.ReadFile(tree, inner)
}

// ReadDir lists the directory name, showing each archive in it as a
// directory.
func (u *unpacked) ReadDir(name string) ([]fs.DirEntry, error) {
	// List an archive's directory from its files.
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}
	tree, inner, inArchive := u.resolve(name)
	entries, err := fs.ReadDir(tree, inner)
	if err != nil || inArchive {
		return entries, err
	}

	// Replace each archive file with its directory.
	for i, entry := range entries {
		if entry.Type().IsRegular() && u.archive(path.Join(name, entry.Name())) != nil {
			entries[i] = archiveDir(entry.Name())
		}
	}
	return entries, nil
}

// resolve returns the tree holding name, name's path in it, and whether that
// tree is an archive's files: those of the archive at name or its nearest
// archive parent, else tree itself.
func (u *unpacked) resolve(name string) (fs.FS, string, bool) {
	parts := strings.Split(name, "/")
	for i := range parts {
		if files := u.archive(strings.Join(parts[:i+1], "/")); files != nil {
			return files, path.Join(append([]string{"."}, parts[i+1:]...)...), true
		}
	}
	return u.tree, name, false
}

// archive returns the files of the single-file archive at name, or nil when
// name holds none. A file that cannot be read is no archive; reading it
// through the tree then reports the error.
func (u *unpacked) archive(name string) fs.FS {
	// Read each path once.
	if !strings.HasSuffix(name, ".vpk") {
		return nil
	}
	u.mtx.Lock()
	defer u.mtx.Unlock()
	if files, ok := u.archives[name]; ok {
		return files
	}
	u.archives[name] = nil

	// Read the archive's files from a regular file.
	if info, err := fs.Stat(u.tree, name); err != nil || !info.Mode().IsRegular() {
		return nil
	}
	data, err := fs.ReadFile(u.tree, name)
	if err != nil {
		return nil
	}
	members, err := Files(data)
	if err != nil {
		return nil
	}

	// Keep them in a tree of their own. fstest.MapFS is the standard
	// library's read-only tree of in-memory files.
	files := make(fstest.MapFS, len(members))
	for file, body := range members {
		files[file] = &fstest.MapFile{Data: body, Mode: 0o644}
	}
	u.archives[name] = files
	return files
}

// archiveDir describes an archive shown as a directory, with the mode
// fstest.MapFS gives the directories inside it.
type archiveDir string

// Name returns the archive's file name.
func (d archiveDir) Name() string { return string(d) }

// IsDir reports true: the archive shows as a directory.
func (d archiveDir) IsDir() bool { return true }

// Type returns the directory type.
func (d archiveDir) Type() fs.FileMode { return fs.ModeDir }

// Info returns d itself.
func (d archiveDir) Info() (fs.FileInfo, error) { return d, nil }

// Size returns zero, as for any directory.
func (d archiveDir) Size() int64 { return 0 }

// Mode returns a read-only directory's mode.
func (d archiveDir) Mode() fs.FileMode { return fs.ModeDir | 0o555 }

// ModTime returns the zero time, so equal trees import equally.
func (d archiveDir) ModTime() time.Time { return time.Time{} }

// Sys returns nil.
func (d archiveDir) Sys() any { return nil }
