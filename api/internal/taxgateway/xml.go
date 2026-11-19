package taxgateway

import (
	"encoding/xml"
	"fmt"
	"time"
)

// The XML layout below follows the field names of the Vietnamese e-invoice data
// format (HDon → DLHDon → TTChung / NDHDon → NBan, NMua, DSHHDVu, TToan). It is a
// reasonable placeholder: when the owner registers and the exact schema/version
// is known, replace BuildXML (and update testdata/*.golden.xml with -update).

type xHDon struct {
	XMLName xml.Name `xml:"HDon"`
	DLHDon  xDLHDon  `xml:"DLHDon"`
}

type xDLHDon struct {
	ID      string   `xml:"Id,attr"`
	TTChung xTTChung `xml:"TTChung"`
	NDHDon  xNDHDon  `xml:"NDHDon"`
}

type xTTChung struct {
	PBan      string    `xml:"PBan"`     // format version
	THDon     string    `xml:"THDon"`    // invoice name
	KHMSHDon  string    `xml:"KHMSHDon"` // template number (first char of the symbol)
	KHHDon    string    `xml:"KHHDon"`   // symbol without the template number
	SHDon     int64     `xml:"SHDon"`    // invoice number
	NLap      string    `xml:"NLap"`     // issue date YYYY-MM-DD (Vietnam time)
	DVTTe     string    `xml:"DVTTe"`    // currency
	TGia      int       `xml:"TGia"`     // exchange rate
	HTTToan   string    `xml:"HTTToan"`  // payment method
	TCHDon    string    `xml:"TCHDon,omitempty"`
	TTHDLQuan *xRelated `xml:"TTHDLQuan,omitempty"` // related (original) invoice
}

type xRelated struct {
	TCHDon       string `xml:"TCHDon"` // nature: "Hủy"
	KHMSHDCLQuan string `xml:"KHMSHDCLQuan"`
	KHHDCLQuan   string `xml:"KHHDCLQuan"`
	SHDCLQuan    int64  `xml:"SHDCLQuan"`
	NLHDCLQuan   string `xml:"NLHDCLQuan"`
}

type xNDHDon struct {
	NBan    xParty   `xml:"NBan"`
	NMua    xParty   `xml:"NMua"`
	DSHHDVu xDSHHDVu `xml:"DSHHDVu"`
	TToan   xTToan   `xml:"TToan"`
}

type xParty struct {
	Ten    string `xml:"Ten"`
	MST    string `xml:"MST,omitempty"`
	DChi   string `xml:"DChi,omitempty"`
	DCTDTu string `xml:"DCTDTu,omitempty"`
}

type xDSHHDVu struct {
	HHDVu []xHHDVu `xml:"HHDVu"`
}

type xHHDVu struct {
	TChat  int    `xml:"TChat"` // 1 = goods/service, 3 = trade discount
	STT    int    `xml:"STT"`
	THHDVu string `xml:"THHDVu"`
	SLuong int    `xml:"SLuong"`
	DGia   int64  `xml:"DGia"`
	ThTien int64  `xml:"ThTien"`
	TSuat  string `xml:"TSuat"`
}

type xTToan struct {
	THTTLTSuat xTHTTLTSuat `xml:"THTTLTSuat"`
	TgTCThue   int64       `xml:"TgTCThue"`
	TgTThue    int64       `xml:"TgTThue"`
	TgTTTBSo   int64       `xml:"TgTTTBSo"`
	TgTTTBChu  string      `xml:"TgTTTBChu"`
}

type xTHTTLTSuat struct {
	LTSuat xLTSuat `xml:"LTSuat"`
}

type xLTSuat struct {
	TSuat  string `xml:"TSuat"`
	ThTien int64  `xml:"ThTien"`
	TThue  int64  `xml:"TThue"`
}

var vn = func() *time.Location {
	l, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		return time.FixedZone("ICT", 7*3600)
	}
	return l
}()

// RateText formats basis points as the tax-rate text, e.g. 800 → "8%", 525 → "5.25%".
func RateText(bp int) string {
	if bp%100 == 0 {
		return fmt.Sprintf("%d%%", bp/100)
	}
	return fmt.Sprintf("%d.%02d%%", bp/100, bp%100)
}

func splitSymbol(s string) (template, symbol string) {
	if len(s) < 2 {
		return "", s
	}
	return s[:1], s[1:]
}

// BuildXML renders the invoice data document.
func BuildXML(inv Invoice) ([]byte, error) {
	tpl, sym := splitSymbol(inv.Symbol)
	rate := RateText(inv.VatRateBp)
	doc := xHDon{DLHDon: xDLHDon{
		ID: inv.InternalID,
		TTChung: xTTChung{
			PBan: "2.1.0", THDon: "HÓA ĐƠN GIÁ TRỊ GIA TĂNG", KHMSHDon: tpl, KHHDon: sym, SHDon: inv.Number,
			NLap: inv.IssuedAt.In(vn).Format("2006-01-02"), DVTTe: "VND", TGia: 1, HTTToan: inv.PaymentMethod,
		},
		NDHDon: xNDHDon{
			NBan: xParty{Ten: inv.Seller.Name, MST: inv.Seller.TaxCode, DChi: inv.Seller.Address},
			NMua: xParty{Ten: inv.Buyer.Name, MST: inv.Buyer.TaxCode, DChi: inv.Buyer.Address, DCTDTu: inv.Buyer.Email},
			TToan: xTToan{
				THTTLTSuat: xTHTTLTSuat{LTSuat: xLTSuat{TSuat: rate, ThTien: inv.AmountBeforeVat, TThue: inv.VatAmount}},
				TgTCThue:   inv.AmountBeforeVat, TgTThue: inv.VatAmount, TgTTTBSo: inv.Total, TgTTTBChu: inv.TotalInWords,
			},
		},
	}}
	if inv.Kind == "cancel" && inv.Original != nil {
		otpl, osym := splitSymbol(inv.Original.Symbol)
		doc.DLHDon.TTChung.TCHDon = "1"
		doc.DLHDon.TTChung.TTHDLQuan = &xRelated{
			TCHDon: "Hủy", KHMSHDCLQuan: otpl, KHHDCLQuan: osym, SHDCLQuan: inv.Original.Number,
			NLHDCLQuan: inv.Original.IssuedAt.In(vn).Format("2006-01-02"),
		}
	}
	for _, l := range inv.Lines {
		tchat := 1
		if l.Amount < 0 {
			tchat = 3 // trade discount line
		}
		doc.DLHDon.NDHDon.DSHHDVu.HHDVu = append(doc.DLHDon.NDHDon.DSHHDVu.HHDVu, xHHDVu{
			TChat: tchat, STT: l.No, THHDVu: l.Name, SLuong: l.Qty, DGia: l.UnitPrice, ThTien: l.Amount, TSuat: rate,
		})
	}
	b, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), append(b, '\n')...), nil
}
