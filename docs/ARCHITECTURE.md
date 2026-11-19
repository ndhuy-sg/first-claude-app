# Kiến trúc hệ thống Mộc POS

Bản vẽ trực quan của tài liệu này nằm ở trang "Kiến trúc hệ thống" trên canvas thiết kế: https://claude.ai/artifact/Ni5jcUueZ2vxKKy1vD6rws

## 1. Tổng quan

```
Máy tính bảng POS (iPad/Android, PWA) ─┐
Máy tính quản lý (/admin) ─────────────┤ HTTPS (+ SSE)
Khách quét QR tra cứu hoá đơn ─────────┘
                                        ▼
        Cloudflare (gói Free): tên miền, HTTPS, chống tấn công
                                        │ Cloudflare Tunnel (máy chủ không mở cổng ra Internet)
                                        ▼
   ┌──────── Máy chủ Oracle Cloud Always Free · Singapore · Docker Compose ────────┐
   │  cloudflared ── /       → web (Next.js, chỉ giao diện)                        │
   │              └─ /api/*  → api (Go + Gin) ──► PostgreSQL ◄── worker (Go)       │
   └───────────────────────────────────────────────────────────────────────────────┘
          PostgreSQL ──► sao lưu hằng đêm lên Cloudflare R2
          worker     ──► cơ quan thuế (chỉ khi chủ chuỗi bật công tắc gửi)
```

Toàn bộ chạy bằng Docker Compose trên **một máy chủ miễn phí** (Oracle Cloud Always Free) phục vụ cả 5 chi nhánh và kho tổng. Bảng so sánh các phương án máy chủ miễn phí nằm trên canvas, trong khung "Phương án chốt". API và worker là **cùng một chương trình Go**, chạy ở hai chế độ.

## 2. Nguyên tắc
1. **Go là nơi duy nhất quyết định** giá, phân quyền, trạng thái đơn và tồn kho. Next.js chỉ hiển thị và gọi API.
2. **Tách dữ liệu theo chi nhánh:** mọi bảng nghiệp vụ có `branch_id`. Middleware lấy chi nhánh từ phiên đăng nhập, không bao giờ lấy từ dữ liệu gửi lên. Xem dữ liệu chi nhánh khác thì trả 404.
3. **Tiền là số nguyên VND** (`BIGINT`). Thời gian lưu `timestamptz` (UTC), báo cáo tính theo giờ `Asia/Ho_Chi_Minh`.
4. **Đơn đã thanh toán không sửa được.** Muốn huỷ thì làm thao tác hoàn tiền, có lý do và nhật ký.
5. **Chỉ thêm, không sửa:** `stock_movements` (sổ nhập xuất) và `audit_logs` (nhật ký) không có lệnh UPDATE hay DELETE.
6. **Không mất và không trùng:**
   - Thanh toán, cấp số hoá đơn và đưa việc gửi hoá đơn vào hàng chờ nằm trong **cùng một giao dịch cơ sở dữ liệu**.
   - Mọi yêu cầu ghi dữ liệu từ POS kèm `Idempotency-Key`, nên mạng gửi lại một yêu cầu cũng không ghi trùng.

## 3. Phía giao diện (`web/`)
- **Next.js App Router + Tailwind.** Hai khu vực:
  - `/pos`: tối ưu cho màn hình cảm ứng nằm ngang, khổ 1180×820 (iPad) và 1280×800 (Android).
  - `/admin`: dùng trên máy tính, vẫn hiển thị được trên màn hình hẹp.
- **PWA:** cài lên màn hình chính, mở toàn màn hình. Dùng Wake Lock để giữ màn hình sáng trong ca.
- **Gọi API:** code gọi API bằng TypeScript được tự sinh từ `api/openapi.yaml`. Dùng TanStack Query để lưu tạm dữ liệu và tự thử lại khi lỗi.
- **Cập nhật tức thì:** một kết nối SSE cho mỗi máy POS, dùng cho các việc: hết món, khoá nhân viên, phiếu chuyển kho đến.
- **Mất mạng:** hiện thanh báo "Mất kết nối" và khoá nút thanh toán. Chưa lưu đơn khi mất mạng; việc này để giai đoạn 4.
- **Bảng màu:**

  | Màu | Mã |
  |---|---|
  | Xanh navy (thanh trên cùng, menu bên, chữ chính, mục đang chọn) | `#14213D` |
  | Vàng (nút chính, logo) | `#F5B700`, chữ trên nút vàng dùng navy |
  | Trắng (thẻ, ô nhập) | `#FFFFFF` |
  | Nền trang | `#F5F7FA` |
  | Chữ phụ | `#5B6478` |
  | Viền | `#DDE3EC` |
  | Thành công | `#2F6B4F` |
  | Lỗi | `#A23B2A` |

  Font Be Vietnam Pro; logo dùng Lora. Mọi nút cao từ 44px trở lên.

