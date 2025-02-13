package internal

import "strconv"

// LocalBuffer is a simplified equivalent to bytes.Buffer that allows usage of a preallocated local
// byte slice.
//
// The bytes.Buffer type is very efficient except in one regard: it must always allocate the byte
// slice on the heap, since its internal "buf" field starts out as nil and cannot be directly modified.
// LocalBuffer exports this field so it can be initialized to a slice which, if allowed by Go's escape
// analysis, can remain on the stack as long as its capacity is not exceeded. For example:
//
//	buf := LocalBuffer{Data: make([]byte, 0, 100)}
//	buf.AppendString("some data") // etc.
//	result := buf.Data
//
// In the example, as long as no more than 100 bytes are written to the buffer, there are no heap
// allocations. As soon as the capacity is exceeded, the byte slice is moved to the heap and its
// capacity expands exponentially, similar to bytes.Buffer.
//
// To simplify the API, the LocalBuffer methods return nothing; that's why they're named Append
// rather than Write, because Write has a connotation of imitating the io.Writer API.
type LocalBuffer struct {
	// Data is the current content of the buffer.
	Data []byte
}

// Append appends a byte slice to the buffer.
func (b *LocalBuffer) Append(data []byte) {
	oldLen := b.grow(len(data))
	copy(b.Data[oldLen:], data)
}

// AppendByte appends a single byte to the buffer.
func (b *LocalBuffer) AppendByte(ch byte) { //nolint:stdmethods
	oldLen := b.grow(1)
	b.Data[oldLen] = ch
}

// AppendInt appends the base-10 string representation of an integer to the buffer.
func (b *LocalBuffer) AppendInt(n int) {
	temp := make([]byte, 0, 20)
	temp = strconv.AppendInt(temp, int64(n), 10)
	b.Append(temp)
}

// AppendString appends the bytes of a string to the buffer.
func (b *LocalBuffer) AppendString(s string) {
	oldLen := b.grow(len(s))
	copy(b.Data[oldLen:], s)
}

func (b *LocalBuffer) grow(addBytes int) int {
	oldLen := len(b.Data)
	newLen := oldLen + addBytes
	if cap(b.Data) >= newLen {
		b.Data = b.Data[0:newLen]
	} else {
		newCap := cap(b.Data) * 2
		if newCap < newLen {
			newCap = newLen * 2
		}
		newData := make([]byte, newLen, newCap)
		copy(newData[0:oldLen], b.Data)
		b.Data = newData
	}
	return oldLen
}
