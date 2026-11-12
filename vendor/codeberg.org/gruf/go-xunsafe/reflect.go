//go:build go1.24 && !go1.28

package xunsafe

import (
	"reflect"
	"unsafe"
)

// see: go/src/reflect/value.go
type Reflect_flag uintptr

// see: go/src/reflect/value.go ro.kind()
func (f Reflect_flag) Kind() reflect.Kind {
	return reflect.Kind(f & Reflect_flagKindMask)
}

// see: go/src/reflect/value.go flag.ro()
func (f Reflect_flag) RO() Reflect_flag {
	if f&Reflect_flagRO != 0 {
		return Reflect_flagStickyRO
	}
	return 0
}

// Indirect returns whether Reflect_flagIndir is set on receiving Reflect_flag.
func (f Reflect_flag) Indirect() bool { return f&Reflect_flagIndir != 0 }

const (
	// see: go/src/reflect/value.go
	Reflect_flagKindWidth                = 5 // there are 27 kinds
	Reflect_flagKindMask    Reflect_flag = 1<<Reflect_flagKindWidth - 1
	Reflect_flagStickyRO    Reflect_flag = 1 << 5
	Reflect_flagEmbedRO     Reflect_flag = 1 << 6
	Reflect_flagIndir       Reflect_flag = 1 << 7
	Reflect_flagAddr        Reflect_flag = 1 << 8
	Reflect_flagMethod      Reflect_flag = 1 << 9
	Reflect_flagMethodShift              = 10
	Reflect_flagRO          Reflect_flag = Reflect_flagStickyRO | Reflect_flagEmbedRO
)

// ReflectIfaceElemFlags returns the reflect_flag expected of an unboxed interface element of type.
//
// see: go/src/reflect/value.go unpackEface() and Value.Elem()
func ReflectIfaceElemFlags(ifaceFlags Reflect_flag, elemType reflect.Type) Reflect_flag {
	if elemType == nil {
		return 0
	}
	f := Reflect_flag(Abi_Type_Kind(elemType))
	if !Abi_Type_IsDirectIface(elemType) {
		f |= Reflect_flagIndir
	}
	if f != 0 {
		f |= ifaceFlags.RO()
	}
	return f
}

// ReflectPointerElemFlags returns the reflect_flag expected of a dereferenced pointer element of type.
//
// see: go/src/reflect/value.go Value.Elem()
func ReflectPointerElemFlags(ptrFlags Reflect_flag, elemType reflect.Type) Reflect_flag {
	return ptrFlags&Reflect_flagRO | Reflect_flagIndir | Reflect_flagAddr | Reflect_flag(Abi_Type_Kind(elemType))
}

// ReflectArrayElemFlags returns the reflect_flag expected of an element of type in an array.
//
// see: go/src/reflect/value.go Value.Index()
func ReflectArrayElemFlags(arrayFlags Reflect_flag, arrayType, elemType reflect.Type) Reflect_flag {
	f := arrayFlags&(Reflect_flagIndir|Reflect_flagAddr) | arrayFlags.RO() | Reflect_flag(Abi_Type_Kind(elemType))

	if arrayType.Len() == 1 && !arrayFlags.Indirect() {
		// single elem arrays are direct
		// memory access by default
		f &= ^Reflect_flagIndir
	}

	return f
}

// reflect_slice_elem_flags returns the reflect_flag expected of a slice element of type.
//
// see: go/src/reflect/value.go Value.Index()
func ReflectSliceElemFlags(sliceFlags Reflect_flag, elemType reflect.Type) Reflect_flag {
	return Reflect_flagAddr | Reflect_flagIndir | sliceFlags.RO() | Reflect_flag(Abi_Type_Kind(elemType))
}

// ReflectStructFieldFlags returns the reflect_flag expected of a struct field of type.
//
// see: go/src/reflect/value.go Value.Field()
func ReflectStructFieldFlags(structFlags Reflect_flag, structType reflect.Type, field reflect.StructField) Reflect_flag {
	f := structFlags&(Reflect_flagStickyRO|Reflect_flagIndir|Reflect_flagAddr) | Reflect_flag(Abi_Type_Kind(field.Type))

	// Unexported forces RO.
	if !field.IsExported() {
		if field.Anonymous {
			f |= Reflect_flagEmbedRO
		} else {
			f |= Reflect_flagStickyRO
		}
	}

	if structType.NumField() == 1 && !structFlags.Indirect() {
		// single elem structs are direct
		// memory access by default
		f &= ^Reflect_flagIndir
	}

	return f
}