## 4. Phía máy chủ (`api/`)
```
api/
  cmd/mocpos/main.go        ← chạy ở chế độ "serve" (API) hoặc "worker"
  internal/
    http/                   ← định tuyến Gin, middleware: đăng nhập, phân quyền, chi nhánh, chống trùng, log
    domain/                 ← quy tắc thuần Go: tính tiền, tách thuế, VietQR, phân quyền (test không cần cơ sở dữ liệu)
    service/                ← nghiệp vụ: menu, đơn, thanh toán, ca, hoá đơn, báo cáo, kho
    store/                  ← code do sqlc sinh ra từ các file .sql
    invoice/                ← dựng dữ liệu hoá đơn, cấp số, trang tra cứu
    taxgateway/             ← cách gửi tới cơ quan thuế: Off (mặc định) | Direct | TVAN | Fake (để test)
    worker/                 ← lấy việc từ bảng jobs (FOR UPDATE SKIP LOCKED), tự thử lại, lần sau chờ lâu hơn lần trước
  migrations/               ← goose
  openapi.yaml
```

**Các nhóm API chính** (đều nằm dưới `/api/v1`):

| Nhóm | Ví dụ |
|---|---|
| Đăng nhập | `POST /auth/login`, `POST /auth/pin`, `POST /devices/activate`, `POST /auth/logout` |
| Menu | `GET /menu`, `PUT /branches/:id/sold-out/:productId` |
| Bàn và đơn | `GET /tables`, `POST /orders`, `POST /orders/:id/items`, `POST /orders/:id/discount`, `POST /orders/:id/move`, `POST /orders/:id/merge`, `POST /orders/:id/split` |
| Thanh toán | `POST /orders/:id/payments`, `POST /payments/:id/confirm`, `POST /orders/:id/refund` |
| Hoá đơn | `GET /invoices`, `POST /invoices/:id/resend`, `GET /tra-cuu/:code` (trang tra cứu công khai cho khách) |
| Ca làm | `POST /shifts/open`, `POST /shifts/:id/close` |
| Báo cáo | `GET /reports/sales?branch=&from=&to=&groupBy=` |
| Kho (giai đoạn 2) | `/ingredients`, `/receipts`, `/transfers`, `/productions`, `/stocktakes`, `/stock` |
| Thời gian thực | `GET /events` (SSE) |

## 5. Hoá đơn điện tử tự phát hành
1. Thanh toán thành công. Trong cùng một giao dịch cơ sở dữ liệu, hệ thống:
   - lấy số hoá đơn tiếp theo trong `invoice_series` của chi nhánh, khoá dòng đó bằng `SELECT … FOR UPDATE` để số liên tục và không trùng;
   - tạo `invoices` và `invoice_lines`;
   - thêm một việc vào `jobs`.
2. POS hiện ngay mã QR trỏ tới `/tra-cuu/:code`. Khách xem được hoá đơn ngay, không phải chờ.
3. Nếu công tắc gửi đang **bật**, worker dựng dữ liệu hoá đơn theo định dạng của cơ quan thuế và gửi qua `taxgateway`. Nếu đang **tắt** (mặc định), hoá đơn được lưu ở trạng thái "chưa gửi" và sẽ được gửi khi công tắc bật. Kết quả mỗi lần gửi lưu vào `tax_submissions`.
4. Nếu gửi lỗi, worker tự thử lại sau 1, 5, 15, 60 phút… Sau 10 lần vẫn lỗi thì chuyển trạng thái "thất bại" và báo trên trang quản trị, có nút "Gửi lại".
5. Hoàn tiền thì tạo hoá đơn huỷ hoặc điều chỉnh theo cùng luồng trên.

> Chủ chuỗi tự nhập thông tin thuế của từng chi nhánh trong trang quản trị. Công tắc gửi chỉ nên bật sau khi đã đăng ký với cơ quan thuế (xem `docs/PLAN.md` mục 6).

## 6. Mô hình dữ liệu

| Nhóm | Bảng | Giai đoạn |
|---|---|---|
| Tổ chức | `branches`, `warehouses`, `users`, `devices`, `sessions` | 1 |
| Menu | `categories`, `products`, `product_sizes`, `modifiers`, `branch_sold_out` | 1 |
| Bán hàng | `areas`, `tables`, `shifts`, `orders`, `order_items`, `payments`, `refunds` | 1 |
| Hoá đơn | `invoice_series`, `invoices`, `invoice_lines`, `tax_submissions` | 1 |
| Kho | `ingredients`, `unit_conversions`, `lots`, `stock_balances`, `stock_movements`, `receipts`, `transfers`, `productions`, `recipes`, `stocktakes`, `wastes` | 2 |
| Hệ thống | `audit_logs`, `jobs`, `idempotency_keys` | 1 |

**Tồn kho (giai đoạn 2):**
- `stock_balances` lưu tồn theo từng (kho, nguyên liệu, lô) và được phép âm.
- Mỗi thay đổi tồn kho thêm một dòng vào `stock_movements` **trong cùng giao dịch** với việc cập nhật `stock_balances`.
- Khi bán: trừ theo công thức món, lấy lô có hạn sử dụng gần nhất trước. Không đủ thì để tồn âm và tạo cảnh báo.

