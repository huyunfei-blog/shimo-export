package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cavaliergopher/grab/v3"
)

// Config 结构体用于存储配置文件的信息
type Config struct {
	Cookie    string `json:"Cookie"`
	Path      string `json:"Path"`
	Folder    string `json:"Folder"`
	Lasttime  int64  `json:"Lasttime"`
	Sleep     int    `json:"Sleep"`
	Retry     int    `json:"Retry"`
	Recursive bool   `json:"Recursive"`
}

// Item 结构体表示从 API 获取的文件或文件夹
type Item struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	GUID      string `json:"guid"`
	IsFolder  bool   `json:"is_folder"`
	UpdatedAt string `json:"updatedAt"`
}

type Comment struct {
	ID               int    `json:"id"`
	TargetGuid       string `json:"targetGuid"`
	UserID           int    `json:"userId"`
	CreatedAt        string `json:"createdAt"`
	UpdatedAt        string `json:"updatedAt"`
	CommentGuid      string `json:"commentGuid"`
	TargetID         int    `json:"targetId"`
	TargetType       int    `json:"targetType"`
	Content          string `json:"content"`
	SelectionTitle   string `json:"selectionTitle"`
	SelectionGuid    string `json:"selectionGuid"`
	Like             int    `json:"like"`
	IsLike           int    `json:"isLike"`
	LastLike         string `json:"lastLike"`
	ReplyTo          string `json:"replyTo"`
	SelectionContent string `json:"selectionContent"`
	HasRead          bool   `json:"hasRead"`
	User             struct {
		ID     int    `json:"id"`
		Name   string `json:"name"`
		Avatar string `json:"avatar"`
	} `json:"User"`
}

type CommentGroup struct {
	SelectionGuid    string `json:"selectionGuid"`
	SelectionContent string `json:"selectionContent"`
	Comments         []struct {
		CommentGuid string `json:"commentGuid"`
		Content     string `json:"content"`
		Name        string `json:"name"`
		ReplyTo     string `json:"replyTo"`
	} `json:"comments"`
}

// 全局变量
var (
	config         Config
	headersOptions map[string]string
	desktopHeaders map[string]string
	localFileMap   map[string]os.FileInfo
)

type CommentGroups []CommentGroup

func main() {
	// 加载配置文件
	err := loadConfig("config.json")
	if err != nil {
		fmt.Println("加载配置文件出错：", err)
		return
	}

	configModTime, err := getConfigModTime("config.json")
	if err != nil {
		fmt.Println("获取配置文件修改时间出错：", err)
		return
	}

	// 设置请求头
	setupHeaders()

	go func() {
		for {
			time.Sleep(2 * time.Minute)

			newModTime, err := getConfigModTime("config.json")
			if err != nil {
				fmt.Println("获取配置文件修改时间出错：", err)
				continue
			}

			// 如果配置文件已修改
			if newModTime.After(configModTime) {
				fmt.Println("检测到 config.json 已修改，重新加载配置。")
				err = loadConfig("config.json")
				if err != nil {
					fmt.Println("重新加载配置文件出错：", err)
					continue
				}
				// 更新配置文件修改时间
				configModTime = newModTime
				// 重新设置请求头
				setupHeaders()
			}
		}
	}()

	// 获取本地文件列表
	localFileMap = getLocalFileMap(config.Path)

	// 开始同步文件
	err = syncFiles(config.Folder, config.Path)
	if err != nil {
		fmt.Println("同步文件出错：", err)
	}

	select {}
}

func getConfigModTime(configFile string) (time.Time, error) {
	fileInfo, err := os.Stat(configFile)
	if err != nil {
		return time.Time{}, err
	}
	return fileInfo.ModTime(), nil
}

// 加载配置文件
func loadConfig(configFile string) error {
	configData, err := ioutil.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("无法读取配置文件：%v", err)
	}
	err = json.Unmarshal(configData, &config)
	if err != nil {
		return fmt.Errorf("解析配置文件出错：%v", err)
	}
	fmt.Println("配置：", config)
	return nil
}

// 设置请求头
func setupHeaders() {
	headersOptions = map[string]string{
		"Cookie":  config.Cookie,
		"Referer": "https://shimo.im/folder/123",
	}
	desktopHeaders = map[string]string{
		"Cookie":  config.Cookie,
		"Referer": "https://shimo.im/desktop",
	}
}

