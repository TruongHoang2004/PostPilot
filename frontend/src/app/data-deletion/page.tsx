import type { Metadata } from "next";
import { PolicyContact, PolicyLayout } from "@/components/PolicyLayout";

export const metadata: Metadata = {
  title: "Hướng dẫn xóa dữ liệu | PostPilot",
  description: "Cách yêu cầu xóa thông tin Page, token và dữ liệu liên quan trong PostPilot, thu hồi quyền Facebook và theo dõi yêu cầu qua email.",
};

export default function DataDeletionPage() {
  return (
    <PolicyLayout
      title="Yêu cầu xóa dữ liệu"
      description="Bạn có thể yêu cầu người vận hành xóa dữ liệu liên quan đến mình trong PostPilot qua email. Yêu cầu được tiếp nhận và xử lý thủ công."
    >
      <section aria-labelledby="request">
        <h2 id="request">1. Cách gửi yêu cầu</h2>
        <ol>
          <li>Gửi email đến <PolicyContact subject="PostPilot - Yêu cầu xóa dữ liệu" /> với tiêu đề <strong>PostPilot - Yêu cầu xóa dữ liệu</strong>.</li>
          <li>Cho biết tên và mã Facebook Page đã kết nối, cùng thông tin đủ để người vận hành xác định dữ liệu liên quan. Nếu có nhiều Page, hãy liệt kê những Page thuộc phạm vi yêu cầu.</li>
          <li>Nêu rõ bạn muốn xóa toàn bộ dữ liệu liên quan hay chỉ một phần, chẳng hạn token kết nối, lịch đăng, bài viết hoặc tệp đã tải lên.</li>
        </ol>
        <p><strong>Không gửi mật khẩu Facebook, access token, App Secret hoặc giấy tờ tùy thân trong email yêu cầu ban đầu.</strong></p>
      </section>

      <section aria-labelledby="process">
        <h2 id="process">2. Xác minh và theo dõi</h2>
        <p>Người vận hành sẽ phản hồi qua email để xác nhận tiếp nhận, xác minh quyền quản lý Page hoặc mối liên hệ của bạn với dữ liệu được yêu cầu, làm rõ phạm vi và thông báo thời gian xử lý dự kiến.</p>
        <p>Sau khi xử lý, bạn sẽ nhận email xác nhận kết quả. Bạn có thể trả lời trong cùng chuỗi email để hỏi tiến độ. Không cần tạo tài khoản mới hoặc đăng nhập PostPilot để gửi yêu cầu.</p>
      </section>

      <section aria-labelledby="scope">
        <h2 id="scope">3. Phạm vi dữ liệu trong PostPilot</h2>
        <p>Tùy phạm vi yêu cầu đã xác minh, việc xử lý có thể bao gồm:</p>
        <ul>
          <li>Thông tin Page kết nối và access token được lưu trong ứng dụng.</li>
          <li>Bài viết, lịch đăng chưa thực hiện, lịch sử và nhật ký đăng liên quan đến Page.</li>
          <li>Ảnh, video, thông tin tệp và các bản sao tệp do PostPilot quản lý thuộc phạm vi yêu cầu.</li>
          <li>Dữ liệu liên quan trong cảnh báo vận hành hoặc bản sao lưu, nếu có và thuộc quyền kiểm soát của người vận hành.</li>
        </ul>
        <p>Nếu một phần dữ liệu cần được lưu lại theo nghĩa vụ áp dụng, hoặc nằm trong bản sao lưu chưa thể xóa riêng ngay, người vận hành sẽ giải thích phạm vi, lý do và thời gian xử lý trong phản hồi. Trang này không phải hệ thống xóa tự động.</p>
      </section>

      <section aria-labelledby="disconnect">
        <h2 id="disconnect">4. Thu hồi quyền trên Facebook</h2>
        <p>Bạn có thể vào phần cài đặt Facebook, tìm mục <strong>Tích hợp kinh doanh (Business integrations)</strong> hoặc <strong>Ứng dụng và trang web (Apps and websites)</strong>, chọn ứng dụng Meta đã dùng để kết nối PostPilot và gỡ quyền truy cập. Tên ứng dụng trong Facebook có thể khác tên PostPilot.</p>
        <p>Thu hồi quyền giúp ngăn ứng dụng tiếp tục sử dụng quyền được cấp. Để xóa dữ liệu đã lưu trong PostPilot, bạn vẫn cần gửi yêu cầu qua email theo hướng dẫn ở trên.</p>
      </section>

      <section aria-labelledby="facebook-content">
        <h2 id="facebook-content">5. Bài đã đăng trên Facebook và dịch vụ khác</h2>
        <p>Xóa dữ liệu trong PostPilot không đồng nghĩa với xóa bài đã đăng trên Facebook. Nếu muốn xóa các bài đó, bạn cần thực hiện trên Page hoặc qua công cụ quản lý của Meta.</p>
        <p>Dữ liệu do Meta, Google hoặc nhà cung cấp khác lưu độc lập thuộc quy trình của nhà cung cấp đó. Người vận hành sẽ làm rõ phần dữ liệu mình có thể xử lý và hướng dẫn thêm khi cần.</p>
      </section>
    </PolicyLayout>
  );
}
