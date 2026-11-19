# Kế hoạch hệ thống "Mộc" – POS chuỗi 5 chi nhánh

Tài liệu này ghi lại các quyết định **đã được chủ dự án duyệt**. Mọi thay đổi về phạm vi cần cập nhật tại đây trước khi viết code.

Quy trình làm việc: **lập kế hoạch → thiết kế UI/UX và kiến trúc → viết test case → chủ dự án duyệt → viết code test → viết code cho đến khi test qua.**

- Thiết kế UI/UX và sơ đồ kiến trúc (canvas): https://claude.ai/artifact/Ni5jcUueZ2vxKKy1vD6rws
- Kiến trúc chi tiết: [`docs/ARCHITECTURE.md`](ARCHITECTURE.md)

## 1. Phạm vi

| Khu vực | Đường dẫn | Người dùng | Thiết bị |
|---|---|---|---|
| Bán hàng (POS) | `/pos` | Thu ngân, pha chế, quản lý chi nhánh | **Máy tính bảng iPad hoặc Android, nằm ngang** |
| Quản trị | `/admin` | Chủ chuỗi, quản lý kho tổng, quản lý chi nhánh | Máy tính |

Quy mô: **5 chi nhánh** và **1 kho tổng** quản lý nguyên vật liệu.
**Không làm website giới thiệu** (giai đoạn 0 cũ đã bị huỷ).

## 2. Công nghệ

| Phần | Lựa chọn |
|---|---|
| Giao diện | Next.js (App Router, TypeScript) + Tailwind CSS. POS là ứng dụng web cài lên màn hình chính (PWA), chạy trên Safari (iPad) và Chrome (Android) |
| Máy chủ (API) | **Go + Gin**. Đã so sánh với Fiber: Fiber nhanh hơn khoảng 30–40% khi đo framework đơn thuần, nhưng Gin phổ biến gấp đôi (89,5 nghìn so với 40,3 nghìn sao GitHub) và tương thích thư viện chuẩn của Go. Ở quy mô 5 chi nhánh, khác biệt tốc độ không đáng kể |
| Cơ sở dữ liệu | PostgreSQL. Kết nối bằng `pgx`, truy vấn bằng `sqlc`, thay đổi cấu trúc bảng bằng `goose` |
| Đăng nhập | Phiên đăng nhập lưu trong PostgreSQL, gửi qua cookie `httpOnly`. Mật khẩu và PIN mã hoá bcrypt |
| Cập nhật tức thì | Server-Sent Events |
| Triển khai | Docker Compose trên một máy chủ cloud (Caddy, Next.js, API Go, PostgreSQL). Sao lưu cơ sở dữ liệu mỗi ngày ra nơi khác |
| Test | Go: `go test` và testcontainers-go. Giao diện: Vitest và Playwright (Chromium và WebKit) |

## 3. Quyết định nghiệp vụ đã chốt

| # | Quyết định |
|---|---|
| 1 | Framework phía máy chủ: **Gin** |
| 2 | **Menu và giá giống nhau ở cả 5 chi nhánh.** Chi nhánh chỉ được bật/tắt "hết món" |
| 3 | Chi nhánh **được tự mua một số nguyên liệu tại chỗ** (phiếu nhập trực tiếp vào kho chi nhánh) |
| 4 | Kho tổng **pha chế bán thành phẩm** (cốt cà phê, syrup…) bằng phiếu sản xuất |
| 5 | **Quản lý hạn sử dụng theo lô**: lô hết hạn sớm hơn thì xuất trước |
| 6 | Khi hết nguyên liệu trên hệ thống: **vẫn cho bán, cho phép tồn âm** và cảnh báo quản lý |
| 7 | **Hoá đơn điện tử, không in giấy.** Khách nhận hoá đơn bằng mã QR tra cứu trên màn hình, hoặc qua email |
| 8 | **Hệ thống tự phát hành hoá đơn điện tử, không dùng nhà cung cấp hoá đơn.** Hệ thống tự cấp số, dựng dữ liệu hoá đơn, có trang tra cứu riêng và tự gửi dữ liệu tới cơ quan thuế |
| 9 | Thiết bị bán hàng: **iPad hoặc máy tính bảng Android** |
| 10 | **Làm thẳng POS, không làm website giới thiệu** |

## 4. Các giai đoạn

| Giai đoạn | Nội dung |
|---|---|
| **1 – Nền tảng chuỗi và bán hàng** | Chi nhánh, phân quyền theo chi nhánh, kích hoạt máy POS, menu chung (size, tuỳ chọn, hết món), bàn, màn hình bán hàng, giảm giá thủ công, thanh toán tiền mặt và VietQR (xác nhận bằng tay), **tự phát hành hoá đơn điện tử**, ca làm, báo cáo bán hàng theo chi nhánh và toàn chuỗi, nhật ký thao tác |
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
- **Tự phát hành hoá đơn điện tử (quan trọng nhất):** dù hệ thống tự làm toàn bộ việc cấp số và dựng hoá đơn, dữ liệu hoá đơn **vẫn phải được chuyển tới cơ quan thuế**.
  - Muốn gửi trực tiếp, doanh nghiệp phải đăng ký và được cơ quan thuế chấp thuận, đồng thời hệ thống phải đáp ứng yêu cầu kỹ thuật của họ.
  - Nếu chưa được chấp thuận, vẫn phải gửi qua một đơn vị truyền nhận dữ liệu (T-VAN). Đây chỉ là bên chuyển dữ liệu, không phải nhà cung cấp phần mềm hoá đơn.
  - Phần mềm được thiết kế để đổi giữa hai cách gửi mà không ảnh hưởng phần còn lại.
  - **Cần kế toán hoặc cơ quan thuế xác nhận trước khi làm phần gửi dữ liệu:** quán thuộc diện hoá đơn nào, định dạng dữ liệu, có cần chữ ký số không. Nếu cần chữ ký số, phải có chứng thư số dùng được trên máy chủ (ví dụ ký số từ xa).
- **Thuế:** mã số thuế, ký hiệu hoá đơn và thuế suất của từng chi nhánh, và giá bán đã gồm thuế hay chưa, cần kế toán xác nhận.
- **iPad và Safari:** POS chạy như ứng dụng web nên không cần đưa lên App Store. Cần kiểm tra trên Safari thật: giữ màn hình luôn sáng, âm báo, bàn phím ảo che ô nhập.
- **Mạng:** mỗi chi nhánh cần 4G dự phòng. Chế độ chạy khi mất mạng để ở giai đoạn 4.
- **Xác nhận chuyển khoản:** giai đoạn 1 thu ngân bấm "Đã nhận tiền" bằng tay.

## 7. Test case
- Giai đoạn 1 – Nền tảng chuỗi và bán hàng: [`docs/test-cases/phase-1-pos.md`](test-cases/phase-1-pos.md)