// 获取本地文件列表，返回文件路径与文件信息的映射
func getLocalFileMap(dirPath string) map[string]os.FileInfo {
	fileMap := make(map[string]os.FileInfo)
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// 过滤隐藏文件
		if strings.HasPrefix(info.Name(), ".") {
			return nil
		}
		if !info.IsDir() {
			relPath, _ := filepath.Rel(dirPath, path)
			fileMap[relPath] = info
		}
		return nil
	})
	if err != nil {
		fmt.Println("获取本地文件列表出错：", err)
	}
	return fileMap
}

// 同步文件和文件夹
func syncFiles(folder string, basePath string) error {
	time.Sleep(time.Duration(config.Sleep) * time.Millisecond)

	// 从 API 获取文件列表
	files, err := getFileListFromAPI(folder)
	if err != nil {
		return fmt.Errorf("获取文件列表出错：%v", err)
	}

	for _, file := range files {
		err = processItem(file, basePath)
		if err != nil {
			fmt.Println("处理项出错：", err)
			continue
		}
	}
	return nil
}

// 保存文件列表到指定的 JSON 文件
func saveItemsToFile(items []Item, filename string) error {
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化文件列表失败：%v", err)
	}
	err = ioutil.WriteFile(filename, data, 0644)
	if err != nil {
		return fmt.Errorf("写入文件 %s 失败：%v", filename, err)
	}
	fmt.Printf("已将文件列表保存到 %s\n", filename)
	return nil
}

// 从指定的 JSON 文件中读取文件列表
func readItemsFromFile(filename string) ([]Item, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("读取文件 %s 失败：%v", filename, err)
	}
	var items []Item
	err = json.Unmarshal(data, &items)
	if err != nil {
		return nil, fmt.Errorf("解析文件 %s 失败：%v", filename, err)
	}
	return items, nil
}

// 从 API 获取文件列表
func getFileListFromAPI(folder string) ([]Item, error) {
	params := map[string]string{
		"collaboratorCount": "true",
	}
	if folder != "" {
		params["folder"] = folder
	}

	headers := headersOptions
	if folder == "" {
		headers = desktopHeaders
	}

	url := "https://shimo.im/lizard-api/files"

	body, err := makeGetRequest(url, params, headers)
	if err != nil {
		return nil, err
	}

	var items []Item
	err = json.Unmarshal(body, &items)
	if err != nil {
		return nil, fmt.Errorf("解析响应失败：%v", err)
	}

	return items, nil
}

// 处理单个文件或文件夹
func processItem(item Item, basePath string) error {
	atime, err := time.Parse(time.RFC3339, item.UpdatedAt)
	if err != nil {
		fmt.Println("解析时间失败：", item.Name, err)
		return nil
	}

	if atime.Unix() <= config.Lasttime {
		fmt.Println("已同步至上次更新时间，结束。")
		os.Exit(0)
	}

	name := sanitizeFileName(item.Name)

	if item.IsFolder {
		// 处理文件夹
		if config.Recursive {
			newBasePath := filepath.Join(basePath, name)
			os.MkdirAll(newBasePath, os.ModePerm)
			return syncFiles(item.GUID, newBasePath)
		}
		return nil
	}

	// 处理文件
	return processFile(item, basePath, name)
}

// 处理文件的下载和转换
func processFile(item Item, basePath, name string) error {
	typ := getType(item)
	if typ == "1" {
		return nil // 不支持的类型
	}

	relFilePath := filepath.Join(name + "." + typ)
	localFilePath := filepath.Join(basePath, relFilePath)

	// 检查本地文件是否存在且为最新
	if info, exists := localFileMap[relFilePath]; exists {
		itemTime, _ := time.Parse(time.RFC3339, item.UpdatedAt)
		if !itemTime.After(info.ModTime()) {
			fmt.Println("跳过已是最新的文件：", localFilePath)
			return nil
		}
		fmt.Println("更新文件：", localFilePath)
	}

	// 下载并转换文件
	for retry := 0; retry <= config.Retry; retry++ {
		if retry > 0 {
			fmt.Printf("重试第 %d 次：%s\n", retry, name)
			time.Sleep(time.Duration(config.Sleep*2) * time.Millisecond)
		}

		err := downloadAndConvertFile(item, basePath, name)
		if err == nil {
			// 下载评论
			err = downloadComments(item.GUID, basePath, name)
			if err == nil {
				return nil
			}
			fmt.Println("下载评论失败：", err)
		} else {
			fmt.Println("下载或转换文件出错：", err)
		}
	}

	return fmt.Errorf("多次重试后仍然失败：%s", name)
}

