// Package space holds a PC's local Space: a World on a database in the
// daemon's directory. Modlock keeps game files there as UnixFS objects, which
// store each file as content-addressed blocks.
package space

import (
	"context"
	"path/filepath"
	"slices"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/bucket"
	db_core "github.com/s4wave/spacewave/db/core"
	node_controller "github.com/s4wave/spacewave/db/node/controller"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/volume"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/s4wave/spacewave/db/world"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// Identifiers of the Space's volume, bucket and World on its own bus.
const (
	volumeID = "modlock"
	bucketID = "modlock"
	engineID = "modlock"
)

// fsType is the kind of UnixFS object the Space creates.
const fsType = unixfs_world.FSType_FSType_FS_NODE

// Space is an open local Space. Release it after every handle it opened.
type Space struct {
	// le logs the Space's objects.
	le *logrus.Entry
	// ws reads and writes the World.
	ws world.WorldState
	// sender is the volume's peer, which signs the Space's changes.
	sender peer.ID
	// release unwinds the bus's controllers in reverse order.
	release []func()
}

// Open opens the Space in the database at path, creating it when absent. Only
// one process may open path at a time; the daemon's lock ensures it.
func Open(ctx context.Context, le *logrus.Entry, path string) (_ *Space, rerr error) {
	// Tie the bus to the Space's lifetime.
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	s := &Space{le: le, release: []func(){cancel}}
	defer func() {
		if rerr != nil {
			s.Release()
		}
	}()

	// Open the database as a volume. The volume keeps its own key, which
	// names the writer of each change.
	b, sr, err := db_core.NewCoreBus(ctx, le)
	if err != nil {
		return nil, err
	}
	sr.AddFactory(world_block_engine.NewFactory(b))
	ctrl, _, volumeRef, err := loader.WaitExecControllerRunningTyped[volume.Controller](ctx, b,
		resolver.NewLoadControllerWithConfig(&volume_s4db.Config{
			Path:          path,
			SingleProcess: true,
			VolumeConfig:  &volume_controller.Config{VolumeIdAlias: []string{volumeID}},
		}), nil)
	if err != nil {
		return nil, errors.Wrap(err, "open the Space's database")
	}
	s.release = append(s.release, volumeRef.Release)

	// Expose the volume's stores to the World through a node.
	_, _, nodeRef, err := loader.WaitExecControllerRunning(ctx, b, resolver.NewLoadControllerWithConfig(&node_controller.Config{}), nil)
	if err != nil {
		return nil, err
	}
	s.release = append(s.release, nodeRef.Release)

	// Find the volume's peer and keep the World's blocks in one bucket.
	vol, err := ctrl.GetVolume(ctx)
	if err != nil {
		return nil, err
	}
	s.sender = vol.GetPeerID()
	conf, err := bucket.NewConfig(bucketID, 1, nil)
	if err != nil {
		return nil, err
	}
	if _, err := bucket.ExApplyBucketConfig(ctx, b, bucket.NewApplyBucketConfigToVolume(conf, volumeID)); err != nil {
		return nil, err
	}

	// Run the World.
	engConf := world_block_engine.NewConfig(engineID, volumeID, bucketID, bucketID, nil, nil, false)
	eng, engRef, err := world.StartEngine(ctx, b, engConf, nil)
	if err != nil {
		return nil, errors.Wrap(err, "start the Space's World")
	}
	s.release = append(s.release, engRef.Release)

	// Apply UnixFS operations to the World.
	ops := world.NewLookupOpController("modlock/space/ops", engineID, world.NewSchema(unixfs_world.UnixfsTypes).LookupOp)
	opsRel, err := b.AddController(ctx, ops, nil)
	if err != nil {
		return nil, err
	}
	s.release = append(s.release, opsRel)
	s.ws = world.NewEngineWorldState(eng, true)
	return s, nil
}

// FS is an open UnixFS object in the Space.
type FS struct {
	// Handle is the object's root directory.
	Handle *unixfs.FSHandle
	// Key is the object's key in the World.
	Key string
	// Type is the object's UnixFS kind.
	Type unixfs_world.FSType
	// World is the World the object lives in.
	World world.WorldState
	// release closes the handle and its cursor.
	release func()
}

// Release closes the object.
func (f *FS) Release() {
	f.release()
}

// OpenFS opens the UnixFS object at key, creating an empty one when absent.
func (s *Space) OpenFS(ctx context.Context, key string) (*FS, error) {
	// Create the object the first time.
	_, found, err := s.ws.GetObject(ctx, key)
	if err != nil {
		return nil, err
	}
	if !found {
		if _, _, err := unixfs_world.FsInit(ctx, s.ws, s.sender, key, fsType, nil, false, time.Now()); err != nil {
			return nil, errors.Wrapf(err, "create %s", key)
		}
	}

	// Open its root, following its later changes.
	cursor, err := unixfs_world.FollowUnixfsRef(ctx, s.le, s.ws, &unixfs_world.UnixfsRef{ObjectKey: key}, s.sender, true)
	if err != nil {
		return nil, errors.Wrapf(err, "open %s", key)
	}
	handle, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		cursor.Release()
		return nil, err
	}
	return &FS{Handle: handle, Key: key, Type: fsType, World: s.ws, release: func() {
		handle.Release()
		cursor.Release()
	}}, nil
}

// Release stops the World and closes the database.
func (s *Space) Release() {
	for _, rel := range slices.Backward(s.release) {
		rel()
	}
	s.release = nil
}
