//go:build go1.26 && !go1.28

package xunsafe

import (
	"reflect"
	"unsafe"
)

const (
	// see: go/src/internal/abi/type.go
	Abi_TFlagDirectIface uint8 = 1 << 5
)

// see: go/src/internal/abi/type.go Type.Kind()
func Abi_Type_Kind(t reflect.Type) uint8 {
	iface := (*Abi_NonEmptyInterface)(unsafe.Pointer(&t))
	atype := (*Abi_Type)(unsafe.Pointer(iface.Data))
	return atype.Kind_
}

// see: go/src/internal/abi/type.go Type.IsDirectIface()
func Abi_Type_IsDirectIface(t reflect.Type) bool {
	iface := (*Abi_NonEmptyInterface)(unsafe.Pointer(&t))
	atype := (*Abi_Type)(unsafe.Pointer(iface.Data))
	return atype.TFlag&Abi_TFlagDirectIface != 0
}

// see: go/src/internal/abi/type.go Type.IfaceIndir()
//
// Deprecated: use Abi_Type_IsDirectIface.
func Abi_Type_IfaceIndir(t reflect.Type) bool {
	return !Abi_Type_IsDirectIface(t)
}
