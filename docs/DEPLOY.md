# Hướng dẫn triển khai Mộc POS (dành cho chủ chuỗi)

Tài liệu này hướng dẫn từng bước đưa hệ thống lên máy chủ **miễn phí** của Oracle Cloud, dùng Cloudflare
cho tên miền và HTTPS, sao lưu lên Cloudflare R2. Kiến trúc: [`docs/ARCHITECTURE.md`](ARCHITECTURE.md) mục 8 và 12.

| Hạng mục | Chi phí |
|---|---|
| Máy chủ Oracle Cloud Always Free (Singapore, ARM 2 nhân, 12 GB RAM) | 0đ |
| Cloudflare gói Free (tên miền, HTTPS, Tunnel) | 0đ |
| Cloudflare R2 (sao lưu, 10 GB miễn phí) | 0đ |
| Tên miền | khoảng 300.000đ/năm |

> **Chỉ chủ chuỗi được tạo tài khoản và nhập thẻ ngân hàng** (Oracle, Cloudflare, nhà đăng ký tên miền).
> Không đưa mật khẩu, mã thẻ hay mã khôi phục cho người khác, kể cả người hỗ trợ kỹ thuật. Người hỗ trợ
> chỉ cần quyền SSH vào máy chủ (bước 3) là đủ để cài đặt.

Thời gian làm lần đầu: khoảng 2–3 giờ. Nên làm ngoài giờ cao điểm (không làm 06:30–09:00).

Ký hiệu trong tài liệu:
- `pos.example.vn`: thay bằng tên miền thật của bạn.
- Dòng bắt đầu bằng `$`: lệnh gõ trên máy chủ (bỏ dấu `$`).

---

## Bước 1. Tạo tài khoản Oracle Cloud

1. Vào https://www.oracle.com/cloud/free/ → **Start for free**.
2. Điền email, tên, quốc gia **Vietnam**.
3. **Home Region: chọn `Singapore`.** Không đổi được về sau. Máy chủ miễn phí chỉ tạo được ở vùng nhà.
4. Nhập thẻ ngân hàng để xác minh (Oracle có thể giữ tạm một khoản nhỏ rồi hoàn lại).
5. **Nâng lên Pay-As-You-Go** để Oracle không thu hồi máy "nhàn rỗi":
   - Menu ☰ → **Billing & Cost Management** → **Upgrade and Manage Payment** → **Upgrade to Pay As You Go**.
   - Vẫn **0đ** nếu chỉ dùng tài nguyên Always Free. Kiểm tra lại điều khoản trên trang của Oracle lúc tạo,
     vì Oracle có thể thay đổi (xem `docs/PLAN.md` mục 6).
6. **Đặt cảnh báo chi phí** (bắt buộc, để biết ngay nếu có gì phát sinh tiền):
   - ☰ → **Billing & Cost Management** → **Budgets** → **Create Budget**.
   - Số tiền: `1` USD/tháng. Thêm cảnh báo khi **Actual spend** vượt **1%**, gửi về email của bạn.

## Bước 2. Tạo máy chủ ARM

1. ☰ → **Compute** → **Instances** → **Create instance**.
2. **Name:** `mocpos`.
3. **Image:** *Canonical Ubuntu 24.04*.
4. **Shape:** **Ampere** → `VM.Standard.A1.Flex`, **2 OCPU**, **12 GB** memory (nằm trong hạn mức miễn phí).
5. **Networking:** tạo VCN mới, subnet công khai, **Assign a public IPv4 address**.
6. **SSH keys:** chọn **Generate a key pair for me** → **tải cả hai file khoá** về và cất kỹ
   (mất khoá là không vào được máy). Hoặc dán khoá công khai của người hỗ trợ kỹ thuật.
