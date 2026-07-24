package appdata

import (
	"os"
	"path/filepath"
	"time"
)

const defaultHistoryRetentionDays = 30

// PruneOldHistory 删除 history 目录下超过 retentionDays 天未修改的 session 目录。
// 只处理 UUID 形式的子目录，跳过 usage.json 等文件。
func PruneOldHistory(retentionDays int) error {
	root := HistoryRootPath()
	cutoff := time.Now().AddDate(0, 0, -retentionDays)

	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sessionDir := filepath.Join(root, entry.Name())
		// 用 context.json 的 mtime 判断最后活跃时间；
		// 文件不存在时回退到目录 mtime。
		mtime := dirOrFileMtime(sessionDir, "context.json")
		if mtime.Before(cutoff) {
			_ = os.RemoveAll(sessionDir)
		}
	}
	return nil
}

func dirOrFileMtime(dir string, filename string) time.Time {
	if info, err := os.Stat(filepath.Join(dir, filename)); err == nil {
		return info.ModTime()
	}
	if info, err := os.Stat(dir); err == nil {
		return info.ModTime()
	}
	return time.Time{}
}
