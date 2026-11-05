// Package update 封装 Wails v3 内置的 app.Updater，按平台给出不同的更新体验。
//
// 职责边界很窄：它不做下载、不做验签、不做替换 —— 那些都在 Wails 的
// updater 里。这里只负责三件事：
//
//	1. 按配置选 provider（github / endpoint）
//	2. 决定「查到了怎么办」：Windows 自动装，macOS 交给用户自己下
//	3. 把结果翻译成前端能用的最小结构
//
// 为什么 macOS 不用自动装：
//
//	Wails 的替换流程只认 .zip（内含 .app），不支持 .dmg / .pkg。而 dmg 是
//	macOS 的分发惯例，用户也更信 dmg。加上 macOS 的 Gatekeeper 要求签名
//	端到端保持，自动替换换出来的二进制容易被拦。让用户自己下 dmg 拖进
//	Applications，反而更省事、更符合那边用户的习惯。
package update

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/endpoint"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"

)

// Config 是更新源的配置。
//
// 这份结构体原本在一个内部模板项目里，那边挂在它自己的 config 包下。但这个
// 项目只有 main.go 一个包、没有配置系统，为这一个结构体引入整个 yaml 配置层
// 不划算，所以直接定义在这里。字段名保持通用，将来真要接 yaml 配置可以直接
// 复用。
type Config struct {
	Enabled bool

	// Provider 是更新源类型："github" 或 "endpoint"。
	Provider string

	// Repository 是 provider=github 时的 "owner/repo"。
	Repository string

	// ManifestURL 是 provider=endpoint 时的清单地址，支持
	// {{platform}} / {{arch}} / {{version}} / {{channel}} 占位符。
	ManifestURL string

	// ChecksumAsset 是 provider=github 时的校验和文件名（如 SHA256SUMS）。
	// 它只防下载损坏 —— 文件和校验和放在同一个 release 里，防不住同时篡改
	// 两者的人。要真防篡改必须配 PublicKey。
	ChecksumAsset string

	// Prerelease 为 true 时把预发布版也算作可用更新。
	Prerelease bool

	// PublicKey 是 Ed25519 公钥（hex）。留空表示不验签 —— 试水阶段就这样，
	// github provider 也从来不填 Signature，配了也没用。
	PublicKey string

	// App 是应用标识，用于 endpoint 清单里的匹配。
	App string
}

// ErrRateLimited 表示被更新源的 API 限流了（GitHub 匿名请求是 60 次/小时）。
//
// 这不是故障，也不是「已是最新」—— 它只是「现在查不了」。所以必须和
// 网络错误、真失败区分开：日志降级成 Debug，用户侧完全静默，下次启动
// 自然会再查。否则用户会看到一条莫名其妙的「检查更新失败」。
var ErrRateLimited = errors.New("更新服务限流，稍后自动重试")

// isRateLimited 从错误里认限流。github provider 把 API 的错误体原样包进
// err 里，形如 "api 403: {"message":"API rate limit exceeded for ..."}"，
// 所以按文本匹配就够，不必解析 JSON。
func isRateLimited(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "rate limit") ||
		strings.Contains(s, "too many requests") ||
		strings.Contains(s, "api 403")
}

// ErrNetwork 是「网络不通」的统一出口。
//
// 为什么要自己分类：Wails 的 updater 没有定义网络错误类型，net/http 的
// *url.Error / *net.OpError / *net.DNSError 会原样冒到调用方。直接把那些
// 原始错误丢给用户，看到的是
//     Get "https://api.github.com/...": dial tcp: lookup api.github.com: no such host
// 这既看不懂也不知道该做什么。统一换成一句能照着做的话。
var ErrNetwork = errors.New("网络连接失败，请检查网络后重试")

