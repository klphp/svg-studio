package main

import (
	"context"
	"embed"
	"errors"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/klphp/svg-studio/internal/services"
	"github.com/klphp/svg-studio/internal/update"
)

// Version 是当前打包版本号，必须和 release tag 对应（tag 上的 v 前缀由
// provider 自己剥掉，这里不要带 v）。
//
// 关键：updater 是拿这个值去和最新的 release tag 比大小，所以发版时一定要改
// 这里。本地调试想看"能不能检测到新版"，就把它改小一版（比如发 0.0.2 的
// release，这里留 0.0.1），否则永远看不到更新提示。
const Version = "0.0.4"

// updateRepo 是自更新的 GitHub 仓库（owner/repo）。
// 试水方案用 github provider：免费、零服务器、零签名成本。
const updateRepo = "klphp/svg-studio"

// Wails uses Go's `embed` package to embed the frontend files into the binary.
// Any files in the frontend/dist folder will be embedded into the binary and
// made available to the frontend.
// See https://pkg.go.dev/embed for more information.

//go:embed all:frontend/dist
var assets embed.FS

// licenseText 是 MIT 许可证全文，嵌进二进制。
//
// 为什么要嵌：MIT 的原文要求「版权声明和许可证须包含在软件的所有副本或
// 实质性部分中」。我们的分发形态是两个单文件，
//   · Windows 是裸 exe 打包成 zip，而 Wails 的 updater 要求 zip 里只能有
//     一个顶层条目 —— 塞不进多余的 LICENSE 文件
//   · macOS 是 .app bundle，可以放进 Contents/Resources
// 所以最稳妥的做法是让许可证成为二进制的一部分，任何一份拷贝都自带它。
// macOS 那边还会额外在打包时往 Contents/Resources 放一份，
// 这样用户右键「显示包内容」能直接看到。
//
//go:embed LICENSE
var licenseText string

// ensureLicenseFile 把许可证写到用户配置目录，已存在则跳过。
//
// 光 embed 不够：如果没人读这个变量，Go 的 linker 会把整个字符串优化掉
// （实测 strings 搜不到 MIT 正文）。而 MIT 要求许可证随副本分发，所以
// 把它落成用户目录下的一个真实文件 —— 既保证内容真在程序里，用户也能
// 直接打开看。macOS 打包时还会另外往 Contents/Resources 放一份。
func ensureLicenseFile() {
	dir, err := os.UserConfigDir()
	if err != nil {
		return
	}
	appDir := filepath.Join(dir, "svg-studio")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return
	}
	path := filepath.Join(appDir, "LICENSE")
	if _, err := os.Stat(path); err == nil {
		return // 已经有就跳过，别每次启动都写
	}
	if err := os.WriteFile(path, []byte(licenseText), 0o644); err != nil {
		slog.Warn("写许可证文件失败", "err", err)
	}
}

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

	// wails-update-* staging 目录。
	//
	// 这里必须按时间过滤，只删「老的」。原因是 Restart() 会 spawn 一个
	// helper 进程（还是这个 exe），helper 启动时同样会跑到这个函数 ——
	// 如果无脑删，就会把 helper 正在等着安装的 staging 目录一起删掉，
	// 更新直接失败：
	//   updater: helper did not signal readiness: staged artifact is
	//   unavailable: stat .../wails-update-187661749/svg-studio.app
	// 这个 bug 是我自己引入的，实测才暴露出来。
	//
	// 10 分钟足够覆盖任何正常下载；超过这个时间还留着的，肯定是上次更新
	// 中途崩掉或断电的残骸，删掉是安全的。
	dir := filepath.Dir(exe)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "wails-update-") {
			continue
		}
		info, err := e.Info()
		if err != nil || time.Since(info.ModTime()) < 10*time.Minute {
			continue // 可能是正在进行的更新，别动
		}
		if os.RemoveAll(filepath.Join(dir, e.Name())) == nil {
			slog.Info("清理遗留的更新临时目录", "path", e.Name())
		}
	}
}

