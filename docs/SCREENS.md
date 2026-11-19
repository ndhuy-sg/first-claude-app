# Danh sách màn hình – Mộc POS

Bản thiết kế chi tiết từng màn hình nằm trên canvas: https://claude.ai/artifact/Ni5jcUueZ2vxKKy1vD6rws

- Trang "POS (máy tính bảng)": màn hình POS.
- Trang "Khách hàng (điện thoại)": tra cứu hoá đơn.
- Trang "Quản trị": các trang quản trị.
- Trang "System Design": bản đồ màn hình, các lớp phần mềm, trạng thái, phân quyền, API, vận hành.

Tổng cộng **37 màn hình**: POS 17, khách hàng 2, quản trị 18. GĐ = giai đoạn.

## POS – máy tính bảng (iPad / Android, nằm ngang)
| # | Màn hình | GĐ | Ghi chú |
|---|---|---|---|
| 1 | Đăng nhập bằng PIN | 1 | Chọn nhân viên, PIN 4–6 số, tự khoá sau 15 phút |
| 2 | Sơ đồ bàn | 1 | Khu vực; bàn trống / có khách / chờ thanh toán; đơn mang đi; hàng chuyển kho đang đến |
| 3 | Bán hàng – gọi món | 1 | Nhóm món, lưới món, giỏ hàng, hộp chọn size / tuỳ chọn / ghi chú |
| 4 | Thanh toán | 1 | Tiền mặt (gợi ý mệnh giá, tiền thối), VietQR, kết hợp; khách lẻ hoặc hoá đơn công ty |
| 5 | Hoàn tất | 1 | QR tra cứu hoá đơn điện tử, gửi email |
| 6 | Chốt ca | 1 | Đếm tiền theo mệnh giá, chênh lệch, kiểm tra trước khi chốt |
| 7 | Kích hoạt máy POS | 1 | Quản lý đăng nhập, chọn chi nhánh, đặt tên máy |
| 8 | Mở ca | 1 | Tiền đầu ca, đối chiếu với ca trước |
| 9 | Giảm giá | 1 | Theo % hoặc số tiền, lý do; trên 20% cần PIN quản lý |
| 10 | Chuyển / gộp / tách bàn | 1 | Tách hoá đơn bằng cách chạm món |
| 11 | Đơn trong ca | 1 | Tìm, lọc, xem chi tiết, hoàn tiền (cần quản lý) |
| 12 | Báo hết món | 1 | Bật / tắt theo chi nhánh |
| 13 | Nhận hàng chuyển kho | 2 | Đếm thực nhận, ghi lý do khi thiếu |
| 14 | Yêu cầu hàng từ kho tổng | 2 | Gợi ý theo mức tối thiểu |
| 15 | Kiểm kê & báo huỷ | 2 | Nhập số đếm, xem chênh lệch |
| 16 | Mất kết nối | 1 | Thanh báo, khoá thanh toán, hướng dẫn xử lý |
| 17 | Quầy pha chế | 3 | Mới › Đang làm › Xong, cảnh báo chờ lâu |

## Khách hàng – điện thoại
| # | Màn hình | GĐ |
|---|---|---|
| 18 | Tra cứu hoá đơn – nhập mã | 1 |
| 19 | Tra cứu hoá đơn – kết quả (xem, tải PDF, gửi email) | 1 |

## Quản trị – máy tính
| # | Trang | GĐ |
|---|---|---|
| 20 | Đăng nhập | 1 |
| 21 | Tổng quan toàn chuỗi | 1 |
| 22 | Báo cáo bán hàng | 1 |
| 23 | Đơn hàng | 1 |
| 24 | Hoá đơn điện tử (danh sách, công tắc gửi, gửi lại) | 1 |
| 25 | Khuyến mãi | 3 |
| 26 | Món & giá | 1 |
| 27 | Sửa món & công thức | 1 (món) / 2 (công thức) |
| 28 | Tồn kho | 2 |
| 29 | Nhập hàng | 2 |
| 30 | Chuyển kho | 2 |
| 31 | Sản xuất bán thành phẩm | 2 |
| 32 | Kiểm kê & huỷ hàng (duyệt lệch trên 5%) | 2 |
| 33 | Nhà cung cấp | 2 |
| 34 | Chi nhánh, bàn & máy POS (thông tin thuế, VietQR) | 1 |
| 35 | Nhân viên & phân quyền | 1 |
| 36 | Nhật ký thao tác | 1 |
| 37 | Cài đặt hệ thống (hoá đơn điện tử, quy tắc bán hàng, sao lưu) | 1 |
