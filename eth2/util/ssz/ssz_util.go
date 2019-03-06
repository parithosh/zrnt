package ssz

import (
	"encoding/binary"
	"github.com/protolambda/go-beacon-transition/eth2"
	"github.com/protolambda/go-beacon-transition/eth2/util/merkle"
	"reflect"
)

// constructs a merkle_root of the given struct (panics if it's not a struct, or a pointer to one),
// but ignores any fields that are tagged with `ssz:"signature"`
func Signed_root(input interface{}) eth2.Root {
	subRoots := make([]eth2.Bytes32, 0)
	v := reflect.ValueOf(input)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		panic("cannot get partial root for signing, input is not a struct")
	}
	vType := v.Type()
	for i, fields := 0, v.NumField(); i < fields; i++ {
		// ignore all fields with a signatures
		if tag, ok := vType.Field(i).Tag.Lookup("ssz"); ok && tag == "signature" {
			break
		}
		subRoots = append(subRoots, eth2.Bytes32(sszHashTreeRoot(v.Field(i))))
	}
	return merkle.Merkle_root(subRoots)
}

func SSZEncode(input interface{}) []byte {
	out := make([]byte, 0)
	sszSerialize(reflect.ValueOf(input), &out)
	return out
}

func withSize(dst *[]byte, size uint64) (start uint64, end uint64) {
	// if capacity is too low, extend it.
	start, end = uint64(len(*dst)), uint64(len(*dst))+size
	if uint64(cap(*dst)) < end {
		res := make([]byte, end, end*2)
		copy(res[0:start], *dst)
		*dst = res
	}
	*dst = (*dst)[:end]
	return start, end
}

// Note: when this is changed,
//  don't forget to change the PutUint32 calls that make put the length in this allocated space.
const BYTES_PER_LENGTH_PREFIX = 4

func sszSerialize(v reflect.Value, dst *[]byte) (encodedLen uint32) {
	switch v.Kind() {
	case reflect.Ptr:
		return sszSerialize(v.Elem(), dst)
	case reflect.Uint8: // "uintN"
		s, _ := withSize(dst, 1)
		(*dst)[s] = byte(v.Uint())
		return 1
	case reflect.Uint32: // "uintN"
		s, e := withSize(dst, 4)
		binary.LittleEndian.PutUint32((*dst)[s:e], uint32(v.Uint()))
		return 4
	case reflect.Uint64: // "uintN"
		s, e := withSize(dst, 8)
		binary.LittleEndian.PutUint64((*dst)[s:e], uint64(v.Uint()))
		return 8
	case reflect.Bool: // "bool"
		s, _ := withSize(dst, 1)
		if v.Bool() {
			(*dst)[s] = 1
		} else {
			(*dst)[s] = 0
		}
		return 1
	case reflect.Array: // "tuple"
		if isFixedLength(v.Type().Elem()) {
			for i, size := 0, v.Len(); i < size; i++ {
				serializedSize := sszSerialize(v.Index(i), dst)
				encodedLen += serializedSize
			}
		} else {
			for i, size := 0, v.Len(); i < size; i++ {
				// allocate size prefix
				s, e := withSize(dst, BYTES_PER_LENGTH_PREFIX)
				serializedSize := sszSerialize(v.Index(i), dst)
				binary.LittleEndian.PutUint32((*dst)[s:e], serializedSize)
				encodedLen += BYTES_PER_LENGTH_PREFIX + serializedSize
			}
		}
		return encodedLen
	case reflect.Slice: // "list"
		for i, size := 0, v.Len(); i < size; i++ {
			// allocate size prefix
			s, e := withSize(dst, BYTES_PER_LENGTH_PREFIX)
			serializedSize := sszSerialize(v.Index(i), dst)
			binary.LittleEndian.PutUint32((*dst)[s:e], serializedSize)
			encodedLen += BYTES_PER_LENGTH_PREFIX + serializedSize
		}
		return encodedLen
	case reflect.Struct: // "container"
		for i, size := 0, v.NumField(); i < size; i++ {
			// allocate size prefix
			s, e := withSize(dst, BYTES_PER_LENGTH_PREFIX)
			serializedSize := sszSerialize(v.Field(i), dst)
			binary.LittleEndian.PutUint32((*dst)[s:e], serializedSize)
			encodedLen += BYTES_PER_LENGTH_PREFIX + serializedSize
		}
		return encodedLen
	default:
		panic("ssz encoding: unsupported value kind: " + v.Kind().String())
	}
}

