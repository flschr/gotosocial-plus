//go:build go1.24 && !go1.28

package xunsafe

import (
	"reflect"
	"unsafe"
)

func init() {
	// TypeOf(reflect.Type{}) == *struct{ abi.Type{} }
	t := reflect.TypeOf(reflect.TypeOf(0)).Elem()
	if t.Size() != unsafe.Sizeof(Abi_Type{}) {
		panic("Abi_Type{} not in sync with abi.Type{}")
	}
}

// Abi_Type is a copy of the memory layout of abi.Type{}.
//
// see: go/src/internal/abi/type.go
type Abi_Type struct {
	Size_       uintptr
	PtrBytes    uintptr
	Hash        uint32
	TFlag       uint8
	Align_      uint8
	FieldAlign_ uint8
	Kind_       uint8
	Equal       func(unsafe.Pointer, unsafe.Pointer) bool
	GCData      *byte
	Str         int32
	PtrToThis   int32
}

// Abi_EmptyInterface is a copy of the memory layout of abi.EmptyInterface{},
// which is to say also the memory layout of any method-less interface.
//
// see: go/src/internal/abi/iface.go
type Abi_EmptyInterface struct {
	Type *Abi_Type
	Data unsafe.Pointer
}

// Abi_NonEmptyInterface is a copy of the memory layout of abi.NonEmptyInterface{},
// which is to say also the memory layout of any interface containing method(s).
//
// see: go/src/internal/abi/iface.go on 1.25+
// see: go/src/reflect/value.go on 1.24
type Abi_NonEmptyInterface struct {
	ITab uintptr
	Data unsafe.Pointer
}

// PackIface packs a new reflect.nonEmptyInterface{} using shielded
// itab and data pointer, returning a pointer for caller casting.
func PackIface(itab uintptr, word unsafe.Pointer) unsafe.Pointer {
	return unsafe.Pointer(&Abi_NonEmptyInterface{
		ITab: itab,
		Data: word,
	})
}

// GetIfaceITab generates a new value of given type,
// casts it to the generic param interface type, and
// returns the .itab portion of the abi.NonEmptyInterface{}.
// this is useful for later calls to PackIface for known type.
func GetIfaceITab[I any](t reflect.Type) uintptr {
	s := reflect.New(t).Elem().Interface().(I)
	i := (*Abi_NonEmptyInterface)(unsafe.Pointer(&s))
	return i.ITab
}

// UnpackEface returns the .Data portion of an abi.EmptyInterface{}.
func UnpackEface(a any) unsafe.Pointer {
	return (*Abi_EmptyInterface)(unsafe.Pointer((&a))).Data
}

// see: go/src/internal/unsafeheader/unsafeheader.go
type Unsafeheader_Slice struct {
	Data unsafe.Pointer
	Len  int
	Cap  int
}

// see: go/src/internal/unsafeheader/unsafeheader.go
type Unsafeheader_String struct {
	Data unsafe.Pointer
	Len  int
}
