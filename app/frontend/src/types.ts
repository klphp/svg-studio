
/**
 * @license
 * SPDX-License-Identifier: Apache-2.0
*/

export enum GenerationStatus {
  IDLE = 'IDLE',
  LOADING = 'LOADING',
  SUCCESS = 'SUCCESS',
  ERROR = 'ERROR'
}

export interface GeneratedSvg {
  id: string;
  content: string;
  prompt: string;
  timestamp: number;
}

export interface ApiError {
  message: string;
  details?: string;
}

/**
 * 服务商类型。
 *
 * openrouter / opencode / custom 三者走的是同一套 OpenAI 兼容的
 * /chat/completions 接口，只是 baseURL 不同 —— 所以不用为每家写一遍。
 * custom 让用户可以接任何兼容端点（DeepSeek、本地 Ollama、自建网关等）。
 */
export type ModelProvider = 'google' | 'openrouter' | 'opencode' | 'custom';

export interface ModelConfig {
  provider: ModelProvider;
  model: string;
  apiKey?: string; // google 可用环境变量兜底，其它服务商必填
  /** provider=custom 时的端点，形如 https://example.com/v1 */
  baseURL?: string;
}

/** 各 OpenAI 兼容服务商的默认端点。custom 由用户自己填。 */
export const OPENAI_COMPAT_ENDPOINTS: Record<string, string> = {
  openrouter: 'https://openrouter.ai/api/v1',
  opencode: 'https://opencode.ai/zen/go/v1',
};

export interface GenerationOptions {
  prompt: string;
  image?: string;
  color: string;
  theme: string;
  style: string;
  ratio: string;
  complexity: string;
  animated: boolean;
  stroke: string;
  negativePrompt: string;
  viewpoint?: string;
  mood?: string;
  designType?: string;
  modelConfig: ModelConfig; // Added config
}
