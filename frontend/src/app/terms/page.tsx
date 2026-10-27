import type { Metadata } from "next";
import Link from "next/link";
import { PolicyContact, PolicyLayout } from "@/components/PolicyLayout";

export const metadata: Metadata = {
  title: "Điều khoản dịch vụ | PostPilot",
  description: "Điều khoản sử dụng dịch vụ PostPilot cho việc kết nối, quản lý và đăng bài tự động lên Facebook Page.",
};

export default function TermsPage() {
  return (
    <PolicyLayout
      title="Điều khoản dịch vụ"
      description="Các điều khoản và quy định khi sử dụng hệ thống quản lý, lập lịch và tự động đăng bài PostPilot trên các nền tảng mạng xã hội."
    >
      <section aria-labelledby="acceptance">
        <h2 id="acceptance">1. Chấp thuận điều khoản</h2>
        <p>Bằng việc truy cập hoặc sử dụng ứng dụng PostPilot tại postpilot.hexta.org, bạn đồng ý tuân thủ các điều khoản dịch vụ này. Nếu bạn không đồng ý với bất kỳ phần nào của điều khoản, vui lòng ngừng sử dụng dịch vụ.</p>
        <p>Nếu có thắc mắc về điều khoản dịch vụ, bạn có thể liên hệ với chúng tôi qua email: <PolicyContact subject="PostPilot - Điều khoản dịch vụ" /></p>
      </section>

      <section aria-labelledby="services-provided">
        <h2 id="services-provided">2. Mô tả dịch vụ</h2>
        <p>PostPilot cung cấp công cụ tự động hóa xuất bản nội dung, hỗ trợ quản lý Fanpage, soạn thảo nội dung, tải lên hình ảnh/video, lên lịch đăng bài và tích hợp công nghệ AI để hỗ trợ sáng tạo nội dung cho các trang mạng xã hội thuộc quyền quản lý của bạn.</p>
      </section>

      <section aria-labelledby="user-responsibilities">
        <h2 id="user-responsibilities">3. Trách nhiệm của người dùng</h2>
        <ul>
          <li><strong>Quyền sở hữu và uỷ quyền:</strong> Bạn cam kết rằng bạn là chủ sở hữu hợp pháp hoặc được uỷ quyền quản trị các Facebook Page mà bạn kết nối vào PostPilot.</li>
          <li><strong>Tuân thủ chính sách nền tảng:</strong> Bạn cam kết tuân thủ Tiêu chuẩn cộng đồng và Chính sách nền tảng của Meta/Facebook khi xuất bản bài viết, video hoặc Reels.</li>
          <li><strong>Nội dung hợp pháp:</strong> Bạn chịu hoàn toàn trách nhiệm đối với toàn bộ nội dung, hình ảnh, video và thông tin đăng tải thông qua hệ thống; không sử dụng hệ thống để phát tán nội dung độc hại, lừa đảo, vi phạm bản quyền hoặc vi phạm pháp luật.</li>
          <li><strong>Bảo mật thông tin:</strong> Bạn có trách nhiệm bảo mật các thông tin đăng nhập, token và tài khoản quản trị của mình.</li>
        </ul>
      </section>

      <section aria-labelledby="meta-compliance">
        <h2 id="meta-compliance">4. Tích hợp với Meta / Facebook</h2>
        <p>Dịch vụ của chúng tôi sử dụng API chính thức của Meta (Graph API). Việc sử dụng của bạn phải tuân thủ:</p>
        <ul>
          <li>Chính sách nền tảng của Meta: <a href="https://developers.facebook.com/terms" target="_blank" rel="noopener noreferrer" className="text-blue-700 underline underline-offset-4 dark:text-blue-300">Meta Platform Terms</a></li>
          <li>Tiêu chuẩn cộng đồng Facebook: <a href="https://transparency.fb.com/policies/community-standards/" target="_blank" rel="noopener noreferrer" className="text-blue-700 underline underline-offset-4 dark:text-blue-300">Facebook Community Standards</a></li>
        </ul>
        <p>Chúng tôi không chịu trách nhiệm trong trường hợp Facebook hạn chế hoặc tạm khoá trang do vi phạm chính sách nội dung từ phía người dùng.</p>
      </section>

      <section aria-labelledby="data-and-privacy">
        <h2 id="data-and-privacy">5. Quyền riêng tư và dữ liệu</h2>
        <p>Cách chúng tôi thu thập, lưu trữ và bảo vệ dữ liệu cá nhân của bạn được quy định chi tiết tại <Link href="/privacy" className="font-medium text-blue-700 underline underline-offset-4 dark:text-blue-300">Chính sách quyền riêng tư</Link>.</p>
        <p>Bạn có thể yêu cầu xóa dữ liệu bất kỳ lúc nào theo <Link href="/data-deletion" className="font-medium text-blue-700 underline underline-offset-4 dark:text-blue-300">Hướng dẫn yêu cầu xóa dữ liệu</Link>.</p>
      </section>

      <section aria-labelledby="disclaimer">
        <h2 id="disclaimer">6. Miễn trừ trách nhiệm</h2>
        <p>Dịch vụ được cung cấp trên cơ sở &quot;nguyên trạng&quot; (as-is). Chúng tôi nỗ lực tối đa để đảm bảo hệ thống hoạt động ổn định và an toàn, nhưng không bảo đảm rằng dịch vụ sẽ không bao giờ bị gián đoạn do sự cố từ nhà cung cấp dịch vụ mạng, thay đổi API từ phía Meta hoặc các trường hợp bất khả kháng.</p>
      </section>

      <section aria-labelledby="changes">
        <h2 id="changes">7. Thay đổi điều khoản</h2>
        <p>Chúng tôi có quyền điều chỉnh hoặc cập nhật Điều khoản dịch vụ này bất cứ lúc nào để phù hợp với quy định pháp luật và sự phát triển của hệ thống. Thay đổi sẽ có hiệu lực ngay khi được đăng tải trên trang web.</p>
      </section>
    </PolicyLayout>
  );
}