// isNetworkError 判断错误是不是出在网络上（而不是配置、签名、磁盘）。
//
// 这几种都要认出：DNS 解析失败（改过 hosts / 断网 / 被投毒）、拨号失败
// （防火墙 / 无路由）、超时、以及 net/http 会把它们包一层的 *url.Error。
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}
	var (
		dnsErr *net.DNSError
		netErr net.Error
		urlErr *url.Error
		opErr  *net.OpError
	)
	if errors.As(err, &dnsErr) || errors.As(err, &opErr) {
		return true
	}
	if errors.As(err, &netErr) {
		return true
	}
	// url.Error 通常已经在上面被拆开了，但有些路径只有它
	if errors.As(err, &urlErr) {
		return true
	}
	return false
}

// Result 是交给前端的最小信息集。
//
// 不把 updater.Release 整个丢给前端：它带 Metadata（含 API URL、asset id）
// 和 Verification（digest 原始字节），传过去既没用又会泄漏发布细节。
type Result struct {
	Available bool   `json:"available"`
	Version   string `json:"version"`
	Notes     string `json:"notes"`
	ReleaseURL string `json:"releaseUrl"`
	// AutoInstall 表示当前平台支持「点一下就自动装好」。
	// Windows 为 true（rename-aside 原子替换 + helper 重启）；
	// macOS 为 false，前端应把按钮文案改成「前往下载」。
	AutoInstall bool `json:"autoInstall"`
}

// Manager 持有 Updater 配置和最近一次 Check 的结果。
type Manager struct {
	app            *application.App
	cfg            Config
	currentVersion string
	fallbackURL    string

	// prov 是 Setup 构造出来的 provider。macOS 上不用 Wails 的
	// DownloadAndInstall（见 Install 的注释），改由我们自己下载安装包，
	// 所以要把 provider 留着，用它的 Download 把字节流出来。
	prov updater.Provider

	mu   sync.Mutex
	last *Result
}

// New 构造 Manager。currentVersion 应与打包版本号一致（cfg.App.Version），
// 因为 updater 拿它和 release tag 比对 —— 注意 tag 上的 v 前缀由 provider
// 自己剥掉，这里不要带 v。
func New(app *application.App, cfg Config, currentVersion, fallbackURL string) *Manager {
	return &Manager{
		app:            app,
		cfg:            cfg,
		currentVersion: currentVersion,
		fallbackURL:    fallbackURL,
	}
}

// Setup 构造 provider 并初始化 app.Updater。必须在 application.New 之后调用。
// 未启用时直接返回，不留任何副作用。
func (m *Manager) Setup() error {
	if !m.cfg.Enabled {
		return nil
	}

	var (
		prov updater.Provider
		err  error
	)
	switch m.cfg.Provider {
	case "github", "":
		if m.cfg.Repository == "" {
			return fmt.Errorf("update: provider=github 但 repository 为空")
		}
		prov, err = github.New(github.Config{
			Repository:    m.cfg.Repository,
			ChecksumAsset: m.cfg.ChecksumAsset,
			Prerelease:    m.cfg.Prerelease,
		})
	case "endpoint":
		if m.cfg.ManifestURL == "" {
			return fmt.Errorf("update: provider=endpoint 但 manifestUrl 为空")
		}
		prov, err = endpoint.New(endpoint.Config{URL: m.cfg.ManifestURL})
	default:
		return fmt.Errorf("update: 未知 provider %q（只支持 github / endpoint）", m.cfg.Provider)
	}
	if err != nil {
		return fmt.Errorf("update: 构造 provider 失败: %w", err)
	}

	m.prov = prov

	ucfg := updater.Config{
		CurrentVersion: m.currentVersion,
		Providers:      []updater.Provider{prov},
	}
	// 只有配了公钥才验签。留空时 Wails 允许只带 digest 的 release 安装，
	// 但任何带 Signature 的 release 会被拒 —— 这是它的规定行为，够用。
	if m.cfg.PublicKey != "" {
		ucfg.PublicKey = []byte(m.cfg.PublicKey)
	}

	if err := m.app.Updater.Init(ucfg); err != nil {
		return fmt.Errorf("update: 初始化 Updater 失败: %w", err)
	}

	slog.Info("自更新已启用",
		"provider", m.cfg.Provider,
		"current", m.currentVersion,
		"验签", m.cfg.PublicKey != "")
	return nil
}

