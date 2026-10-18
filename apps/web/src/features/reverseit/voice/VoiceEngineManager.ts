/**
 * Universal Multilingual Voice Engine Manager
 * 
 * Supports 3 Free High-Clarity Voice Modes:
 * 1. Studio Native Audio Prompts (100% Free, Zero-Config, Authentic Nigerian Human Voices)
 * 2. Microsoft Azure Speech F0 Free Tier (500k chars/mo free: Bunmi & Kola for Yorùbá, Gambo for Hausa, Nkechi for Igbo)
 * 3. YarnGPT / African Voice AI (Lagos-built models for Yorùbá, Hausa, Igbo, and Pidgin)
 * 4. Enhanced Browser Neural Fallback (Phonetic Normalization + Hardware Acoustic Warm-up)
 */

import {
  type SupportedLanguage,
  SUPPORTED_LANGUAGES,
  speakNativeVoice,
  type SpeechOptions,
} from './VoiceDisputeEngine';

export type VoiceEngineType = 'studio_native' | 'azure_speech' | 'yarngpt' | 'browser_neural';

export interface VoiceEngineConfig {
  engine: VoiceEngineType;
  azureKey: string;
  azureRegion: string;
  yarnGptKey: string;
  speed: number;
  pitch: number;
  volume: number;
}

const STORAGE_KEY = 'smartreversal_voice_engine_config_v1';

export const DEFAULT_VOICE_CONFIG: VoiceEngineConfig = {
  engine: 'studio_native',
  azureKey: '',
  azureRegion: 'eastus',
  yarnGptKey: '',
  speed: 0.88,
  pitch: 1.06,
  volume: 1.0,
};

export interface EngineMeta {
  id: VoiceEngineType;
  name: string;
  badge: string;
  description: string;
  freeTierInfo: string;
  nativeActors: Record<SupportedLanguage, string>;
  isConfigured: (config: VoiceEngineConfig) => boolean;
}

export const VOICE_ENGINES: Record<VoiceEngineType, EngineMeta> = {
  studio_native: {
    id: 'studio_native',
    name: 'Studio Native Human Voice',
    badge: '100% Free • Zero Setup',
    description: 'Pre-rendered authentic native Nigerian human audio for all banking dispute dialogue prompts.',
    freeTierInfo: 'Permanently free, zero API keys or signup required.',
    nativeActors: {
      yo: 'Yorùbá Studio Voice (Lagos)',
      ha: 'Hausa Studio Voice (Kano)',
      ig: 'Asụsụ Igbo Studio Voice (Enugu)',
      pcm: 'Naija Pidgin Studio Voice (Warri/Lagos)',
      fr: 'Français Studio Voice (Abidjan/Paris)',
      en: 'English Nigeria Studio Voice (Lagos)',
    },
    isConfigured: () => true,
  },
  azure_speech: {
    id: 'azure_speech',
    name: 'Microsoft Azure Neural (F0 Free Tier)',
    badge: 'State-of-the-Art AI • Bunmi & Kola',
    description: 'Ultra-realistic African neural voices trained on native Nigerian voice actors.',
    freeTierInfo: '500,000 characters free every month forever via Azure Cognitive Services F0 tier.',
    nativeActors: {
      yo: 'Bunmi & Kola Neural (yo-NG)',
      ha: 'Gambo & Balarabe Neural (ha-NG)',
      ig: 'Nkechi & Eze Neural (ig-NG)',
      pcm: 'Ekele & Abeo Neural (en-NG)',
      fr: 'Denise & Henri Neural (fr-FR)',
      en: 'Ekele & Abeo Neural (en-NG)',
    },
    isConfigured: (cfg) => Boolean(cfg.azureKey && cfg.azureKey.trim().length > 8),
  },
  yarngpt: {
    id: 'yarngpt',
    name: 'YarnGPT African Voice AI',
    badge: 'African NLP API',
    description: 'Dedicated Lagos-built voice AI platform specializing in Nigerian tonal accents.',
    freeTierInfo: 'Free developer tier credits for testing African voice synthesis.',
    nativeActors: {
      yo: 'YarnGPT Yorùbá Native AI',
      ha: 'YarnGPT Hausa Native AI',
      ig: 'YarnGPT Igbo Native AI',
      pcm: 'YarnGPT Naija Pidgin AI',
      fr: 'YarnGPT Multilingual AI',
      en: 'YarnGPT Nigerian English AI',
    },
    isConfigured: (cfg) => Boolean(cfg.yarnGptKey && cfg.yarnGptKey.trim().length > 6),
  },
  browser_neural: {
    id: 'browser_neural',
    name: 'Enhanced Browser Neural Synthesizer',
    badge: 'Local • Zero Network',
    description: 'Tuned browser speech engine with West African phonetic transliteration and DAC warm-up.',
    freeTierInfo: '100% free, runs entirely offline inside your browser.',
    nativeActors: {
      yo: 'Browser Phonetic Transliteration',
      ha: 'Browser Phonetic Transliteration',
      ig: 'Browser Phonetic Transliteration',
      pcm: 'Browser Nigerian English',
      fr: 'Browser French Voice',
      en: 'Browser Nigerian English',
    },
    isConfigured: () => true,
  },
};

