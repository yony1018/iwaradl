package downloader

import (
	"fmt"
	"iwaradl/api"
	"iwaradl/config"
	"iwaradl/util"
	"os"
	"path/filepath"
	"strings"
)

func ExtractCreatorUsernameFromDirName(dirName string) string {
	username, _ := parseCreatorDirName(dirName)
	if username != "" {
		return username
	}
	trimmed := strings.TrimSpace(dirName)
	if trimmed == "" {
		return ""
	}
	if strings.Contains(trimmed, "-[") && strings.Contains(trimmed, "]") {
		idx1 := strings.LastIndex(trimmed, "-[")
		idx2 := strings.LastIndex(trimmed, "]")
		if idx1 >= 0 && idx2 > idx1 {
			return strings.ToLower(strings.TrimSpace(trimmed[idx1+2 : idx2]))
		}
	}
	return strings.ToLower(strings.ReplaceAll(trimmed, " ", ""))
}

func SyncCreatorsVideos(creatorRoot string, host string) {
	util.DebugLog("Start syncing creator videos from root: %s, host: %s", creatorRoot, host)

	rootInfo, err := os.Stat(creatorRoot)
	if err != nil {
		fmt.Println("creator root not found", err)
		return
	}
	if !rootInfo.IsDir() {
		fmt.Println("creator root is not directory")
		return
	}

	entries, err := os.ReadDir(creatorRoot)
	if err != nil {
		fmt.Println("Failed to read creator root", err)
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		creatorName := ExtractCreatorUsernameFromDirName(entry.Name())
		if creatorName == "" {
			fmt.Printf("[WARN] skip empty creator directory name: %s\n", entry.Name())
			continue
		}

		destDir := filepath.Join(creatorRoot, entry.Name())
		fmt.Printf("[INFO] Sync creator %s into %s\n", creatorName, destDir)

		videoList := api.GetVideoListByUser(creatorName, host)
		if len(videoList) == 0 {
			fmt.Printf("[WARN] no videos found for creator %s\n", creatorName)
			continue
		}

		VidList = VidList[:0]
		for _, v := range videoList {
			VidList = append(VidList, v.Id+"@"+host)
		}

		opts := DownloadOptions{RootDir: destDir, UseSubDir: false, UseSubDirSet: true}
		failed := ConcurrentDownloadWithOptions(opts)
		if failed > 0 {
			fmt.Printf("[WARN] creator %s: %d videos failed\n", creatorName, failed)
		}
	}

	fmt.Println("Sync creators videos finished.")

	// restore config rootDir when done
	config.Cfg.RootDir = creatorRoot
}
