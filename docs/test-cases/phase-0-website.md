# Test case – Giai đoạn 0: Website giới thiệu

Mỗi test trình duyệt (Playwright) chạy ở 2 khổ màn hình: điện thoại 375×812 và máy tính 1280×800.
So với bản đã duyệt trước đây: menu lấy từ API thay vì file `data/menu.ts`, và form liên hệ được lưu vào hệ thống thay vì chỉ hiện thông báo. Các case thay đổi được đánh dấu ✱.

## A. Phần chung
| ID | Kiểm tra | Kết quả mong đợi |
|---|---|---|
| C-01 | Mở `/`, `/menu`, `/lien-he` | Trang tải được, không có lỗi trong console |
| C-02 | Mở đường dẫn không tồn tại | Trang 404 bằng tiếng Việt, có nút về trang chủ |
| C-03 | Ngôn ngữ trang | `<html lang="vi">` |
| C-04 | Tiêu đề tab | Mỗi trang có tiêu đề riêng chứa "Mộc" |
| C-05 | Header trên máy tính | Logo và 3 liên kết, bấm vào đi đúng trang |
| C-06 | Header trên điện thoại | Chỉ có nút ☰. Bấm thì mở menu, chọn liên kết thì chuyển trang và menu tự đóng |
| C-07 | Trang đang xem | Liên kết của trang hiện tại được đánh dấu (`aria-current="page"`) |
| C-08 ✱ | Footer | Hiện **danh sách 5 chi nhánh** kèm địa chỉ, giờ mở cửa và SĐT dạng `tel:` |
| C-09 | Tràn ngang | Không trang nào kéo ngang được ở khổ 375px |

## B. Trang chủ
| ID | Kiểm tra | Kết quả mong đợi |
|---|---|---|
| H-01 | Hero | Có tiêu đề "Mộc" và câu slogan |
| H-02 | Nút "Xem menu" | Chuyển sang `/menu` |
| H-03 | Nút "Ghé quán" | Chuyển sang `/lien-he` |
| H-04 | Giới thiệu | Có tiêu đề "Về Mộc", đoạn văn và ảnh có alt |
| H-05 ✱ | Món nổi bật | Hiện đúng các món mà **API đánh dấu nổi bật** (3–4 món) |
| H-06 | Thẻ món | Có tên, mô tả, giá dạng `xx.000đ` |
| H-07 ✱ | Dải thông tin | Hiện giờ mở cửa và số chi nhánh, có liên kết sang trang Liên hệ |

## C. Trang Menu
| ID | Kiểm tra | Kết quả mong đợi |
|---|---|---|
| M-01 ✱ | Nhóm món | Đúng các nhóm và thứ tự mà API trả về |
| M-02 ✱ | Số món | Bằng số món **đang bán** trong API. Món đã ẩn không hiện |
| M-03 | Nội dung món | Có tên, mô tả, giá. Món có nhiều size thì hiện "từ xx.000đ" |
| M-04 | Thanh chọn nhóm | Bấm "Trà" thì trang cuộn đến nhóm Trà |
| M-05 | Thanh chọn nhóm khi cuộn trên điện thoại | Dính ở đầu màn hình |
| M-06 | Bố cục | Điện thoại 1 cột, máy tính từ 2 cột trở lên |
| M-07 ✱ | Cập nhật giá | Đổi giá ở trang quản trị → trang `/menu` hiện giá mới trong vòng 60 giây |
| M-08 ✱ | API lỗi | Trang vẫn hiện bản menu gần nhất đã lưu, không bị trắng trang |

## D. Trang Liên hệ
| ID | Kiểm tra | Kết quả mong đợi |
|---|---|---|
| L-01 ✱ | Thông tin | Có 5 chi nhánh, mỗi chi nhánh có địa chỉ, giờ mở cửa và SĐT dạng `tel:` |
| L-02 ✱ | Bản đồ | Chọn một chi nhánh thì Google Maps chuyển sang địa chỉ đó. Khung bản đồ có `title` và chỉ tải khi cuộn tới |
| L-03 | Bản đồ trên điện thoại | Rộng bằng màn hình, không tràn |
| L-04 | Nhãn ô nhập | Có nhãn Họ tên, SĐT/Email, Lời nhắn |
| L-05 | Gửi form trống | Không gửi được, mỗi ô bắt buộc hiện lỗi tiếng Việt |
| L-06 | Họ tên 1 ký tự | "Họ tên tối thiểu 2 ký tự" |
| L-07 | SĐT/Email sai (`abc`) | "SĐT hoặc email không hợp lệ" |
| L-08 | SĐT `0901234567` | Được chấp nhận |
| L-09 | Email `a@b.vn` | Được chấp nhận |
| L-10 | Lời nhắn dưới 10 ký tự | Báo lỗi |
| L-11 ✱ | Gửi form hợp lệ | **Được lưu vào hệ thống** và hiện trong hộp thư của trang quản trị. Hiện "Cảm ơn bạn, Mộc sẽ liên hệ sớm!" và xoá trắng form |
| L-12 | Sửa ô đang lỗi | Lỗi của ô đó biến mất |
| L-13 ✱ | Gửi quá nhiều lần | Gửi quá 5 lần/phút từ một địa chỉ IP thì bị chặn kèm thông báo tiếng Việt |
| L-14 ✱ | API lỗi khi gửi | Báo "Gửi chưa thành công, vui lòng thử lại" và giữ nguyên nội dung đã nhập |

## E. Test hàm nhỏ (Vitest)
| ID | Hàm | Trường hợp |
|---|---|---|
| U-01 | `formatPrice` | `35000` → `35.000đ`, `0` → `0đ`, `1250000` → `1.250.000đ` |
| U-02 | `validateContact` | Dữ liệu hợp lệ → không lỗi |
| U-03 | `validateContact` | Các lỗi giống L-05 đến L-10, kể cả chuỗi chỉ toàn khoảng trắng |
| U-04 | `validateContact` | Chấp nhận `+84 90 123 4567` |

## F. Chất lượng
| ID | Kiểm tra | Kết quả mong đợi |
|---|---|---|
| Q-01 | `npm run lint` | Không lỗi |
| Q-02 | `npm run build` | Build thành công |
| Q-03 | Ảnh | Mọi `<img>` có alt |
| Q-04 | Độ tương phản | Đạt chuẩn WCAG AA (≥ 4,5:1) |
| Q-05 | Bàn phím | Tab đi qua được header, nút và form, luôn thấy rõ đang ở đâu |
