import type { Metadata } from "next";
import Link from "next/link";
import { PolicyContact, PolicyLayout } from "@/components/PolicyLayout";

export const metadata: Metadata = {
  title: "Chính sách quyền riêng tư | PostPilot",
  description: "Thông tin về dữ liệu PostPilot xử lý khi kết nối Facebook Page, lên lịch đăng bài và cách liên hệ về quyền riêng tư.",
};

export default function PrivacyPage() {
  return (
    <PolicyLayout
      title="Chính sách quyền riêng tư"
      description="Chính sách này giải thích cách PostPilot xử lý dữ liệu để kết nối Facebook Page, soạn nội dung và đăng bài theo lịch bạn thiết lập."
    >
      <section aria-labelledby="operator">
        <h2 id="operator">1. Phạm vi và liên hệ</h2>
        <p>Chính sách áp dụng cho ứng dụng PostPilot tại postpilot.hexta.org. Người vận hành PostPilot tiếp nhận câu hỏi và yêu cầu liên quan đến dữ liệu qua email:</p>
        <p><PolicyContact subject="PostPilot - Quyền riêng tư" /></p>
      </section>

      <section aria-labelledby="data">
        <h2 id="data">2. Dữ liệu được xử lý</h2>
        <ul>
          <li><strong>Thông tin Page kết nối:</strong> mã Page, tên, danh mục và thông tin quyền truy cập được Meta trả về khi bạn cấp quyền.</li>
          <li><strong>Thông tin xác thực:</strong> access token dùng để thực hiện các thao tác với Page bạn kết nối. Token Page được mã hóa khi lưu trong cơ sở dữ liệu.</li>
          <li><strong>Nội dung bạn cung cấp:</strong> văn bản, liên kết, ảnh, video, lịch đăng và các thông tin bài viết liên quan.</li>
          <li><strong>Dữ liệu vận hành:</strong> trạng thái đăng bài, mã bài viết Facebook, thời điểm xử lý, phản hồi hoặc lỗi từ dịch vụ tích hợp và nhật ký cần cho việc kiểm tra sự cố.</li>
          <li><strong>Tùy chọn giao diện:</strong> lựa chọn giao diện sáng hoặc tối được lưu trong bộ nhớ của trình duyệt.</li>
          <li><strong>Thông tin liên hệ:</strong> email và nội dung bạn gửi khi yêu cầu hỗ trợ hoặc xóa dữ liệu.</li>
        </ul>
      </section>

      <section aria-labelledby="purpose">
        <h2 id="purpose">3. Mục đích sử dụng</h2>
        <p>PostPilot xử lý dữ liệu để hiển thị các Page đã kết nối, soạn và lưu nội dung, quản lý lịch đăng, gửi bài lên Facebook theo thao tác hoặc lịch bạn thiết lập, kiểm tra trạng thái token và hỗ trợ khắc phục sự cố.</p>
        <p>Khi bạn chủ động sử dụng tính năng hỗ trợ viết nội dung bằng AI, chủ đề và giọng văn bạn nhập được dùng để tạo nội dung gợi ý. Việc tạo nội dung không tự động đăng bài thay cho bạn.</p>
      </section>

      <section aria-labelledby="services">
        <h2 id="services">4. Dịch vụ tham gia xử lý dữ liệu</h2>
        <ul>
          <li><strong>Meta/Facebook:</strong> nhận các yêu cầu xác thực, kiểm tra quyền và nội dung đăng lên Page. Nội dung sau khi đăng chịu các cài đặt hiển thị và chính sách của Facebook.</li>
          <li><strong>Hạ tầng lưu trữ:</strong> máy chủ, cơ sở dữ liệu và kho lưu trữ tệp do người vận hành cấu hình lưu nội dung, tệp tải lên và dữ liệu vận hành.</li>
          <li><strong>Google Gemini, khi được cấu hình:</strong> nhận chủ đề và giọng văn khi bạn sử dụng tính năng AI để tạo nội dung gợi ý.</li>
          <li><strong>Telegram, khi được cấu hình:</strong> nhận cảnh báo vận hành có thể bao gồm tên hoặc mã Page, mã bài viết và thông tin lỗi, gửi đến cuộc trò chuyện của người vận hành.</li>
        </ul>
        <p>Người vận hành có thể truy cập dữ liệu để vận hành ứng dụng, giải quyết yêu cầu hỗ trợ và xử lý yêu cầu xóa. Việc xử lý tại các dịch vụ tích hợp còn tuân theo chính sách của từng nhà cung cấp.</p>
      </section>

      <section aria-labelledby="storage">
        <h2 id="storage">5. Lưu trữ và bảo vệ dữ liệu</h2>
        <p>Thông tin Page, token, bài viết, tệp tải lên và nhật ký có thể được lưu để phục vụ chức năng của ứng dụng và kiểm tra sự cố cho đến khi được người vận hành xóa. Ứng dụng hiện không cung cấp thời hạn tự động xóa chung cho mọi loại dữ liệu.</p>
        <p>Token Page được mã hóa khi lưu. Bạn không nên đưa mật khẩu Facebook, access token hoặc thông tin nhạy cảm vào nội dung bài viết, lời nhắc AI hay email hỗ trợ.</p>
      </section>

      <section aria-labelledby="choices">
        <h2 id="choices">6. Lựa chọn và yêu cầu của bạn</h2>
        <p>Bạn có thể ngừng sử dụng PostPilot, thu hồi quyền truy cập của ứng dụng trong phần cài đặt Facebook và liên hệ người vận hành để yêu cầu kiểm tra, chỉnh sửa hoặc xóa dữ liệu liên quan đến mình.</p>
        <p>Thu hồi quyền trên Facebook không tự động xóa mọi dữ liệu đã lưu trong PostPilot. Xem <Link href="/data-deletion" className="font-medium text-blue-700 underline underline-offset-4 dark:text-blue-300">hướng dẫn yêu cầu xóa dữ liệu</Link> để biết cách gửi yêu cầu và phạm vi xử lý.</p>
      </section>

      <section aria-labelledby="updates">
        <h2 id="updates">7. Cập nhật chính sách</h2>
        <p>Chính sách sẽ được cập nhật khi cách vận hành hoặc xử lý dữ liệu của PostPilot thay đổi. Ngày cập nhật được hiển thị ở đầu trang. Nếu cần làm rõ nội dung chính sách, hãy liên hệ email nêu trên.</p>
      </section>
    </PolicyLayout>
  );
}
