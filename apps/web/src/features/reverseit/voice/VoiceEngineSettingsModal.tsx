import React, { useState } from 'react';
import {
  X,
  Volume2,
  Sparkles,
  Cloud,
  Cpu,
  Key,
  Globe,
  CheckCircle2,
  ExternalLink,
} from 'lucide-react';
import clsx from 'clsx';
import { Button } from '../../../components/Button';
import {
  type VoiceEngineConfig,
  type VoiceEngineType,
  VOICE_ENGINES,
  playUniversalBotVoice,
} from './VoiceEngineManager';
import {
  type SupportedLanguage,
  SUPPORTED_LANGUAGES,
} from './VoiceDisputeEngine';

interface VoiceEngineSettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
  config: VoiceEngineConfig;
  onSaveConfig: (newConfig: VoiceEngineConfig) => void;
  currentLanguage: SupportedLanguage;
}

export const VoiceEngineSettingsModal: React.FC<VoiceEngineSettingsModalProps> = ({
  isOpen,
  onClose,
  config,
  onSaveConfig,
  currentLanguage,
}) => {
  const [selectedEngine, setSelectedEngine] = useState<VoiceEngineType>(config.engine);
  const [azureKey, setAzureKey] = useState(config.azureKey);
  const [azureRegion, setAzureRegion] = useState(config.azureRegion || 'eastus');
  const [yarnGptKey, setYarnGptKey] = useState(config.yarnGptKey);
  const [speed, setSpeed] = useState(config.speed || 0.88);
  const [isTestingAudio, setIsTestingAudio] = useState(false);
  const [testStatus, setTestStatus] = useState<string | null>(null);

  if (!isOpen) return null;

  const currentLangInfo = SUPPORTED_LANGUAGES[currentLanguage];

  const handleSave = () => {
    const updated: VoiceEngineConfig = {
      ...config,
      engine: selectedEngine,
      azureKey: azureKey.trim(),
      azureRegion: azureRegion.trim(),
      yarnGptKey: yarnGptKey.trim(),
      speed,
    };
    onSaveConfig(updated);
    onClose();
  };

  const handleTestPlayback = async () => {
    setIsTestingAudio(true);
    setTestStatus('Generating speech...');

    const tempConfig: VoiceEngineConfig = {
      ...config,
      engine: selectedEngine,
      azureKey: azureKey.trim(),
      azureRegion: azureRegion.trim(),
      yarnGptKey: yarnGptKey.trim(),
      speed,
    };

    try {
      await playUniversalBotVoice(
        currentLangInfo.greetingPrompt,
        currentLanguage,
        tempConfig,
        {
          onStart: () => setTestStatus('Playing audio...'),
          onEnd: () => {
            setIsTestingAudio(false);
            setTestStatus('Audio playback complete');
          },
          onError: () => {
            setIsTestingAudio(false);
            setTestStatus('Playback error. Reverting to fallback.');
          },
        }
      );
    } catch {
      setIsTestingAudio(false);
      setTestStatus('Playback error.');
    }
  };

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="engine-settings-title"
      className="fixed inset-0 z-60 flex items-center justify-center p-3 sm:p-4 bg-slate-950/80 backdrop-blur-md animate-in fade-in duration-200"
    >
      <div className="bg-[var(--surface)] border border-[var(--surface-border)] rounded-2xl shadow-2xl max-w-xl w-full overflow-hidden flex flex-col max-h-[90vh]">
        {/* Header */}
        <div className="p-4 sm:p-5 border-b border-[var(--surface-border)] bg-[var(--canvas-subdued)] flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-full bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-300 flex items-center justify-center">
              <Sparkles className="w-5 h-5" />
            </div>
            <div>
              <h3 id="engine-settings-title" className="text-[16px] font-bold text-[var(--text-primary)]">
                Voice Engine & Language API
              </h3>
              <p className="text-[12px] text-[var(--text-muted)]">
                Choose between 100% Free Studio Prompts, Free Azure Neural, or African Voice AI
              </p>
            </div>
          </div>

          <button
            onClick={onClose}
            className="p-1.5 rounded-full text-[var(--text-muted)] hover:text-[var(--text-primary)] hover:bg-[var(--surface-hover)] transition-colors"
            aria-label="Close settings"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Engine Selection Feed */}
        <div className="flex-1 p-4 sm:p-5 overflow-y-auto space-y-3.5">
          {/* Option 1: Studio Native Human Voice */}
          <div
            onClick={() => setSelectedEngine('studio_native')}
            className={clsx(
              'p-3.5 rounded-xl border cursor-pointer transition-all',
              selectedEngine === 'studio_native'
                ? 'border-emerald-500 bg-emerald-50/70 dark:bg-emerald-950/40 shadow-xs'
                : 'border-[var(--surface-border-subtle)] bg-[var(--surface)] hover:border-emerald-300'
            )}
          >
            <div className="flex items-start justify-between gap-2">
              <div className="flex items-center gap-2">
                <div className={clsx(
                  'w-4 h-4 rounded-full border flex items-center justify-center',
                  selectedEngine === 'studio_native'
                    ? 'border-emerald-600 bg-emerald-600'
                    : 'border-zinc-400'
                )}>
                  {selectedEngine === 'studio_native' && <div className="w-1.5 h-1.5 rounded-full bg-white" />}
                </div>
                <div>
                  <div className="flex items-center gap-2">
                    <span className="text-[13px] font-bold text-[var(--text-primary)]">
                      1. Studio Native Voice (Recommended)
                    </span>
                    <span className="text-[10px] font-mono-financial px-1.5 py-0.5 rounded bg-emerald-100 text-emerald-800 dark:bg-emerald-900/60 dark:text-emerald-300 font-extrabold">
                      100% Free • No Signup
                    </span>
                  </div>
                  <p className="text-[12px] text-[var(--text-muted)] mt-0.5 leading-relaxed">
                    Authentic native Nigerian voice audio for all 6 dispute steps (Greeting, Channel, Amount, Bank, Account, Confirmation).
                  </p>
                </div>
              </div>
              <CheckCircle2 className={clsx('w-4 h-4 shrink-0', selectedEngine === 'studio_native' ? 'text-emerald-600' : 'text-transparent')} />
            </div>
            <div className="mt-2 text-[11px] font-medium text-emerald-700 dark:text-emerald-400 bg-emerald-100/50 dark:bg-emerald-950/60 px-2 py-1 rounded">
              Current Language Voice: {VOICE_ENGINES.studio_native.nativeActors[currentLanguage]}
            </div>
          </div>

          {/* Option 2: Microsoft Azure Speech F0 Free Tier */}
          <div
            onClick={() => setSelectedEngine('azure_speech')}
            className={clsx(
              'p-3.5 rounded-xl border cursor-pointer transition-all',
              selectedEngine === 'azure_speech'
                ? 'border-emerald-500 bg-emerald-50/70 dark:bg-emerald-950/40 shadow-xs'
                : 'border-[var(--surface-border-subtle)] bg-[var(--surface)] hover:border-emerald-300'
            )}
          >
            <div className="flex items-start justify-between gap-2">
              <div className="flex items-center gap-2">
                <div className={clsx(
                  'w-4 h-4 rounded-full border flex items-center justify-center',
                  selectedEngine === 'azure_speech'
                    ? 'border-emerald-600 bg-emerald-600'
                    : 'border-zinc-400'
                )}>
                  {selectedEngine === 'azure_speech' && <div className="w-1.5 h-1.5 rounded-full bg-white" />}
                </div>
                <div>
                  <div className="flex items-center gap-2">
                    <span className="text-[13px] font-bold text-[var(--text-primary)]">
                      2. Microsoft Azure Neural (F0 Free Tier)
                    </span>
                    <span className="text-[10px] font-mono-financial px-1.5 py-0.5 rounded bg-blue-100 text-blue-800 dark:bg-blue-900/60 dark:text-blue-300 font-extrabold">
                      500k Chars/Mo Free
                    </span>
                  </div>
                  <p className="text-[12px] text-[var(--text-muted)] mt-0.5 leading-relaxed">
                    Uses official Microsoft neural models: Bunmi & Kola (Yorùbá), Gambo (Hausa), Nkechi (Igbo), Ekele (Pidgin/NG).
                  </p>
                </div>
              </div>
              <Cloud className={clsx('w-4 h-4 shrink-0', selectedEngine === 'azure_speech' ? 'text-emerald-600' : 'text-zinc-400')} />
            </div>

            {selectedEngine === 'azure_speech' && (
              <div
                className="mt-3 pt-3 border-t border-[var(--surface-border-subtle)] space-y-2.5 text-[12px]"
                onClick={(e) => e.stopPropagation()}
              >
                <div>
                  <label className="block text-[11px] font-bold text-[var(--text-secondary)] mb-1">
                    Azure Speech Key (Free F0 Subscription Key):
                  </label>
                  <div className="flex items-center gap-2">
                    <div className="relative flex-1">
                      <Key className="w-3.5 h-3.5 absolute left-2.5 top-2.5 text-zinc-400" />
                      <input
                        type="password"
                        value={azureKey}
                        onChange={(e) => setAzureKey(e.target.value)}
                        placeholder="Paste your free 32-character Azure key"
                        className="w-full pl-8 pr-3 py-1.5 bg-[var(--canvas-subdued)] border border-[var(--surface-border-subtle)] rounded-lg text-[12px] font-mono-financial focus:outline-none focus:border-emerald-500"
                      />
                    </div>
                  </div>
                </div>

                <div className="flex items-center justify-between gap-3">
                  <div className="flex-1">
                    <label className="block text-[10px] font-bold text-[var(--text-muted)] mb-1 uppercase">
                      Region:
                    </label>
                    <select
                      value={azureRegion}
                      onChange={(e) => setAzureRegion(e.target.value)}
                      className="w-full px-2 py-1 bg-[var(--canvas-subdued)] border border-[var(--surface-border-subtle)] rounded-lg text-[11px]"
                    >
                      <option value="eastus">East US (eastus)</option>
                      <option value="westeurope">West Europe (westeurope)</option>
                      <option value="southafricanorth">South Africa North (southafricanorth)</option>
                      <option value="centralus">Central US (centralus)</option>
                    </select>
                  </div>

                  <a
                    href="https://portal.azure.com/#create/Microsoft.CognitiveServicesSpeechServices"
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-[11px] text-blue-600 dark:text-blue-400 hover:underline flex items-center gap-1 shrink-0 self-end mb-1"
                  >
                    Get Free Azure Key <ExternalLink className="w-3 h-3" />
                  </a>
                </div>
              </div>
            )}
          </div>

          {/* Option 3: YarnGPT African Voice AI */}
          <div
            onClick={() => setSelectedEngine('yarngpt')}
            className={clsx(
              'p-3.5 rounded-xl border cursor-pointer transition-all',
              selectedEngine === 'yarngpt'
                ? 'border-emerald-500 bg-emerald-50/70 dark:bg-emerald-950/40 shadow-xs'
                : 'border-[var(--surface-border-subtle)] bg-[var(--surface)] hover:border-emerald-300'
            )}
          >
            <div className="flex items-start justify-between gap-2">
              <div className="flex items-center gap-2">
                <div className={clsx(
                  'w-4 h-4 rounded-full border flex items-center justify-center',
                  selectedEngine === 'yarngpt'
                    ? 'border-emerald-600 bg-emerald-600'
                    : 'border-zinc-400'
                )}>
                  {selectedEngine === 'yarngpt' && <div className="w-1.5 h-1.5 rounded-full bg-white" />}
                </div>
                <div>
                  <div className="flex items-center gap-2">
                    <span className="text-[13px] font-bold text-[var(--text-primary)]">
                      3. YarnGPT African Voice AI
                    </span>
                    <span className="text-[10px] font-mono-financial px-1.5 py-0.5 rounded bg-amber-100 text-amber-800 dark:bg-amber-900/60 dark:text-amber-300 font-extrabold">
                      Free Developer Credits
                    </span>
                  </div>
                  <p className="text-[12px] text-[var(--text-muted)] mt-0.5 leading-relaxed">
                    Lagos-built voice model specialized in Yorùbá, Hausa, Igbo, and Naija Pidgin accents.
                  </p>
                </div>
              </div>
              <Globe className={clsx('w-4 h-4 shrink-0', selectedEngine === 'yarngpt' ? 'text-emerald-600' : 'text-zinc-400')} />
            </div>

            {selectedEngine === 'yarngpt' && (
              <div
                className="mt-3 pt-3 border-t border-[var(--surface-border-subtle)] space-y-2 text-[12px]"
                onClick={(e) => e.stopPropagation()}
              >
                <label className="block text-[11px] font-bold text-[var(--text-secondary)]">
                  YarnGPT API Key:
                </label>
                <div className="flex items-center gap-2">
                  <input
                    type="password"
                    value={yarnGptKey}
                    onChange={(e) => setYarnGptKey(e.target.value)}
                    placeholder="Enter YarnGPT API Key"
                    className="flex-1 px-3 py-1.5 bg-[var(--canvas-subdued)] border border-[var(--surface-border-subtle)] rounded-lg text-[12px] font-mono-financial focus:outline-none focus:border-emerald-500"
                  />
                  <a
                    href="https://yarngpt.ai"
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-[11px] text-amber-600 dark:text-amber-400 hover:underline flex items-center gap-1 shrink-0"
                  >
                    yarngpt.ai <ExternalLink className="w-3 h-3" />
                  </a>
                </div>
              </div>
            )}
          </div>

          {/* Option 4: Enhanced Browser Synthesizer */}
          <div
            onClick={() => setSelectedEngine('browser_neural')}
            className={clsx(
              'p-3.5 rounded-xl border cursor-pointer transition-all',
              selectedEngine === 'browser_neural'
                ? 'border-emerald-500 bg-emerald-50/70 dark:bg-emerald-950/40 shadow-xs'
                : 'border-[var(--surface-border-subtle)] bg-[var(--surface)] hover:border-emerald-300'
            )}
          >
            <div className="flex items-start justify-between gap-2">
              <div className="flex items-center gap-2">
                <div className={clsx(
                  'w-4 h-4 rounded-full border flex items-center justify-center',
                  selectedEngine === 'browser_neural'
                    ? 'border-emerald-600 bg-emerald-600'
                    : 'border-zinc-400'
                )}>
                  {selectedEngine === 'browser_neural' && <div className="w-1.5 h-1.5 rounded-full bg-white" />}
                </div>
                <div>
                  <div className="flex items-center gap-2">
                    <span className="text-[13px] font-bold text-[var(--text-primary)]">
                      4. Enhanced Browser Neural Synthesizer
                    </span>
                    <span className="text-[10px] font-mono-financial px-1.5 py-0.5 rounded bg-zinc-100 text-zinc-800 dark:bg-zinc-800 dark:text-zinc-300 font-extrabold">
                      Offline Fallback
                    </span>
                  </div>
                  <p className="text-[12px] text-[var(--text-muted)] mt-0.5 leading-relaxed">
                    Uses local browser speech synthesis combined with our phonetic diacritic transliteration engine.
                  </p>
                </div>
              </div>
              <Cpu className={clsx('w-4 h-4 shrink-0', selectedEngine === 'browser_neural' ? 'text-emerald-600' : 'text-zinc-400')} />
            </div>
          </div>

          {/* Speed Cadence Slider */}
          <div className="p-3 bg-[var(--canvas-subdued)] rounded-xl border border-[var(--surface-border-subtle)] space-y-1.5">
            <div className="flex items-center justify-between text-[11px] font-bold text-[var(--text-secondary)]">
              <span>Speech Speed / Deliberate Articulation:</span>
              <span className="font-mono-financial text-emerald-600 dark:text-emerald-400">
                {speed.toFixed(2)}x {speed < 0.9 ? '(Ultra-Clear & Sharp)' : '(Standard)'}
              </span>
            </div>
            <input
              type="range"
              min="0.75"
              max="1.15"
              step="0.01"
              value={speed}
              onChange={(e) => setSpeed(parseFloat(e.target.value))}
              className="w-full accent-emerald-600 cursor-pointer"
            />
          </div>

          {/* Sound Check in Modal */}
          <div className="flex items-center justify-between p-3 rounded-xl bg-slate-900 text-white text-[12px]">
            <div className="flex items-center gap-2">
              <Volume2 className={clsx('w-4 h-4 text-emerald-400', isTestingAudio && 'animate-bounce')} />
              <div>
                <div className="font-bold">Test {currentLangInfo.nativeName} Audio</div>
                <div className="text-[10px] text-zinc-400">{testStatus || 'Hear how the selected engine sounds'}</div>
              </div>
            </div>

            <button
              onClick={handleTestPlayback}
              disabled={isTestingAudio}
              className="px-3 py-1.5 rounded-lg bg-emerald-600 hover:bg-emerald-700 disabled:opacity-50 text-white font-bold text-[11px] shadow-xs transition-transform active:scale-95"
            >
              {isTestingAudio ? 'Playing...' : 'Play Sample 🔊'}
            </button>
          </div>
        </div>

        {/* Footer Actions */}
        <div className="p-4 border-t border-[var(--surface-border)] bg-[var(--canvas-subdued)] flex items-center justify-between">
          <Button variant="ghost" size="sm" onClick={onClose}>
            Cancel
          </Button>

          <Button variant="primary" size="sm" onClick={handleSave}>
            Save & Apply Voice Engine
          </Button>
        </div>
      </div>
    </div>
  );
};