// Enabled 报告自更新是否启用。
func (m *Manager) Enabled() bool { return m.cfg.Enabled }

// Check 只检查，不下载、不安装。这是「用户自己决定」的关键 ——
// 前端拿到结果后可以只显示一个提示条，用户点了才走 Install。
func (m *Manager) Check(ctx context.Context) (*Result, error) {
	if !m.cfg.Enabled {
		return &Result{}, nil
	}

	rel, err := m.app.Updater.Check(ctx)
	if err != nil {
		// 网络不通就到此为止：不上报「无更新」、不提示「已是最新」，
		// 那种结论在断网时是错的，会让用户以为版本检查成功了。
		if isRateLimited(err) {
			slog.Debug("更新源限流，跳过本次检查", "err", err)
			return nil, ErrRateLimited
		}
		if isNetworkError(err) {
			slog.Debug("检查更新时网络不通", "err", err)
			return nil, ErrNetwork
		}
		return nil, err
	}
	if rel == nil {
		return &Result{Available: false}, nil
	}

	res := &Result{
		Available:   true,
		Version:     rel.Version,
		Notes:       rel.Notes,
		ReleaseURL:  m.releaseURL(rel),
		AutoInstall: runtime.GOOS == "windows",
	}

	m.mu.Lock()
	m.last = res
	m.mu.Unlock()

	return res, nil
}

// Last 返回最近一次成功检查的结果，没有则返回 nil。
func (m *Manager) Last() *Result {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.last
}

