# Mộc POS

Hệ thống bán hàng (POS) cho chuỗi cà phê **Mộc**: 5 chi nhánh và 1 kho tổng, bán trên máy tính bảng iPad/Android,
quản trị trên máy tính, **tự phát hành hoá đơn điện tử** (khách quét mã QR để nhận hoá đơn, không in giấy).
Không có website giới thiệu.

| Phần | Công nghệ |
|---|---|
| Giao diện (`web/`) | Next.js + Tailwind CSS, cài lên máy tính bảng như ứng dụng (PWA) |
| Máy chủ (`api/`) | Go + Gin, một chương trình chạy hai chế độ: API và worker |
| Cơ sở dữ liệu | PostgreSQL 16 |
| Triển khai | Docker Compose trên Oracle Cloud Always Free + Cloudflare Tunnel, sao lưu lên Cloudflare R2 |

## Tài liệu

| Tài liệu | Nội dung |
|---|---|
| [`docs/PLAN.md`](docs/PLAN.md) | Kế hoạch và các quyết định đã duyệt |
| [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) | Kiến trúc hệ thống |
| [`docs/SCREENS.md`](docs/SCREENS.md) | Danh sách 37 màn hình |
| [`docs/test-cases/phase-1-pos.md`](docs/test-cases/phase-1-pos.md) | Test case giai đoạn 1 |
| [`docs/DEV.md`](docs/DEV.md) | Hướng dẫn cho lập trình viên: cấu trúc mã, lệnh build/test, quy ước |
| [`docs/DEPLOY.md`](docs/DEPLOY.md) | **Hướng dẫn triển khai từng bước cho chủ chuỗi**, cập nhật, sao lưu và khôi phục |
| [`load/README.md`](load/README.md) | Kiểm thử tải 50 máy POS (k6) |

## Chạy thử trên máy lập trình

```bash
api/scripts/test-db.sh start                         # PostgreSQL tạm cho test (cổng 54329)
export TEST_DATABASE_URL="postgres://postgres@localhost:54329/postgres?sslmode=disable"
cd api && go test ./...                              # test Go
cd web && npm ci && npm test                         # test giao diện
cd web && npx playwright test                        # test toàn luồng (tự dựng API + dữ liệu mẫu)
```
Chi tiết: [`docs/DEV.md`](docs/DEV.md). CI chạy trên GitHub Actions (`.github/workflows/ci.yml`).

## Giấy phép

MIT – xem [`LICENSE`](LICENSE).
