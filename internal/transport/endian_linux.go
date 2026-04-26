//go:build linux

package transport

import (
	"encoding/binary"
	"unsafe"
)

var nativeEndian binary.ByteOrder = func() binary.ByteOrder {
	var v uint16 = 0x0102
	if *(*byte)(unsafe.Pointer(&v)) == 0x02 {
		return binary.LittleEndian
	}
	return binary.BigEndian
}()
