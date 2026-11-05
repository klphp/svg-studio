# SVG Studio

AI 驱动的 SVG 生成与编辑器。描述你想要什么，模型直接吐出一份可编辑的矢量图，然后在内置编辑器里调整图形、图层、变换、滤镜和排版。

内置 Gemini 和 OpenRouter 两种服务商，可切换。

## 下载

到 [Releases](https://github.com/klphp/svg-studio/releases/latest) 页下载对应平台的文件。

| 平台 | 文件 |
|---|---|
| macOS (Intel) | `svg-studio-<版本>-darwin-amd64.dmg` |
| Windows (x64) | `svg-studio-<版本>-windows-amd64.zip` |

macOS 打开 dmg 后，把 svg-studio 拖到里面的 Applications 快捷方式上即可。

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

两个平台能力不同：

- **Windows**：全自动。下载 zip → 替换 exe → 重启。没有操作确认环节，所以更新前会弹框问一次，因为未保存的编辑会丢
- **macOS**：半自动。程序下载 dmg 到 `~/Downloads` 并自动打开，用户把 app 拖到 Applications 完成替换

**macOS 为什么不能全自动**：替换 `Contents/MacOS/` 里的二进制会让 `_CodeSignature/CodeResources` 的哈希对不上，签名失效，macOS 立刻弹「隐私与安全」要求手动放行；重新 ad-hoc 签名也没用，签名内容变了 Gatekeeper 照样拦。根因是没有 Apple 开发者 ID 和公证，系统不允许未公证的应用静默替换自身。所以只能退到「程序下载，用户拖一下」。

日志在各平台的用户配置目录下：`<UserConfigDir>/svg-studio/app.log`。

### 发布新版本

1. 同步版本号：`app/main.go` 的 `const Version`、`app/build/darwin/Info.plist`、`app/build/windows/info.json`、`app/frontend/package.json`
2. 构建并打包。产物名必须含平台和架构字面量 —— Wails 的资产匹配器只认 `darwin` / `windows` / `linux` 加 `amd64` / `arm64`
3. macOS 出 dmg（`wails3 tool package --format dmg`），Windows 出内含单个 exe 的 zip
4. 发 release，tag 形如 `v0.0.1`

**macOS 打包有个容易漏的步骤**：往 `Contents/Resources` 放 LICENSE 之后，原签名会失效，必须重新 `codesign --force --deep --sign - bin/svg-studio.app`，否则打出来的 dmg 里的 app 是签名损坏状态。

## 许可

[MIT](./LICENSE)。

本项目的界面与交互源自 [SVG-ORA-Studio](https://github.com/seeb4coding/SVG-ORA-Studio)，
原作者 seeb4coding，以 MIT 协议发布。MIT 允许修改、分发与再授权，因此在其基础上
套 Go 桌面壳、做中英双语、换应用图标、加自更新都是允许的，只要保留原版权声明
和许可证（已保留在本文件的 LICENSE 里）。

**关于源码文件头的一处不一致**：上游源码每个文件的开头写着
`SPDX-License-Identifier: Apache-2.0`，而仓库根目录的 `LICENSE` 是 MIT。这是
Google AI Studio 导出代码时自动加的模板头，与仓库的实际协议冲突。本项目以
仓库根的 `LICENSE`（MIT）为准；文件头的原始声明按原样保留，不做改动 ——
修改他人的版权声明本身就不合适。

### 相对上游的改动

- 增加 Go + Wails3 桌面壳（原生窗口、应用图标、自更新、日志落盘）
- 界面中英双语（126 处文案，新增 `src/i18n/`）
- 修正上游在桌面环境下的若干问题：frameless 窗口红绿灯遮挡、严格 TS
  模式下编译不过的类型推断、WebView 里不存在的 `process.env`
- 删除 `components/Header.tsx`（上游从未引用的死代码）与模板占位元数据
