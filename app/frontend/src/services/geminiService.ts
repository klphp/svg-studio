
/**
 * @license
 * SPDX-License-Identifier: Apache-2.0
*/

import { GoogleGenAI } from "@google/genai";
import { GenerationOptions, ModelConfig, OPENAI_COMPAT_ENDPOINTS } from "../types";
import { AIService } from "../../bindings/github.com/klphp/svg-studio/internal/services";

// Helper to get the GoogleGenAI client
const getClient = (config: ModelConfig) => {
    // 上游这里写的是 `config.apiKey || process.env.API_KEY`。
    //
    // process.env 是 Node/AI Studio 服务端的写法，在 Wails 的 WebView 里
    // 根本不存在 —— config.apiKey 有值时靠短路求值躲过去了，一旦为空
    // （首次运行、用户没填 key）就会抛 `process is not defined`。
    //
    // 桌面端的 key 只从 localStorage 来（存在 modelConfig.apiKey 里），
    // 所以这里直接返回空串，让上层给出「请先填 key」的可读提示。
    const apiKey = config.apiKey || "";
    return new GoogleGenAI({ apiKey });
};

// System Instructions - Shared between providers
const getSystemPrompt = (options: GenerationOptions) => `
      You are a world-class expert in Scalable Vector Graphics (SVG) design and coding. 
      Your task is to generate a high-quality, visually stunning, and detailed SVG based on the user's description.
      
      Guidelines:
      1.  **Output Format**: Return ONLY the raw SVG code. Do not wrap it in markdown code blocks.
      2.  **Style**: The artistic style must be strictly: "${options.style}".
      3.  **Composition**:
          - Viewpoint: "${options.viewpoint || 'Optimized for subject'}".
          - Mood/Vibe: "${options.mood || 'Neutral'}".
      4.  **Color Palette**: 
          - Primary Color: "${options.color}". 
          - Theme/Harmony: "${options.theme}". Use this to select complementary or analogous colors.
      5.  **Stroke/Line Work**: "${options.stroke}".
      6.  **Complexity**: The level of detail should be: "${options.complexity}".
          - Minimal: Use few paths, simple shapes, abstract.
          - Medium: Balanced detail, standard iconography.
          - Detailed: Intricate paths, shading, texture, complex geometry.
      7.  **Animation**: ${options.animated ? "You MUST include CSS keyframe animations (`<style>`) or SMIL animations (`<animate>`) to bring the artwork to life. Make it loop smoothly." : "Do NOT include any animations. Static vector art only."}
      8.  **Negative Prompt**: Avoid these elements: "${options.negativePrompt}".
      9.  **Technical**: 
          - Always include a \`viewBox\` attribute that matches the requested aspect ratio: "${options.ratio}".
          - Ensure the SVG is self-contained.
          - Use semantic IDs or classes if helpful.
          - Do not use external resources (images/fonts).
      10. **Space Usage**: IMPORTANT: The artwork should completely fill the specified viewBox. Minimize empty margins around the subject.
`;

/**
 * 走 OpenAI 兼容的 /chat/completions 接口。
 *
 * OpenRouter、opencode zen、以及用户自填的 custom 端点都是这套协议，
 * 所以一个函数覆盖，靠 baseURL 区分。只有 OpenRouter 需要额外的
 * HTTP-Referer / X-Title 两个归属头（它按这两个字段统计来源，
 * 别的服务商不认识、也不需要）。
 */
const resolveBaseURL = (config: ModelConfig): string => {
    if (config.provider === 'custom') {
        return (config.baseURL || '').trim().replace(/\/+$/, '');
    }
    return OPENAI_COMPAT_ENDPOINTS[config.provider] || '';
};