/**
 * Load Voice Engine Configuration from LocalStorage
 */
export function loadVoiceEngineConfig(): VoiceEngineConfig {
  if (typeof window === 'undefined') return DEFAULT_VOICE_CONFIG;
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return DEFAULT_VOICE_CONFIG;
    const parsed = JSON.parse(raw);
    return { ...DEFAULT_VOICE_CONFIG, ...parsed };
  } catch {
    return DEFAULT_VOICE_CONFIG;
  }
}

/**
 * Save Voice Engine Configuration to LocalStorage
 */
export function saveVoiceEngineConfig(config: VoiceEngineConfig): void {
  if (typeof window === 'undefined') return;
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(config));
  } catch {
    // ignore storage error
  }
}

/**
 * Azure Speech API Voice Synthesizer (F0 Free Tier)
 */
export async function synthesizeWithAzureSpeech(
  text: string,
  language: SupportedLanguage,
  config: VoiceEngineConfig
): Promise<Blob> {
  const region = config.azureRegion || 'eastus';
  const url = `https://${region}.tts.speech.microsoft.com/cognitiveservices/v1`;

  const voiceMap: Record<SupportedLanguage, string> = {
    yo: 'yo-NG-BunmiNeural',
    ha: 'ha-NG-GamboNeural',
    ig: 'ig-NG-NkechiNeural',
    pcm: 'en-NG-EkeleNeural',
    fr: 'fr-FR-DeniseNeural',
    en: 'en-NG-EkeleNeural',
  };
  const voiceName = voiceMap[language] || 'yo-NG-BunmiNeural';
  const langCode = SUPPORTED_LANGUAGES[language].speechCode;

  const ratePercent = Math.round((config.speed - 1) * 100);
  const rateStr = ratePercent >= 0 ? `+${ratePercent}%` : `${ratePercent}%`;

  const ssml = `<speak version='1.0' xml:lang='${langCode}'>
    <voice name='${voiceName}'>
      <prosody rate='${rateStr}' pitch='+5%'>
        ${escapeXml(text)}
      </prosody>
    </voice>
  </speak>`;

  const response = await fetch(url, {
    method: 'POST',
    headers: {
      'Ocp-Apim-Subscription-Key': config.azureKey.trim(),
      'Content-Type': 'application/ssml+xml',
      'X-Microsoft-OutputFormat': 'audio-16khz-128kbitrate-mono-mp3',
      'User-Agent': 'SmartReversal-VoiceEngine',
    },
    body: ssml,
  });

  if (!response.ok) {
    throw new Error(`Azure Speech API failed: ${response.status} ${response.statusText}`);
  }

  return await response.blob();
}

/**
 * YarnGPT African Voice API Synthesizer (Free Tier)
 */
export async function synthesizeWithYarnGPT(
  text: string,
  language: SupportedLanguage,
  config: VoiceEngineConfig
): Promise<Blob> {
  const url = 'https://api.yarngpt.ai/v1/tts';
  const response = await fetch(url, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${config.yarnGptKey.trim()}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      text,
      language,
      voice: 'natural',
      format: 'mp3',
      speed: config.speed,
    }),
  });

  if (!response.ok) {
    throw new Error(`YarnGPT API failed: ${response.status} ${response.statusText}`);
  }

  return await response.blob();
}

