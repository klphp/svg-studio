package main

import (
	"context"
	"embed"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/klphp/svg-studio/internal/update"
)

// Version 是当前打包版本号，必须和 release tag 对应（tag 上的 v 前缀由
// provider 自己剥掉，这里不要带 v）。
//
// 关键：updater 是拿这个值去和最新的 release tag 比大小，所以发版时一定要改
// 这里。本地调试想看"能不能检测到新版"，就把它改小一版（比如发 0.0.2 的
// release，这里留 0.0.1），否则永远看不到更新提示。
const Version = "0.0.1"

// updateRepo 是自更新的 GitHub 仓库（owner/repo）。
// 试水方案用 github provider：免费、零服务器、零签名成本。
const updateRepo = "klphp/svg-studio"

// Wails uses Go's `embed` package to embed the frontend files into the binary.
// Any files in the frontend/dist folder will be embedded into the binary and
// made available to the frontend.
// See https://pkg.go.dev/embed for more information.

//go:embed all:frontend/dist
var assets embed.FS

// setupLogging 把 log 和 slog 同时写到 stderr 和日志文件。
//
// 为什么必须要文件：Windows 构建用 -ldflags="-H windowsgui"，进程没有控制台，
// stderr 是无效句柄 —— 所有 log.Printf / slog.Info 直接蒸发。自更新在 Windows
// 上又是全自动静默执行的（下载、替换、重启一气呵成），没有日志文件就完全无法
// 判断它到底跑没跑、失败在哪一步。
//
// 位置：macOS ~/Library/Application Support/svg-studio/app.log
//       Windows %APPDATA%\svg-studiopp.log
//
// 拿不到目录就算了，不能因为日志写不了就让应用起不来。
func setupLogging() {
	dir, err := os.UserConfigDir()
	if err != nil {
		return
	}
	logDir := filepath.Join(dir, "svg-studio")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(logDir, "app.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}

	w := io.MultiWriter(os.Stderr, f)
	log.SetOutput(w)
	slog.SetDefault(slog.New(slog.NewTextHandler(w, nil)))
}

// cleanupUpdateLeftovers 清掉自更新在可执行文件旁边留下的残留。
//
// Wails 的 Windows 更新走 rename-aside：先把正在运行的 exe 改名成
// svg-studio.exe.old.<纳秒时间戳>，再把新 exe 放进去，重启后由 helper 删掉。
// 实测 helper 常常没能删成（文件还映射着），于是用户下次打开软件就会看到
// 目录里凭空多出一个 .old.* 文件 —— 观感很差，像是被什么远程东西动过。
//
// Wails 自己也有 sweepRenameAsides()，但它只在「下一次更新时」才扫，正常
// 使用下几个月都不会触发。所以这里启动时主动扫一次。
//
// staging 目录（wails-update-*）同理：它是下载时建在 exe 同目录的临时目录，
// 因为 os.Rename 不能跨文件系统，这个位置是 Wails 的硬约束挪不走，只能扫掉。
//
// 全部 best-effort：删不掉（还占着）就放过，下次启动再试。绝不能因为清理
// 失败影响启动。
func cleanupUpdateLeftovers() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	// 解析符号链接，拿到真实路径（有些启动方式下 os.Executable 给的是链接）
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}

	// .old.* 兄弟文件
	if matches, err := filepath.Glob(exe + ".old.*"); err == nil {
		for _, m := range matches {
			if os.Remove(m) == nil {
				slog.Info("清理上次更新的残留", "path", filepath.Base(m))
			}
		}
	}

	// wails-update-* staging 目录
	dir := filepath.Dir(exe)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "wails-update-") {
			if os.RemoveAll(filepath.Join(dir, e.Name())) == nil {
				slog.Info("清理更新临时目录", "path", e.Name())
			}
		}
	}
}

func init() {
	// Register a custom event whose associated data type is string.
	// This is not required, but the binding generator will pick up registered events
	// and provide a strongly typed JS/TS API for them.
	application.RegisterEvent[string]("time")
}