func downloadComments(fileGuid, basePath, name string) error {
	getCommentPath := fmt.Sprintf("https://shimo.im/lizard-api/files/%s/comments", fileGuid)
	params := map[string]string{}
	headers := headersOptions

	// 获取评论数据
	body, err := makeGetRequest(getCommentPath, params, headers)
	if err != nil {
		return fmt.Errorf("获取评论数据失败：%v", err)
	}

	// 解析评论数据
	var comments []Comment
	err = json.Unmarshal(body, &comments)
	if err != nil {
		return fmt.Errorf("解析评论数据失败：%v", err)
	}

	// 按照 selectionGuid 分组
	commentGroupMap := make(map[string]*CommentGroup)
	for _, comment := range comments {
		selectionGuid := comment.SelectionGuid
		if selectionGuid == "" {
			continue // 跳过没有 selectionGuid 的评论
		}
		if _, exists := commentGroupMap[selectionGuid]; !exists {
			commentGroupMap[selectionGuid] = &CommentGroup{
				SelectionGuid:    selectionGuid,
				SelectionContent: comment.SelectionContent,
				Comments: []struct {
					CommentGuid string `json:"commentGuid"`
					Content     string `json:"content"`
					Name        string `json:"name"`
					ReplyTo     string `json:"replyTo"`
				}{},
			}
		}
		// 添加评论到对应的分组
		commentGroupMap[selectionGuid].Comments = append(commentGroupMap[selectionGuid].Comments, struct {
			CommentGuid string `json:"commentGuid"`
			Content     string `json:"content"`
			Name        string `json:"name"`
			ReplyTo     string `json:"replyTo"`
		}{
			CommentGuid: comment.CommentGuid,
			Content:     comment.Content,
			Name:        comment.User.Name,
			ReplyTo:     comment.ReplyTo,
		})
	}

	// 将分组转换为切片
	var commentGroups []CommentGroup
	for _, group := range commentGroupMap {
		commentGroups = append(commentGroups, *group)
	}

	commentDir := filepath.Join(basePath, name)

	// 生成评论文件路径
	commentFilePath := filepath.Join(commentDir, "comments.json")

	// 将数据写入文件
	commentData, err := json.MarshalIndent(commentGroups, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化评论数据失败：%v", err)
	}

	err = ioutil.WriteFile(commentFilePath, commentData, 0644)
	if err != nil {
		return fmt.Errorf("写入评论文件失败：%v", err)
	}

	fmt.Printf("已保存评论数据到：%s\n", commentFilePath)
	return nil
}

// 下载并转换文件
func downloadAndConvertFile(item Item, basePath, name string) error {
	time.Sleep(time.Duration(config.Sleep) * time.Millisecond)

	// 获取下载链接
	downloadURL, err := getDownloadURL(item)
	if err != nil {
		return err
	}

	// 下载文件
	fileExt := "docx" // 下载为 .docx 以便转换为 Markdown
	docxPath := filepath.Join(basePath, fmt.Sprintf("%s.%s", name, fileExt))
	err = downloadFile(downloadURL, docxPath)
	if err != nil {
		return fmt.Errorf("下载文件出错：%v", err)
	}

	// 转换为 Markdown
	mdPath := filepath.Join(basePath, fmt.Sprintf("%s.md", name))
	err = convertDocxToMarkdown(docxPath, mdPath)
	if err != nil {
		return fmt.Errorf("转换为 Markdown 出错：%v", err)
	}

	// 删除原始的 .docx 文件
	err = os.Remove(docxPath)
	if err != nil {
		fmt.Println("警告：无法删除文件：", docxPath)
	}

	return nil
}

