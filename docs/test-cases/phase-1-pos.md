# Test case – Giai đoạn 1: Nền tảng chuỗi và bán hàng

## Cách chạy

| Loại | Công cụ | Mã ID |
|---|---|---|
| Quy tắc nghiệp vụ (Go, không cần cơ sở dữ liệu) | `go test` | D |
| API (Go, chạy trên PostgreSQL thật dựng bằng testcontainers) | `go test` | A, B, O, P, E, S, R, L |
| Giao diện POS và trang quản trị | Playwright | W |
| Toàn luồng qua cả hệ thống (Docker Compose) | Playwright | F |
| Hiệu năng, bảo mật, chất lượng | k6, script, lint | N |

**Dữ liệu mẫu dùng cho test:**
- 2 chi nhánh: **CN1** và **CN2** (đủ để kiểm tra việc tách dữ liệu; khi chạy thật có 5 chi nhánh).
- Tài khoản:
  - `owner`: chủ chuỗi.
  - `ql1`: quản lý CN1.
  - `tn1`: thu ngân CN1, PIN 1234.
  - `tn2`: thu ngân CN2, PIN 5678.
- Món mẫu "Cà phê sữa":
  - Giá gốc 29.000đ.
  - Size: M +0đ, L +6.000đ.
  - Tuỳ chọn: thêm shot +8.000đ, ít đá +0đ.
- Thuế suất mẫu: 8%, giá bán đã gồm thuế.
- Nhà cung cấp hoá đơn điện tử: **giả lập**. Có thể cài cho nó trả thành công, trả lỗi hoặc không phản hồi.

Các case đánh dấu ❓ dựa trên một giả định cần bạn xác nhận (xem cuối tài liệu).

---

## D. Quy tắc tính tiền và nghiệp vụ (test Go thuần)
| ID | Kiểm tra | Kết quả mong đợi |
|---|---|---|
| D-01 | Tiền một dòng = (giá gốc + giá size + giá các tuỳ chọn) × số lượng | Cà phê sữa, size L, thêm shot, SL 2 → (29.000 + 6.000 + 8.000) × 2 = **86.000đ** |
| D-02 | Tuỳ chọn 0đ ("ít đá") | Không làm đổi giá |
| D-03 | Giảm giá theo % | 10% của 86.000 → giảm 8.600, còn **77.400đ**. Tiền giảm làm tròn xuống đến đồng |
| D-04 | Giảm giá theo số tiền lớn hơn tổng đơn | Bị từ chối, tổng đơn không bao giờ âm |
| D-05 ❓ | Giảm giá trên 20% | Hàm trả về "cần quản lý duyệt" |
| D-06 | Tiền thối | Khách đưa 100.000 cho đơn 86.000 → thối 14.000. Khách đưa ít hơn tổng đơn → báo lỗi |
| D-07 | Gợi ý mệnh giá cho đơn 86.000 | 86.000, 90.000, 100.000, 200.000, 500.000 (không trùng, tăng dần, đều ≥ tổng) |
| D-08 ❓ | Tách thuế từ giá đã gồm thuế | 86.000đ, thuế 8% → trước thuế **79.630**, thuế **6.370** (cộng lại đúng 86.000). Thuế 10% → 78.182 và 7.818 |
| D-09 | Nội dung mã VietQR | Đúng chuẩn (mã kiểm tra CRC hợp lệ), chứa đúng ngân hàng, số tài khoản, số tiền và nội dung là mã đơn |
| D-10 | Mã đơn | Dạng `CN1-20261119-0001`. Đánh số tăng dần theo chi nhánh và theo ngày (giờ Việt Nam), không trùng |
| D-11 | Tiền cuối ca theo hệ thống | = tiền đầu ca + tiền mặt thu − tiền mặt hoàn. Chênh lệch = tiền đếm được − tiền theo hệ thống |
| D-12 | Bảng phân quyền | Kiểm tra lần lượt mọi cặp (vai trò, thao tác), mỗi cặp phải đúng là được phép hoặc bị cấm như bảng phân quyền |
| D-13 | PIN | Chỉ nhận 4–6 chữ số. Không trùng với PIN khác trong cùng chi nhánh |
| D-14 | Mã số thuế người mua | Nhận `0101234567` và `0101234567-001`. Từ chối `12345` và `abcdefghij` |

