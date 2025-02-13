package helpers

import (
	"io"
	"time"
)

// ReadWithTimeout reads data until it gets the desired number of bytes or times out.
//
// This is an inefficient implementation that should only be used in tests.
func ReadWithTimeout(r io.Reader, n int, timeout time.Duration) []byte {
	byteCh := make(chan byte)
	closer := make(chan struct{})

	go func() {
		count := 0
		for {
			select {
			case <-closer:
				return
			default:
				b := make([]byte, 1)
				got, err := r.Read(b)
				if err != nil {
					return
				}
				if got > 0 {
					byteCh <- b[0]
				}
				count++
				if count >= n {
					return
				}
			}
		}
	}()

	buf := make([]byte, 0, n)
	deadline := time.After(timeout)

ReadLoop:
	for {
		select {
		case b := <-byteCh:
			buf = append(buf, b)
			if len(buf) >= n {
				break ReadLoop
			}
		case <-deadline:
			break ReadLoop
		}
	}
	close(closer)
	return buf
}
