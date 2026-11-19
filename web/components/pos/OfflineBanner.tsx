"use client";
import { useOnline } from "@/lib/pos/connectivity";

/** Screen 16 / W-14: red banner while the server cannot be reached. Payment is disabled meanwhile. */
export function OfflineBanner() {
  const online = useOnline();
  if (online) return null;
  return (
    <div role="alert" className="flex items-start gap-3 border-b border-bad/30 bg-bad-soft px-5 py-3 text-[15px] text-[#7a2a1e]">
      <span aria-hidden className="mt-0.5 font-bold">!</span>
      <div>
        <p className="font-bold">Mất kết nối tới máy chủ. Tạm khoá thanh toán.</p>
        <p>
          Đơn đang mở vẫn được giữ trên máy chủ, các món đã thêm không bị mất. Kiểm tra Wi-Fi hoặc bật 4G dự phòng; khi có mạng lại máy sẽ tự
          tiếp tục.
        </p>
      </div>
    </div>
  );
}
