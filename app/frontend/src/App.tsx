
/**
 * @license
 * SPDX-License-Identifier: Apache-2.0
*/

import React, { useState, useEffect } from 'react';
import { InputSection } from './components/InputSection';
import { SvgPreview } from './components/SvgPreview';
import { generateSvgFromPrompt, generateSvgFromImage, refineSvg } from './services/geminiService';
import { GeneratedSvg, GenerationStatus, ApiError, GenerationOptions, ModelConfig } from './types';
import { AlertCircle, PanelLeftClose, PanelLeftOpen } from 'lucide-react';
import { useT } from "./i18n";
import { HistoryService, GenerationRecord } from "../bindings/github.com/klphp/svg-studio/internal/services";

// Helper to generate UUIDs locally
const generateUUID = (): string => {
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    const r = Math.random() * 16 | 0;
    const v = c === 'x' ? r : (r & 0x3 | 0x8);
    return v.toString(16);
  });
};

const DEFAULT_CONFIG: ModelConfig = {
  provider: 'google',
  model: 'gemini-2.0-flash', // Updated to a more standard/available default
  apiKey: ''
};

const App: React.FC = () => {
  const t = useT();
  const [status, setStatus] = useState<GenerationStatus>(GenerationStatus.IDLE);
  const [currentSvg, setCurrentSvg] = useState<GeneratedSvg | null>(null);
  const [history, setHistory] = useState<GeneratedSvg[]>([]);

  /**
   * 启动时从 SQLite 读历史。
   *
   * 之前历史只活在 useState 里，关掉应用就全没了 —— 「历史 (0)」一直是 0
   * 就是这个原因。现在落到 SQLite，重启还在。
   */
  useEffect(() => {
    (async () => {
      try {
        const [records, err] = await HistoryService.List(200);
        if (err) {
          console.warn("读取历史失败:", err.message);
          return;
        }
        // GenerationRecord 和 GeneratedSvg 字段名不同（createdAt vs timestamp），
        // 转换一下再喂给现有 UI，省得把整个历史侧栏都改一遍。
        setHistory((records || []).map(r => ({
          id: r.id,
          content: r.content,
          prompt: r.prompt,
          timestamp: r.createdAt,
        })));
      } catch (e) {
        console.warn("读取历史异常:", e);
      }
    })();
  }, []);

  /**
   * 把一次生成存进 SQLite，并更新界面。
   *
   * 除了 SVG 本身，参数（风格/配色/比例等）也一起存 —— 光有源码，回头看
   * 历史根本不知道当时怎么生成的，想微调都得重猜。
   */
  const persistHistory = async (svg: GeneratedSvg, options?: any) => {
    setHistory(prev => [svg, ...prev]);
    try {
      // bindings 里 GenerationRecord 是 `export type`，只有类型没有构造函数，
      // 所以不能 new —— 直接给结构匹配的字面量即可（或者用 as GenerationRecord
      // 明确意图）。这里用后者，多一层编译期检查。
      await HistoryService.Save({
        id: svg.id,
        prompt: svg.prompt,
        content: svg.content,
        style: options?.style ?? "",
        color: options?.color ?? "",
        theme: options?.theme ?? "",
        ratio: options?.ratio ?? "",
        complexity: options?.complexity ?? "",
        provider: options?.modelConfig?.provider ?? "",
        model: options?.modelConfig?.model ?? "",
        createdAt: svg.timestamp,
      } as GenerationRecord);
    } catch (e) {
      // 存不进库不该影响用户看图，记一条就够了
      console.warn("保存历史失败:", e);
    }
  };
  const [error, setError] = useState<ApiError | null>(null);
  const [sidebarVisible, setSidebarVisible] = useState(true);

  // Model Configuration State with LocalStorage Persistence
  const [modelConfig, setModelConfig] = useState<ModelConfig>(() => {
    const saved = localStorage.getItem('svg_ora_studio_model_config');
    return saved ? JSON.parse(saved) : DEFAULT_CONFIG;
  });

  useEffect(() => {
    localStorage.setItem('svg_ora_studio_model_config', JSON.stringify(modelConfig));
  }, [modelConfig]);

  // Handle responsiveness for sidebar on initial load
  useEffect(() => {
    if (window.innerWidth < 768) {
      setSidebarVisible(true); // Show sidebar initially on mobile to start creation
    }
  }, []);

  // Auto-dismiss error toast after 3 seconds
  useEffect(() => {
    if (status === GenerationStatus.ERROR && error) {
      const timer = setTimeout(() => {
        setError(null);
        // Return to previous valid state or idle
        setStatus(currentSvg ? GenerationStatus.SUCCESS : GenerationStatus.IDLE);
      }, 3000);
      return () => clearTimeout(timer);
    }
  }, [status, error, currentSvg]);

  const handleGenerate = async (options: GenerationOptions) => {
    setStatus(GenerationStatus.LOADING);
    setError(null);
    setCurrentSvg(null); 
    
    // Auto-close sidebar on mobile to show loading/result
    if (window.innerWidth < 768) {
        setSidebarVisible(false);
    }

    try {
      let svgContent = '';
      
      // Pass the current model config to the service
      const optionsWithConfig = { ...options, modelConfig };

      if (options.image) {
        svgContent = await generateSvgFromImage(optionsWithConfig);
      } else {
        svgContent = await generateSvgFromPrompt(optionsWithConfig);
      }
      
      const newSvg: GeneratedSvg = {
        id: generateUUID(),
        content: svgContent,
        prompt: options.prompt || (options.image ? 'Image to Vector' : 'Untitled'),
        timestamp: Date.now()
      };
      
      await persistHistory(newSvg, options);
      setCurrentSvg(newSvg);
      setStatus(GenerationStatus.SUCCESS);
    } catch (err: any) {
      setStatus(GenerationStatus.ERROR);
      setError({
        message: "Generation Failed",
        details: err.message || "An unexpected error occurred."
      });
      // Sidebar stays closed so user sees the error toast
    }
  };

  const handleRefine = async (instruction: string) => {
    if (!currentSvg) return;
    
    setStatus(GenerationStatus.LOADING);
    setError(null);

    try {
      // Pass model config to refine
      const refinedContent = await refineSvg(currentSvg.content, instruction, modelConfig);
      
      const newSvg: GeneratedSvg = {
        id: generateUUID(),
        content: refinedContent,
        prompt: `${currentSvg.prompt} (Refined: ${instruction})`,
        timestamp: Date.now()
      };

      // 精修这条路径没有风格/配色这些参数，只有模型信息可存
      await persistHistory(newSvg, { modelConfig });
      setCurrentSvg(newSvg);
      setStatus(GenerationStatus.SUCCESS);
    } catch (err: any) {
      setStatus(GenerationStatus.ERROR);
      setError({
        message: "Refinement Failed",
        details: err.message || "Could not refine the SVG."
      });
    }
  };

  const handleSelectHistory = (svg: GeneratedSvg) => {
    setCurrentSvg(svg);
    setStatus(GenerationStatus.SUCCESS);
    setError(null);
    // Auto-close on mobile selection
    if (window.innerWidth < 768) {
        setSidebarVisible(false);
    }
  };

  return (
    <div className="h-screen bg-black text-zinc-100 font-sans flex overflow-hidden selection:bg-indigo-500/30">      
      
      {/* Left Sidebar - Responsive Drawer/Sidebar */}
      <aside 
        className={`
          fixed inset-y-0 left-0 z-50 h-full bg-zinc-950 border-r border-white/10 flex flex-col transition-all duration-300 ease-in-out
          w-full md:relative md:flex-shrink-0 md:z-20
          ${sidebarVisible 
            ? 'translate-x-0 md:w-[360px]' 
            : '-translate-x-full md:translate-x-0 md:w-0 md:opacity-0 md:overflow-hidden md:-ml-[1px]' // md:-ml-1px removes border flash
          }
        `}
      >
        <div className="w-full h-full md:w-[360px]"> 
           <InputSection 
              onGenerate={handleGenerate} 
              onRefine={handleRefine}
              status={status} 
              history={history}
              onSelectHistory={handleSelectHistory}
              selectedId={currentSvg?.id}
              modelConfig={modelConfig}
              onConfigChange={setModelConfig}
              onCloseMobile={() => setSidebarVisible(false)}
           />
        </div>
      </aside>

      {/* Main Content Area */}
      <main className="flex-1 flex flex-col relative min-w-0 bg-[#09090b]">
        
        {/* Sidebar Toggle */}
        <button 
          onClick={() => setSidebarVisible(!sidebarVisible)}
          className={`absolute top-4 left-4 z-40 p-2 bg-zinc-900/80 backdrop-blur border border-white/10 rounded-lg text-zinc-400 hover:text-white shadow-lg hover:bg-zinc-800 transition-colors ${sidebarVisible ? 'md:hidden' : 'flex'}`}
          title={sidebarVisible ? "Hide Sidebar" : "Show Sidebar"}
        >
          {sidebarVisible ? <PanelLeftClose className="w-5 h-5" /> : <PanelLeftOpen className="w-5 h-5" />}
        </button>

        <div className="flex-1 overflow-hidden relative flex flex-col">
            {/* Error Toast */}
            {status === GenerationStatus.ERROR && error && (
                <div className="absolute top-6 left-1/2 -translate-x-1/2 z-50 w-[90%] md:w-auto min-w-[300px] max-w-lg animate-in fade-in slide-in-from-top-4 duration-300">
                  <div className="bg-zinc-900/95 backdrop-blur-md border border-red-500/30 rounded-xl p-4 flex items-start gap-3 shadow-2xl shadow-red-900/20 ring-1 ring-red-500/20">
                      <AlertCircle className="w-5 h-5 text-red-500 flex-shrink-0 mt-0.5" />
                      <div>
                        <h4 className="font-semibold text-red-200 text-sm">{error.message}</h4>
                        <p className="text-xs text-red-300/70 mt-1 leading-relaxed">{error.details}</p>
                      </div>
                  </div>
                </div>
            )}

            {/* Render Preview if Success OR if Loading but we have a current SVG (Refining) */}
            {(status === GenerationStatus.SUCCESS || (status === GenerationStatus.LOADING && currentSvg) || (status === GenerationStatus.ERROR && currentSvg)) && currentSvg ? (
                <SvgPreview 
                    data={currentSvg} 
                    onRefine={handleRefine}
                    status={status}
                />
            ) : status === GenerationStatus.LOADING && !currentSvg ? (
                // Initial Loading State
                <div className="absolute inset-0 flex flex-col items-center justify-center bg-zinc-950 text-zinc-500 gap-6">
                    <div className="relative">
                        <div className="absolute inset-0 blur-xl bg-indigo-500/20 rounded-full animate-pulse"></div>
                        <div className="w-20 h-20 rounded-2xl border border-white/10 flex items-center justify-center relative bg-zinc-900 overflow-hidden">
                           <div className="absolute inset-0 bg-gradient-to-tr from-transparent via-white/5 to-transparent skew-x-12 animate-[shimmer_2s_infinite]"></div>
                           <div className="w-8 h-8 border-2 border-indigo-500 border-t-transparent rounded-full animate-spin"></div>
                        </div>
                    </div>
                    <div className="text-center">
                        <h3 className="text-zinc-200 font-medium mb-1">{t("Generating Vectors")}</h3>
                        <p className="text-xs text-zinc-500">Using {modelConfig.provider === 'google' ? 'Gemini' : modelConfig.model}...</p>
                    </div>
                </div>
            ) : (
                // Empty State (Show this when IDLE or ERROR without content)
                (status === GenerationStatus.IDLE || (status === GenerationStatus.ERROR && !currentSvg)) && (
                <div className="absolute inset-0 flex flex-col items-center justify-center text-zinc-600 select-none bg-zinc-950 bg-[radial-gradient(#27272a_1px,transparent_1px)] [background-size:16px_16px] p-6">
                    <div className="w-24 h-24 md:w-32 md:h-32 rounded-3xl bg-zinc-900 border border-white/5 flex items-center justify-center mb-6 shadow-2xl shadow-black">
                         <div className="w-12 h-12 md:w-16 md:h-16 rounded border-2 border-dashed border-zinc-700 flex items-center justify-center">
                            <span className="text-xl md:text-2xl font-mono text-zinc-700">+</span>
                         </div>
                    </div>
                    <h2 className="text-lg md:text-xl font-semibold text-zinc-300 mb-2">{t("Ready to Create")}</h2>
                    <p className="text-xs md:text-sm text-zinc-500 max-w-xs text-center">
                        {window.innerWidth < 768 ? "Tap the menu icon to start designing." : "Use the panel on the left to describe your idea or upload a reference image."}
                    </p>
                    {window.innerWidth < 768 && (
                         <button onClick={() => setSidebarVisible(true)} className="mt-6 px-4 py-2 bg-indigo-600 text-white text-sm font-medium rounded-lg hover:bg-indigo-500 transition-colors">
                             {t("Start Creating")}
                         </button>
                    )}
                </div>
                )
            )}
          </div>
      </main>
    </div>
  );
};

export default App;
