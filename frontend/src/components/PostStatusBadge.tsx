import { PostStatus } from "@/lib/types";
import { postStatusLabels } from "@/lib/schedule";

export function PostStatusBadge({ status }: { status: PostStatus }) {
  const tone = status === "FAILED" || status === "REJECTED" ? "danger" : status === "PUBLISHED" ? "success" : status === "PENDING_APPROVAL" ? "warning" : ["SCHEDULED", "QUEUED", "PROCESSING"].includes(status) ? "info" : "neutral";
  return <span className={`dispatch-status ${tone}`}><span aria-hidden="true" />{postStatusLabels[status] || status}</span>;
}