7. **Boot volume:** đặt `100` GB (miễn phí tới 200 GB tổng).
8. **Create**. Nếu báo *Out of capacity*, vùng Singapore tạm hết máy ARM: thử lại sau vài giờ hoặc vào sáng sớm.
9. Ghi lại **Public IP** của máy.
10. Chỉ mở cổng SSH cho IP của bạn: trang máy → **Subnet** → **Security List** → sửa luật cổng `22`,
    **Source CIDR** = `IP-của-bạn/32` (xem IP ở https://ifconfig.me). **Không mở cổng 80/443**: Cloudflare Tunnel
    tự kết nối ra ngoài, máy chủ không cần nhận kết nối từ Internet.

## Bước 3. Cài Docker trên máy chủ

Kết nối SSH (trên máy tính của bạn):
```bash
ssh -i duong-dan/khoa-ssh.key ubuntu@<Public IP>
```

Trên máy chủ:
```bash
$ sudo apt-get update && sudo apt-get -y upgrade
$ sudo timedatectl set-timezone Asia/Ho_Chi_Minh
$ sudo apt-get install -y unattended-upgrades git        # tự cài bản vá bảo mật
$ curl -fsSL https://get.docker.com | sudo sh             # Docker + Docker Compose
$ sudo usermod -aG docker ubuntu && exit                  # thoát rồi SSH lại để có quyền docker
```
Kiểm tra: `docker compose version` phải in ra phiên bản.

## Bước 4. Mua tên miền và đưa vào Cloudflare

1. Mua tên miền (ví dụ `.vn` hoặc `.com`) ở nhà đăng ký bất kỳ, khoảng 300.000đ/năm.
2. Tạo tài khoản https://dash.cloudflare.com (gói **Free**) → **Add a site** → nhập tên miền → chọn **Free**.
3. Cloudflare đưa 2 **nameserver**. Vào trang quản lý tên miền ở nhà đăng ký, thay nameserver bằng 2 địa chỉ đó.
   Chờ vài phút tới vài giờ cho tới khi Cloudflare báo **Active**.
4. Trong Cloudflare: **SSL/TLS** → chế độ **Full**; **Edge Certificates** → bật **Always Use HTTPS**.

## Bước 5. Lấy mã nguồn cấu hình lên máy chủ

```bash
$ git clone https://github.com/ndhuy-sg/first-claude-app.git mocpos && cd mocpos
$ cp .env.example .env && chmod 600 .env
```
Kho mã riêng tư? Dùng *deploy key* chỉ-đọc, hoặc chép 4 thứ cần thiết lên máy: `docker-compose.yml`,
`.env.example`, thư mục `deploy/`.

Ảnh Docker (image) do GitHub Actions build sẵn ở `ghcr.io/ndhuy-sg/…`. Nếu gói (package) trên GitHub để riêng tư,
đăng nhập một lần bằng token chỉ có quyền `read:packages`:
```bash
$ echo <TOKEN> | docker login ghcr.io -u <tên-github> --password-stdin
```

## Bước 6. Tạo Cloudflare Tunnel

Tunnel nối máy chủ với Cloudflare mà không mở cổng nào. Luật chia đường dẫn nằm sẵn trong
`deploy/cloudflared/config.yml`: `/api/*` → API Go, còn lại → giao diện Next.js.

Chạy trên máy chủ (dùng chính image cloudflared, không cần cài thêm):
```bash
$ mkdir -p ~/.cloudflared
$ alias cf='docker run --rm -it --user "$(id -u):$(id -g)" -e HOME=/cf -v ~/.cloudflared:/cf/.cloudflared cloudflare/cloudflared:2026.11.1'
$ cf tunnel login                              # mở đường link in ra, chọn tên miền của bạn
$ cf tunnel create mocpos
$ cf tunnel route dns mocpos pos.example.vn    # tạo bản ghi DNS trỏ vào tunnel
$ cf tunnel token mocpos                       # in ra token dài → dán vào .env: CLOUDFLARE_TUNNEL_TOKEN=
```
> Cách khác (không dùng dòng lệnh): Cloudflare **Zero Trust** → **Networks** → **Tunnels** → **Create a tunnel**
> → **Cloudflared** → chép token. Khi đó luật chia đường dẫn lấy theo trang web: thêm 2 **Public Hostname**
> cho `pos.example.vn`: path `^/api/` → `http://api:8080`, rồi path trống → `http://web:3000` (đúng thứ tự).

## Bước 7. Tạo kho sao lưu R2

1. Cloudflare → **R2 Object Storage** → bật R2 (có thể phải nhập thẻ; 10 GB đầu miễn phí).
2. **Create bucket** → tên `mocpos-backup`, vị trí **Asia-Pacific (APAC)**.
3. **Manage R2 API Tokens** → **Create API token** → quyền **Object Read & Write**, chỉ cho bucket
   `mocpos-backup` → tạo. Ghi lại **Access Key ID**, **Secret Access Key** và **Account ID**
   (Account ID nằm ở trang R2, cột bên phải).
4. Tạo **khoá mã hoá sao lưu** trên máy chủ:
   ```bash
   $ openssl rand -hex 32
   ```
   **Chép khoá này ra giấy và vào trình quản lý mật khẩu của bạn.** Mọi bản sao lưu được mã hoá bằng khoá này;
   mất khoá là bản sao lưu vô dụng.

## Bước 8. Điền file `.env`

```bash
$ nano .env
```
| Dòng | Giá trị |
|---|---|
| `DOMAIN` | `pos.example.vn` |
| `POSTGRES_PASSWORD` | kết quả của `openssl rand -hex 24` |
| `CLOUDFLARE_TUNNEL_TOKEN` | token ở bước 6 |
| `BACKUP_ENABLED` | `true` |
| `R2_ACCOUNT_ID`, `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY` | ở bước 7 |
| `R2_BUCKET` | `mocpos-backup` |
| `BACKUP_ENCRYPTION_KEY` | khoá 64 ký tự ở bước 7 |
| `BACKUP_HEARTBEAT_URL` | (nên có) xem bước 11 |

Lưu (`Ctrl+O`, `Enter`, `Ctrl+X`). File `.env` không bao giờ được đưa lên GitHub.

## Bước 9. Khởi động

```bash
$ docker compose pull
$ docker compose up -d
$ docker compose ps          # postgres, api, worker, web, cloudflared, backup: "running"/"healthy"; migrate: "exited (0)"
```
Thứ tự tự động: PostgreSQL → `migrate` (tạo/cập nhật bảng) → `api`, `worker` → `web` → `cloudflared`.
Xem log khi cần: `docker compose logs -f api` (thoát bằng `Ctrl+C`).

Mở `https://pos.example.vn/api/v1/healthz` → phải thấy `{"status":"ok"}`.

## Bước 10. Lần chạy đầu: dữ liệu ban đầu và tài khoản chủ chuỗi

1. Nạp dữ liệu khởi tạo (5 chi nhánh, menu, bàn, tài khoản mẫu). **Chỉ chạy một lần, trên cơ sở dữ liệu trống:**
   ```bash
   $ docker compose run --rm api seed
   ```
2. **Ngay sau đó** vào `https://pos.example.vn/admin`, đăng nhập `owner@moc.vn` / `matkhau123` và:
   - **đổi mật khẩu chủ chuỗi** (và email thật của bạn);
   - **khoá hoặc đổi mật khẩu/PIN mọi tài khoản mẫu** (`kho@`, `ql1@`, `ql2@`, PIN 1234, 4321, 5678, 8888, 9999);
   - sửa tên, địa chỉ, **mã số thuế, ký hiệu hoá đơn, thuế suất** của từng chi nhánh, tài khoản VietQR;
   - sửa menu và giá thật; tạo nhân viên thật cho từng chi nhánh.
   > Mật khẩu mẫu ai cũng biết (nằm trong mã nguồn). Không để hệ thống chạy với các tài khoản mẫu còn mở.
3. Kích hoạt từng máy tính bảng: trên iPad/Android mở `https://pos.example.vn/pos`, quản lý chi nhánh đăng nhập
   và chọn chi nhánh → "Thêm vào màn hình chính" (Safari: nút Chia sẻ → *Thêm vào MH chính*).
4. Công tắc gửi hoá đơn tới cơ quan thuế để **TẮT** cho tới khi bạn đăng ký xong (xem `docs/PLAN.md` mục 6).

## Bước 11. Theo dõi máy chủ (miễn phí)

- **Máy chủ ngừng:** tạo tài khoản https://uptimerobot.com → *New monitor* → HTTP(s) →
  `https://pos.example.vn/api/v1/healthz`, mỗi 5 phút, báo về email/điện thoại.
- **Sao lưu đêm không chạy:** tạo một *check* ở https://healthchecks.io (chu kỳ 1 ngày, grace 2 giờ) →
  chép đường link ping vào `.env`: `BACKUP_HEARTBEAT_URL=https://hc-ping.com/…` → `docker compose up -d`.
  Đêm nào sao lưu lỗi hoặc không chạy, bạn nhận email.

## Sao lưu hoạt động thế nào

- **Liên tục:** PostgreSQL gửi nhật ký thay đổi (WAL) lên R2, ít nhất 5 phút một lần → nếu máy chủ hỏng,
  mất tối đa khoảng 5 phút dữ liệu.
- **Hằng đêm 02:30:** container `backup` chụp một bản đầy đủ, rồi xoá bản cũ, **giữ 30 bản (30 ngày)**.
- Mọi file được **mã hoá** trước khi rời máy chủ (khoá ở bước 7).

Kiểm tra:
```bash
$ docker compose logs --tail 50 backup
$ docker compose exec backup bash -c '. walg-env.sh && wal-g backup-list'     # danh sách bản sao lưu
$ docker compose exec backup base-backup.sh                                    # sao lưu ngay (không cần chờ đêm)
```

**Thử khôi phục mỗi tháng một lần (test N-02)** — không đụng tới dữ liệu đang chạy:
```bash
$ docker compose run --rm backup restore-test.sh
```
Lệnh này lấy bản sao lưu mới nhất về một cơ sở dữ liệu tạm, so số dòng từng bảng với dữ liệu đang chạy,
in `N-02 OK` nếu khớp, rồi tự xoá bản tạm.

## Cập nhật phiên bản mới

Khi GitHub Actions trên nhánh `main` đã xanh (có image mới), ngoài giờ cao điểm:
```bash
$ cd ~/mocpos && git pull                  # lấy docker-compose.yml/deploy mới (nếu có)
$ docker compose pull
$ docker compose up -d                     # migrate chạy trước, rồi khởi động lại api, worker, web
$ docker compose ps
```
**Quay lại bản trước:** mỗi image có thêm nhãn `sha-<7 ký tự commit>` (xem trang Packages trên GitHub).
Đặt `IMAGE_TAG=sha-xxxxxxx` trong `.env` → `docker compose up -d`. Lưu ý: thay đổi cấu trúc bảng (migration)
không tự quay lại; nếu bản mới đã đổi bảng, khôi phục từ sao lưu theo mục dưới.

## Khôi phục từ sao lưu (máy hỏng, xoá nhầm dữ liệu)

Trên máy chủ (máy cũ hoặc máy mới đã làm bước 3, 5, 8):
```bash
$ docker compose stop api worker web backup postgres
$ docker compose run --rm --no-deps backup restore.sh                 # về trạng thái mới nhất
# hoặc về một thời điểm trước khi xảy ra sự cố (giờ Việt Nam):
$ docker compose run --rm --no-deps -e RECOVERY_TARGET_TIME="2026-11-19 14:30:00+07" backup restore.sh
$ docker compose up -d
$ docker compose logs -f postgres          # chờ "database system is ready to accept connections"
```
- Nếu máy cũ còn dữ liệu, `restore.sh` từ chối ghi đè. Chạy lại với `-e FORCE=yes`: dữ liệu cũ được
  **chuyển sang thư mục bên cạnh** (không xoá).
- Nếu đã dùng `RECOVERY_TARGET_TIME`: sau khi chạy xong, xoá dòng `recovery_target_time` ở cuối file
  `postgresql.auto.conf` (lệnh: `docker compose exec postgres sed -i '/recovery_target_time/d' /var/lib/postgresql/data/postgresql.auto.conf`).

## Chuyển sang máy chủ thuê (khoảng 1 giờ)

Khi Oracle đổi điều khoản hoặc thu hồi máy, chuyển sang VPS thuê (khoảng 100–200 nghìn đồng/tháng, nên chọn
máy ở Singapore hoặc Việt Nam, Ubuntu 24.04, từ 2 nhân/4 GB RAM). Image chạy được cả ARM64 và x86 (amd64).

1. Trên VPS mới: làm **bước 3** (cài Docker) và **bước 5** (lấy cấu hình). Chép file `.env` từ máy cũ sang
   (qua `scp`, **không** gửi qua chat/email).
2. Ngoài giờ bán hàng, trên **máy cũ** (nếu còn vào được), dừng bán và đẩy nốt nhật ký lên R2:
   ```bash
   $ docker compose stop web api worker
   $ docker compose exec postgres psql -U mocpos -d mocpos -c "SELECT pg_switch_wal()"
   $ sleep 30 && docker compose stop       # dừng hẳn, kể cả cloudflared
   ```
3. Trên **VPS mới**:
   ```bash
   $ docker compose pull
   $ docker compose run --rm --no-deps backup restore.sh
   $ docker compose up -d
   ```
   `cloudflared` dùng cùng token nên tên miền tự trỏ sang máy mới, **không cần đổi DNS**.
4. Mở `https://pos.example.vn/api/v1/healthz`, bán thử một đơn, kiểm tra báo cáo.
5. Sau vài ngày ổn định, xoá máy cũ trên Oracle.

> Không bao giờ để **hai máy cùng chạy** với cùng `.env`: cả hai sẽ cùng ghi sao lưu vào một chỗ.

## Khi gặp sự cố

| Hiện tượng | Kiểm tra |
|---|---|
| Trang web không mở được | `docker compose ps`; `docker compose logs --tail 100 cloudflared` |
| Báo lỗi khi bán | `docker compose logs --tail 200 api` |
| Hoá đơn không gửi đi được | Trang quản trị → Hoá đơn điện tử (trạng thái "thất bại", nút "Gửi lại"); `docker compose logs worker` |
| Ổ đĩa đầy | `df -h`; `docker system prune` (xoá image cũ không dùng) |
| Sao lưu lỗi | `docker compose logs backup`; kiểm tra token R2 còn hạn và `.env` |
