# Kiến trúc hệ thống Mộc POS

Bản vẽ trực quan của tài liệu này nằm ở trang "Kiến trúc hệ thống" trên canvas thiết kế: https://claude.ai/artifact/Ni5jcUueZ2vxKKy1vD6rws

## 1. Tổng quan

```
Máy tính bảng POS (iPad/Android, PWA) ─┐
Máy tính quản lý (/admin) ─────────────┤ HTTPS (+ SSE)
                                        ▼
                    Caddy ── HTTPS tự động, chung một tên miền
                    ├── /       → Next.js (chỉ giao diện)
                    └── /api/*  → API Go + Gin ──► PostgreSQL ──► sao lưu hằng đêm ra lưu trữ ngoài
                                   │ hàng chờ (bảng jobs)
                                   ▼
                                 Worker Go ──► Cơ quan thuế (trực tiếp hoặc qua T-VAN)
```

Toàn bộ chạy bằng Docker Compose trên **một máy chủ cloud** phục vụ cả 5 chi nhánh và kho tổng. API và worker là **cùng một chương trình Go**, chạy ở hai chế độ.

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
    taxgateway/             ← cách gửi tới cơ quan thuế: Direct | TVAN | Fake (để test)
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
3. Worker dựng dữ liệu hoá đơn theo định dạng của cơ quan thuế và gửi qua `taxgateway`. Kết quả mỗi lần gửi lưu vào `tax_submissions`.
4. Nếu gửi lỗi, worker tự thử lại sau 1, 5, 15, 60 phút… Sau 10 lần vẫn lỗi thì chuyển trạng thái "thất bại" và báo trên trang quản trị, có nút "Gửi lại".
5. Hoàn tiền thì tạo hoá đơn huỷ hoặc điều chỉnh theo cùng luồng trên.

> Phải xác nhận trước khi làm bước 3: được gửi trực tiếp tới cơ quan thuế hay phải gửi qua T-VAN, định dạng dữ liệu, có cần chữ ký số không (xem `docs/PLAN.md` mục 6).

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
- Docker Compose gồm `caddy`, `web`, `api`, `worker`, `postgres`. Một máy chủ 2–4 nhân là đủ.
- Sao lưu `pg_dump` hằng đêm ra lưu trữ ngoài máy chủ, giữ 30 ngày. Mỗi tháng thử khôi phục một lần.
- Log có cấu trúc (`slog`). Có endpoint kiểm tra tình trạng máy chủ, và cảnh báo khi hàng chờ hoá đơn bị dồn.
