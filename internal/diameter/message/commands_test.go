package message

import "testing"

func TestCommandName(t *testing.T) {
	tests := []struct {
		name string
		app  uint32
		cmd  uint32
		want string
	}{
		{name: "common reauth fallback", app: AppDiameterCommon, cmd: CmdReAuth, want: "RAR/RAA"},
		{name: "cx 303 is mar", app: App3GPP_Cx, cmd: CmdMultimediaAuth, want: "MAR/MAA"},
		{name: "zh 303 is mar", app: App3GPP_Zh, cmd: CmdMultimediaAuth, want: "MAR/MAA"},
		{name: "sh 306 is udr", app: App3GPP_Sh, cmd: CmdUserData, want: "UDR/UDA"},
		{name: "sh 308 is snr", app: App3GPP_Sh, cmd: CmdSubscribeNotif, want: "SNR/SNA"},
		{name: "wx 301 is sar", app: App3GPP_Wx, cmd: CmdServerAssignment, want: "SAR/SAA"},
		{name: "swx 301 is sar", app: App3GPP_SWx, cmd: CmdServerAssignment, want: "SAR/SAA"},
		{name: "swm 268 is der", app: App3GPP_SWm, cmd: CmdDiameterEAP, want: "DER/DEA"},
		{name: "s9 trigger establishment", app: App3GPP_S9, cmd: CmdTriggerEstablishment, want: "TER/TEA"},
		{name: "s6a 321 is purge ue", app: App3GPP_S6a, cmd: CmdPurgeUE, want: "PUR/PUA"},
		{name: "s13 324 is equipment check", app: App3GPP_S13, cmd: CmdEquipmentCheck, want: "ECR/ECA"},
		{name: "rx aa", app: App3GPP_Rx, cmd: CmdAA, want: "AAR/AAA"},
		{name: "gx credit control", app: App3GPP_Gx, cmd: CmdCreditControl, want: "CCR/CCA"},
		{name: "slh rir", app: App3GPP_SLh, cmd: CmdRoutingInfo, want: "RIR/RIA"},
		{name: "s6c srr", app: App3GPP_S6c, cmd: CmdSendRoutingInfoSM, want: "SRR/SRA"},
		{name: "s6c alr", app: App3GPP_S6c, cmd: CmdAlertServiceCentre, want: "ALR/ALA"},
		{name: "s6c rdr", app: App3GPP_S6c, cmd: CmdReportSMDelivery, want: "RDR/RDA"},
		{name: "sgd ofr", app: App3GPP_SGd, cmd: CmdMOForwardShortMsg, want: "OFR/OFA"},
		{name: "sgd tfr", app: App3GPP_SGd, cmd: CmdMTForwardShortMsg, want: "TFR/TFA"},
		{name: "unknown pair", app: App3GPP_S6c, cmd: 9999999, want: "Unknown"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := CommandName(tc.app, tc.cmd); got != tc.want {
				t.Fatalf("CommandName(%d, %d) = %q, want %q", tc.app, tc.cmd, got, tc.want)
			}
		})
	}
}