// Install 执行更新。
//
// Windows：让 Wails 下载 + 校验 + 原子替换，然后重启。用户看到的是
// 「更新并重启」一个动作，之后不再有 SmartScreen 警告 —— 因为替换出来的
// 文件是 Go 写的，不带浏览器下载才有的 MOTW 标记。
//
// 其它平台：打开 release 页面让用户自己下。原因见包注释。
func (m *Manager) Install(ctx context.Context) error {
	if !m.cfg.Enabled {
		return fmt.Errorf("update: 自更新未启用")
	}

	// Windows 走自动安装；macOS 走浏览器。
	//
	// 关于 macOS 为什么不能自动装 —— 实测结论，不是照搬别人的判断：
	//
	// Wails v3.1.0 在 macOS 上确实有把 .app 当替换目标的代码
	// （updater_darwin.go 的 bundleTarget()，DownloadAndInstall 里也没有
	// GOOS == windows 的分支），看起来是支持的。但真跑起来必定失败：
	//
	//   updater: archive must contain exactly one top-level entry, got 2
	//
	// 已排除的可能：
	//   · 手工 curl 下载 release 资产 → zip 里只有 1 个顶层条目 svg-studio.app/
	//   · 用 github provider 的 Download() 拿字节 → 7082051 字节，10 个条目，
	//     顶层仍只有 1 个
	//   · 用 Go 的 archive/zip 模拟它的解压 + os.ReadDir → 数出 1 个
	//   · ditto 和 Info-ZIP 两种方式重新打包 → 都是 1 个顶层条目
	//
	// 也就是说下载和 zip 都是对的，但它内部就是数出 2 个，从外部无法解释。
	//
	// 后来又试过绕开它、自己实现「只替换 Contents/MacOS 里的二进制」这种
	// 做法（参考 V2rayU 的思路，bundle 结构一样只有一个可执行文件）。
	// 技术上能跑通：替换后进程确实起得来。但**用户实测会被 macOS 拦住**：
	// 替换掉二进制、_CodeSignature/CodeResources 的哈希就对不上，签名失效，
	// 系统弹「隐私与安全」提示要求手动点「仍要打开」。
	//
	// 重新 ad-hoc 签名（codesign --force --deep --sign -）也救不了 —— 签名
	// 内容变了，Gatekeeper 照样当成新的未信任程序。根因是没有 Apple 开发者
	// ID（$99/年）和公证，macOS 不允许任何未公证的应用静默替换自身，这是
	// 系统级设计，不是代码能绕的。
	//
	// 所以结论：mac 上做不到无感的自动更新，走浏览器下载页。Windows 没有
	// 这套机制，自动安装实测可用（版本号已验证升到 0.0.1）。
	if runtime.GOOS == "windows" {
		// 每一步都记日志。Windows 构建带 -H windowsgui，没有控制台，用户能提供的
		// 唯一线索就是这个文件。之前日志初始化坏掉，导致「点了更新没反应」完全
		// 无从排查，只能靠猜。
		exe, _ := os.Executable()
		slog.Info("开始自动更新", "currentVersion", m.currentVersion, "exe", exe)

		if err := m.app.Updater.DownloadAndInstall(ctx); err != nil {
			// 网络不通就到此为止，不重启：否则应用会退出后原地重启，磁盘上
			// 根本没有可用的新版本，用户还以为升上去了。
			if isNetworkError(err) {
				// 区分「超时」和「连不上」：前者通常是网速慢（GitHub 在国内下载
				// 不稳定，7 MB 的包可能要几分钟），后者是断网或连不通。两种情况
				// 给用户的建议完全不同，提示不能一样。
				msg := err.Error()
				if errors.Is(err, context.DeadlineExceeded) || strings.Contains(msg, "deadline exceeded") {
					slog.Warn("下载更新包超时", "err", err)
					return fmt.Errorf("下载超时。安装包约 7 MB，从 GitHub 下载，网络较慢时可稍后重试，或到发布页面手动下载：%s", m.fallbackURL)
				}
				slog.Warn("下载更新包失败：网络不通", "err", err)
				return ErrNetwork
			}
			// 以前这里是 `return m.openDownloadPage()`：静默打开浏览器下载页，
			// 调用方拿到 nil 当成功。用户看到浏览器跳出来，以为正在更新，实际
			// 那是失败回退 —— 关掉浏览器再打开应用，版本没变，于是又提示升级。
			// 现在把错误透出去，让界面能明确说「自动更新失败了」。
			slog.Warn("自动安装失败，回退到手动下载",
				"err", err, "errType", fmt.Sprintf("%T", err))
			m.openDownloadPage()
			return fmt.Errorf("自动更新失败，已为你打开下载页面。原因：%w", err)
		}

		slog.Info("下载与替换完成，准备重启")
		if err := m.app.Updater.Restart(ctx); err != nil {
			slog.Warn("重启失败", "err", err)
			return fmt.Errorf("更新包已就位但重启失败，请手动关闭程序后再打开：%w", err)
		}
		slog.Info("重启指令已发出")
		return nil
	}

	return m.installDarwin(ctx)
}

