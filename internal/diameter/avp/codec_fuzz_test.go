package avp

import (
	"bytes"
	"testing"
)

func FuzzDecode(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0, 0, 0, 1, 0, 0, 0, 8})
	f.Add([]byte{0, 0, 0, 1, 0, 0, 0, 9, 0})
	f.Add([]byte{0, 0, 0, 1, FlagVendorSpecific, 0, 0, 12, 0, 0, 0, 1})

	f.Fuzz(func(t *testing.T, data []byte) {
		decoded, consumed, err := Decode(data)
		if err != nil {
			return
		}
		if decoded == nil {
			t.Fatal("Decode succeeded with a nil AVP")
		}
		if consumed <= 0 || consumed > len(data) || consumed%4 != 0 {
			t.Fatalf("Decode consumed invalid byte count %d from %d bytes", consumed, len(data))
		}

		encoded, err := Encode(decoded)
		if err != nil {
			t.Fatalf("Encode of decoded AVP failed: %v", err)
		}
		roundTrip, roundTripConsumed, err := Decode(encoded)
		if err != nil {
			t.Fatalf("Decode after Encode failed: %v", err)
		}
		if roundTripConsumed != len(encoded) {
			t.Fatalf("round trip consumed %d bytes, want %d", roundTripConsumed, len(encoded))
		}
		if decoded.Code != roundTrip.Code ||
			decoded.VendorID != roundTrip.VendorID ||
			decoded.Flags != roundTrip.Flags ||
			!bytes.Equal(decoded.Data, roundTrip.Data) {
			t.Fatalf("AVP changed during round trip: before=%+v after=%+v", decoded, roundTrip)
		}
	})
}