const generateOpenAICompatSvg = async (
    messages: any[],
    config: ModelConfig
): Promise<string> => {
    const baseURL = resolveBaseURL(config);
    if (!baseURL) {
        throw new Error("接口地址为空，请在设置里填写 Base URL。");
    }
    if (!config.apiKey) {
        throw new Error("API Key 为空，请在设置里填写。");
    }

    const headers: Record<string, string> = {
        "Authorization": `Bearer ${config.apiKey}`,
        "Content-Type": "application/json",
    };
    if (config.provider === 'openrouter') {
        headers["HTTP-Referer"] = window.location.origin;
        headers["X-Title"] = "SVG Studio";
    }

    const bodyJSON = JSON.stringify({ model: config.model, messages });

    // 走 Go 侧代理，不在 WebView 里 fetch。
    //
    // 实测 opencode.ai/zen/go/v1 的 OPTIONS 预检返回 404、不带任何
    // Access-Control 头，浏览器的跨域请求会在预检阶段就被拦掉 —— 报的是
    // 一个笼统的 CORS/failed to fetch，看不出真实原因。OpenRouter 带了
    // allow-origin: *，所以在它上面直连能跑，问题被掩盖了很久。
    //
    // Go 侧不受 CORS 约束，还能加浏览器加不了的头（opencode 要求
    // x-opencode-session，缺了直接 400 MissingSessionID）。
    //
    // 顺带一句：上面的 headers 变量现在只剩 openrouter 的归属头有实际意义，
    // 但既然改由 Go 发请求，这些头也就没必要从这边传了。
    try {
        const [raw, errInfo] = await AIService.ChatCompletion(baseURL, config.apiKey, bodyJSON);

        if (errInfo) {
            // Go 侧已经把状态码和错误体解析好了，这里只负责组装一句
            // 能直接给用户看的话。
            const prefix = {
                config: "配置问题",
                auth: "鉴权失败",
                ratelimit: "请求过于频繁",
                network: "网络错误",
                server: "服务端错误",
                request: "请求被拒绝",
            }[errInfo.kind] || "请求失败";
            throw new Error(`${prefix}：${errInfo.message}`);
        }

        const result = JSON.parse(raw);
        return cleanSvgResponse(result.choices?.[0]?.message?.content);
    } catch (error: any) {
        console.error("OpenAI-compat request failed:", error);
        throw new Error(error.message || "请求失败，请检查网络和接口配置。");
    }
};


/**
 * Generates an SVG string based on the user's prompt and settings.
 */
export const generateSvgFromPrompt = async (options: GenerationOptions): Promise<string> => {
  const { modelConfig } = options;
  const systemPrompt = getSystemPrompt(options);
  const fullPrompt = `Create an ${options.animated ? 'ANIMATED' : ''} SVG representation of: "${options.prompt}". \nStyle: ${options.style}.\nViewpoint: ${options.viewpoint}.\nMood: ${options.mood}.\nColor Theme: ${options.theme} (Base: ${options.color}).\nStroke Style: ${options.stroke}.\nAspect Ratio: ${options.ratio}.\nComplexity: ${options.complexity}.\nNegative Prompt: ${options.negativePrompt}`;

  // Route based on provider
  if (modelConfig.provider !== 'google') {
      return generateOpenAICompatSvg([
          { role: 'system', content: systemPrompt },
          { role: 'user', content: fullPrompt }
      ], modelConfig);
  }

  // Default: Google Gemini SDK
  try {
    const ai = getClient(modelConfig);
    const response = await ai.models.generateContent({
      model: modelConfig.model || 'gemini-3-pro-preview',
      contents: fullPrompt,
      config: {
        systemInstruction: systemPrompt,
        temperature: 0.4,
        topP: 0.95,
        topK: 40,
      },
    });

    return cleanSvgResponse(response.text);

  } catch (error: any) {
    console.error("Gemini API Error:", error);
    throw new Error(error.message || "Failed to generate SVG.");
  }
};

/**
 * Generates an SVG based on an input image and settings.
 */
