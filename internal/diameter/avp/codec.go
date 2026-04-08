package avp

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
)

// AVP represents a single Diameter AVP.
type AVP struct {
	Code     uint32
	VendorID uint32 // 0 if no vendor
	Flags    byte
	Data     []byte // raw data bytes, unpadded
	Children []*AVP // populated for Grouped AVPs by DecodeGrouped
}

// headerSize returns the size of the AVP header (8 or 12 bytes depending on V bit).
func (a *AVP) headerSize() int {
	if a.Flags&FlagVendorSpecific != 0 {
		return 12
	}
	return 8
}

// Encode encodes the AVP to wire format (including padding).
func Encode(a *AVP) ([]byte, error) {
	hdrSize := 8
	if a.VendorID != 0 {
		hdrSize = 12
	}
	dataLen := len(a.Data)
	totalLen := hdrSize + dataLen
	// pad to 4-byte boundary
	padded := (totalLen + 3) &^ 3
	buf := make([]byte, padded)

	binary.BigEndian.PutUint32(buf[0:4], a.Code)

	flags := a.Flags
	if a.VendorID != 0 {
		flags |= FlagVendorSpecific
	} else {
		flags &^= FlagVendorSpecific
	}
	buf[4] = flags
	// length = hdrSize + dataLen (not including padding)
	length := uint32(totalLen)
	buf[5] = byte(length >> 16)
	buf[6] = byte(length >> 8)
	buf[7] = byte(length)

	if a.VendorID != 0 {
		binary.BigEndian.PutUint32(buf[8:12], a.VendorID)
		copy(buf[12:], a.Data)
	} else {
		copy(buf[8:], a.Data)
	}

	return buf, nil
}

// Decode decodes a single AVP from b, returning the AVP and bytes consumed.
func Decode(b []byte) (*AVP, int, error) {
	if len(b) < 8 {
		return nil, 0, errors.New("avp: buffer too short for AVP header")
	}

	a := &AVP{}
	a.Code = binary.BigEndian.Uint32(b[0:4])
	a.Flags = b[4]
	length := uint32(b[5])<<16 | uint32(b[6])<<8 | uint32(b[7])

	if length < 8 {
		return nil, 0, fmt.Errorf("avp: invalid AVP length %d", length)
	}

	hdrSize := 8
	if a.Flags&FlagVendorSpecific != 0 {
		if len(b) < 12 {
			return nil, 0, errors.New("avp: buffer too short for vendor-specific header")
		}
		a.VendorID = binary.BigEndian.Uint32(b[8:12])
		hdrSize = 12
	}

	dataLen := int(length) - hdrSize
	if dataLen < 0 {
		return nil, 0, fmt.Errorf("avp: negative data length for AVP code %d", a.Code)
	}

	totalWithHeader := int(length)
	if len(b) < totalWithHeader {
		return nil, 0, fmt.Errorf("avp: buffer too short: need %d bytes, have %d", totalWithHeader, len(b))
	}

	a.Data = make([]byte, dataLen)
	copy(a.Data, b[hdrSize:hdrSize+dataLen])

	// consumed = padded to 4-byte boundary
	consumed := (totalWithHeader + 3) &^ 3
	if consumed > len(b) {
		consumed = len(b)
	}

	return a, consumed, nil
}

// DecodeAll decodes all AVPs from b.
func DecodeAll(b []byte) ([]*AVP, error) {
	var avps []*AVP
	for len(b) > 0 {
		if len(b) < 8 {
			return nil, fmt.Errorf("avp: trailing bytes too short: %d", len(b))
		}
		a, n, err := Decode(b)
		if err != nil {
			return nil, err
		}
		avps = append(avps, a)
		b = b[n:]
	}
	return avps, nil
}

// DecodeGrouped decodes the children of a Grouped AVP.
func DecodeGrouped(a *AVP) ([]*AVP, error) {
	children, err := DecodeAll(a.Data)
	if err != nil {
		return nil, fmt.Errorf("avp: decoding grouped AVP %d: %w", a.Code, err)
	}
	a.Children = children
	return children, nil
}

// NewUint32 creates an AVP with a uint32 value.
func NewUint32(code uint32, vendorID uint32, flags byte, val uint32) *AVP {
	data := make([]byte, 4)
	binary.BigEndian.PutUint32(data, val)
	a := &AVP{Code: code, VendorID: vendorID, Flags: flags, Data: data}
	if vendorID != 0 {
		a.Flags |= FlagVendorSpecific
	}
	return a
}

