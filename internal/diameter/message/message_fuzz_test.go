package message

import (
	"bytes"
	"testing"
)

func FuzzDecodeBytes(f *testing.F) {
	f.Add([]byte{})
	f.Add(make([]byte, HeaderLen))
	f.Add([]byte{
		1, 0, 0, 19,
		0, 0, 0, 0,
		0, 0, 0, 0,
		0, 0, 0, 0,
		0, 0, 0, 0,
	})
	f.Add([]byte{
		1, 0, 0, 20,
		FlagRequest, 0, 1, 1,
		0, 0, 0, 0,
		0, 0, 0, 1,
		0, 0, 0, 2,
	})

	f.Fuzz(func(t *testing.T, data []byte) {
		decoded, err := decodeBytes(data)
		if err != nil {
			return
		}
		if decoded.Header.Version != 1 {
			t.Fatalf("Decode accepted unsupported version %d", decoded.Header.Version)
		}
		if decoded.Header.Length < HeaderLen ||
			int(decoded.Header.Length) > len(data) ||
			decoded.Header.Length%4 != 0 {
			t.Fatalf("Decode accepted invalid message length %d for %d bytes", decoded.Header.Length, len(data))
		}

		encoded, err := decoded.Encode()
		if err != nil {
			t.Fatalf("Encode of decoded message failed: %v", err)
		}
		roundTrip, err := Decode(bytes.NewReader(encoded))
		if err != nil {
			t.Fatalf("Decode after Encode failed: %v", err)
		}
		if roundTrip.Header.CommandCode != decoded.Header.CommandCode ||
			roundTrip.Header.AppID != decoded.Header.AppID ||
			roundTrip.Header.HopByHop != decoded.Header.HopByHop ||
			roundTrip.Header.EndToEnd != decoded.Header.EndToEnd ||
			len(roundTrip.AVPs) != len(decoded.AVPs) {
			t.Fatalf("message changed during round trip: before=%+v after=%+v", decoded.Header, roundTrip.Header)
		}
	})
}