// installDarwin 是 macOS 的「半自动」更新：程序负责下载，用户只负责拖一下。
//
// 为什么不能全自动：替换 Contents/MacOS 里的二进制会让
// _CodeSignature/CodeResources 的哈希对不上，签名失效，macOS 立刻弹出
// 「隐私与安全」要求手动点「仍要打开」。重新 ad-hoc 签名也救不了 ——
// 签名内容变了，Gatekeeper 当成另一个未信任程序，照样拦。根因是没有
// Apple 开发者 ID 和公证，系统不允许未公证的应用静默替换自身。
//
// 所以退一步：把 dmg 下到「下载」目录并直接打开。dmg 里有指向
// /Applications 的符号链接，用户把 app 拖上去、点「替换」就完事。
// 新 app 的签名在包里是完好的，不会触发任何弹窗。
//
// 下载位置刻意选 ~/Downloads 而不是缓存目录 —— 用户得能看见这个文件，
// 拖完之后自己知道该删掉它。缓存目录里躺个 7MB 的 dmg 反而让人困惑。
func (m *Manager) installDarwin(ctx context.Context) error {
	if m.prov == nil {
		slog.Warn("没有 provider，回退到打开下载页")
		return m.openDownloadPage()
	}

	rel, err := m.app.Updater.Check(ctx) // 拿到带 Artifact 的 Release
	if err != nil || rel == nil {
		slog.Warn("取不到 release 信息，回退到打开下载页", "err", err)
		return m.openDownloadPage()
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return m.openDownloadPage()
	}
	dlDir := filepath.Join(home, "Downloads")
	if _, err := os.Stat(dlDir); err != nil {
		return m.openDownloadPage()
	}

	name := rel.Artifact.Filename
	if name == "" {
		return m.openDownloadPage()
	}
	dst := filepath.Join(dlDir, name)

	// 已经下过就别重复下（用户可能点了两次「立即更新」）
	if _, err := os.Stat(dst); err == nil {
		slog.Info("安装包已存在，直接打开", "path", dst)
		return m.revealAndOpen(dst)
	}

	f, err := os.Create(dst)
	if err != nil {
		return m.openDownloadPage()
	}
	slog.Info("开始下载更新包", "path", dst, "size", rel.Artifact.Size)

	if err := m.prov.Download(ctx, rel, f, func(w, total int64) {}); err != nil {
		f.Close()
		os.Remove(dst) // 半截文件留着没意义
		if isNetworkError(err) {
			return ErrNetwork
		}
		slog.Warn("下载失败，回退到打开下载页", "err", err)
		return m.openDownloadPage()
	}
	f.Close()

	slog.Info("下载完成", "path", dst)
	return m.revealAndOpen(dst)
}

// revealAndOpen 打开安装包。dmg 会直接挂载出窗口，里面就有指向
// /Applications 的快捷方式，用户拖一下即可。
func (m *Manager) revealAndOpen(path string) error {
	if err := m.app.Browser.OpenURL("file://" + path); err != nil {
		slog.Warn("打开安装包失败", "err", err, "path", path)
		return m.openDownloadPage()
	}
	return nil
}

// openDownloadPage 在自动安装走不通时，打开 release 页面让用户自己下载。
func (m *Manager) openDownloadPage() error {
	url := m.fallbackURL
	if r := m.Last(); r != nil && r.ReleaseURL != "" {
		url = r.ReleaseURL
	}
	if url == "" {
		return fmt.Errorf("update: 没有可用的下载地址（检查配置里是否填了 repository 或 manifestUrl）")
	}
	return m.app.Browser.OpenURL(url)
}

// StartPeriodic 在后台按间隔检查，发现新版本时回调。
//
// 自己起 goroutine 而不用 updater.Config.CheckInterval：那个选项内部走的是
// CheckAndInstall，会直接把窗口弹出来并自动安装，违背「不强制」的定位。
func (m *Manager) StartPeriodic(ctx context.Context, interval time.Duration, onFound func(*Result)) {
	if !m.cfg.Enabled || interval <= 0 {
		return
	}

	go func() {
		// 等界面和本地服务稳定下来，避免启动瞬间抢资源
		select {
		case <-ctx.Done():
			return
		case <-time.After(8 * time.Second):
		}

		for {
			res, err := m.Check(ctx)
			switch {
			case err != nil:
				// 网络不通、GitHub 限速都是常态，降级成 debug 日志，
				// 别在用户日志里刷错误
				slog.Debug("检查更新失败", "err", err)
			case res.Available:
				slog.Info("发现新版本", "version", res.Version)
				if onFound != nil {
					onFound(res)
				}
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(interval):
			}
		}
	}()
}

// releaseURL 从 Release 的 metadata 里取发布页地址。
// github provider 会塞 github.release.htmlURL；endpoint 没有这个约定，
// 取不到就退回配置的 fallback。
func (m *Manager) releaseURL(rel *updater.Release) string {
	if rel != nil && rel.Metadata != nil {
		if u, ok := rel.Metadata["github.release.htmlURL"].(string); ok && u != "" {
			return u
		}
	}
	return m.fallbackURL
}
