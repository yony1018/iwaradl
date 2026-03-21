// classify.go - 文件分类和整理功能
// 该文件包含将下载的 mp4 和 nfo 文件根据作者分类到指定目录的功能

package downloader

import (
	"encoding/json" // 用于解析 JSON 响应
	// 用于解析 NFO 文件的 XML 格式
	"errors" // 错误处理
	"fmt"    // 格式化输出
	"io"     // 输入输出接口
	"io/fs"  // 文件系统接口

	// API 相关功能
	"iwaradl/config" // 配置
	// 配置
	"iwaradl/util" // 工具函数
	"net/http"     // HTTP 请求
	"net/url"

	// URL 解析，用于 proxy
	"os"            // 操作系统接口
	"path/filepath" // 路径处理
	"strings"       // 字符串处理
	"time"          // 时间处理，用于超时

	"github.com/flytam/filenamify" // 文件名安全化
)

// Classify 将源目录中的 mp4 和 nfo 文件根据作者分类到目标目录的子目录中
// sourceDir: 源目录路径，包含待分类的文件
// targetDir: 目标目录路径，分类后的文件将移动到其子目录中
func Classify(sourceDir string, targetDir string) {
	util.DebugLog("Checking source & target dir for classify func")
	// 检查源目录是否存在且为目录
	srcDirInfo, err := os.Stat(sourceDir)
	if err != nil {
		fmt.Println("Failed to read sourceDir", err.Error())
		return
	}
	if !srcDirInfo.IsDir() {
		fmt.Println("sourceDir exists, but NOT A DIR")
		return
	}

	// 检查目标目录是否存在且为目录
	tgtDirInfo, err := os.Stat(targetDir)
	if err != nil {
		fmt.Println("Failed to read targetDir", err.Error())
		return
	}
	if !tgtDirInfo.IsDir() {
		fmt.Println("targetDir exists, but NOT A DIR")
		return
	}

	// 加载目标目录中已存在的创作者目录
	util.DebugLog("Classifying *.mp4 & *.nfo in source dir")
	creatorDirs := loadCreatorDirectories(targetDir)

	// 扫描源目录中的所有 mp4 文件
	mp4Files := []string{}
	err = filepath.Walk(sourceDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// 只收集 mp4 文件（忽略大小写）
		if !info.IsDir() && strings.HasSuffix(strings.ToLower(info.Name()), ".mp4") {
			mp4Files = append(mp4Files, path)
		}
		return nil
	})
	if err != nil {
		fmt.Println("Error scanning source directory:", err)
		return
	}

	moved := 0   // 成功移动的文件数
	skipped := 0 // 跳过的文件数

	// 遍历每个 mp4 文件，进行分类
	for _, mp4Path := range mp4Files {
		// 从 mp4 文件解析作者信息
		author, authorLatestNickName, err := resolveAuthorFromMp4(mp4Path)
		if err != nil {
			fmt.Printf("[WARN] %s auteur not found: %v\n", mp4Path, err)
			continue
		}
		fmt.Printf("Resolved latest author: %s (nickname: %s)\n", author, authorLatestNickName)

		// 查找匹配的创作者目录（严格匹配 username）
		var targetSubDir string
		var foundIndex = -1
		for i, dir := range creatorDirs {
			if dir.userName == author {
				foundIndex = i
				break
			}
		}

		if foundIndex >= 0 {
			// 检查 nickname 是否需要更新
			if creatorDirs[foundIndex].nickName != authorLatestNickName {
				// 重命名目录
				oldPath := creatorDirs[foundIndex].path
				newDirName := formatCreatorDir(authorLatestNickName, author)
				newPath := filepath.Join(targetDir, newDirName)
				if err := os.Rename(oldPath, newPath); err != nil {
					fmt.Printf("[ERROR] Failed to rename directory from %s to %s: %v\n", oldPath, newPath, err)
					skipped++
					continue
				}
				fmt.Printf("[INFO] Renamed directory from %s to %s\n", oldPath, newPath)
				creatorDirs[foundIndex].path = newPath
				creatorDirs[foundIndex].nickName = authorLatestNickName
			}
			targetSubDir = creatorDirs[foundIndex].path
		} else {
			// 没有找到匹配的目录，则跳过该文件
			fmt.Printf("[WARN] No matching creator directory found for author '%s', skipping file: %s\n", author, mp4Path)
			skipped++
			continue
		}

		// 移动 mp4 文件到目标目录
		if err := moveFileToDirectory(mp4Path, targetSubDir); err != nil {
			fmt.Printf("[ERROR] Move mp4 failed: %v\n", err)
			skipped++
			continue
		}

		// 移动对应的 nfo 文件（如果存在）
		nfoPath := strings.TrimSuffix(mp4Path, ".mp4") + ".nfo"
		if _, err := os.Stat(nfoPath); err == nil {
			if err := moveFileToDirectory(nfoPath, targetSubDir); err != nil {
				fmt.Printf("[ERROR] Move nfo failed: %v\n", err)
			}
		}

		moved++
	}

	fmt.Printf("Finished classify: moved %d files, skipped %d files.\n", moved, skipped)
}

// creatorDir 表示一个创作者目录的信息
type creatorDir struct {
	path     string // 目录的完整路径
	userName string // 用户名（小写）
	nickName string // 显示名称
}

