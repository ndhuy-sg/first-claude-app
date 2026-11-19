package domain

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// VietQR builds Napas VietQR (EMVCo merchant-presented QR) payloads for bank
// transfers: "Chuyển nhanh Napas247 đến tài khoản" with a fixed amount and the
// order code as the transfer purpose (test D-09).

// VietQRGUID is the Napas application id (tag 38, sub-tag 00).
const VietQRGUID = "A000000727"

// VietQRServiceAccount is the service code for transfers to an account number.
const VietQRServiceAccount = "QRIBFTTA"

var ErrVietQRAccount = errors.New("vietqr: bank BIN and account number are required")

// VietQRInput is what a payment QR contains.
type VietQRInput struct {
	BankBIN   string // 6 digits, e.g. "970436"
	AccountNo string
	Amount    int64  // VND, > 0
	Purpose   string // transfer note; we use the order code
}

// tlv encodes one EMVCo data object: 2-digit id, 2-digit length, value.
func tlv(id, value string) string {
	return fmt.Sprintf("%s%02d%s", id, len(value), value)
}

// sanitizePurpose keeps characters every Vietnamese bank accepts in a transfer note.
func sanitizePurpose(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == ' ':
			b.WriteRune(r)
		}
		if b.Len() >= 25 {
			break
		}
	}
	return b.String()
}

// BuildVietQR returns the EMVCo string to render as a QR code.
func BuildVietQR(in VietQRInput) (string, error) {
	if in.BankBIN == "" || in.AccountNo == "" {
		return "", ErrVietQRAccount
	}
	if in.Amount <= 0 {
		return "", errors.New("vietqr: amount must be positive")
	}
	beneficiary := tlv("00", in.BankBIN) + tlv("01", in.AccountNo)
	merchant := tlv("00", VietQRGUID) + tlv("01", beneficiary) + tlv("02", VietQRServiceAccount)
	var s strings.Builder
	s.WriteString(tlv("00", "01"))                             // payload format indicator
	s.WriteString(tlv("01", "12"))                             // point of initiation: 12 = dynamic (one-time, with amount)
	s.WriteString(tlv("38", merchant))                         // Napas merchant account information
	s.WriteString(tlv("53", "704"))                            // currency VND
	s.WriteString(tlv("54", strconv.FormatInt(in.Amount, 10))) // amount
	s.WriteString(tlv("58", "VN"))                             // country
	if p := sanitizePurpose(in.Purpose); p != "" {
		s.WriteString(tlv("62", tlv("08", p))) // additional data: purpose of transaction
	}
	s.WriteString("6304")
	return s.String() + fmt.Sprintf("%04X", CRC16CCITT([]byte(s.String()))), nil
}

// CRC16CCITT is CRC-16/CCITT-FALSE (poly 0x1021, init 0xFFFF), as required by EMVCo tag 63.
func CRC16CCITT(data []byte) uint16 {
	crc := uint16(0xFFFF)
	for _, b := range data {
		crc ^= uint16(b) << 8
		for i := 0; i < 8; i++ {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

// ParseTLV splits an EMVCo string into its top-level data objects (id → value).
// Used by tests and by anyone who needs to read a payload back.
func ParseTLV(s string) (map[string]string, error) {
	out := map[string]string{}
	for len(s) > 0 {
		if len(s) < 4 {
			return nil, errors.New("tlv: truncated header")
		}
		id := s[:2]
		n, err := strconv.Atoi(s[2:4])
		if err != nil || len(s) < 4+n {
			return nil, errors.New("tlv: bad length")
		}
		out[id] = s[4 : 4+n]
		s = s[4+n:]
	}
	return out, nil
}