function escapeXml(unsafe: string): string {
  return unsafe.replace(/[<>&'"]/g, (c) => {
    switch (c) {
      case '<':
        return '&lt;';
      case '>':
        return '&gt;';
      case '&':
        return '&amp;';
      case '\'':
        return '&apos;';
      case '"':
        return '&quot;';
      default:
        return c;
    }
  });
}

/**
 * Audio Playback Controller
 */
class UniversalAudioPlayer {
  private currentAudio: HTMLAudioElement | null = null;
  private currentBlobUrl: string | null = null;

  public stop(): void {
    if (this.currentAudio) {
      this.currentAudio.pause();
      this.currentAudio.currentTime = 0;
      this.currentAudio = null;
    }
    if (this.currentBlobUrl) {
      URL.revokeObjectURL(this.currentBlobUrl);
      this.currentBlobUrl = null;
    }
    if (typeof window !== 'undefined' && 'speechSynthesis' in window) {
      window.speechSynthesis.cancel();
    }
  }

  public async playUrl(url: string, onEnd?: () => void): Promise<void> {
    this.stop();
    if (typeof window === 'undefined' || typeof Audio === 'undefined') {
      if (onEnd) onEnd();
      return;
    }
    const audio = new Audio(url);
    this.currentAudio = audio;
    audio.volume = 1.0;

    audio.onended = () => {
      this.cleanup();
      if (onEnd) onEnd();
    };

    audio.onerror = () => {
      this.cleanup();
      if (onEnd) onEnd();
    };

    try {
      await audio.play();
    } catch {
      this.cleanup();
      if (onEnd) onEnd();
    }
  }

  public async playBlob(blob: Blob, onEnd?: () => void): Promise<void> {
    this.stop();
    if (typeof window === 'undefined' || typeof Audio === 'undefined') {
      if (onEnd) onEnd();
      return;
    }
    this.currentBlobUrl = URL.createObjectURL(blob);
    const audio = new Audio(this.currentBlobUrl);
    this.currentAudio = audio;
    audio.volume = 1.0;

    audio.onended = () => {
      this.cleanup();
      if (onEnd) onEnd();
    };

    audio.onerror = () => {
      this.cleanup();
      if (onEnd) onEnd();
    };

    try {
      await audio.play();
    } catch {
      this.cleanup();
      if (onEnd) onEnd();
    }
  }

  private cleanup(): void {
    if (this.currentBlobUrl) {
      URL.revokeObjectURL(this.currentBlobUrl);
      this.currentBlobUrl = null;
    }
    this.currentAudio = null;
  }
}

export const universalAudioPlayer = new UniversalAudioPlayer();