// main function serves as the application's entry point. It initializes the application, creates a window,
// and starts a goroutine that emits a time-based event every second. It subsequently runs the application and
// logs any error that might occur.
func main() {
	setupLogging()
	cleanupUpdateLeftovers()

	// Create a new Wails application by providing the necessary options.
	// Variables 'Name' and 'Description' are for application metadata.
	// 'Assets' configures the asset server with the 'FS' variable pointing to the frontend files.
	// 'Bind' is a list of Go struct instances. The frontend has access to the methods of these instances.
	// 'Mac' options tailor the application when running an macOS.
	app := application.New(application.Options{
		Name:        "svg-studio",
		Description: "A demo of using raw HTML & CSS",
		Services: []application.Service{
			application.NewService(&GreetService{}),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	// Create a new window with the necessary options.
	// 'Title' is the title of the window.
	// 'Mac' options tailor the window when running on macOS.
	// 'BackgroundColour' is the background colour of the window.
	// 'URL' is the URL that will be loaded into the webview.
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "Window 1",
		// Window sized to the golden ratio (1000 / 618 ≈ 1.618).
		Width:  1800,
		Height: 1400,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(6, 7, 15),
		URL:              "/",
	})

	log.Printf("=== svg-studio %s 启动 ===", Version)

	// ── 自更新 ──
	//
	// Setup 必须在 application.New 之后调用（它要 app.Updater）。
	// 这里做失败降级：初始化不了就只记日志，绝不能让更新模块把应用拖住 ——
	// 用户装这个软件是为了画 SVG，不是为了更新。
	updateMgr := update.New(app, update.Config{
		Enabled:    true,
		Provider:   "github",
		Repository: updateRepo,
	}, Version, "https://github.com/"+updateRepo+"/releases/latest")

	if err := updateMgr.Setup(); err != nil {
		slog.Warn("自更新未启用", "err", err)
	}

	// Create a goroutine that emits an event containing the current time every second.
	// The frontend can listen to this event and update the UI accordingly.
	go func() {
		for {
			now := time.Now().Format(time.RFC1123)
			app.Event.Emit("time", now)
			time.Sleep(time.Second)
		}
	}()

	// 启动后异步查一次更新。放 goroutine 里是为了不阻塞启动 —— 网络慢的话
	// 同步查会让窗口卡在启动阶段，体感很差。
	//
	// 检查失败静默处理（断网、GitHub 被墙、API 限流都算正常情况），
	// 用户下次开应用会再查。
	go func() {
		// 等窗口真正起来再查，避免和首屏渲染抢资源
		time.Sleep(5 * time.Second)

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		rel, err := updateMgr.Check(ctx)
		if err != nil {
			slog.Warn("检查更新失败", "err", err)
			return
		}
		if rel == nil || !rel.Available {
			slog.Info("已是最新版本", "current", Version)
			return
		}

		// 用 slog 而不是 log.Printf：log 包在这套设置下会被桥接到 slog，
		// 而桥接后格式串里的 %s 不会展开，参数变成被丢弃的 attr —— 上一版
		// 日志里出现 "发现新版本 " 后面空空的就是这个原因，看起来像
		// Release.Version 是空的，其实只是没打印出来。
		slog.Info("发现新版本",
			"current", Version,
			"latest", rel.Version,
			"url", rel.ReleaseURL,
			"autoInstall", rel.AutoInstall,
		)

		// macOS 不自动装：Wails 的替换流程只认 .zip，不支持 dmg，
		// 而 dmg 才是 macOS 的分发惯例。这里只提示，让用户自己去下。
		// Windows 才走自动安装。
		if err := updateMgr.Install(ctx); err != nil {
			slog.Warn("安装更新失败", "err", err, "autoInstall", rel.AutoInstall)
		}
	}()

	// Run the application. This blocks until the application has been exited.
	err := app.Run()

	// If an error occurred while running the application, log it and exit.
	if err != nil {
		log.Fatal(err)
	}
}