// 获取文件的下载链接
func getDownloadURL(item Item) (string, error) {
	if isDirectDownloadType(item.Type) {
		return fmt.Sprintf("https://shimo.im/lizard-api/files/%s/download", item.GUID), nil
	}

	typ := getType(item)
	url := fmt.Sprintf("https://shimo.im/lizard-api/files/%s/export", item.GUID)

	params := map[string]string{
		"type":       typ,
		"file":       item.GUID,
		"returnJson": "1",
		"name":       item.Name,
		"isAsync":    "0",
	}

	headers := headersOptions

	body, err := makeGetRequest(url, params, headers)
	if err != nil {
		return "", fmt.Errorf("请求导出出错：%v", err)
	}

	var responseData map[string]interface{}
	err = json.Unmarshal(body, &responseData)
	if err != nil {
		return "", fmt.Errorf("解析导出响应失败：%v", err)
	}

	fmt.Println(url, responseData)
	if val, ok := responseData["redirectUrl"].(string); ok {
		return val, nil
	} else if data, ok := responseData["data"].(map[string]interface{}); ok {
		if url, ok := data["downloadUrl"].(string); ok {
			return url, nil
		}
	}

	if responseData["errorCode"].(float64) == 110002 {
		fmt.Println("接口频繁了。")
		re := regexp.MustCompile(`\d+`)
		match := re.FindString(responseData["error"].(string))

		if match != "" {
			// 将字符串转换为整数
			seconds, err := strconv.Atoi(match)
			if err != nil {
				fmt.Println("转换数字时出错：", err)
			}
			fmt.Println("提取的数字是：", seconds)
			time.Sleep(time.Duration(seconds) * time.Second)
		} else {
			fmt.Println("未找到数字。")
		}
	}

	return "", fmt.Errorf("在响应中未找到下载链接")
}

// 发送 GET 请求
func makeGetRequest(url string, params, headers map[string]string) ([]byte, error) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败：%v", err)
	}

	// 设置查询参数
	q := req.URL.Query()
	for k, v := range params {
		q.Add(k, v)
	}
	req.URL.RawQuery = q.Encode()

	// 设置请求头
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// 发送请求
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("发送请求失败：%v", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败：%v", err)
	}

	return body, nil
}

// 下载文件
func downloadFile(url string, filepath string) error {
	client := grab.NewClient()
	req, _ := grab.NewRequest(filepath, url)
	req.HTTPRequest.Header.Set("Cookie", config.Cookie)
	req.HTTPRequest.Header.Set("Referer", "https://shimo.im/folder/123")

	// 开始下载
	resp := client.Do(req)
	fmt.Printf("正在下载 %s...\n", filepath)

	// 等待下载完成
	return resp.Err()
}

// 将 .docx 文件转换为 .md 文件
func convertDocxToMarkdown(docxPath string, mdPath string) error {
	// 确保 Pandoc 已安装，并在系统 PATH 中可用

	// 获取 Markdown 文件所在的目录和文件名
	mdDir := filepath.Dir(mdPath)
	mdFileName := filepath.Base(mdPath)
	imageDir := strings.TrimSuffix(mdFileName, ".md") // 图片将存放在以文章标题命名的文件夹中

	// 创建图片目录
	imageDirPath := filepath.Join(mdDir, imageDir)
	os.MkdirAll(imageDirPath, os.ModePerm)

	// 设置 Pandoc 命令
	cmd := exec.Command("pandoc", "-s", docxPath, "-t", "markdown", "-o", mdFileName, "--extract-media", imageDir)
	cmd.Dir = mdDir // 设置工作目录为 Markdown 文件所在目录
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// 运行 Pandoc 命令
	err := cmd.Run()
	if err != nil {
		return err
	}

	fmt.Printf("已转换为 Markdown：%s\n", mdPath)
	return nil
}

// 清理文件名中的非法字符
func sanitizeFileName(fileName string) string {
	// 替换非法字符为 '-'
	reg := regexp.MustCompile(`[\\/:*?"<>|]`)
	fileName = reg.ReplaceAllString(fileName, "-")
	// 删除控制字符
	regControl := regexp.MustCompile(`[\x00-\x1f]`)
	fileName = regControl.ReplaceAllString(fileName, "")
	return fileName
}

// 获取文件类型对应的扩展名
func getType(item Item) string {
	switch item.Type {
	case "docx", "doc", "pptx", "ppt", "pdf":
		return item.Type
	case "newdoc", "document", "modoc":
		return "docx"
	case "sheet", "mosheet", "spreadsheet", "table":
		return "xlsx"
	case "slide", "presentation":
		return "pptx"
	case "mindmap":
		return "xmind"
	default:
		fmt.Printf("[错误] %s 不支持的类型: %s\n", item.Name, item.Type)
		return "1"
	}
}

// 判断是否为直接下载类型
func isDirectDownloadType(itemType string) bool {
	switch itemType {
	case "docx", "doc", "pptx", "ppt", "pdf":
		return true
	default:
		return false
	}
}
