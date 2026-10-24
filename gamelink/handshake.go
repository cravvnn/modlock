package gamelink

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"time"

	"github.com/pkg/errors"
)

// handshakeLimit bounds the handshake on each side, so an abandoned attempt
// closes promptly.
const handshakeLimit = 15 * time.Second

// handshake runs exchange on the game stream's control stream: the guest's
// eight-byte big-endian server generation, then the host's one-byte
// acceptance (1). Game packets flow only afterward.
func handshake(ctx context.Context, stream io.ReadWriteCloser, exchange func() error) error {
	// Close the stream if the exchange outlives its bound or ctx.
	deadline, cancel := context.WithTimeout(ctx, handshakeLimit)
	defer cancel()
	stop := context.AfterFunc(deadline, func() { stream.Close() })
	defer stop()
	err := exchange()
	if deadline.Err() != nil {
		return deadline.Err()
	}
	return err
}

// requestAdmission sends the server generation and blocks until the host
// accepts it.
func requestAdmission(ctx context.Context, stream io.ReadWriteCloser, generation uint64) error {
	return handshake(ctx, stream, func() error {
		// Name the server process.
		var request [8]byte
		binary.BigEndian.PutUint64(request[:], generation)
		if _, err := io.Copy(stream, bytes.NewReader(request[:])); err != nil {
			return err
		}

		// Wait for the host's answer.
		var accepted [1]byte
		if _, err := io.ReadFull(stream, accepted[:]); err != nil {
			return errors.Wrap(err, "the game host did not accept this session")
		}
		if accepted[0] != 1 {
			return errors.New("the game host rejected this session")
		}
		return nil
	})
}

// ReadGeneration reads the server generation a guest asks for, before any
// game packets. The caller compares it with the server it runs.
func ReadGeneration(ctx context.Context, stream io.ReadWriteCloser) (uint64, error) {
	// Read the request and reject the empty generation.
	var request [8]byte
	err := handshake(ctx, stream, func() error { _, err := io.ReadFull(stream, request[:]); return err })
	if err != nil {
		return 0, err
	}
	generation := binary.BigEndian.Uint64(request[:])
	if generation == 0 {
		return 0, errors.New("the game host generation is required")
	}
	return generation, nil
}

// AcceptGeneration tells the guest that its forwarding socket is in place, so
// game packets may flow.
func AcceptGeneration(ctx context.Context, stream io.ReadWriteCloser) error {
	return handshake(ctx, stream, func() error {
		_, err := io.Copy(stream, bytes.NewReader([]byte{1}))
		return err
	})
}
