
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
  const [status, setStatus] = useState<GenerationStatus>(GenerationStatus.IDLE);
  const [currentSvg, setCurrentSvg] = useState<GeneratedSvg | null>(null);
  const [history, setHistory] = useState<GeneratedSvg[]>([]);
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
      
      setHistory(prev => [newSvg, ...prev]);
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

      setHistory(prev => [newSvg, ...prev]);
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
                        <h3 className="text-zinc-200 font-medium mb-1">Generating Vectors</h3>
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
                    <h2 className="text-lg md:text-xl font-semibold text-zinc-300 mb-2">Ready to Create</h2>
                    <p className="text-xs md:text-sm text-zinc-500 max-w-xs text-center">
                        {window.innerWidth < 768 ? "Tap the menu icon to start designing." : "Use the panel on the left to describe your idea or upload a reference image."}
                    </p>
                    {window.innerWidth < 768 && (
                         <button onClick={() => setSidebarVisible(true)} className="mt-6 px-4 py-2 bg-indigo-600 text-white text-sm font-medium rounded-lg hover:bg-indigo-500 transition-colors">
                             Start Creating
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
