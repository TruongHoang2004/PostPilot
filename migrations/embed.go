package migrations

import "embed"

// FS nhúng toàn bộ các file SQL migration vào thẳng trong binary Golang khi build.
// Nhờ đó container/binary chạy migration không bị phụ thuộc vào đường dẫn file bên ngoài.
//
//go:embed *.sql
var FS embed.FS