// loadCreatorDirectories 加载目标目录下的所有创作者子目录
// root: 目标根目录
// 返回创作者目录列表
func loadCreatorDirectories(root string) []creatorDir {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var dirs []creatorDir
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// 解析目录名，提取用户名和显示名
		username, nickname := parseCreatorDirName(e.Name())
		if username == "" || nickname == "" {
			util.DebugLog("Warning! Unrecognized creator folder format, skipping: %s", e.Name())
			fmt.Printf("Warning! Unrecognized creator folder format, skipping: %s\n", e.Name())
		}
		dirs = append(dirs, creatorDir{path: filepath.Join(root, e.Name()), userName: username, nickName: nickname})
		// fmt.Printf("Loaded creator path: %s (username: %s, nickname: %s)\n", filepath.Join(root, e.Name()), username, nickname)
	}
	return dirs
}

// parseCreatorDirName 解析创作者目录名，提取用户名和显示名
// 只支持严格格式 {nickname}-[username]，不匹配则返回 "",""
func parseCreatorDirName(name string) (username string, nickname string) {
	if name == "" {
		return "", ""
	}

	// 必须以 { 开头，以 ] 结尾
	if !strings.HasPrefix(name, "{") || !strings.HasSuffix(name, "]") {
		return "", ""
	}

	// 查找分隔符 "}-[" 的第一个位置，确保这是 display 与 username 的分界
	sep := "}-["
	idx := strings.Index(name, sep)
	if idx <= 0 {
		return "", ""
	}

	// nickname 在第一个 '{' 和 sep 之间
	if !strings.HasPrefix(name[:idx+1], "{") {
		return "", ""
	}
	nickNamePart := name[1:idx]
	userNamePart := name[idx+len(sep) : len(name)-1]

	if nickNamePart == "" || userNamePart == "" {
		return "", ""
	}

	return userNamePart, nickNamePart
}

// formatCreatorDir 格式化创作者目录名
// display: 显示名称
// username: 用户名
// 返回格式化的目录名，如 {display}-[username]
func formatCreatorDir(display, username string) string {
	display = strings.TrimSpace(display)
	username = strings.TrimSpace(username)
	if display == "" && username == "" {
		return "unknown"
	}
	if display == "" {
		display = username
	}
	if username == "" {
		username = strings.ToLower(strings.ReplaceAll(display, " ", ""))
	}
	// 安全化文件名
	displaySafe, _ := filenamify.Filenamify(display, filenamify.Options{Replacement: "_", MaxLength: 64})
	usernameSafe, _ := filenamify.Filenamify(username, filenamify.Options{Replacement: "_", MaxLength: 64})
	return fmt.Sprintf("{%s}-[%s]", displaySafe, usernameSafe)
}

// IwaraProfileResponse 表示 Iwara API profile 响应的结构
type IwaraProfileResponse struct {
	User struct {
		Name string `json:"name"`
	} `json:"user"`
	Name string `json:"name"`
}

// resolveAuthorFromMp4 从 mp4 文件解析作者信息
// mp4Path: mp4 文件路径
// 返回作者名（nickname）和错误
func resolveAuthorFromMp4(mp4Path string) (string, string, error) {
	// 从文件名提取 username
	base := filepath.Base(mp4Path)
	stem := strings.TrimSuffix(base, ".mp4")

	if !strings.HasPrefix(stem, "[") {
		return "", "", errors.New("filename does not start with '['")
	}

	end := strings.Index(stem, "]")
	if end <= 1 {
		return "", "", errors.New("invalid filename format, no closing ']' found")
	}

	username := stem[1:end]

	// 通过 API 查询 nickname
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// 如果配置了 proxy，使用它
	if config.Cfg.ProxyUrl != "" {
		proxyURL, err := url.Parse(config.Cfg.ProxyUrl)
		if err != nil {
			return "", "", fmt.Errorf("invalid proxy URL: %v", err)
		}
		client.Transport = &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		}
	}

	url := fmt.Sprintf("https://api.iwara.tv/profile/%s", username)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", "", err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("API request failed with status: %d", resp.StatusCode)
	}

	var profile IwaraProfileResponse
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return "", "", err
	}

	// 优先使用 .user.name，如果为空则使用 .name
	nickname := profile.User.Name
	if nickname == "" {
		nickname = profile.Name
	}

	if nickname == "" {
		return "", "", errors.New("nickname not found in API response")
	}

	return username, nickname, nil
}

// moveFileToDirectory 将文件移动到指定目录
// src: 源文件路径
// dstDir: 目标目录路径
// 返回错误
func moveFileToDirectory(src, dstDir string) error {
	filename := filepath.Base(src)
	dst := filepath.Join(dstDir, filename)
	// 如果源和目标相同，不移动
	if strings.EqualFold(filepath.Clean(src), filepath.Clean(dst)) {
		return nil
	}
	// 检查目标文件是否已存在
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("destination file exists: %s", dst)
	}

	// 确保目标目录存在，如果不存在则返回错误
	if _, err := os.Stat(dstDir); os.IsNotExist(err) {
		return fmt.Errorf("destination directory does not exist: %s", dstDir)
	}

	// 尝试重命名（快速移动）
	if err := os.Rename(src, dst); err == nil {
		return nil
	}

	// 如果重命名失败，使用复制+删除
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Remove(src)
}