export const generateSvgFromImage = async (options: GenerationOptions): Promise<string> => {
  const { image, prompt, modelConfig } = options;
  if (!image) throw new Error("No image provided");

  const systemPrompt = `
      You are an expert in vectorizing images. Convert the provided image into a clean, high-quality Scalable Vector Graphic (SVG).
      
      Guidelines:
      1.  **Output**: Return ONLY the raw SVG code. No markdown.
      2.  **Fidelity**: Capture the essence, shapes, and composition of the source image.
      3.  **Style Adaptation**: Apply the requested style: "${options.style}".
      4.  **Composition**: Viewpoint: "${options.viewpoint || 'Match source'}", Mood: "${options.mood || 'Match source'}".
      5.  **Color**: The output should adhere to the primary color "${options.color}" and theme "${options.theme}" while respecting the image's original contrast if needed.
      6.  **Complexity**: Target complexity level: "${options.complexity}".
      7.  **Line Work**: Apply stroke style: "${options.stroke}".
      8.  **Animation**: ${options.animated ? "Include subtle animations." : "Static SVG only."}
      9.  **Technical**: Ensure valid XML, viewBox="${options.ratio}", and self-contained paths.
      10. **Space Usage**: Ensure the subject occupies the majority of the canvas with minimal whitespace.
    `;
  
  const userPromptText = `Convert this image to SVG. ${prompt ? `Additional instructions: ${prompt}` : ''} \nStyle: ${options.style}.\nViewpoint: ${options.viewpoint}.\nMood: ${options.mood}.\nTheme: ${options.theme}.\nComplexity: ${options.complexity}.\nNegative Prompt: ${options.negativePrompt}`;

  // OpenRouter Image Handling
  if (modelConfig.provider !== 'google') {
      // Ensure the model supports vision (e.g., gpt-4o, gemini-flash, etc.)
      return generateOpenAICompatSvg([
          { role: 'system', content: systemPrompt },
          { 
              role: 'user', 
              content: [
                  { type: 'text', text: userPromptText },
                  { type: 'image_url', image_url: { url: image } }
              ]
          }
      ], modelConfig);
  }

  // Google Gemini SDK Handling
  try {
    const ai = getClient(modelConfig);
    // Remove data URL prefix if present (e.g., "data:image/png;base64,")
    const base64Data = image.split(',')[1] || image;
    const mimeType = image.match(/data:([^;]+);/)?.[1] || 'image/png';

    const response = await ai.models.generateContent({
      model: modelConfig.model || 'gemini-3-pro-preview',
      contents: [
        { text: userPromptText },
        {
            inlineData: {
                mimeType: mimeType,
                data: base64Data
            }
        }
      ],
      config: {
        systemInstruction: systemPrompt,
        temperature: 0.4,
      },
    });

    return cleanSvgResponse(response.text);

  } catch (error: any) {
    console.error("Gemini Vision API Error:", error);
    throw new Error(error.message || "Failed to convert image to SVG.");
  }
};

/**
 * Refines an existing SVG based on user instructions.
 */
export const refineSvg = async (
  currentSvg: string,
  instruction: string,
  modelConfig: ModelConfig
): Promise<string> => {
    
  const systemPrompt = `
      You are an expert SVG editor. 
      Your task is to modify an existing SVG code based strictly on the user's instruction.
      
      Guidelines:
      1.  **Output Format**: Return ONLY the full, valid, updated SVG code. No markdown.
      2.  **Goal**: Implement the user's requested change while preserving the original style and quality as much as possible, unless asked to change it.
      3.  **Integrity**: Ensure the code remains valid XML/SVG.
  `;

  const fullPrompt = `
      Here is the original SVG code:
      \`\`\`xml
      ${currentSvg}
      \`\`\`

      User Instruction: "${instruction}"

      Return the updated SVG.
  `;

  if (modelConfig.provider !== 'google') {
      return generateOpenAICompatSvg([
          { role: 'system', content: systemPrompt },
          { role: 'user', content: fullPrompt }
      ], modelConfig);
  }

  try {
    const ai = getClient(modelConfig);
    const response = await ai.models.generateContent({
      model: modelConfig.model || 'gemini-3-pro-preview',
      contents: fullPrompt,
      config: {
        systemInstruction: systemPrompt,
        temperature: 0.3, // Lower temperature for precise editing
      },
    });

    return cleanSvgResponse(response.text);

  } catch (error: any) {
    console.error("Gemini API Refine Error:", error);
    throw new Error(error.message || "Failed to refine SVG.");
  }
}

/**
 * Helper to clean up markdown from response
 */
function cleanSvgResponse(rawText: string | undefined): string {
  if (!rawText) return '';
  
  // 1. Remove markdown code blocks to clean up the string
  let text = rawText.replace(/```xml/gi, '').replace(/```svg/gi, '').replace(/```/g, '');

  // 2. Find the start of the SVG tag
  const start = text.search(/<svg/i);
  
  if (start !== -1) {
    // 3. Find the last closing SVG tag to handle nested SVGs correctly
    const lastEnd = text.toLowerCase().lastIndexOf('</svg>');
    
    if (lastEnd !== -1 && lastEnd > start) {
      // Extract everything from start to end + length of closing tag
      return text.substring(start, lastEnd + 6).trim();
    }
    
    // Fallback: If no closing tag found, return from start (best effort)
    return text.substring(start).trim();
  }

  // If no <svg> tag found, return stripped text
  return text.trim();
}
