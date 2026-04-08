package message

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/svinson1121/vectorcore-dra/internal/diameter/avp"
)

// Message flag bits
const (
	FlagRequest     = 0x80
	FlagProxiable   = 0x40
	FlagError       = 0x20
	FlagRetransmit  = 0x10

	HeaderLen = 20
)

// Header is the 20-byte Diameter message header.
type Header struct {
	Version     uint8
	Length      uint32
	Flags       byte
	CommandCode uint32
	AppID       uint32
	HopByHop    uint32
	EndToEnd    uint32
}

// Message is a decoded Diameter message.
type Message struct {
	Header Header
	AVPs   []*avp.AVP
}

// IsRequest returns true if the R flag is set.
func (m *Message) IsRequest() bool {
	return m.Header.Flags&FlagRequest != 0
}

// Encode encodes the message to wire format.
func (m *Message) Encode() ([]byte, error) {
	// encode all AVPs first
	var avpBytes []byte
	for _, a := range m.AVPs {
		encoded, err := avp.Encode(a)
		if err != nil {
			return nil, fmt.Errorf("message: encoding AVP %d: %w", a.Code, err)
		}
		avpBytes = append(avpBytes, encoded...)
	}

	totalLen := HeaderLen + len(avpBytes)
	buf := make([]byte, totalLen)

	buf[0] = 1 // Version
	// Length is 3 bytes at offset 1
	buf[1] = byte(totalLen >> 16)
	buf[2] = byte(totalLen >> 8)
	buf[3] = byte(totalLen)

	buf[4] = m.Header.Flags
	// Command Code is 3 bytes at offset 5
	buf[5] = byte(m.Header.CommandCode >> 16)
	buf[6] = byte(m.Header.CommandCode >> 8)
	buf[7] = byte(m.Header.CommandCode)

	binary.BigEndian.PutUint32(buf[8:12], m.Header.AppID)
	binary.BigEndian.PutUint32(buf[12:16], m.Header.HopByHop)
	binary.BigEndian.PutUint32(buf[16:20], m.Header.EndToEnd)

	copy(buf[HeaderLen:], avpBytes)
	return buf, nil
}

// Decode reads a full Diameter message from r.
// It reads 4 bytes first to get version+length, then reads the remaining bytes.
func Decode(r io.Reader) (*Message, error) {
	// Read first 4 bytes: version (1) + length (3)
	hdrBuf := make([]byte, 4)
	if _, err := io.ReadFull(r, hdrBuf); err != nil {
		return nil, fmt.Errorf("message: reading header prefix: %w", err)
	}

	version := hdrBuf[0]
	if version != 1 {
		return nil, fmt.Errorf("message: unsupported Diameter version %d", version)
	}

	totalLen := uint32(hdrBuf[1])<<16 | uint32(hdrBuf[2])<<8 | uint32(hdrBuf[3])
	if totalLen < HeaderLen {
		return nil, fmt.Errorf("message: message length %d too short (min %d)", totalLen, HeaderLen)
	}

	// Read remaining bytes
	rest := make([]byte, totalLen-4)
	if _, err := io.ReadFull(r, rest); err != nil {
		return nil, fmt.Errorf("message: reading message body: %w", err)
	}

	// Reassemble full message
	full := make([]byte, totalLen)
	copy(full[0:4], hdrBuf)
	copy(full[4:], rest)

	return decodeBytes(full)
}

// decodeBytes decodes a Diameter message from a complete byte slice.
func decodeBytes(b []byte) (*Message, error) {
	if len(b) < HeaderLen {
		return nil, errors.New("message: buffer too short for header")
	}

	m := &Message{}
	m.Header.Version = b[0]
	m.Header.Length = uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	m.Header.Flags = b[4]
	m.Header.CommandCode = uint32(b[5])<<16 | uint32(b[6])<<8 | uint32(b[7])
	m.Header.AppID = binary.BigEndian.Uint32(b[8:12])
	m.Header.HopByHop = binary.BigEndian.Uint32(b[12:16])
	m.Header.EndToEnd = binary.BigEndian.Uint32(b[16:20])

	if int(m.Header.Length) > len(b) {
		return nil, fmt.Errorf("message: declared length %d exceeds buffer %d", m.Header.Length, len(b))
	}

	avpData := b[HeaderLen:m.Header.Length]
	var err error
	m.AVPs, err = avp.DecodeAll(avpData)
	if err != nil {
		return nil, fmt.Errorf("message: decoding AVPs: %w", err)
	}

	return m, nil
}

// FindAVP returns the first AVP matching code and vendorID, or nil.
func (m *Message) FindAVP(code uint32, vendorID uint32) *avp.AVP {
	for _, a := range m.AVPs {
		if a.Code == code && a.VendorID == vendorID {
			return a
		}
	}
	return nil
}

// FindAVPs returns all AVPs matching code and vendorID.
func (m *Message) FindAVPs(code uint32, vendorID uint32) []*avp.AVP {
	var result []*avp.AVP
	for _, a := range m.AVPs {
		if a.Code == code && a.VendorID == vendorID {
			result = append(result, a)
		}
	}
	return result
}