func Hash_tree_root(input interface{}) eth2.Root {
	return sszHashTreeRoot(reflect.ValueOf(input))
}

// TODO: see specs #679, comment.
// Implementation here simply assumes fixed-length arrays only have elements of fixed-length.

func isFixedLength(vt reflect.Type) bool {
	switch vt.Kind() {
	case reflect.Uint8, reflect.Uint32, reflect.Uint64, reflect.Bool:
		return true
	case reflect.Slice:
		return false
	case reflect.Array:
		return isFixedLength(vt.Elem())
	case reflect.Struct:
		for i, length := 0, vt.NumField(); i < length; i++ {
			if !isFixedLength(vt.Field(i).Type) {
				return false
			}
		}
		return true
	default:
		panic("is-fixed length: unsupported value type: " + vt.String())
		return false
	}
}

// only call this for slices and arrays, not structs
func varSizeListElementsRoot(v reflect.Value) eth2.Root {
	items := v.Len()
	data := make([]eth2.Bytes32, items)
	for i := 0; i < items; i++ {
		data[i] = eth2.Bytes32(sszHashTreeRoot(v.Index(i)))
	}
	return merkle.Merkle_root(data)
}

func varSizeStructElementsRoot(v reflect.Value) eth2.Root {
	fields := v.NumField()
	data := make([]eth2.Bytes32, fields)
	for i := 0; i < fields; i++ {
		data[i] = eth2.Bytes32(sszHashTreeRoot(v.Field(i)))
	}
	return merkle.Merkle_root(data)
}

// Compute hash tree root for a value
func sszHashTreeRoot(v reflect.Value) eth2.Root {
	switch v.Kind() {
	case reflect.Ptr:
		return sszHashTreeRoot(v.Elem())
	// "basic object? -> pack and merkle_root
	case reflect.Uint8, reflect.Uint32, reflect.Uint64, reflect.Bool:
		return merkle.Merkle_root(sszPack(v))
	// "array of fixed length items? -> pack and merkle_root
	// "array of var length items? -> take the merkle root of every element
	// 		(if it aligns in a single chunk, it will just be as-is, not hashed again, see merklezeition)
	case reflect.Array:
		if isFixedLength(v.Type().Elem()) {
			return merkle.Merkle_root(sszPack(v))
		} else {
			return varSizeListElementsRoot(v)
		}
	case reflect.Slice:
		if isFixedLength(v.Type().Elem()) {
			return sszMixInLength(merkle.Merkle_root(sszPack(v)), uint64(v.Len()))
		} else {
			return sszMixInLength(varSizeListElementsRoot(v), uint64(v.Len()))
		}
	case reflect.Struct:
		return varSizeStructElementsRoot(v)
	default:
		panic("tree-hash: unsupported value kind: " + v.Kind().String())
	}
}

func sszPack(input reflect.Value) []eth2.Bytes32 {
	serialized := make([]byte, 0)
	switch input.Kind() {
	case reflect.Array, reflect.Slice:
		for i, length := 0, input.Len(); i < length; i++ {
			sszSerialize(input.Index(i), &serialized)
		}
	case reflect.Struct:
		for i, length := 0, input.NumField(); i < length; i++ {
			sszSerialize(input.Field(i), &serialized)
		}
	default:
		sszSerialize(input, &serialized)
	}
	// floored: handle all normal chunks first
	flooredChunkCount := len(serialized) / 32
	// ceiled: include any partial chunk at end as full chunk (with padding)
	out := make([]eth2.Bytes32, (len(serialized)+31)/32)
	for i := 0; i < flooredChunkCount; i++ {
		copy(out[i][:], serialized[i<<5:(i+1)<<5])
	}
	// if there is a partial chunk at the end, handle it as a special case:
	if len(serialized)&31 != 0 {
		copy(out[flooredChunkCount][:len(serialized)&0x1F], serialized[flooredChunkCount<<5:])
	}
	return out
}

func sszMixInLength(data eth2.Root, length uint64) eth2.Root {
	lengthInput := eth2.Bytes32{}
	binary.LittleEndian.PutUint64(lengthInput[:], length)
	return merkle.Merkle_root([]eth2.Bytes32{eth2.Bytes32(data), lengthInput})
}
