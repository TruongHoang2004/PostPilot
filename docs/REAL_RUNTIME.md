# Chạy PostPilot với danh tính và dịch vụ thật

Bản thay đổi này bỏ quyền mô phỏng, anonymous OWNER, workspace/profile giả, bài AI mẫu và việc tự chuyển PostgreSQL/Redis sang RAM. Không thay dữ liệu Page/bài viết hiện có. Các memory Store/transport chỉ được khởi tạo tường minh trong test.

## Chuẩn bị cấu hình

- `DATABASE_URL`: PostgreSQL thật đã chạy migrations 000001–000005; migration lỗi làm API dừng.
- `REDIS_ADDR`: Redis thật. API/worker dừng nếu không kết nối được, không nhận việc vào queue RAM.
- `API_ADMIN_KEY` (chỉ API, worker không cần JWT secret): secret ngẫu nhiên riêng, ít nhất 32 ký tự; dùng ký JWT và xác minh bước nhận workspace cũ. Có thể tạo bằng `openssl rand -hex 32`. Không đưa vào `NEXT_PUBLIC_*`, source, log hoặc chat.
- `TOKEN_ENCRYPTION_KEY`: khóa AES riêng. Nếu đang có token Page mã hóa trong database, **giữ nguyên khóa đang dùng**, không tạo lại vì đổi khóa sẽ làm token cũ không giải mã được. Khóa mẫu mặc định không được chấp nhận lúc khởi động.
- `FB_APP_ID`, `FB_APP_SECRET`, `FB_GRAPH_VERSION`: dùng cấu hình app Meta thật. `FB_REDIRECT_URI` phải cùng origin với `FRONTEND_URL` để cookie OAuth gắn đúng trình duyệt. Ví dụ với `FRONTEND_URL=https://postpilot.hexta.org`, dùng `https://postpilot.hexta.org/api/v1/auth/facebook/callback`, thêm chính xác URL này vào Valid OAuth Redirect URIs trên Meta Dashboard và cập nhật biến môi trường API. Next.js proxy `/api/*` đã hỗ trợ callback này. State được lưu server-side, chỉ dùng một lần và hết hạn sau 10 phút; API và frontend không được tự đổi callback sang subdomain khác. Manual Page token được đối chiếu `/me` với Page mục tiêu trước khi lưu.
- Media upload cần S3/MinIO/R2 thật và `S3_PUBLIC_BASE_URL` HTTPS mà Meta tải được. Storage lỗi trả lỗi, không lưu âm thầm vào local. Media cũ chỉ có `/uploads/` local cần tải lại lên storage trước khi đăng bằng bản mới.
- Thiếu `GEMINI_API_KEY`: chức năng AI trả 503, không bịa bài mẫu. Telegram không cấu hình được coi là disabled và không báo đã gửi; đăng bài vẫn hoạt động độc lập.

## Tài khoản và dữ liệu cũ

1. Mở frontend, đăng ký tài khoản bằng email và mật khẩu tối thiểu 12 ký tự. Tài khoản nhận một workspace riêng.
2. Với các Page/bài trước đây nằm trong `Default Workspace`, vào **Đội nhóm → Nhận workspace từ phiên bản cũ (một lần)**.
3. Nhập `API_ADMIN_KEY` của server. Chỉ thực hiện trên HTTPS của chính PostPilot. Frontend không lưu khóa vào localStorage; khóa chỉ dùng cho request chuyển quyền.
4. Chọn `Default Workspace` ở thanh trên. Token Page, lịch đăng, bài và media metadata vẫn giữ nguyên. Chỉ membership OWNER của user seed được chuyển sang user thật. Lần nhận thứ hai trả conflict; người đăng ký đầu tiên không tự được nhận dữ liệu cũ.
5. Thêm thành viên đã có tài khoản, hoặc tạo mã lời mời và gửi cho đúng người qua kênh riêng. Người nhận đăng nhập đúng email, vào Đội nhóm và nhập mã. Chưa có dịch vụ gửi email tự động; giao diện không tuyên bố đã gửi email.

## Chuyển worker sang bản mới

Dừng worker cũ và hạn chế thay đổi lịch trong lúc cutover. Job mới có `organization_id`; worker tái tạo context theo organization của job và mọi truy vấn entity đều có scope. Scheduler quét từng workspace thật.

Các job Redis từ phiên bản cũ thiếu `organization_id` sẽ bị từ chối và ghi log `Legacy job missing organization; operator reconciliation required`. Trước khi deploy, cần đợi queue cũ hoàn tất hoặc đối chiếu các bài QUEUED/PROCESSING với Page và publish_logs, rồi tạo lại lịch có kiểm soát cho bài chắc chắn chưa được đăng. Không tự reset hàng loạt vì có thể đăng trùng bài đã được Meta nhận.

## Chức năng chưa có

Webhook event processing trước đây chỉ log rồi ACK. Hiện POST webhook trả 501 rõ ràng; không đăng ký event subscription để trông chờ đồng bộ trạng thái. GET challenge verification vẫn có, việc lên lịch dùng worker. Xóa dữ liệu vẫn là yêu cầu xử lý thủ công qua trang hướng dẫn; không có callback xóa tự động.

Chỉ Facebook có adapter thực thi; các platform khác bị từ chối khi kết nối. Giao diện tạo nhóm không còn cho cấu hình màu chưa được backend lưu.

## Build và deploy

Thứ tự deploy cho bản có migration `000006_post_batches` (lô bài hàng loạt): chỉ `fb-api` chạy migration lúc khởi động, còn mọi lần đọc bài (kể cả worker) đều JOIN bảng `post_batches`. Vì vậy **deploy/khởi động API trước, worker sau**; khi rollback thì hạ binary cũ trước rồi mới `migrate down`. `CREATE INDEX idx_posts_org_channel_sched` chạy không `CONCURRENTLY` nên khoá ghi bảng `posts` trong vài giây với bảng lớn; chọn lúc ít lịch đến hạn.


Chạy `go test ./...`, `go build ./...`, `npm run lint` và `npm run build` trong frontend. Script lint hiện là TypeScript `tsc --noEmit`, không phải ESLint.

Build/deploy frontend và backend từ cùng revision vì auth và queue contract thay đổi. Image production chỉ dùng immutable tag `sha-<commit>` hoặc `vX.Y.Z`; không dùng `:latest`. Phiên làm việc này không triển khai lên VPS hoặc thay cấu hình Meta.

Kiểm thử PostgreSQL tùy chọn dùng `POSTPILOT_TEST_DATABASE_URL` trỏ đến **database thử nghiệm có thể xóa**, không dùng production. Chạy `go test ./internal/database -run TestPostgresAtomicSignupAndConcurrentApproval -v` để kiểm tra rollback đăng ký và hai người duyệt đồng thời.
