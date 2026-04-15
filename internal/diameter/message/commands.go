package message

// Command codes per RFC 6733 and 3GPP specs
const (
	CmdCapabilitiesExchange = 257     // CER/CEA
	CmdAA                   = 265     // AAR/AAA
	CmdReAuth               = 258     // RAR/RAA
	CmdCreditControl        = 272     // CCR/CCA (RFC 4006 / DCCA)
	CmdAccountingRequest    = 271     // ACR/ACA
	CmdAbortSession         = 274     // ASR/ASA
	CmdSessionTermination   = 275     // STR/STA
	CmdDeviceWatchdog       = 280     // DWR/DWA
	CmdDisconnectPeer       = 282     // DPR/DPA
	CmdUserAuthorization    = 300     // UAR/UAA (3GPP TS 29.229, Cx/Dx)
	CmdServerAssignment     = 301     // SAR/SAA (3GPP TS 29.229/29.273, Cx/Dx/SWx)
	CmdLocationInfo         = 302     // LIR/LIA (3GPP TS 29.229, Cx/Dx)
	CmdMultimediaAuth       = 303     // MAR/MAA (3GPP TS 29.229/29.109/29.273, Cx/Zh/SWx)
	CmdRegistrationTerm     = 304     // RTR/RTA (3GPP TS 29.229/29.273, Cx/SWx)
	CmdPushProfile          = 305     // PPR/PPA (3GPP TS 29.229/29.273, Cx/SWx)
	CmdUserData             = 306     // UDR/UDA (3GPP TS 29.329, Sh)
	CmdProfileUpdate        = 307     // PUR/PUA (3GPP TS 29.329, Sh)
	CmdSubscribeNotif       = 308     // SNR/SNA (3GPP TS 29.329, Sh)
	CmdPushNotification     = 309     // PNR/PNA (3GPP TS 29.329, Sh)
	CmdBootstrappingInfo    = 310     // BIR/BIA (3GPP TS 29.109, Zh)
	CmdUpdateLocation       = 316     // ULR/ULA (3GPP TS 29.272, S6a/S13)
	CmdCancelLocation       = 317     // CLR/CLA (3GPP TS 29.272, S6a/S13)
	CmdAuthenticationInfo   = 318     // AIR/AIA (3GPP TS 29.272, S6a/S13)
	CmdInsertSubscriberData = 319     // IDR/IDA (3GPP TS 29.272, S6a/S13)
	CmdDeleteSubscriberData = 320     // DSR/DSA (3GPP TS 29.272, S6a/S13)
	CmdPurgeUE              = 321     // PUR/PUA (3GPP TS 29.272, S6a/S13)
	CmdReset                = 322     // RSR/RSA (3GPP TS 29.272, S6a/S13)
	CmdNotify               = 323     // NOR/NOA (3GPP TS 29.272, S6a/S13)
	CmdEquipmentCheck       = 324     // ECR/ECA (3GPP TS 29.272, S13 EIR)
	CmdDiameterEAP          = 268     // DER/DEA (RFC 4072 / 3GPP TS 29.273, SWm)
	CmdRoutingInfo          = 8388622 // RIR/RIA (3GPP TS 29.173, SLh)
	CmdTriggerEstablishment = 8388656 // TER/TEA (3GPP TS 29.215, S9)
	CmdMOForwardShortMsg    = 8388645 // OFR/OFA (3GPP TS 29.338, SGd)
	CmdMTForwardShortMsg    = 8388646 // TFR/TFA (3GPP TS 29.338, SGd)
	CmdSendRoutingInfoSM    = 8388647 // SRR/SRA (3GPP TS 29.338, S6c)
	CmdAlertServiceCentre   = 8388648 // ALR/ALA (3GPP TS 29.338, S6c)
	CmdReportSMDelivery     = 8388649 // RDR/RDA (3GPP TS 29.338, S6c)
)