## A. Đăng nhập và phân quyền (API)
| ID | Kiểm tra | Kết quả mong đợi |
|---|---|---|
| A-01 | Quản lý đăng nhập đúng email + mật khẩu | 200. Cookie có `HttpOnly`, `Secure`, `SameSite=Lax` |
| A-02 ❓ | Sai mật khẩu | 401 với thông báo chung chung (không nói sai email hay sai mật khẩu). Sai 5 lần thì khoá 15 phút |
| A-03 | Kích hoạt máy POS | Quản lý CN1 đăng nhập trên máy tính bảng và kích hoạt nó → máy được gắn vào CN1 |
| A-04 | Đăng nhập bằng PIN trên máy POS của CN1 | PIN của `tn1` → thành công. PIN của `tn2` (nhân viên CN2) → 401 |
| A-05 ❓ | Không thao tác 15 phút | Phiên PIN tự khoá, phải nhập lại PIN |
| A-06 | Khoá tài khoản nhân viên | Phiên của người đó mất hiệu lực ngay, yêu cầu tiếp theo bị 401 |
| A-07 | Đăng xuất | Cookie cũ không dùng được nữa |
| A-08 | Gọi API khi chưa đăng nhập | 401. Trừ các API công khai: menu, gửi liên hệ |
| A-09 | Thu ngân gọi API quản trị (tạo món) | 403 |
| A-10 | `ql1` xem đơn của CN2 | **404** (không để lộ việc đơn đó có tồn tại) |
| A-11 | `ql1` tạo nhân viên cho CN2 | Bị từ chối |
| A-12 | `tn1` gửi kèm mã chi nhánh giả là CN2 khi tạo đơn | Mã giả bị bỏ qua, đơn vẫn thuộc CN1 |
| A-13 | `owner` | Xem được dữ liệu cả 2 chi nhánh |
| A-14 | Mật khẩu và PIN | Lưu dạng mã hoá bcrypt. Không API nào trả về mật khẩu hay PIN đã mã hoá |
| A-15 | Gửi yêu cầu thay đổi dữ liệu từ tên miền khác | Bị từ chối (chống CSRF bằng kiểm tra `Origin`) |

## B. Chi nhánh, menu, bàn, liên hệ (API)
| ID | Kiểm tra | Kết quả mong đợi |
|---|---|---|
| B-01 | `owner` tạo chi nhánh kèm thông tin thuế (mã số thuế, ký hiệu hoá đơn, thuế suất) | Thành công. `ql1` làm việc này → 403 |
| B-02 | `owner` tạo, sửa nhóm món, món, size, tuỳ chọn | Thành công. Giá phải là số nguyên ≥ 0. Trùng tên món trong cùng nhóm → 409 |
| B-03 | Giá giống nhau ở mọi chi nhánh | API menu của CN1 và CN2 trả về giá giống hệt nhau |
| B-04 | `ql1` bật "hết món" cho Cà phê sữa | CN1 không thêm được món này vào đơn, CN2 vẫn thêm được |
| B-05 | Món bị ẩn | Không có trong menu công khai và menu POS |
| B-06 | API menu công khai | Không cần đăng nhập. Chỉ có món đang bán, nhóm theo thứ tự đã đặt. Có cờ "nổi bật" |
| B-07 | `ql1` tạo khu vực và bàn | Chỉ tạo được cho CN1 |
| B-08 | Gửi form liên hệ công khai | Được lưu lại. Bị chặn khi quá 5 lần/phút từ một IP. Dữ liệu sai → 400, thông báo lỗi tiếng Việt |