// main function serves as the application's entry point. It initializes the application, creates a window,
// and starts a goroutine that emits a time-based event every second. It subsequently runs the application and
// logs any error that might occur.
func main() {
	setupLogging()
	cleanupUpdateLeftovers()
	ensureLicenseFile()

	// Create a new Wails application by providing the necessary options.
	// Variables 'Name' and 'Description' are for application metadata.
	// 'Assets' configures the asset server with the 'FS' variable pointing to the frontend files.
	// 'Bind' is a list of Go struct instances. The frontend has access to the methods of these instances.
	// 'Mac' options tailor the application when running an macOS.
	app := application.New(application.Options{
		Name:        "svg-studio",
		Description: "AI 驱动的 SVG 生成与编辑器",
		Services: []application.Service{
			application.NewService(services.NewAIService()),
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
		Title: "SVG Studio",
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

	slog.Info("启动", "app", "svg-studio", "version", Version)

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
			// 限流不算故障：GitHub 匿名 API 只有 60 次/小时，反复调试很容易撞满。
			// 静默跳过，下次启动自然会再查，没必要打扰用户。
			if errors.Is(err, update.ErrRateLimited) {
				slog.Debug("更新源限流，本次跳过")
				return
			}
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

		// 自动安装会重启应用，用户没导出的 SVG 会丢。所以必须先问一句，
		// 不能悄悄重启 —— 用户正画到一半突然黑屏重启，比不更新更糟。
		//
		// Show() 内部走 InvokeSync，会阻塞到对话框关闭，回调在此之前已触发，
		// 所以返回后用普通变量判断就行，不需要 channel。
		// 测试用旁路：设了 SVG_STUDIO_AUTO_UPDATE=1 就跳过确认框直接装。
		// 存在的意义是把「下载 + 替换 + 重启」这条链路和「弹框等人点」解耦，
		// 否则每次验证安装都要有人在机器前面点一下。
		var confirmed bool
		if os.Getenv("SVG_STUDIO_AUTO_UPDATE") == "1" {
			slog.Info("跳过确认框（SVG_STUDIO_AUTO_UPDATE=1）")
			confirmed = true
		}

		// 两个平台的后果不一样，文案必须分开写。
		//
		// Windows：全自动，下载 → 替换 exe → 重启，未保存的内容会丢。
		// macOS：程序只负责把 dmg 下好并打开，拖拽替换由用户完成 —— 因为
		// 未公证的应用替换自身会让签名失效，被系统隐私保护拦下（详见
		// internal/update 里 installDarwin 的注释）。所以那边不会重启，
		// 当前编辑不受影响。
		message := "程序会自动下载安装包并打开它。\n\n" +
			"把里面的 svg-studio 拖到 Applications 文件夹，选择「替换」即可完成更新。" +
			"应用不会自动关闭，当前编辑不受影响。"
		okLabel := "下载并打开"
		if rel.AutoInstall {
			message = "更新会自动下载并安装，完成后应用将重启。\n\n" +
				"未保存的编辑会丢失，建议先把需要的 SVG 导出再更新。"
			okLabel = "立即更新"
		}

		dlg := app.Dialog.Question()
		dlg.SetTitle("发现新版本 " + rel.Version)
		dlg.SetMessage(message)
		dlg.AddButton(okLabel).OnClick(func() { confirmed = true })
		dlg.AddButton("稍后").OnClick(func() {})

		if !confirmed {
			slog.Info("弹出更新确认框", "title", "发现新版本 "+rel.Version)
			shownAt := time.Now()
			dlg.Show()
			slog.Info("确认框已关闭", "waitSec", time.Since(shownAt).Seconds(), "confirmed", confirmed)
		}

		if !confirmed {
			slog.Info("用户推迟了更新", "latest", rel.Version)
			return
		}

		slog.Info("用户确认更新，开始安装", "latest", rel.Version)

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
