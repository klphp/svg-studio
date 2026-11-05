package services

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// FileService 把生成结果保存到磁盘。
//
// 为什么不让前端用 Blob + <a download>：那是浏览器网页的做法，Wails 的
// WebView 里点击后通常没有任何反应 —— 用户点了下载图标，屏幕上什么都不会
// 发生，也没有任何报错。桌面应用该走原生保存对话框，用户能选目录、能看到
// 文件存到哪里了。
type FileService struct{}

func NewFileService() *FileService { return &FileService{} }

// SaveText 弹出原生保存对话框，把内容写到用户选的位置。
//
// filterName/filterPattern 决定对话框里的文件类型过滤，例如
// ("SVG 图片", "*.svg")。用户取消时返回空字符串且没有错误 —— 取消不是失败，
// 前端不该把它当异常处理。
func (s *FileService) SaveText(content, defaultName, filterName, filterPattern string) (string, *ErrorInfo) {
	app := application.Get()
	if app == nil {
		return "", &ErrorInfo{Kind: "internal", Message: "应用尚未就绪，无法打开保存对话框。"}
	}

	if strings.TrimSpace(content) == "" {
		return "", &ErrorInfo{Kind: "config", Message: "没有可保存的内容。"}
	}

	// SaveFileDialogStruct 没有 SetTitle 这类逐项 setter，只能整块给 options。
	opts := &application.SaveFileDialogOptions{
		Title:    "保存",
		Filename: sanitizeFilename(defaultName),
	}
	if filterName != "" && filterPattern != "" {
		opts.Filters = []application.FileFilter{
			{DisplayName: filterName, Pattern: filterPattern},
		}
	}

	dlg := app.Dialog.SaveFile()
	dlg.SetOptions(opts)

	path, err := dlg.PromptForSingleSelection()
	if err != nil {
		// 取消在选择框里通常表现为错误或空路径。区分「用户主动取消」和
		// 「真的出错」意义不大 —— 两者都不该弹错误提示，记一条日志即可。
		slog.Debug("保存对话框关闭", "err", err)
		return "", nil
	}
	if strings.TrimSpace(path) == "" {
		return "", nil // 用户取消
	}

	// 用户可能没写扩展名（尤其是手动改名时），补上，
	// 否则双击打不开对应的查看器。
	if ext := filepath.Ext(path); ext == "" && filterPattern != "" {
		if e := strings.TrimPrefix(filterPattern, "*"); e != "" {
			path += e
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", &ErrorInfo{Kind: "io", Message: "创建目录失败：" + err.Error()}
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", &ErrorInfo{Kind: "io", Message: "写入文件失败：" + err.Error()}
	}

	slog.Info("已保存文件", "path", path, "bytes", len(content))
	return path, nil
}

// sanitizeFilename 去掉文件名里各平台不允许的字符。
//
// 默认文件名可能带用户输入的内容（比如从提示词生成的名字），斜杠、
// 冒号之类的字符会让保存直接失败，所以先洗一遍。
func sanitizeFilename(name string) string {
	if strings.TrimSpace(name) == "" {
		return "untitled"
	}
	bad := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|", "\x00"}
	out := name
	for _, b := range bad {
		out = strings.ReplaceAll(out, b, "-")
	}
	out = strings.TrimSpace(out)
	if out == "" {
		out = "untitled"
	}
	if len(out) > 120 {
		out = out[:120]
	}
	return out
}

// OpenInFolder 在文件管理器里定位到某个文件（保存成功后方便用户查看）。
func (s *FileService) OpenInFolder(path string) *ErrorInfo {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		return &ErrorInfo{Kind: "io", Message: fmt.Sprintf("文件不存在：%s", path)}
	}
	// macOS 和 Windows 都能用 open / explorer 定位；Wails 没提供现成的
	// reveal 接口，用 Browser.OpenURL 打开文件所在目录即可。
	app := application.Get()
	if app == nil {
		return nil
	}
	dir := filepath.Dir(path)
	if err := app.Browser.OpenURL("file://" + dir); err != nil {
		slog.Debug("打开目录失败", "err", err, "dir", dir)
	}
	return nil
}
