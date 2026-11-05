/**
 * 中文词条。key 是英文原文（上游文案），value 是中文。
 *
 * 查不到就回退到 key 本身，所以这份表**漏了不会显示空白**，只会显示英文
 * —— 可以分批补。英文模式完全不走这张表。
 */
export const zh: Record<string, string> = {
  // ── App.tsx ──
  "Ready to Create": "准备创作",
  "Start Creating": "开始创作",
  "Generating Vectors": "正在生成矢量图",

  // ── InputSection：基础 ──
  "Create": "创作",
  "History": "历史",
  "Generate SVG": "生成 SVG",
  "Generating": "生成中",
  "Surprise Me": "随机灵感",
  "Refine selected...": "精修选中元素…",
  "No history yet": "还没有历史记录",

  // ── InputSection：提示词 ──
  "e.g. A retro rocket ship...": "例如：一艘复古火箭飞船…",
  "Optional instructions...": "可选指令…",
  "Negative Prompt": "负面提示词",
  "Elements to exclude (e.g. text, background, shading)": "要排除的元素（如文字、背景、阴影）",
  "Presets:": "预设：",

  // ── InputSection：参数 ──
  "Style": "风格",
  "Color": "配色",
  "Theme": "主题",
  "Mood": "氛围",
  "Complexity": "复杂度",
  "Stroke Style": "线条样式",
  "Ratio": "比例",
  "Viewpoint": "视角",
  "Animate": "动画",

  // ── InputSection：模型设置 ──
  "Model Settings": "模型设置",
  "AI Provider": "AI 服务商",
  "Google Gemini": "Google Gemini",
  "OpenRouter": "OpenRouter",
  "API Key": "API Key",
  "API Key (Optional)": "API Key（可选）",
  "Enter a key to override the default.": "填入 key 可覆盖默认值。",
  "Key is stored locally in your browser.": "Key 只保存在本地浏览器里。",
  "Model": "模型",
  "Use system default": "使用系统默认",
  "Custom...": "自定义…",
  "Gemini 2.0 Flash": "Gemini 2.0 Flash",
  "Gemini 2.5 Pro": "Gemini 2.5 Pro",
  "Gemini 3.0 Pro": "Gemini 3.0 Pro",
  "Save Settings": "保存设置",
  "vendor/model-name": "服务商/模型名",
  "sk-or-...": "sk-or-…",
  "Show": "显示",
  "Hide": "隐藏",

  // ── InputSection：上传与侧栏 ──
  "Click to upload reference": "点击上传参考图",
  "Change Image": "更换图片",
  "Close Sidebar": "收起侧栏",

  // ── EditorTools：标签页 ──
  "Tools": "工具",
  "Layers": "图层",
  "Canvas Settings": "画布设置",

  // ── EditorTools：图形 ──
  "Add Elements": "添加元素",
  "Rectangle": "矩形",
  "Circle": "圆形",
  "Ellipse": "椭圆",
  "Triangle": "三角形",
  "Star": "星形",
  "Arrow": "箭头",
  "Bubble": "气泡",
  "Heart": "心形",
  "Text": "文字",
  "Upload Image": "上传图片",
  "Add": "添加",

  // ── EditorTools：图层 ──
  "All Layers": "全部图层",
  "No visible layers": "没有可见图层",
  "Move Up": "上移一层",
  "Move Down": "下移一层",
  "Duplicate": "复制",
  "Delete": "删除",
  "New elements are added to the center. Switch to the": "新元素会加到画布中心。请切换到",
  "Tip:": "提示：",

  // ── EditorTools：形状属性 ──
  "Select an element on canvas to edit": "在画布上选中一个元素来编辑",
  "Shape Properties": "形状属性",
  "Appearance": "外观",
  "Transform & Effects": "变换与特效",
  "Position": "位置",
  "Size (px)": "尺寸（px）",
  "Width": "宽",
  "Height": "高",
  "X Axis": "X 轴",
  "Y Axis": "Y 轴",
  "Rotation": "旋转",
  "Corner Radius": "圆角半径",
  "Fill": "填充",
  "Stroke": "描边",
  "Opacity": "不透明度",
  "Blend Mode": "混合模式",
  "None": "无",
  "Solid": "纯色",
  "Linear": "线性",
  "Radial": "径向",
  "Background Color": "背景色",
  "Global Opacity": "整体不透明度",

  // ── EditorTools：变换 ──
  "Scale": "缩放",
  "Flip H": "水平翻转",
  "Flip V": "垂直翻转",
  "Align Left": "左对齐",
  "Align Center": "水平居中",
  "Align Right": "右对齐",
  "Align Top": "顶端对齐",
  "Align Middle": "垂直居中",
  "Align Bottom": "底端对齐",
  "Alignment": "对齐",

  // ── EditorTools：文字 ──
  "Typography": "排版",
  "Font Family": "字体",
  "Sans Serif": "无衬线",
  "Weight": "字重",
  "Content": "内容",

  // ── EditorTools：滤镜 ──
  "Filters": "滤镜",
  "Blur": "模糊",
  "Grayscale": "灰度",
  "Sepia": "复古",
  "Invert Colors": "反色",
  "Saturate": "饱和度",
  "Hue Rotate": "色相旋转",
  "Drop Shadow": "投影",
  "Start": "起点",
  "End": "终点",

  // ── 选项值：风格 ──
  "Flat": "扁平",
  "Material": "质感",
  "Isometric": "等距",
  "Line Art": "线稿",
  "Cyberpunk": "赛博朋克",
  "Minimalist": "极简",
  "Hand Drawn": "手绘",

  // ── 选项值：复杂度 ──
  "Minimal": "最简",
  "Medium": "中等",
  "Detailed": "精细",

  // ── 选项值：配色主题 ──
  "Vibrant": "鲜明",
  "Pastel": "马卡龙",
  "Neon": "霓虹",
  "Earth Tones": "大地色",
  "Dark Mode": "暗色",
  "Complementary": "互补色",

  // ── 选项值：线条 ──
  "Standard": "标准",
  "Variable Width": "变宽",
  "Sketchy": "素描感",
  "Dashed": "虚线",
  "Thick": "粗线",
  "None (Fill Only)": "无（只填充）",

  // ── 选项值：视角 ──
  "Front": "正面",
  "Top Down": "俯视",
  "Side Profile": "侧面",
  "Macro Close-up": "微距特写",
  "Wide Angle": "广角",

  // ── 选项值：氛围 ──
  "Professional": "专业",
  "Playful": "俏皮",
  "Mysterious": "神秘",
  "Energetic": "动感",
  "Calm": "沉稳",
  "Retro": "复古",
  "Futuristic": "未来感",
  "Neutral": "中性",

  // ── 选项值：比例（title 提示）──
  "Square": "正方形",
  "Wide": "宽屏",
  "Portrait": "竖屏",

  // ── 预设名 ──
  "Tech Logo": "科技 Logo",
  "Isometric World": "等距世界",
  "Neon Cyberpunk": "霓虹赛博",
  "Hand Sketch": "手绘草图",
  "Flat Illustration": "扁平插画",
  "Linear Icon": "线性图标",

  // ── 其它 ──
  "Custom": "自定义",
  "Base URL": "接口地址",
  "Any OpenAI-compatible /chat/completions endpoint works.": "任何 OpenAI 兼容的 /chat/completions 端点都能用。",
  "Switch to Image Input": "切换到图片输入",
  "Switch to Text Input": "切换到文字输入",
  "On": "开",
  "Off": "关",
  "New elements are added to the center. Switch to the Style tab to modify them.":
    "新元素会加到画布中心，切换到「风格」页签可以调整它们。",

  // ── SvgPreview ──
  "Properties": "属性",
  "Source Code": "源代码",
  "Toggle Code": "切换代码",
  "Toggle Editor": "切换编辑器",
  "Rotate": "旋转",
  "Drag to Resize, Ctrl+Drag to Skew X": "拖动缩放，按住 Ctrl 拖动可沿 X 轴斜切",
  "Drag to Resize, Ctrl+Drag to Skew Y": "拖动缩放，按住 Ctrl 拖动可沿 Y 轴斜切",
};
