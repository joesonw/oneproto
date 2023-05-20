package oneprotou_til

import (
	"bytes"
	"fmt"
)

type Buffer struct {
	buf *bytes.Buffer
}

func NewBuffer() *Buffer {
	return &Buffer{buf: &bytes.Buffer{}}
}

func (b *Buffer) Printf(f string, i ...any) {
	_, _ = b.buf.WriteString(fmt.Sprintf(f+"\n", i...))
}

func (b *Buffer) String() string {
	return b.buf.String()
}

func (b *Buffer) Bytes() []byte {
	return b.buf.Bytes()
}
