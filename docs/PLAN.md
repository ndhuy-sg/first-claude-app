# Kế hoạch hệ thống "Mộc" – POS chuỗi 5 chi nhánh

Tài liệu này ghi lại các quyết định **đã được chủ dự án duyệt**. Mọi thay đổi về phạm vi cần cập nhật tại đây trước khi viết code.

Quy trình làm việc: **lập kế hoạch → thiết kế UI/UX và kiến trúc → viết test case → chủ dự án duyệt → viết code test → viết code cho đến khi test qua.**

- Thiết kế UI/UX và sơ đồ kiến trúc (canvas): https://claude.ai/artifact/Ni5jcUueZ2vxKKy1vD6rws
- Kiến trúc chi tiết: [`docs/ARCHITECTURE.md`](ARCHITECTURE.md)
- Danh sách 37 màn hình: [`docs/SCREENS.md`](SCREENS.md)

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
| Triển khai | **Tự dựng trên máy chủ miễn phí** Oracle Cloud Always Free (vùng Singapore, 2 nhân ARM, 12 GB RAM) bằng Docker Compose (Next.js, API Go, worker Go, PostgreSQL). Cloudflare gói Free lo tên miền, HTTPS và Tunnel. Sao lưu hằng đêm lên Cloudflare R2 (10 GB miễn phí). Chi phí duy nhất là tên miền, khoảng 300.000đ/năm |
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
| 11 | Duyệt cả 6 giả định trong bộ test case: giảm giá tối đa 20% (vượt mức phải có PIN quản lý); tự khoá sau 15 phút; sai mật khẩu 5 lần thì khoá 15 phút; giá đã gồm thuế; được trả bằng cả tiền mặt và QR; lệch tiền ca trên 50.000đ phải ghi chú; khách nhận hoá đơn qua QR hoặc email |
| 12 | **Không dùng kế toán.** Mọi thống kê và kiểm toán do hệ thống tự làm. Chủ chuỗi tự nhập thông tin thuế của từng chi nhánh (mã số thuế, ký hiệu hoá đơn, thuế suất) trong trang quản trị. Hệ thống hỗ trợ cả hai trường hợp: 5 chi nhánh chung một mã số thuế, hoặc mỗi chi nhánh một mã riêng |
| 13 | **Máy chủ miễn phí, tự dựng**: Oracle Cloud Always Free + Cloudflare + Cloudflare R2 (xem mục 2) |

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
  - **Cách xử lý đã chốt:** phần gửi dữ liệu tới cơ quan thuế có công tắc trong trang quản trị, **mặc định là TẮT**. Khi tắt, hệ thống vẫn cấp số, lưu và cho khách tra cứu hoá đơn đầy đủ, chỉ không gửi đi. Chủ chuỗi bật công tắc sau khi đã đăng ký với cơ quan thuế.
  - **Chủ chuỗi cần lưu ý:** trong thời gian công tắc còn tắt, hoá đơn chỉ là chứng từ nội bộ và **chưa có giá trị pháp lý**. Việc đăng ký sử dụng hoá đơn điện tử với cơ quan thuế (và chứng thư số, nếu được yêu cầu) là thủ tục chủ doanh nghiệp phải tự làm. Hệ thống không làm thay được.
- **iPad và Safari:** POS chạy như ứng dụng web nên không cần đưa lên App Store. Cần kiểm tra trên Safari thật: giữ màn hình luôn sáng, âm báo, bàn phím ảo che ô nhập.
- **Máy chủ miễn phí:**
  - Oracle có thể đổi điều khoản (tháng 6/2026 đã giảm một nửa hạn mức). Cách phòng: toàn bộ chạy bằng Docker Compose và được sao lưu hằng đêm, nên có thể chuyển sang máy chủ thuê (khoảng 100–200 nghìn đồng/tháng) trong khoảng 1 giờ.
  - Oracle có thể thu hồi máy "nhàn rỗi". Cách phòng: nâng tài khoản lên Pay-As-You-Go (vẫn 0đ trong hạn mức miễn phí) và đặt cảnh báo chi phí. Cần kiểm tra lại điều này trên trang của Oracle khi tạo tài khoản.
  - Vùng Singapore có thể hết máy ARM. Khi đó thử lại vào giờ khác.
  - Không có cam kết thời gian hoạt động, nên cần trang theo dõi và cảnh báo khi máy chủ ngừng.
  - Cần thẻ ngân hàng để xác minh tài khoản Oracle và Cloudflare.
- **Mạng:** mỗi chi nhánh cần 4G dự phòng. Chế độ chạy khi mất mạng để ở giai đoạn 4.
- **Xác nhận chuyển khoản:** giai đoạn 1 thu ngân bấm "Đã nhận tiền" bằng tay.

## 7. Test case
- Giai đoạn 1 – Nền tảng chuỗi và bán hàng: [`docs/test-cases/phase-1-pos.md`](test-cases/phase-1-pos.md)
