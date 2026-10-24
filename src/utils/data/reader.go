package data

import (
	"encoding/binary"
	"errors"
)

var ErrShortBuffer = errors.New("buffer too short")

// Reader reads little endian values from a buffer without ever going out of bounds.
// The first failure sticks: every later read returns zero values and Err() reports it.
type Reader struct {
	buf    []byte
	offset int
	err    error
}

func NewReader(buf []byte) *Reader { return &Reader{buf: buf} }

func (r *Reader) Err() error { return r.err }

func (r *Reader) take(n int) []byte {
	if r.err != nil {
		return nil
	}

	if n < 0 || r.offset+n > len(r.buf) {
		r.err = ErrShortBuffer
		return nil
	}

	out := r.buf[r.offset : r.offset+n]
	r.offset += n

	return out
}

func (r *Reader) U8() uint8 {
	if b := r.take(1); b != nil {
		return b[0]
	}
	return 0
}

func (r *Reader) U16() uint16 {
	if b := r.take(2); b != nil {
		return binary.LittleEndian.Uint16(b)
	}
	return 0
}

func (r *Reader) U32() uint32 {
	if b := r.take(4); b != nil {
		return binary.LittleEndian.Uint32(b)
	}
	return 0
}

func (r *Reader) Skip(n int) { r.take(n) }

// String reads a string prefixed by its length (one byte).
func (r *Reader) String() string {
	length := int(r.U8())
	if b := r.take(length); b != nil {
		return string(b)
	}
	return ""
}