// NewUint64 creates an AVP with a uint64 value.
func NewUint64(code uint32, vendorID uint32, flags byte, val uint64) *AVP {
	data := make([]byte, 8)
	binary.BigEndian.PutUint64(data, val)
	a := &AVP{Code: code, VendorID: vendorID, Flags: flags, Data: data}
	if vendorID != 0 {
		a.Flags |= FlagVendorSpecific
	}
	return a
}

// NewString creates an AVP with a string value (OctetString, UTF8String, DiameterIdentity, etc.).
func NewString(code uint32, vendorID uint32, flags byte, val string) *AVP {
	a := &AVP{Code: code, VendorID: vendorID, Flags: flags, Data: []byte(val)}
	if vendorID != 0 {
		a.Flags |= FlagVendorSpecific
	}
	return a
}

// NewAddress creates an AVP with an IP address value (Address type).
// Encoding: 2-byte address family + address bytes.
func NewAddress(code uint32, vendorID uint32, flags byte, ip net.IP) *AVP {
	var data []byte
	if ip4 := ip.To4(); ip4 != nil {
		data = make([]byte, 6)
		binary.BigEndian.PutUint16(data[0:2], AddressFamilyIPv4)
		copy(data[2:], ip4)
	} else {
		ip6 := ip.To16()
		data = make([]byte, 18)
		binary.BigEndian.PutUint16(data[0:2], AddressFamilyIPv6)
		copy(data[2:], ip6)
	}
	a := &AVP{Code: code, VendorID: vendorID, Flags: flags, Data: data}
	if vendorID != 0 {
		a.Flags |= FlagVendorSpecific
	}
	return a
}

// NewGrouped creates a Grouped AVP by encoding the children into the Data field.
func NewGrouped(code uint32, vendorID uint32, flags byte, children []*AVP) (*AVP, error) {
	var data []byte
	for _, child := range children {
		encoded, err := Encode(child)
		if err != nil {
			return nil, fmt.Errorf("avp: encoding grouped child %d: %w", child.Code, err)
		}
		data = append(data, encoded...)
	}
	a := &AVP{Code: code, VendorID: vendorID, Flags: flags, Data: data, Children: children}
	if vendorID != 0 {
		a.Flags |= FlagVendorSpecific
	}
	return a, nil
}

// Uint32 returns the AVP value as uint32.
func (a *AVP) Uint32() (uint32, error) {
	if len(a.Data) != 4 {
		return 0, fmt.Errorf("avp: expected 4 bytes for Uint32, got %d (code=%d)", len(a.Data), a.Code)
	}
	return binary.BigEndian.Uint32(a.Data), nil
}

// Uint64 returns the AVP value as uint64.
func (a *AVP) Uint64() (uint64, error) {
	if len(a.Data) != 8 {
		return 0, fmt.Errorf("avp: expected 8 bytes for Uint64, got %d (code=%d)", len(a.Data), a.Code)
	}
	return binary.BigEndian.Uint64(a.Data), nil
}

// String returns the AVP value as a string.
func (a *AVP) String() (string, error) {
	return string(a.Data), nil
}

// IP returns the AVP value as a net.IP (for Address type AVPs).
func (a *AVP) IP() (net.IP, error) {
	if len(a.Data) < 2 {
		return nil, fmt.Errorf("avp: Address AVP too short: %d bytes", len(a.Data))
	}
	family := binary.BigEndian.Uint16(a.Data[0:2])
	switch family {
	case AddressFamilyIPv4:
		if len(a.Data) != 6 {
			return nil, fmt.Errorf("avp: IPv4 Address AVP wrong length: %d", len(a.Data))
		}
		ip := make(net.IP, 4)
		copy(ip, a.Data[2:6])
		return ip, nil
	case AddressFamilyIPv6:
		if len(a.Data) != 18 {
			return nil, fmt.Errorf("avp: IPv6 Address AVP wrong length: %d", len(a.Data))
		}
		ip := make(net.IP, 16)
		copy(ip, a.Data[2:18])
		return ip, nil
	default:
		return nil, fmt.Errorf("avp: unknown address family %d", family)
	}
}
