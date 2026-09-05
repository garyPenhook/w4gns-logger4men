package main

import "strings"

// Society abbreviations from https://www.iaru.org/reference/member-societies/
// (2026-09-05). Includes ARAS (listed in parentheses), the contest spelling
// OEVSV for Austria, IARU's secretariat, and the four official codes. This is
// an offline token allowlist, not proof of a station's HQ authorization.
// Update it when the sponsor recognizes a new society/exchange abbreviation.
var iaruSubmissionCodes = func() map[string]bool {
	codes := make(map[string]bool)
	for _, code := range strings.Fields(`
		AC R1 R2 R3 IARU OEVSV ARAS
		AARA ARA URA AARS ABARS RCA FRRA AARC WIA OV BARS BARL ARSB BFRR
		BARC RSB RCB LABRE BVIRL BDARA BFRA ARBF ARTJ RAC CARS RCCH CRAC
		CTARL LCRA URAC RCCR HRS FRC VERONA CRK CRC ARAC EDR ARAD DARCI
		RCD GRC ERASD CRAS ERAU RSE EARS FRA FARA SRAL REF CORA AGRA RSTG
		NARG DARC GARS RAAG GARC CRAG ARGUI GARA RCH HARTS MRASZ IRA ARSI
		ORARI IARS IRTS IARC ARI JARA JARL RJRAS KFRR RSK SHRAK KARS LRAL
		RAL LARS LRAA AFVL LRMD RL ARM MARTS CRAM MARL MARS FMRE MRSF
		MARP ARRAM LRM BARTS NARL VERON ARANC NZART CREN NARS RSM NRRL
		ROARS PARS LPRA PNGARS RCP PARA PIARA PZK REP QARS KARL FRR SRR
		RARU SARC ARRSM SARS SRS SARA SLARS SARTS SZR ZRS SIRS SARL URE
		RSSL SKNARS SVGARC SARU VRAS SSA USKA TARL TARC RAST ARCOT TTARS
		ARAT TRAC LRT TACARS ARRL UARS UARL RSGB RCU VARS RCV VARC RSZ ZARS
	`) {
		codes[code] = true
	}
	return codes
}()