// Result codes per RFC 6733
const (
	DiameterSuccess                = 2001
	DiameterLimitedSuccess         = 2002
	DiameterCommandUnsupported     = 3001
	DiameterUnableToDeliver        = 3002
	DiameterRealmNotServed         = 3003
	DiameterTooBusy                = 3004
	DiameterLoopDetected           = 3005
	DiameterRedirectIndication     = 3006
	DiameterApplicationUnsupported = 3007
	DiameterUnableToComply         = 5012
)

// Application IDs per RFC 6733, RFC 4006, and 3GPP specs
const (
	AppDiameterCommon uint32 = 0 // used in CER
	AppNASREQ         uint32 = 1
	AppMobileIPv4     uint32 = 2
	AppBaseAccounting uint32 = 3
	AppCreditControl  uint32 = 4          // RFC 4006 -- standard Gy/Ro online charging
	AppRelayAgent     uint32 = 0xFFFFFFFF // relay = no app-specific processing

	App3GPP_Cx  uint32 = 16777216 // Cx/Dx IMS
	App3GPP_Sh  uint32 = 16777217 // Sh user data
	App3GPP_Wx  uint32 = 16777219 // Wx AAA-HSS (3GPP TS 29.273)
	App3GPP_Zh  uint32 = 16777221 // Zh Bootstrapping (3GPP TS 29.109)
	App3GPP_Rx  uint32 = 16777236 // Rx P-CSCF/PCRF
	App3GPP_Gx  uint32 = 16777238 // Gx PCEF-PCRF
	App3GPP_Gy  uint32 = 16777239 // Gy online charging (3GPP vendor-specific)
	App3GPP_S6a uint32 = 16777251 // S6a MME-HSS
	App3GPP_S13 uint32 = 16777252 // S13 EIR
	App3GPP_SWm uint32 = 16777264 // SWm ePDG-AAA (3GPP TS 29.273)
	App3GPP_SWx uint32 = 16777265 // SWx HSS-AAA (EPC WiFi)
	App3GPP_S9  uint32 = 16777267 // S9 V-PCRF/H-PCRF (3GPP TS 29.215)
	App3GPP_S6b uint32 = 16777272 // S6b PGW-AAA
	App3GPP_SLh uint32 = 16777291 // SLh LCS routing
	App3GPP_S6c uint32 = 16777312 // S6c HSS-SMSC (3GPP TS 29.338)
	App3GPP_SGd uint32 = 16777313 // SGd SMS over Diameter (3GPP TS 29.338)
)

// 3GPP Vendor ID
const Vendor3GPP uint32 = 10415

var commonCommandNames = map[uint32]string{
	CmdCapabilitiesExchange: "CER/CEA",
	CmdAA:                   "AAR/AAA",
	CmdReAuth:               "RAR/RAA",
	CmdAccountingRequest:    "ACR/ACA",
	CmdCreditControl:        "CCR/CCA",
	CmdAbortSession:         "ASR/ASA",
	CmdSessionTermination:   "STR/STA",
	CmdDeviceWatchdog:       "DWR/DWA",
	CmdDisconnectPeer:       "DPR/DPA",
}