## 7. Bảo mật
- Cookie `HttpOnly`, `Secure`, `SameSite=Lax`, kèm kiểm tra `Origin` với mọi yêu cầu ghi dữ liệu.
- Sai mật khẩu 5 lần thì khoá 15 phút. POS tự khoá sau 15 phút không thao tác.
- Mọi truy vấn đều dùng tham số (sqlc), không ghép chuỗi SQL.

## 8. Triển khai và vận hành
- **Máy chủ:** Oracle Cloud Always Free, vùng Singapore, máy ARM 2 nhân, 12 GB RAM, 200 GB ổ đĩa, chạy Ubuntu.
  - Nâng tài khoản lên Pay-As-You-Go để tránh bị thu hồi máy "nhàn rỗi", và đặt cảnh báo chi phí.
- **Docker Compose** gồm `cloudflared`, `web`, `api`, `worker`, `postgres`. Mọi image đều build được cho ARM64.
- **Cloudflare gói Free:** quản lý tên miền, HTTPS và tường lửa. `cloudflared` chia đường dẫn: `/` vào `web`, `/api/*` vào `api`. Máy chủ chỉ mở cổng SSH cho quản trị, chỉ cho phép từ IP được chỉ định.
- **Sao lưu:** mỗi đêm sao lưu đầy đủ, đồng thời ghi liên tục nhật ký thay đổi của PostgreSQL (WAL) lên Cloudflare R2. Mọi bản sao lưu đều được mã hoá và giữ 30 ngày. Mỗi tháng thử khôi phục một lần.
  - Nếu máy chủ hỏng, mất tối đa khoảng 5 phút dữ liệu.
  - Dựng lại hệ thống trên máy mới mất khoảng 1 giờ.
- **Chuyển máy chủ khi cần:** dựng Docker Compose trên máy mới, khôi phục bản sao lưu mới nhất, chuyển tunnel sang máy mới. Mục tiêu dưới 1 giờ.
- **Giám sát:** log có cấu trúc (`slog`), endpoint `/healthz`, dịch vụ theo dõi miễn phí gọi vào định kỳ và cảnh báo khi máy chủ ngừng hoặc hàng chờ hoá đơn bị dồn.

## 9. Lớp HTTP (middleware)
Thứ tự xử lý một yêu cầu: gắn mã yêu cầu → ghi log → bắt lỗi (không để máy chủ sập) → kiểm tra phiên đăng nhập → kiểm tra máy POS → lọc theo chi nhánh → phân quyền theo vai trò → chống ghi trùng (`Idempotency-Key`) → kiểm tra `Origin` → handler.

## 10. Trạng thái
| Đối tượng | Trạng thái |
|---|---|
| Đơn hàng | Đang mở → Đã thanh toán → Đã hoàn tiền; hoặc Đang mở → Đã huỷ / Đã gộp |
| Thanh toán QR | Chờ khách chuyển → Đã nhận tiền; hoặc Đã huỷ |
| Hoá đơn | Đã cấp số → Chưa gửi (công tắc tắt) → Đang gửi → Đã gửi cơ quan thuế; nếu lỗi: thử lại → thất bại; hoàn tiền: huỷ / điều chỉnh |
| Phiếu chuyển kho | Yêu cầu → Đã duyệt → Đã xuất (trừ kho tổng) → Đã nhận (cộng kho chi nhánh); hoặc Từ chối |
| Ca làm | Đang mở → Đã chốt |
| Kiểm kê | Nháp → Đã gửi → Tự duyệt (lệch dưới 5%) hoặc Chờ duyệt → Đã duyệt |

**Thanh toán một đơn nằm trong một giao dịch duy nhất**, gồm các bước theo thứ tự:
1. Khoá đơn.
2. Tính lại giá.
3. Ghi khoản thanh toán, có kiểm tra chống trùng.
4. Đổi trạng thái đơn.
5. Cấp số hoá đơn (khoá dãy số của chi nhánh).
6. Tạo hoá đơn.
7. Trừ kho theo công thức (giai đoạn 2).
8. Ghi nhật ký.
9. Thêm việc "gửi hoá đơn" vào hàng chờ.

## 11. Phân quyền
Ma trận đầy đủ nằm ở khung "Ma trận phân quyền" trên canvas. Có 5 vai trò: chủ chuỗi, quản lý kho tổng, quản lý chi nhánh, thu ngân, pha chế. Máy chủ kiểm tra quyền ở mọi API; giao diện chỉ ẩn những nút người dùng không được bấm.

## 12. Đưa code lên máy chủ
- **GitHub Actions** (gói miễn phí) chạy lần lượt: lint, test Go, test giao diện, test toàn luồng, build image ARM64, đẩy lên GitHub Container Registry.
- **Triển khai:** chạy thay đổi cấu trúc bảng trước, rồi khởi động lại từng dịch vụ. Giữ image bản trước để quay lại khi cần.
- Không triển khai trong giờ cao điểm (06:30–09:00).

## 13. Mục tiêu hiệu năng
- 95% yêu cầu hoàn thành dưới 200 ms khi có 50 máy POS hoạt động cùng lúc.
- Mở màn hình bán hàng dưới 3 giây trên iPad.
- Báo cáo 30 ngày của toàn chuỗi hiện ra dưới 2 giây.