export const STUDIO_PROMPT_MAP: Record<string, string> = {
  // Greetings
  greeting_yo: '/audio/prompts/greeting_yo.mp3',
  greeting_ha: '/audio/prompts/greeting_ha.mp3',
  greeting_ig: '/audio/prompts/greeting_ig.mp3',
  greeting_pcm: '/audio/prompts/greeting_pcm.mp3',
  greeting_fr: '/audio/prompts/greeting_fr.mp3',
  greeting_en: '/audio/prompts/greeting_en.mp3',

  // Slots
  slot_channel_yo: '/audio/prompts/slot_channel_yo.mp3',
  slot_channel_ha: '/audio/prompts/slot_channel_ha.mp3',
  slot_channel_ig: '/audio/prompts/slot_channel_ig.mp3',
  slot_channel_pcm: '/audio/prompts/slot_channel_pcm.mp3',
  slot_channel_fr: '/audio/prompts/slot_channel_fr.mp3',
  slot_channel_en: '/audio/prompts/slot_channel_en.mp3',

  slot_amount_yo: '/audio/prompts/slot_amount_yo.mp3',
  slot_amount_ha: '/audio/prompts/slot_amount_ha.mp3',
  slot_amount_ig: '/audio/prompts/slot_amount_ig.mp3',
  slot_amount_pcm: '/audio/prompts/slot_amount_pcm.mp3',
  slot_amount_fr: '/audio/prompts/slot_amount_fr.mp3',
  slot_amount_en: '/audio/prompts/slot_amount_en.mp3',

  slot_locationOrBank_yo: '/audio/prompts/slot_locationOrBank_yo.mp3',
  slot_locationOrBank_ha: '/audio/prompts/slot_locationOrBank_ha.mp3',
  slot_locationOrBank_ig: '/audio/prompts/slot_locationOrBank_ig.mp3',
  slot_locationOrBank_pcm: '/audio/prompts/slot_locationOrBank_pcm.mp3',
  slot_locationOrBank_fr: '/audio/prompts/slot_locationOrBank_fr.mp3',
  slot_locationOrBank_en: '/audio/prompts/slot_locationOrBank_en.mp3',

  slot_accountOrPhone_yo: '/audio/prompts/slot_accountOrPhone_yo.mp3',
  slot_accountOrPhone_ha: '/audio/prompts/slot_accountOrPhone_ha.mp3',
  slot_accountOrPhone_ig: '/audio/prompts/slot_accountOrPhone_ig.mp3',
  slot_accountOrPhone_pcm: '/audio/prompts/slot_accountOrPhone_pcm.mp3',
  slot_accountOrPhone_fr: '/audio/prompts/slot_accountOrPhone_fr.mp3',
  slot_accountOrPhone_en: '/audio/prompts/slot_accountOrPhone_en.mp3',

  confirmation_success_yo: '/audio/prompts/confirmation_success_yo.mp3',
  confirmation_success_ha: '/audio/prompts/confirmation_success_ha.mp3',
  confirmation_success_ig: '/audio/prompts/confirmation_success_ig.mp3',
  confirmation_success_pcm: '/audio/prompts/confirmation_success_pcm.mp3',
  confirmation_success_fr: '/audio/prompts/confirmation_success_fr.mp3',
  confirmation_success_en: '/audio/prompts/confirmation_success_en.mp3',

  // Demos
  demo_yo_initial: '/audio/prompts/demo_yo_initial.mp3',
  demo_yo_followup: '/audio/prompts/demo_yo_followup.mp3',
  demo_ha_initial: '/audio/prompts/demo_ha_initial.mp3',
  demo_ha_followup: '/audio/prompts/demo_ha_followup.mp3',
  demo_ig_initial: '/audio/prompts/demo_ig_initial.mp3',
  demo_ig_followup: '/audio/prompts/demo_ig_followup.mp3',
  demo_pcm_initial: '/audio/prompts/demo_pcm_initial.mp3',
  demo_pcm_followup: '/audio/prompts/demo_pcm_followup.mp3',
  demo_fr_initial: '/audio/prompts/demo_fr_initial.mp3',
  demo_fr_followup: '/audio/prompts/demo_fr_followup.mp3',
};

