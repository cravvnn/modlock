package vpk

import (
	"io"
	"io/fs"
	"path"
	"strings"
	"time"
)

// Packed returns tree with each directory named *.vpk shown as the archive
// Write packs from its files, the inverse of Unpacked. Its size comes from the
// files' sizes alone; opening it starts packing, and reading it streams the
// archive as Write produces it.
func Packed(tree fs.FS) fs.FS {
	return packed{tree: tree}
}

// packed is the tree Packed returns.
type packed struct {
	// tree is the tree holding the archives' directories.
	tree fs.FS
}

// Open opens name, packing it when it is an archive's directory.
func (p packed) Open(name string) (fs.File, error) {
	// Open anything but an archive's directory as it is.
	info, err := p.Stat(name)
	if err != nil {
		return nil, err
	}
	archive, ok := info.(*archiveFile)
	if !ok {
		return p.tree.Open(name)
	}

	// Pack the directory into a pipe the file reads; closing the file stops
	// the packing.
	files, err := fs.Sub(p.tree, name)
	if err != nil {
		return nil, err
	}
	read, write := io.Pipe()
	go func() {
		write.CloseWithError(Write(write, files))
	}()
	return &packing{info: archive, read: read}, nil
}

// Stat describes name, showing an archive's directory as its archive.
func (p packed) Stat(name string) (fs.FileInfo, error) {
	info, err := fs.Stat(p.tree, name)
	if err != nil || !info.IsDir() || !strings.HasSuffix(name, ".vpk") {
		return info, err
	}
	return p.archive(name)
}

// ReadDir lists the directory name, showing each archive's directory in it
// as its archive.
func (p packed) ReadDir(name string) ([]fs.DirEntry, error) {
	entries, err := fs.ReadDir(p.tree, name)
	if err != nil {
		return nil, err
	}
	for i, entry := range entries {
		if entry.IsDir() && strings.HasSuffix(entry.Name(), ".vpk") {
			entries[i] = archiveEntry{tree: p, name: path.Join(name, entry.Name())}
		}
	}
	return entries, nil
}

// archive describes the archive packed from the directory name.
func (p packed) archive(name string) (*archiveFile, error) {
	// Size the archive from its files' sizes.
	files, err := fs.Sub(p.tree, name)
	if err != nil {
		return nil, err
	}
	size, err := Size(files)
	if err != nil {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: err}
	}
	return &archiveFile{name: path.Base(name), size: size}, nil
}

// archiveEntry lists an archive's directory as its archive, sizing it only
// when asked.
type archiveEntry struct {
	// tree is the tree listing the archive.
	tree packed
	// name is the directory's path in tree.
	name string
}

// Name returns the archive's file name.
func (e archiveEntry) Name() string { return path.Base(e.name) }

// IsDir reports false: the directory shows as its archive.
func (e archiveEntry) IsDir() bool { return false }

// Type returns the regular file type.
func (e archiveEntry) Type() fs.FileMode { return 0 }

// Info sizes the archive.
func (e archiveEntry) Info() (fs.FileInfo, error) { return e.tree.archive(e.name) }

// archiveFile describes an archive packed from a directory.
type archiveFile struct {
	// name is the archive's file name.
	name string
	// size is the packed archive's length.
	size int64
}

// Name returns the archive's file name.
func (f *archiveFile) Name() string { return f.name }

// Size returns the packed archive's length.
func (f *archiveFile) Size() int64 { return f.size }

// Mode returns a regular file's mode.
func (f *archiveFile) Mode() fs.FileMode { return 0o644 }

// ModTime returns the zero time.
func (f *archiveFile) ModTime() time.Time { return time.Time{} }

// IsDir reports false.
func (f *archiveFile) IsDir() bool { return false }

// Sys returns nil.
func (f *archiveFile) Sys() any { return nil }

// packing is an open archive, read as Write packs it.
type packing struct {
	// info describes the archive.
	info *archiveFile
	// read is the pipe Write packs into.
	read *io.PipeReader
}

// Stat describes the archive.
func (f *packing) Stat() (fs.FileInfo, error) { return f.info, nil }

// Read reads the next packed bytes.
func (f *packing) Read(b []byte) (int, error) { return f.read.Read(b) }

// Close stops the packing.
func (f *packing) Close() error { return f.read.Close() }
