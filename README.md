# SVG Studio

AI 驱动的 SVG 生成与编辑器。描述你想要什么，模型直接吐出一份可编辑的矢量图，然后在内置编辑器里调整图形、图层、变换、滤镜和排版。

内置 Gemini 和 OpenRouter 两种服务商，可切换。

## 下载

到 [Releases](https://github.com/klphp/svg-studio/releases/latest) 页下载对应平台的文件。

| 平台 | 文件 |
|---|---|
| macOS (Intel) | `svg-studio-<版本>-darwin-amd64.zip` |
| Windows (x64) | `svg-studio-<版本>-windows-amd64.zip` |

**首次打开**

- **macOS**：ad-hoc 签名，未做公证。被 Gatekeeper 拦下时右键 →「打开」，或执行 `xattr -cr /Applications/svg-studio.app`
- **Windows**：未做代码签名，SmartScreen 会提示「已保护你的电脑」，点「更多信息」→「仍要运行」

## 首次使用

需要自备 API Key。点右上角齿轮，选服务商：

- **Google Gemini** — 到 [Google AI Studio](https://aistudio.google.com/apikey) 申请
- **OpenRouter** — 到 [OpenRouter](https://openrouter.ai/keys) 申请

Key 只保存在本机浏览器的 localStorage 里，不会上传，也不进 git。

## 功能

**生成**

- 文字描述或参考图两种输入方式
- 预设：科技 Logo / 等距世界 / 线性图标 / 霓虹赛博 / 手绘草图
- 风格、配色、主题、比例、复杂度、线条、视角、氛围可调
- 负面提示词
- 历史记录本地保存

**编辑**

- 图形：矩形、圆形、椭圆、三角形、星形、箭头、气泡、心形、文字、图片
- 图层：排序、复制、删除、显隐
- 变换：位置、尺寸、旋转、缩放、翻转、九宫格对齐
- 样式：填充、描边、圆角、不透明度、混合模式、线性/径向渐变
- 滤镜：模糊、灰度、复古、反色、饱和度、色相旋转、投影
- 排版：字体、字重

**界面**

- 中英双语，设置面板可切换

## 开发

需要 Go 1.25+、Node 18+、[Wails3 CLI](https://v3.wails.io/)。

```bash
cd app

# 生成前端与 Go 之间的类型绑定（改了 Go 侧服务后必须重跑）
wails3 generate bindings -ts -d frontend/bindings

# 开发模式（热重载）
wails3 dev

# 构建
wails3 build      # 裸二进制 → bin/
wails3 package    # macOS 打 .app，Windows 打 exe
```

前端单独跑：

```bash
cd app/frontend
npm install
npm run build
```

### 技术栈

- **Go + Wails3 v3.1** — 桌面壳、原生窗口、自更新
- **React 19 + TypeScript** — 界面
- **Vite 8** — 构建
- **Tailwind CSS** — 样式（当前走 CDN）
- **Gemini / OpenRouter** — 生成后端

### 目录结构

```
app/
├── main.go                 应用入口、窗口创建、自更新检查
├── internal/update/        自更新封装（Wails updater 的 provider 分支）
├── frontend/
│   └── src/
│       ├── App.tsx         主布局、生成流程编排
│       ├── components/     界面组件
│       ├── services/       Gemini / OpenRouter 调用
│       └── i18n/           中英词条
└── build/                  各平台打包配置与图标
```

### 自更新

走 Wails 内置的 updater，`github` provider 指向本仓库的 Releases。版本号在 `app/main.go` 的 `const Version`，发版时同步改这里和 release tag（tag 的 `v` 前缀由 provider 自己剥掉）。

行为差异：

- **Windows**：下载 zip → 替换 exe → 重启，全自动
- **macOS**：打开 release 页面，由用户手动下载替换。Wails 的替换流程只认 `.zip`（内含 `.app`），而 dmg 才是 macOS 的分发惯例

日志在各平台的用户配置目录下：`<UserConfigDir>/svg-studio/app.log`。

### 发布新版本

1. 同步版本号：`app/main.go` 的 `const Version`、`app/build/darwin/Info.plist`、`app/build/windows/info.json`、`app/frontend/package.json`
2. 构建并打包。产物名必须是 `svg-studio-<版本>-<平台>-<架构>.zip` —— Wails 的资产匹配器只认 `darwin` / `windows` / `linux` 加 `amd64` / `arm64` 这些字面量
3. zip 里**只能有一个顶层条目**：macOS 是 `svg-studio.app/`，Windows 是 `svg-studio.exe`
4. 发 release，tag 形如 `v0.0.1`

## 致谢

界面与交互源自 [SVG-ORA-Studio](https://github.com/seeb4coding/SVG-ORA-Studio)（Apache-2.0），在其基础上加了 Go 桌面壳、中英双语、应用图标和自更新。

## 许可

Apache-2.0
