package codec

import (
	"io"
	"time"
)

type Header struct {
	ServiceMethod string
	Seq           uint64
	Error         string
	Timeout       time.Duration
}

type Codec interface {
	io.Closer
	ReadHeader(*Header) error
	ReadBody(interface{}) error
	Write(*Header, interface{}) error
}

type NewCodecFunc func(io.ReadWriteCloser) Codec

const (
	GobType  uint64 = 1
	JsonType uint64 = 2
)

var NewCodecFuncMap map[uint64]NewCodecFunc

func init() {
	NewCodecFuncMap = make(map[uint64]NewCodecFunc)
	NewCodecFuncMap[GobType] = NewGobCodec
}