// ReflectMapKeyFlags returns the reflect_flag expected of a map key of type.
//
// see: go/src/reflect/map.go MapIter.Key() and copyVal()
func ReflectMapKeyFlags(mapFlags Reflect_flag, keyType reflect.Type) Reflect_flag {
	flags := mapFlags.RO() | Reflect_flag(Abi_Type_Kind(keyType))
	if Abi_Type_IfaceIndir(keyType) {
		flags |= Reflect_flagIndir
	}
	return flags
}

// ReflectMapElemFlags returns the reflect_flag expected of a map element of type.
//
// see: go/src/reflect/map.go MapIter.Value() and copyVal()
func ReflectMapElemFlags(mapFlags Reflect_flag, elemType reflect.Type) Reflect_flag {
	flags := mapFlags.RO() | Reflect_flag(Abi_Type_Kind(elemType))
	if Abi_Type_IfaceIndir(elemType) {
		flags |= Reflect_flagIndir
	}
	return flags
}

// reflect_Value is a copy of the memory layout of reflect.Value{}.
//
// see: go/src/reflect/value.go
type reflect_Value struct {
	typ_ unsafe.Pointer
	ptr  unsafe.Pointer
	Reflect_flag
}

func init() {
	if unsafe.Sizeof(reflect_Value{}) != unsafe.Sizeof(reflect.Value{}) {
		panic("reflect_Value{} not in sync with reflect.Value{}")
	}
}

// reflect_type_data returns the .word from the reflect.Type{} cast
// as the reflect.nonEmptyInterface{}, which itself will be a pointer
// to the actual abi.Type{} that this reflect.Type{} is wrapping.
func ReflectTypeData(t reflect.Type) unsafe.Pointer {
	return (*Abi_NonEmptyInterface)(unsafe.Pointer(&t)).Data
}

// BuildReflectValue manually builds a reflect.Value{} by setting the internal field members.
func BuildReflectValue(rtype reflect.Type, data unsafe.Pointer, flags Reflect_flag) reflect.Value {
	return *(*reflect.Value)(unsafe.Pointer(&reflect_Value{ReflectTypeData(rtype), data, flags}))
}

// Reflect_MapIter is a copy of the memory layout of reflect.MapIter{}.
//
// see: go/src/reflect/map.go
type Reflect_MapIter struct {
	m     reflect.Value
	hiter maps_Iter
}

// maps_Iter is a copy of the memory layout of maps.Iter{}.
//
// see: go/src/internal/runtime/maps/table.go
type maps_Iter struct {
	key  unsafe.Pointer
	elem unsafe.Pointer
	_    uintptr
	_    uintptr
	_    uint64
	_    uint64
	_    uint64
	_    uint8
	_    int
	_    uintptr
	_    struct{ _ unsafe.Pointer }
	_    uint64
}

func init() {
	if unsafe.Sizeof(Reflect_MapIter{}) != unsafe.Sizeof(reflect.MapIter{}) {
		panic("Reflect_MapIter{} not in sync with reflect.MapIter{}")
	}
}

// GetMapIter creates a new map iterator from value,
// skipping the initial v.MapRange() type checking.
func GetMapIter(v reflect.Value) *reflect.MapIter {
	var i Reflect_MapIter
	i.m = v
	return (*reflect.MapIter)(unsafe.Pointer(&i))
}

// Map_Key returns ptr to current map key in iter.
func Map_Key(i *reflect.MapIter) unsafe.Pointer {
	return (*Reflect_MapIter)(unsafe.Pointer(i)).hiter.key
}

// Map_Elem returns ptr to current map element in iter.
func Map_Elem(i *reflect.MapIter) unsafe.Pointer {
	return (*Reflect_MapIter)(unsafe.Pointer(i)).hiter.elem
}