## O. Đơn hàng (API)
| ID | Kiểm tra | Kết quả mong đợi |
|---|---|---|
| O-01 | Tạo đơn tại bàn 5 | Đơn ở trạng thái "đang mở", bàn 5 chuyển sang "có khách" |
| O-02 | Tạo đơn mang đi | Đơn không gắn bàn |
| O-03 | Thêm món có size và tuỳ chọn, gửi kèm **giá giả** 1.000đ | Máy chủ bỏ qua giá giả và tự tính **43.000đ** |
| O-04 | Thêm món đang "hết món" ở chi nhánh | 409 "Món tạm hết" |
| O-05 | Thêm tuỳ chọn không thuộc món đó | 400 |
| O-06 | Đổi giá món sau khi món đã nằm trong đơn | Dòng cũ giữ giá cũ, dòng thêm sau dùng giá mới |
| O-07 | Sửa số lượng, xoá món khi chưa thanh toán | Thành công, tổng đơn tính lại đúng |
| O-08 ❓ | Giảm giá ≤ 20% | Thu ngân tự làm được, có ghi nhật ký |
| O-09 ❓ | Giảm giá > 20% | Không có PIN quản lý → 403. Có PIN quản lý hợp lệ → thành công, nhật ký ghi tên người duyệt |
| O-10 | Chuyển bàn | Sang bàn trống → thành công. Sang bàn đang có khách → 409 |
| O-11 | Gộp bàn 5 vào bàn 6 | Món của bàn 5 chuyển sang đơn bàn 6. Đơn bàn 5 đóng với trạng thái "đã gộp", bàn 5 trống lại |
| O-12 | Tách hoá đơn | Món được chọn chuyển sang đơn mới. Tổng 2 đơn bằng tổng đơn ban đầu |
| O-13 | Huỷ đơn chưa thanh toán | Bắt buộc nhập lý do, có ghi nhật ký, bàn trống lại |
| O-14 | Hai máy cùng thêm món vào một đơn cùng lúc | Không mất món nào, tổng đơn đúng |
| O-15 | Tạo đơn khi chưa mở ca | 409 "Chưa mở ca" |
| O-16 | Thanh toán đơn không có món nào | 400 |

## P. Thanh toán (API)
| ID | Kiểm tra | Kết quả mong đợi |
|---|---|---|
| P-01 | Trả tiền mặt đúng số | Đơn "đã thanh toán", tiền thối 0, bàn trống lại |
| P-02 | Trả tiền mặt nhiều hơn | Lưu đúng số khách đưa và tiền thối |
| P-03 | Trả tiền mặt ít hơn | 400 |
| P-04 | Chuyển khoản VietQR | Tạo mã QR với số tiền và mã đơn, khoản thanh toán ở trạng thái "chờ". Thu ngân bấm "Đã nhận tiền" → đơn "đã thanh toán" |
| P-05 | Huỷ QR rồi chuyển sang trả tiền mặt | QR bị huỷ, đơn thanh toán bằng tiền mặt, chỉ có 1 khoản thanh toán thành công |
| P-06 ❓ | Trả một đơn bằng 2 cách (một phần tiền mặt, một phần QR) | Thành công khi tổng các khoản bằng tổng đơn. Thiếu thì đơn chưa thanh toán xong |
| P-07 | Thêm món, giảm giá hoặc huỷ đơn đã thanh toán | 409 "Đơn đã thanh toán" |
| P-08 | Bấm thanh toán 2 lần hoặc mạng gửi lại yêu cầu (cùng mã chống trùng `Idempotency-Key`) | Chỉ ghi nhận 1 lần thanh toán |
| P-09 | Hoàn tiền cho đơn đã thanh toán | Chỉ quản lý làm được, bắt buộc nhập lý do. Tạo khoản hoàn tiền, ghi nhật ký và **tự gửi yêu cầu huỷ hoá đơn điện tử** |

## E. Hoá đơn điện tử (API, nhà cung cấp giả lập)
| ID | Kiểm tra | Kết quả mong đợi |
|---|---|---|
| E-01 | Thanh toán xong | Việc xuất hoá đơn được ghi vào hàng chờ **trong cùng giao dịch cơ sở dữ liệu** với việc thanh toán (không thể có đơn đã thanh toán mà không có yêu cầu xuất hoá đơn) |
| E-02 | Dữ liệu gửi sang nhà cung cấp | Đúng mã số thuế và ký hiệu hoá đơn **của chi nhánh bán**. Đủ từng món: tên, SL, đơn giá, thành tiền. Có thuế suất, tiền thuế, tổng tiền, cách thanh toán. Người mua mặc định "Khách lẻ" |
| E-03 | Nhà cung cấp trả thành công | Lưu số hoá đơn, mã tra cứu và đường dẫn tra cứu. Trạng thái "đã phát hành" |
| E-04 | Nhà cung cấp lỗi hoặc không phản hồi | Bán hàng vẫn bình thường. Hệ thống tự thử lại, mỗi lần chờ lâu hơn. Sau 10 lần vẫn lỗi thì chuyển "thất bại" và báo trên trang quản trị |
| E-05 | Gửi lại sau khi mạng chập chờn | Không sinh hoá đơn trùng (luôn gửi kèm cùng một mã nội bộ) |
| E-06 | Khách lấy hoá đơn công ty | Nhập mã số thuế, tên công ty, địa chỉ, email → hoá đơn có đủ thông tin người mua. Mã số thuế sai định dạng → 400 |
| E-07 | Đơn của CN2 | Hoá đơn xuất bằng mã số thuế và ký hiệu của CN2 |
| E-08 | Hoàn tiền (P-09) | Gửi yêu cầu huỷ/điều chỉnh hoá đơn tới nhà cung cấp, theo dõi trạng thái |
| E-09 | Quản trị xem danh sách hoá đơn | Lọc được theo trạng thái và chi nhánh. Bấm "Gửi lại" được với hoá đơn thất bại. `ql1` chỉ thấy hoá đơn của CN1 |