export function resolvePromptAudioPath(
  promptKey?: string,
  text?: string,
  language?: SupportedLanguage
): string | null {
  if (promptKey && STUDIO_PROMPT_MAP[promptKey]) {
    return STUDIO_PROMPT_MAP[promptKey];
  }

  if (!text) return null;
  const lang = language || 'yo';
  const clean = text.toLowerCase();

  // Greetings
  if (clean.includes('kúnlẹ̀') || clean.includes('káàbọ̀') || clean.includes('kaabo') || clean.includes('barka da zuwa') || clean.includes('sannu da zuwa') || clean.includes('nnọọ') || clean.includes('welcome') || clean.includes('bienvenue')) {
    const key = `greeting_${lang}`;
    if (STUDIO_PROMPT_MAP[key]) return STUDIO_PROMPT_MAP[key];
  }

  // Channel
  if (clean.includes('atm') || clean.includes('pos') || clean.includes('mobile') || clean.includes('fóònù') || clean.includes('virement')) {
    const key = `slot_channel_${lang}`;
    if (STUDIO_PROMPT_MAP[key]) return STUDIO_PROMPT_MAP[key];
  }

  // Amount
  if (clean.includes('èlò') || clean.includes('nawa') || clean.includes('ego ole') || clean.includes('how much') || clean.includes('montant') || clean.includes('iye owó')) {
    const key = `slot_amount_${lang}`;
    if (STUDIO_PROMPT_MAP[key]) return STUDIO_PROMPT_MAP[key];
  }

  // Location/Bank
  if (clean.includes('orúkọ bánkì') || clean.includes('bánki') || clean.includes('banki') || clean.includes('ụlọ akụ') || clean.includes('bank') || clean.includes('banque')) {
    const key = `slot_locationOrBank_${lang}`;
    if (STUDIO_PROMPT_MAP[key]) return STUDIO_PROMPT_MAP[key];
  }

  // Account/Phone
  if (clean.includes('nọ́mbà àkọ́ọ́lẹ̀') || clean.includes('lambar asusun') || clean.includes('nọmba akaụntụ') || clean.includes('account number') || clean.includes('numéro de compte')) {
    const key = `slot_accountOrPhone_${lang}`;
    if (STUDIO_PROMPT_MAP[key]) return STUDIO_PROMPT_MAP[key];
  }

  // Confirmation Success
  if (clean.includes('ẹ̀sùn yín') || clean.includes('rahoton ku') || clean.includes('mkpesa gị') || clean.includes('wákàtí mẹ́rinlelogun') || clean.includes('sa\'o\'i 24') || clean.includes('24 hours') || clean.includes('24 heures') || clean.includes('registered successfully')) {
    const key = `confirmation_success_${lang}`;
    if (STUDIO_PROMPT_MAP[key]) return STUDIO_PROMPT_MAP[key];
  }

  return null;
}

export interface UniversalPlayOptions {
  promptKey?: string;
  onStart?: () => void;
  onEnd?: () => void;
  onError?: (err: Error) => void;
}

/**
 * Unified Bot Voice Dispatcher
 * Automatically coordinates between Studio Native Voice, Azure Free Tier, YarnGPT, and Browser Neural.
 */
export async function playUniversalBotVoice(
  text: string,
  language: SupportedLanguage,
  config: VoiceEngineConfig,
  options?: UniversalPlayOptions
): Promise<void> {
  universalAudioPlayer.stop();

  if (options?.onStart) options.onStart();

  const handleFinish = () => {
    if (options?.onEnd) options.onEnd();
  };

  // 1. Try Microsoft Azure Speech F0 Free Tier (if configured)
  if (config.engine === 'azure_speech' && VOICE_ENGINES.azure_speech.isConfigured(config)) {
    try {
      const blob = await synthesizeWithAzureSpeech(text, language, config);
      await universalAudioPlayer.playBlob(blob, handleFinish);
      return;
    } catch (err) {
      console.warn('Azure Speech API call failed, falling back to Studio audio:', err);
    }
  }

  // 2. Try YarnGPT African Voice AI Free Tier (if configured)
  if (config.engine === 'yarngpt' && VOICE_ENGINES.yarngpt.isConfigured(config)) {
    try {
      const blob = await synthesizeWithYarnGPT(text, language, config);
      await universalAudioPlayer.playBlob(blob, handleFinish);
      return;
    } catch (err) {
      console.warn('YarnGPT API call failed, falling back to Studio audio:', err);
    }
  }

  // 3. Studio Native Audio Prompts (Default & Primary High-Fidelity Audio Mode)
  // Check if a pre-rendered authentic studio prompt is available for this dialogue turn
  const studioAudioPath = resolvePromptAudioPath(options?.promptKey, text, language);
  if (studioAudioPath) {
    try {
      await universalAudioPlayer.playUrl(studioAudioPath, handleFinish);
      return;
    } catch (err) {
      console.warn(`Failed to play studio prompt ${studioAudioPath}, falling back to browser synthesis:`, err);
    }
  }

  // 4. Fallback: Enhanced Browser Neural Speech Engine with Phonetic Normalization
  const speechOpts: SpeechOptions = {
    rate: config.speed,
    pitch: config.pitch,
    volume: config.volume,
  };

  speakNativeVoice(text, language, speechOpts, handleFinish);
}
