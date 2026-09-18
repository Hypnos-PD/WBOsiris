// Package wbo 只在仓库根提供一份"把卡池嵌进二进制"的入口，
// 桌面客户端（desktop/）用它做到脱机可用：卡牌规则是纯文本，约 4 MB。
package wbo

import "embed"

// Cards 是 cards/**/*.wbo 的只读文件系统（路径形如 cards/10000/10001110.wbo）。
//
//go:embed all:cards
var Cards embed.FS
