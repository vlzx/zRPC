package zrpc

import (
	"fmt"
	"reflect"
	"testing"
)

type Foo struct{}

type Args struct {
	Num1 int
	Num2 int
}

func (f Foo) Sum(args Args, reply *int) error {
	*reply = args.Num1 + args.Num2
	return nil
}

func (f Foo) sum(args Args, reply *int) error {
	*reply = args.Num1 + args.Num2
	return nil
}

func (f Foo) Multiply(args Args, reply *int) error {
	*reply = args.Num1 * args.Num2
	return nil
}

func _assert(condition bool, msg string, v ...interface{}) {
	if !condition {
		panic(fmt.Sprintf("assertion failed: "+msg, v...))
	}
}

func TestNewService(t *testing.T) {
	var foo Foo
	s := newService(&foo)
	_assert(len(s.method) == 2, "invalid service Method, except 2, but got %d", len(s.method))
	mType := s.method["Sum"]
	_assert(mType != nil, "invalid Method, `Sum` is nil")
}

func TestMethodType_Call(t *testing.T) {
	var foo Foo
	s := newService(&foo)
	mType := s.method["Sum"]
	argv, replyv := mType.newArgv(), mType.newReplyv()
	argv.Set(reflect.ValueOf(Args{Num1: 2, Num2: 3}))
	err := s.call(mType, argv, replyv)
	_assert(err == nil && *replyv.Interface().(*int) == 5 && mType.numCalls == 1, "failed to call `Foo.Sum`")
}
