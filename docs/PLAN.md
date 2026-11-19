# Kế hoạch hệ thống "Mộc" – Website + POS chuỗi 5 chi nhánh

Tài liệu này ghi lại các quyết định **đã được chủ dự án duyệt**. Mọi thay đổi về phạm vi cần cập nhật tại đây trước khi viết code.

Quy trình làm việc: **lập kế hoạch → viết test case → chủ dự án duyệt → viết code test → viết code cho đến khi test qua.**

## 1. Phạm vi

| Khu vực | Đường dẫn | Người dùng | Thiết bị |
|---|---|---|---|
| Website giới thiệu | `/`, `/menu`, `/lien-he` | Khách | Điện thoại (ưu tiên), máy tính |
| Bán hàng (POS) | `/pos` | Thu ngân, pha chế | Máy tính bảng nằm ngang, máy tính |
| Quản trị | `/admin` | Chủ chuỗi, quản lý kho tổng, quản lý chi nhánh | Máy tính |

Quy mô: **5 chi nhánh** và **1 kho tổng** quản lý nguyên vật liệu.

## 2. Công nghệ

| Phần | Lựa chọn |
|---|---|
| Giao diện | Next.js (App Router, TypeScript) + Tailwind CSS. Font Lora (tiêu đề) và Be Vietnam Pro (nội dung) |
| Máy chủ (API) | **Go + Gin**. Đã so sánh với Fiber: Fiber nhanh hơn khoảng 30–40% khi đo framework đơn thuần, nhưng Gin phổ biến gấp đôi (89,5 nghìn so với 40,3 nghìn sao GitHub) và tương thích thư viện chuẩn của Go. Ở quy mô 5 chi nhánh, khác biệt tốc độ không đáng kể |
| Cơ sở dữ liệu | PostgreSQL. Kết nối bằng `pgx`, truy vấn bằng `sqlc`, thay đổi cấu trúc bảng bằng `goose` |
| Đăng nhập | Phiên đăng nhập lưu trong PostgreSQL, gửi qua cookie `httpOnly`. Mật khẩu và PIN mã hoá bcrypt. Quản lý đăng nhập bằng email + mật khẩu, nhân viên quầy bằng PIN |
| Cập nhật tức thì | Server-Sent Events |
| Tài liệu API | OpenAPI; tự sinh code gọi API bằng TypeScript cho phía Next.js |
| Triển khai | Docker Compose trên một máy chủ cloud (Caddy, Next.js, API Go, PostgreSQL). Sao lưu cơ sở dữ liệu mỗi ngày ra nơi khác |
| Test | Go: `go test` và testcontainers-go (PostgreSQL thật). Giao diện: Vitest và Playwright |

```
Trình duyệt → Caddy (HTTPS, chung một tên miền)
               ├── /       → Next.js (chỉ giao diện)
               └── /api/*  → Go + Gin (mọi nghiệp vụ) → PostgreSQL
                                   └── Nhà cung cấp hoá đơn điện tử
```

**Nguyên tắc:**
- Go là nơi duy nhất quyết định giá, phân quyền và trạng thái đơn.
- Tiền lưu bằng số nguyên VND.
- Đơn đã thanh toán không sửa được.
- Mọi thay đổi tồn kho được ghi vào sổ nhập xuất, không sửa, không xoá.
- Mọi API tự lọc dữ liệu theo chi nhánh của người đang đăng nhập.

## 3. Quyết định nghiệp vụ đã chốt

| # | Quyết định |
|---|---|
| 1 | Framework phía máy chủ: **Gin** |
| 2 | **Menu và giá giống nhau ở cả 5 chi nhánh.** Chi nhánh chỉ được bật/tắt "hết món" |
| 3 | Chi nhánh **được tự mua một số nguyên liệu tại chỗ** (phiếu nhập trực tiếp vào kho chi nhánh) |
| 4 | Kho tổng **pha chế bán thành phẩm** (cốt cà phê, syrup…) bằng phiếu sản xuất: trừ nguyên liệu thô, cộng thành phẩm |
| 5 | **Quản lý hạn sử dụng theo lô**: lô hết hạn sớm hơn thì xuất trước |
| 6 | Khi hết nguyên liệu trên hệ thống: **vẫn cho bán, cho phép tồn âm** và cảnh báo quản lý |
| 7 | **Xuất hoá đơn điện tử, không in giấy.** Không cần máy in nhiệt. Khách nhận hoá đơn bằng mã QR tra cứu hiện trên màn hình, hoặc qua email |

## 4. Các giai đoạn

| Giai đoạn | Nội dung |
|---|---|
| **0** | Website giới thiệu. Menu lấy từ API, form liên hệ lưu vào hệ thống |
| **1 – Nền tảng chuỗi và bán hàng** | Chi nhánh, phân quyền theo chi nhánh, kích hoạt máy POS, menu chung (size, tuỳ chọn, hết món), bàn, màn hình bán hàng, giảm giá thủ công, thanh toán tiền mặt và VietQR (xác nhận bằng tay), **hoá đơn điện tử**, ca làm, báo cáo bán hàng theo chi nhánh và toàn chuỗi, nhật ký thao tác |
| **2 – Kho** | Nguyên liệu và quy đổi đơn vị, kho tổng và 5 kho chi nhánh, nhà cung cấp, nhập hàng (vào kho tổng hoặc tại chi nhánh), lô và hạn sử dụng, phiếu sản xuất, yêu cầu và chuyển kho, công thức, tự trừ kho khi bán (cho phép tồn âm), phiếu huỷ, kiểm kê, cảnh báo sắp hết |
| **3** | Giá vốn bình quân và lãi gộp, báo cáo hao hụt, đề xuất đặt hàng, màn hình quầy pha chế, khuyến mãi, xuất Excel |
| **4** | Khách thân thiết, gọi món bằng QR tại bàn, chạy được khi mất mạng |

## 5. Vai trò

| Vai trò | Phạm vi |
|---|---|
| Chủ chuỗi | Toàn bộ chuỗi |
| Quản lý kho tổng | Nguyên liệu, nhà cung cấp, nhập hàng, sản xuất, chuyển kho |
| Quản lý chi nhánh | Chỉ chi nhánh của mình |
| Thu ngân / Pha chế | Bán hàng và pha chế tại chi nhánh của mình |

## 6. Rủi ro và việc cần làm rõ
- **Nhà cung cấp hoá đơn điện tử** (MISA meInvoice, Viettel S-Invoice, VNPT, BKAV…) chưa được chọn. Trong lúc chờ, code dùng một lớp kết nối chung kèm nhà cung cấp giả để test; khi đã chọn thì chỉ cần thêm phần kết nối thật. Cần tài khoản thử nghiệm (sandbox) của nhà cung cấp.
- **Thuế:** mã số thuế, ký hiệu hoá đơn và thuế suất của từng chi nhánh, và giá bán đã gồm thuế hay chưa, cần kế toán xác nhận.
- **Mạng:** mỗi chi nhánh cần 4G dự phòng. Chế độ chạy khi mất mạng để ở giai đoạn 4.
- **Xác nhận chuyển khoản:** giai đoạn 1 thu ngân bấm "Đã nhận tiền" bằng tay. Tự động xác nhận (PayOS, SePay, Casso) để sau.

## 7. Test case
- Giai đoạn 0 – Website: [`docs/test-cases/phase-0-website.md`](test-cases/phase-0-website.md)
- Giai đoạn 1 – Nền tảng chuỗi và bán hàng: [`docs/test-cases/phase-1-pos.md`](test-cases/phase-1-pos.md)
