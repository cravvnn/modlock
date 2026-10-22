package vpk

import (
	"bytes"
	"cmp"
	"encoding/binary"
	"hash/crc32"
	"io"
	"io/fs"
	"math"
	"path"
	"slices"
	"strings"

	"github.com/pkg/errors"
)

// member is one file Write packs: its place in the directory and its bytes'
// size and checksum.
type member struct {
	// file is the file's path in the tree.
	file string
	// extension is the file's extension without its dot, or " " for none.
	extension string
	// directory is the file's directory, or " " for the root.
	directory string
	// name is the file's name without its extension.
	name string
	// size is the file's length in bytes.
	size int64
	// crc is the CRC-32 of the file's bytes.
	crc uint32
}

// Write packs every regular file in tree into one version 1 Valve archive on
// w, the layout the resource compiler writes: no preload bytes, and each
// file's data stored after the directory, in directory order. Files are
// ordered by extension, directory and name, so equal trees pack to equal
// bytes. Paths keep their case; the game reads lowercase resource names.
// Write reads each file twice, once for its checksum and once to copy it, and
// fails if a file changes in between.
func Write(w io.Writer, tree fs.FS) error {
	// List every regular file, then checksum each one.
	members, err := list(tree)
	if err != nil {
		return err
	}
	for i := range members {
		if err := checksum(tree, &members[i]); err != nil {
			return err
		}
	}

	// Write the header and the directory.
	dir, err := directory(members)
	if err != nil {
		return err
	}
	var header [12]byte
	binary.LittleEndian.PutUint32(header[0:4], signature)
	binary.LittleEndian.PutUint32(header[4:8], 1)
	binary.LittleEndian.PutUint32(header[8:12], uint32(len(dir)))
	if _, err := w.Write(append(header[:], dir...)); err != nil {
		return err
	}

	// Copy each file's data in directory order.
	for _, file := range members {
		if err := copyMember(w, tree, file); err != nil {
			return err
		}
	}
	return nil
}

// Size returns the length of the archive Write packs from tree, reading only
// the files' sizes.
func Size(tree fs.FS) (int64, error) {
	// The archive is its header, its directory and every file's bytes.
	members, err := list(tree)
	if err != nil {
		return 0, err
	}
	dir, err := directory(members)
	if err != nil {
		return 0, err
	}
	size := int64(12 + len(dir))
	for _, file := range members {
		size += file.size
	}
	return size, nil
}

// list returns every regular file in tree as a member without its checksum,
// sorted by extension, directory and name.
func list(tree fs.FS) ([]member, error) {
	var members []member
	err := fs.WalkDir(tree, ".", func(file string, entry fs.DirEntry, err error) error {
		// Skip directories and anything else that holds no bytes.
		if err != nil || !entry.Type().IsRegular() {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}

		// Refuse a name the directory cannot hold: an empty name ends a group.
		packed := newMember(file, info.Size(), 0)
		if packed.name == "" {
			return errors.Errorf("%s has no name before its extension", file)
		}
		members = append(members, packed)
		return nil
	})
	slices.SortFunc(members, func(a, b member) int {
		return cmp.Or(cmp.Compare(a.extension, b.extension), cmp.Compare(a.directory, b.directory), cmp.Compare(a.name, b.name))
	})
	return members, err
}

// checksum reads file's bytes into its checksum, failing if its size changed
// since list read it.
func checksum(tree fs.FS, file *member) error {
	// Read the file through the checksum.
	f, err := tree.Open(file.file)
	if err != nil {
		return err
	}
	defer f.Close()
	sum := crc32.NewIEEE()
	size, err := io.Copy(sum, f)
	if err != nil {
		return errors.Wrap(err, file.file)
	}

	// Refuse a file whose size changed since list read it.
	if size != file.size {
		return errors.Errorf("%s changed while packing", file.file)
	}
	file.crc = sum.Sum32()
	return nil
}

// directory encodes the archive directory of sorted members: each extension,
// then each directory in it, then each name with its record, every list ended
// by an empty string. Data offsets follow directory order.
func directory(members []member) ([]byte, error) {
	// Encode each member after the group it opens.
	var dir bytes.Buffer
	var offset int64
	for i, file := range members {
		// Open a new extension or directory group when the file starts one.
		switch {
		case i == 0:
			dir.WriteString(file.extension + "\x00" + file.directory + "\x00")
		case file.extension != members[i-1].extension:
			dir.WriteString("\x00\x00" + file.extension + "\x00" + file.directory + "\x00")
		case file.directory != members[i-1].directory:
			dir.WriteString("\x00" + file.directory + "\x00")
		}

		// Write the name and its record, pointing at the next data offset.
		if offset+file.size > math.MaxUint32 {
			return nil, errors.New("archive data exceeds 4 GiB")
		}
		var record [18]byte
		binary.LittleEndian.PutUint32(record[0:4], file.crc)
		binary.LittleEndian.PutUint16(record[6:8], embedded)
		binary.LittleEndian.PutUint32(record[8:12], uint32(offset))
		binary.LittleEndian.PutUint32(record[12:16], uint32(file.size))
		binary.LittleEndian.PutUint16(record[16:18], 0xffff)
		dir.WriteString(file.name + "\x00")
		dir.Write(record[:])
		offset += file.size
	}

	// End the last name and directory lists, then the extensions.
	if len(members) != 0 {
		dir.WriteString("\x00\x00")
	}
	dir.WriteByte(0)
	return dir.Bytes(), nil
}

// newMember splits file into the extension, directory and name the archive
// directory groups it by.
func newMember(file string, size int64, crc uint32) member {
	// Split the path; the archive writes " " for an empty directory or extension.
	directory, name := path.Split(file)
	directory = cmp.Or(strings.TrimSuffix(directory, "/"), " ")
	extension := strings.TrimPrefix(path.Ext(name), ".")
	name = strings.TrimSuffix(name, "."+extension)
	return member{file: file, extension: cmp.Or(extension, " "), directory: directory, name: name, size: size, crc: crc}
}

// copyMember copies file's bytes to w, failing if its size changed since
// list read it.
func copyMember(w io.Writer, tree fs.FS, file member) error {
	// Copy at most one byte more than the checksummed size.
	f, err := tree.Open(file.file)
	if err != nil {
		return err
	}
	defer f.Close()
	copied, err := io.Copy(w, io.LimitReader(f, file.size+1))
	if err != nil {
		return errors.Wrap(err, file.file)
	}

	// Refuse a file whose size changed since its checksum.
	if copied != file.size {
		return errors.Errorf("%s changed while packing", file.file)
	}
	return nil
}