## S. Ca làm (API)
| ID | Kiểm tra | Kết quả mong đợi |
|---|---|---|
| S-01 | Mở ca với tiền đầu ca | Mỗi máy POS chỉ có 1 ca đang mở. Mở ca thứ 2 → 409 |
| S-02 | Chốt ca, nhập tiền đếm được | Hệ thống tính tiền lẽ ra phải có, chênh lệch, tổng tiền mặt, tổng QR và số đơn |
| S-03 | Chốt ca khi còn đơn chưa thanh toán | 409, kèm danh sách các đơn còn mở |
| S-04 ❓ | Chênh lệch tiền lớn hơn 50.000đ | Bắt buộc nhập ghi chú |
| S-05 | Đơn trong ca | Mỗi đơn đã thanh toán thuộc đúng ca đang mở lúc thanh toán |

## R. Báo cáo bán hàng (API)
| ID | Kiểm tra | Kết quả mong đợi |
|---|---|---|
| R-01 | Doanh thu theo ngày của CN1 | Chỉ tính đơn đã thanh toán. Không tính đơn đã huỷ hoặc đã hoàn tiền |
| R-02 | Toàn chuỗi | Bằng tổng các chi nhánh |
| R-03 | Quyền xem | `ql1` chỉ xem được CN1. `owner` xem được tất cả và so sánh giữa các chi nhánh |
| R-04 | Theo món | Đúng số lượng và doanh thu từng món |
| R-05 | Ranh giới ngày | Đơn thanh toán lúc 23:30 ngày 19/11 (giờ Việt Nam) tính vào ngày 19/11. Đơn lúc 00:30 tính vào ngày 20/11 |
| R-06 | Theo khung giờ, theo cách thanh toán, theo nhân viên và ca | Số liệu khớp với dữ liệu mẫu |

## L. Nhật ký thao tác (API)
| ID | Kiểm tra | Kết quả mong đợi |
|---|---|---|
| L-01 | Giảm giá, huỷ đơn, hoàn tiền, đổi giá món, khoá nhân viên | Mỗi thao tác sinh một dòng nhật ký: ai làm, ai duyệt, lúc nào, ở chi nhánh nào, giá trị trước và sau khi đổi |
| L-02 | Sửa hoặc xoá nhật ký qua API | Không có API nào làm được việc này |
| L-03 | Quyền xem nhật ký | `ql1` chỉ xem nhật ký của CN1 |

## W. Giao diện POS và trang quản trị (Playwright)
Màn hình POS được kiểm tra ở khổ 1024×768 và 1280×800 (máy tính bảng nằm ngang). Trang quản trị kiểm tra ở khổ 1280×800.