var appCommandNames = map[uint32]map[uint32]string{
	App3GPP_Cx: {
		CmdUserAuthorization: "UAR/UAA",
		CmdServerAssignment:  "SAR/SAA",
		CmdLocationInfo:      "LIR/LIA",
		CmdMultimediaAuth:    "MAR/MAA",
		CmdRegistrationTerm:  "RTR/RTA",
		CmdPushProfile:       "PPR/PPA",
	},
	App3GPP_Sh: {
		CmdUserData:         "UDR/UDA",
		CmdProfileUpdate:    "PUR/PUA",
		CmdSubscribeNotif:   "SNR/SNA",
		CmdPushNotification: "PNR/PNA",
	},
	App3GPP_Wx: {
		CmdServerAssignment: "SAR/SAA",
		CmdMultimediaAuth:   "MAR/MAA",
		CmdRegistrationTerm: "RTR/RTA",
		CmdPushProfile:      "PPR/PPA",
	},
	App3GPP_Zh: {
		CmdMultimediaAuth:    "MAR/MAA",
		CmdBootstrappingInfo: "BIR/BIA",
	},
	App3GPP_Rx: {
		CmdAA:                 "AAR/AAA",
		CmdReAuth:             "RAR/RAA",
		CmdAbortSession:       "ASR/ASA",
		CmdSessionTermination: "STR/STA",
	},
	App3GPP_Gx: {
		CmdReAuth:        "RAR/RAA",
		CmdCreditControl: "CCR/CCA",
		CmdAbortSession:  "ASR/ASA",
	},
	App3GPP_Gy: {
		CmdCreditControl: "CCR/CCA",
		CmdReAuth:        "RAR/RAA",
		CmdAbortSession:  "ASR/ASA",
	},
	AppCreditControl: {
		CmdCreditControl: "CCR/CCA",
		CmdReAuth:        "RAR/RAA",
		CmdAbortSession:  "ASR/ASA",
	},
	App3GPP_S6a: {
		CmdUpdateLocation:       "ULR/ULA",
		CmdCancelLocation:       "CLR/CLA",
		CmdAuthenticationInfo:   "AIR/AIA",
		CmdInsertSubscriberData: "IDR/IDA",
		CmdDeleteSubscriberData: "DSR/DSA",
		CmdPurgeUE:              "PUR/PUA",
		CmdReset:                "RSR/RSA",
		CmdNotify:               "NOR/NOA",
	},
	App3GPP_S13: {
		CmdUpdateLocation:       "ULR/ULA",
		CmdCancelLocation:       "CLR/CLA",
		CmdAuthenticationInfo:   "AIR/AIA",
		CmdInsertSubscriberData: "IDR/IDA",
		CmdDeleteSubscriberData: "DSR/DSA",
		CmdPurgeUE:              "PUR/PUA",
		CmdReset:                "RSR/RSA",
		CmdNotify:               "NOR/NOA",
		CmdEquipmentCheck:       "ECR/ECA",
	},
	App3GPP_SWx: {
		CmdServerAssignment: "SAR/SAA",
		CmdMultimediaAuth:   "MAR/MAA",
		CmdRegistrationTerm: "RTR/RTA",
		CmdPushProfile:      "PPR/PPA",
	},
	App3GPP_SWm: {
		CmdAA:                 "AAR/AAA",
		CmdDiameterEAP:        "DER/DEA",
		CmdReAuth:             "RAR/RAA",
		CmdAbortSession:       "ASR/ASA",
		CmdSessionTermination: "STR/STA",
	},
	App3GPP_S9: {
		CmdReAuth:               "RAR/RAA",
		CmdCreditControl:        "CCR/CCA",
		CmdAbortSession:         "ASR/ASA",
		CmdTriggerEstablishment: "TER/TEA",
	},
	App3GPP_SLh: {
		CmdRoutingInfo: "RIR/RIA",
	},
	App3GPP_S6c: {
		CmdSendRoutingInfoSM:  "SRR/SRA",
		CmdAlertServiceCentre: "ALR/ALA",
		CmdReportSMDelivery:   "RDR/RDA",
	},
	App3GPP_SGd: {
		CmdMOForwardShortMsg: "OFR/OFA",
		CmdMTForwardShortMsg: "TFR/TFA",
	},
}

// CommandName returns a short Diameter command label for an Application-ID and Command-Code pair.
// App-specific commands take precedence over common RFC 6733 commands that are shared across interfaces.
func CommandName(appID uint32, commandCode uint32) string {
	if byApp, ok := appCommandNames[appID]; ok {
		if name, ok := byApp[commandCode]; ok {
			return name
		}
	}
	if name, ok := commonCommandNames[commandCode]; ok {
		return name
	}
	return "Unknown"
}
