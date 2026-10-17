import { describe, it, expect, beforeEach, vi } from 'vitest';
import {
  DEFAULT_VOICE_CONFIG,
  VOICE_ENGINES,
  loadVoiceEngineConfig,
  saveVoiceEngineConfig,
  playUniversalBotVoice,
  type VoiceEngineConfig,
} from '../features/reverseit/voice/VoiceEngineManager';

describe('Universal Free Multilingual Voice Engine Manager', () => {
  const mockStorage: Record<string, string> = {};

  beforeEach(() => {
    for (const key in mockStorage) {
      delete mockStorage[key];
    }
    const mockLocalStorage = {
      getItem: (k: string) => mockStorage[k] || null,
      setItem: (k: string, v: string) => { mockStorage[k] = v; },
      removeItem: (k: string) => { delete mockStorage[k]; },
      clear: () => {
        for (const k in mockStorage) delete mockStorage[k];
      },
    };
    vi.stubGlobal('localStorage', mockLocalStorage);
    vi.stubGlobal('window', { localStorage: mockLocalStorage });
    vi.restoreAllMocks();
  });

  describe('Free Engine Registry & Voice Actor Roster', () => {
    it('should configure Studio Native Voice as default 100% free engine', () => {
      expect(DEFAULT_VOICE_CONFIG.engine).toBe('studio_native');
      const studioEngine = VOICE_ENGINES.studio_native;
      expect(studioEngine).toBeDefined();
      expect(studioEngine.isConfigured(DEFAULT_VOICE_CONFIG)).toBe(true);
      expect(studioEngine.nativeActors.yo).toContain('Yorùbá');
      expect(studioEngine.nativeActors.ha).toContain('Hausa');
      expect(studioEngine.nativeActors.ig).toContain('Igbo');
      expect(studioEngine.nativeActors.pcm).toContain('Pidgin');
    });

    it('should configure Microsoft Azure Speech F0 Free Tier with native neural actors', () => {
      const azureEngine = VOICE_ENGINES.azure_speech;
      expect(azureEngine.nativeActors.yo).toContain('Bunmi & Kola');
      expect(azureEngine.nativeActors.ha).toContain('Gambo');
      expect(azureEngine.nativeActors.ig).toContain('Nkechi & Eze');
      expect(azureEngine.nativeActors.pcm).toContain('Ekele');
      expect(azureEngine.freeTierInfo).toContain('500,000 characters free');

      // Without key -> not configured
      expect(azureEngine.isConfigured(DEFAULT_VOICE_CONFIG)).toBe(false);

      // With key -> configured
      const withKey: VoiceEngineConfig = {
        ...DEFAULT_VOICE_CONFIG,
        azureKey: 'test_azure_key_1234567890',
      };
      expect(azureEngine.isConfigured(withKey)).toBe(true);
    });

    it('should configure YarnGPT African Voice AI free developer tier', () => {
      const yarnEngine = VOICE_ENGINES.yarngpt;
      expect(yarnEngine.nativeActors.yo).toContain('YarnGPT');
      expect(yarnEngine.badge).toContain('African NLP');

      expect(yarnEngine.isConfigured(DEFAULT_VOICE_CONFIG)).toBe(false);
      const withKey: VoiceEngineConfig = {
        ...DEFAULT_VOICE_CONFIG,
        yarnGptKey: 'yarngpt_token_abc123',
      };
      expect(yarnEngine.isConfigured(withKey)).toBe(true);
    });

    it('should configure Enhanced Browser Neural as offline zero-network fallback', () => {
      const browserEngine = VOICE_ENGINES.browser_neural;
      expect(browserEngine.isConfigured(DEFAULT_VOICE_CONFIG)).toBe(true);
      expect(browserEngine.badge).toContain('Zero Network');
    });
  });

  describe('Configuration Persistence', () => {
    it('should persist and retrieve customized voice settings from localStorage', () => {
      const customConfig: VoiceEngineConfig = {
        engine: 'azure_speech',
        azureKey: 'custom_azure_key_abcdef123',
        azureRegion: 'southafricanorth',
        yarnGptKey: 'yarn_sec_789',
        speed: 0.85,
        pitch: 1.08,
        volume: 1.0,
      };

      saveVoiceEngineConfig(customConfig);
      const loaded = loadVoiceEngineConfig();

      expect(loaded.engine).toBe('azure_speech');
      expect(loaded.azureKey).toBe('custom_azure_key_abcdef123');
      expect(loaded.azureRegion).toBe('southafricanorth');
      expect(loaded.yarnGptKey).toBe('yarn_sec_789');
      expect(loaded.speed).toBe(0.85);
    });
  });

  describe('Unified Voice Dispatcher & Fallback', () => {
    it('should dispatch speech cleanly without throwing when called', async () => {
      const onStart = vi.fn();
      const onEnd = vi.fn();

      await playUniversalBotVoice(
        'Ẹ kúnlẹ̀ o. Ẹ jọ̀wọ́ sọ ohun tí ó ṣẹlẹ̀ sí owó yín...',
        'yo',
        DEFAULT_VOICE_CONFIG,
        { onStart, onEnd }
      );

      expect(onStart).toHaveBeenCalled();
    });
  });
});
