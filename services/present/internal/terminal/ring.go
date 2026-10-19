package terminal

// ring keeps the most recent bytes written to it, up to its size.
type ring struct {
	buf  []byte
	head int  // where the next byte goes
	full bool // the buffer has wrapped at least once
}

func newRing(size int) *ring {
	return &ring{buf: make([]byte, size)}
}

// write appends p, overwriting the oldest bytes once the ring is full.
func (r *ring) write(p []byte) {
	size := len(r.buf)
	if len(p) >= size {
		copy(r.buf, p[len(p)-size:])
		r.head, r.full = 0, true
		return
	}
	n := copy(r.buf[r.head:], p)
	copy(r.buf, p[n:])
	end := r.head + len(p)
	if end >= size {
		r.full = true
	}
	r.head = end % size
}

// bytes returns a copy of the kept bytes, oldest first.
func (r *ring) bytes() []byte {
	if !r.full {
		return append([]byte(nil), r.buf[:r.head]...)
	}
	out := make([]byte, 0, len(r.buf))
	out = append(out, r.buf[r.head:]...)
	return append(out, r.buf[:r.head]...)
}