| ID | Kiểm tra | Kết quả mong đợi |
|---|---|---|
| W-01 | Kích hoạt máy POS | Quản lý đăng nhập → chọn chi nhánh → máy hiện tên chi nhánh ở góc màn hình |
| W-02 | Bàn phím số nhập PIN | PIN đúng thì vào màn hình bán hàng. PIN sai thì hiện lỗi và xoá ô nhập. Không thao tác 15 phút thì quay lại màn hình nhập PIN |
| W-03 | Bố cục POS | Nhìn thấy cùng lúc danh sách nhóm, lưới món và giỏ hàng. Mọi nút có kích thước ≥ 44px |
| W-04 | Chọn bàn → chọn món → hộp chọn size và tuỳ chọn → thêm vào giỏ | Giỏ hiện đúng dòng món và **43.000đ** |
| W-05 | Món "hết món" | Bị làm mờ, không bấm được |
| W-06 ❓ | Giảm giá 25% | Hiện bàn phím nhập PIN quản lý. Duyệt xong thì tổng đơn được cập nhật |
| W-07 | Thanh toán tiền mặt | Có các nút gợi ý mệnh giá, hiện tiền thối, rồi chuyển sang màn hình hoàn tất có **mã QR tra cứu hoá đơn điện tử** |
| W-08 | Thanh toán VietQR | Hiện mã QR kèm số tiền. Bấm "Đã nhận tiền" thì chuyển sang màn hình hoàn tất |
| W-09 | Khách lấy hoá đơn công ty | Form có kiểm tra mã số thuế. Bỏ trống thì hoá đơn ghi "Khách lẻ" |
| W-10 | Mở ca / chốt ca | Đi hết luồng, màn hình chốt ca hiện đúng chênh lệch |
| W-11 | Trang quản trị: tạo món mới | Món xuất hiện trên POS và trên trang `/menu` của website |
| W-12 | Chọn chi nhánh trong trang quản trị | `ql1` chỉ thấy CN1. `owner` thấy tất cả chi nhánh và mục "Toàn chuỗi" |
| W-13 | Trang báo cáo | Lọc theo khoảng ngày và chi nhánh. Các số liệu khớp với API |
| W-14 | Mất mạng trên POS | Hiện thanh báo "Mất kết nối", khoá nút thanh toán, không để mất dữ liệu mà người dùng không biết. Có mạng lại thì tiếp tục được |
| W-15 | Định dạng | Tiền dạng `1.250.000đ`, ngày dạng `19/11/2026`, giờ dạng 24 giờ |

## F. Toàn luồng (Playwright trên Docker Compose)
| ID | Luồng | Kết quả mong đợi |
|---|---|---|
| F-01 | `tn1` mở ca → bàn 5 → thêm 2 món → giảm 10% → trả tiền mặt → hoá đơn điện tử phát hành → hiện QR tra cứu → chốt ca | Báo cáo CN1 có doanh thu đúng, CN2 không có. Chốt ca đúng số tiền |
| F-02 | `owner` đổi giá món | POS và trang `/menu` hiện giá mới. Đơn cũ không đổi |
| F-03 | Nhà cung cấp hoá đơn điện tử lỗi trong lúc bán | Vẫn bán bình thường. Khi nhà cung cấp hoạt động lại, hoá đơn tự phát hành |
| F-04 | Khoá `tn1` giữa ca | Máy POS của `tn1` bị đưa về màn hình nhập PIN ngay |

## N. Hiệu năng, bảo mật, chất lượng
| ID | Kiểm tra | Kết quả mong đợi |
|---|---|---|
| N-01 | 50 máy POS giả lập cùng tạo đơn và thanh toán liên tục (k6) | 95% yêu cầu hoàn thành dưới 200 ms, không có lỗi |
| N-02 | Sao lưu rồi khôi phục cơ sở dữ liệu | Dữ liệu sau khi khôi phục khớp với bản gốc |
| N-03 | Gửi dữ liệu kiểu tấn công SQL injection vào các ô tìm kiếm và nhập liệu | Không ảnh hưởng gì. Mọi truy vấn đều dùng tham số |
| N-04 | Lint và build | `golangci-lint`, `go vet`, `npm run lint`, `npm run build` đều không lỗi |

---

## Giả định cần bạn xác nhận (các case ❓)
1. **Ngưỡng giảm giá:** thu ngân tự giảm tối đa **20%**, vượt mức này phải có PIN quản lý.
2. **Bảo mật đăng nhập:** máy POS tự khoá sau **15 phút** không thao tác. Sai mật khẩu **5 lần** thì khoá **15 phút**.
3. **Thuế:** giá bán **đã gồm thuế**, hệ thống tự tách tiền thuế. Thuế suất cài riêng cho từng chi nhánh (cần kế toán xác nhận).
4. **Thanh toán kết hợp:** cho phép trả một đơn bằng cả tiền mặt và QR.
5. **Chốt ca:** chênh lệch tiền trên **50.000đ** bắt buộc ghi chú.
6. **Không in giấy:** khách nhận hoá đơn điện tử bằng cách quét **mã QR tra cứu trên màn hình POS**, hoặc qua email nếu khách cung cấp.
