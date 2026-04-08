package avp

// AVP flag bits
const (
	FlagVendorSpecific = 0x80 // V bit
	FlagMandatory      = 0x40 // M bit
	FlagProtected      = 0x20 // P bit
)

// AVP type names
const (
	TypeOctetString       = "OctetString"
	TypeUTF8String        = "UTF8String"
	TypeInteger32         = "Integer32"
	TypeInteger64         = "Integer64"
	TypeUnsigned32        = "Unsigned32"
	TypeUnsigned64        = "Unsigned64"
	TypeFloat32           = "Float32"
	TypeFloat64           = "Float64"
	TypeGrouped           = "Grouped"
	TypeAddress           = "Address"
	TypeTime              = "Time"
	TypeDiameterIdentity  = "DiameterIdentity"
	TypeDiameterURI       = "DiameterURI"
	TypeEnumerated        = "Enumerated"
	TypeIPFilterRule      = "IPFilterRule"
)

// Well-known AVP codes (no vendor ID unless noted)
const (
	CodeSessionID                       = 263
	CodeHostIPAddress                   = 257
	CodeAuthApplicationID               = 258
	CodeAcctApplicationID               = 259
	CodeVendorSpecificApplicationID     = 260
	CodeOriginHost                      = 264
	CodeSupportedVendorID               = 265
	CodeVendorID                        = 266
	CodeFirmwareRevision                = 267
	CodeResultCode                      = 268
	CodeProductName                     = 269
	CodeDisconnectCause                 = 273
	CodeOriginRealm                     = 296
	CodeInbandSecurityID                = 299
	CodeDestinationHost                 = 293
	CodeDestinationRealm                = 283
	CodeRouteRecord                     = 282
	CodeErrorMessage                    = 281
	CodeOriginStateID                   = 278

	// AVP codes for IMSI extraction
	CodeUserName             = 1   // RFC 6733; User-Name, NAI format in S6a
	CodeSubscriptionID       = 443 // 3GPP; Grouped: Subscription-Id-Type + Subscription-Id-Data
	CodeSubscriptionIDType   = 450 // Enumerated: 0=E164, 1=IMSI, 2=SIP URI, ...
	CodeSubscriptionIDData   = 444 // UTF8String
)

// Address family values for Address AVP type
const (
	AddressFamilyIPv4 = 1
	AddressFamilyIPv6 = 2
)

// Disconnect-Cause enumerated values
const (
	DisconnectCauseRebooting             = 0
	DisconnectCauseBusy                  = 1
	DisconnectCauseDoNotWantToTalkToYou  = 2
)

// Inband-Security-Id values
const (
	InbandSecurityNoSec = 0
	InbandSecurityTLS   = 1
)
