package avp

import "sync"

// Entry describes a known AVP in the dictionary.
type Entry struct {
	Code     uint32
	VendorID uint32
	Name     string
	Type     string
	MBit     bool
}

// Dictionary is a thread-safe AVP dictionary.
type Dictionary struct {
	mu      sync.RWMutex
	entries map[uint64]*Entry // key = (vendorID << 32) | code
}

// key builds the lookup key for a code + vendorID pair.
func key(code, vendorID uint32) uint64 {
	return uint64(vendorID)<<32 | uint64(code)
}

// NewDictionary creates a dictionary pre-populated with standard AVPs.
func NewDictionary() *Dictionary {
	d := &Dictionary{
		entries: make(map[uint64]*Entry),
	}
	d.loadStandard()
	return d
}

// Lookup returns the Entry for a given code+vendorID, or nil if unknown.
func (d *Dictionary) Lookup(code, vendorID uint32) *Entry {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.entries[key(code, vendorID)]
}

// Add inserts or replaces an entry.
func (d *Dictionary) Add(e *Entry) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.entries[key(e.Code, e.VendorID)] = e
}

// loadStandard pre-populates the dictionary with RFC 6733 base AVPs.
func (d *Dictionary) loadStandard() {
	standard := []*Entry{
		{Code: CodeSessionID, VendorID: 0, Name: "Session-Id", Type: TypeUTF8String, MBit: true},
		{Code: CodeHostIPAddress, VendorID: 0, Name: "Host-IP-Address", Type: TypeAddress, MBit: true},
		{Code: CodeAuthApplicationID, VendorID: 0, Name: "Auth-Application-Id", Type: TypeUnsigned32, MBit: true},
		{Code: CodeAcctApplicationID, VendorID: 0, Name: "Acct-Application-Id", Type: TypeUnsigned32, MBit: true},
		{Code: CodeVendorSpecificApplicationID, VendorID: 0, Name: "Vendor-Specific-Application-Id", Type: TypeGrouped, MBit: true},
		{Code: CodeOriginHost, VendorID: 0, Name: "Origin-Host", Type: TypeDiameterIdentity, MBit: true},
		{Code: CodeSupportedVendorID, VendorID: 0, Name: "Supported-Vendor-Id", Type: TypeUnsigned32, MBit: false},
		{Code: CodeVendorID, VendorID: 0, Name: "Vendor-Id", Type: TypeUnsigned32, MBit: true},
		{Code: CodeFirmwareRevision, VendorID: 0, Name: "Firmware-Revision", Type: TypeUnsigned32, MBit: false},
		{Code: CodeResultCode, VendorID: 0, Name: "Result-Code", Type: TypeUnsigned32, MBit: true},
		{Code: CodeProductName, VendorID: 0, Name: "Product-Name", Type: TypeUTF8String, MBit: false},
		{Code: CodeDisconnectCause, VendorID: 0, Name: "Disconnect-Cause", Type: TypeEnumerated, MBit: true},
		{Code: CodeOriginRealm, VendorID: 0, Name: "Origin-Realm", Type: TypeDiameterIdentity, MBit: true},
		{Code: CodeInbandSecurityID, VendorID: 0, Name: "Inband-Security-Id", Type: TypeUnsigned32, MBit: true},
		{Code: CodeDestinationHost, VendorID: 0, Name: "Destination-Host", Type: TypeDiameterIdentity, MBit: true},
		{Code: CodeDestinationRealm, VendorID: 0, Name: "Destination-Realm", Type: TypeDiameterIdentity, MBit: true},
		{Code: CodeRouteRecord, VendorID: 0, Name: "Route-Record", Type: TypeDiameterIdentity, MBit: true},
		{Code: CodeErrorMessage, VendorID: 0, Name: "Error-Message", Type: TypeUTF8String, MBit: false},
	}
	for _, e := range standard {
		d.entries[key(e.Code, e.VendorID)] = e
	}
}

// DefaultDictionary is the package-level dictionary instance.
var DefaultDictionary = NewDictionary()
