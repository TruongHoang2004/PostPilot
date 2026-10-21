# 🔑 Hướng dẫn Setup Meta App & Lấy Permanent Page Access Token

Để hệ thống đăng bài tự động hoạt động bền vững hàng năm mà không bị gián đoạn vì hết hạn Token, bạn cần thiết lập một ứng dụng trên **Meta for Developers** và lấy **Permanent Page Access Token (Token Trang Vĩnh Viễn)**.

---

## Bước 1: Tạo Ứng dụng trên Meta for Developers

1. Truy cập [https://developers.facebook.com/apps](https://developers.facebook.com/apps) và nhấn **Create App**.
2. Ở bước chọn Use case: Chọn **Other** (Khác) $\rightarrow$ Nhấn **Next**.
3. Chọn loại ứng dụng (App Type): Chọn **Business** (Doanh nghiệp).
4. Đặt tên App (ví dụ: `Darwin Auto Publisher`), điền email liên hệ và chọn Business Account (nếu có) $\rightarrow$ Nhấn **Create App**.
5. Vào mục **App Settings > Basic**:
   * Sao chép **App ID** và **App Secret** điền vào file `.env`:
     ```env
     FB_APP_ID=123456789012345
     FB_APP_SECRET=abcdef1234567890abcdef1234567890
     ```

---

## Bước 2: Cấp Quyền Quản lý Fanpage (Permissions)

1. Trên thanh menu trên cùng của trang Meta for Developers, mở **Tools > Graph API Explorer**.
2. Ở cột bên phải:
   * **Meta App:** Chọn ứng dụng bạn vừa tạo ở Bước 1.
   * **User or Page:** Giữ nguyên là **User Token**.
   * **Permissions (Quyền):** Nhấn *Add a Permission* và thêm đủ 3 quyền sau:
     * `pages_manage_posts` (Quyền cốt lõi để tạo bài đăng Text, Ảnh, Video, Reels).
     * `pages_read_engagement` (Quyền đọc thông tin tương tác và metadata của Trang).
     * `pages_show_list` (Quyền liệt kê danh sách các Fanpage bạn đang làm Quản trị viên).
3. Nhấn nút **Generate Access Token** và xác nhận cấp quyền cho các Fanpage bạn muốn hệ thống quản lý.
4. Sao chép chuỗi **User Access Token** vừa sinh ra (Token này chỉ có hạn 1–2 giờ, gọi là `short_lived_user_token`).

---

## Bước 3: Đổi sang Token Vĩnh Viễn Tự Động qua Hệ thống

Bạn **không cần** tự gọi tay từng bước trên Postman nữa. Hệ thống Golang của chúng ta đã tích hợp sẵn API tự động làm cả 3 việc:
1. Đổi `short_lived_user_token` (1 giờ) $\rightarrow$ `long_lived_user_token` (60 ngày).
2. Gọi `/me/accounts` bằng token 60 ngày để lấy **Permanent Page Access Token** (không bao giờ hết hạn) của tất cả các Fanpage bạn quản lý.
3. Mã hóa các Token này bằng **AES-256-GCM** và lưu vào bảng `channels`.

Chỉ cần chạy 1 lệnh duy nhất:
```bash
curl -X POST http://localhost:8080/api/v1/auth/facebook/exchange-token \
  -H "Content-Type: application/json" \
  -d '{
    "short_lived_user_token": "<DÁN_USER_TOKEN_TỪ_GRAPH_API_EXPLORER_VÀO_ĐÂY>"
  }'
```

Hoặc nếu bạn đã lấy sẵn **Page Access Token** từ trước, bạn có thể nhập trực tiếp trên giao diện Web tại `http://localhost:8080` (Mục *1. Kết nối Fanpage*).

---

## ⚠️ Lưu ý Quan trọng để Token Không Bị Mất Hiệu Lực

Dù là **Permanent Page Access Token**, token vẫn có thể bị Meta thu hồi (trả về mã lỗi `190`) trong 3 trường hợp sau:
1. Tài khoản Facebook cá nhân của Admin (người tạo token) **đổi mật khẩu Facebook**.
2. Admin bị gỡ quyền Quản trị viên khỏi Fanpage.
3. Admin xóa ứng dụng trong phần *Cài đặt & quyền riêng tư > Tích hợp cho doanh nghiệp* trên Facebook.

> **Giải pháp nâng cao trên Cloud:** Nếu doanh nghiệp của bạn đã xác minh doanh nghiệp (Verified Meta Business Manager), hãy vào **Business Settings > Users > System Users**, tạo một **System User**, gán quyền quản lý Fanpage cho System User đó và sinh Token. Token của **System User** hoàn toàn độc lập với mật khẩu cá nhân của nhân sự!
