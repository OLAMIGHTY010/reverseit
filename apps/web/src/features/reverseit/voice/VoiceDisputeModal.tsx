import React, { useState, useEffect, useRef } from 'react';
import {
  Mic,
  MicOff,
  Volume2,
  VolumeX,
  X,
  CheckCircle2,
  RotateCcw,
  Languages,
  ArrowRight,
  Sparkles,
} from 'lucide-react';
import clsx from 'clsx';
import { Button } from '../../../components/Button';
import {
  SUPPORTED_LANGUAGES,
  VOICE_DEMO_PRESETS,
  analyzeVoiceUtterance,
  speakNativeVoice,
  type SupportedLanguage,
  type DisputeSlots,
  type DialogueTurn,
  type MandatorySlot,
  type VoiceDemoPreset,
} from './VoiceDisputeEngine';

interface VoiceDisputeModalProps {
  isOpen: boolean;
  onClose: () => void;
  onDisputeCreated: (disputeData: {
    reference: string;
    channel: string;
    amount: number;
    currency: string;
    accountNumber: string;
    description: string;
    language: SupportedLanguage;
  }) => void;
}

export const VoiceDisputeModal: React.FC<VoiceDisputeModalProps> = ({
  isOpen,
  onClose,
  onDisputeCreated,
}) => {
  const [selectedLanguage, setSelectedLanguage] = useState<SupportedLanguage>('yo');
  const [voiceCadence, setVoiceCadence] = useState<'ultra-clear' | 'natural' | 'brisk'>('ultra-clear');
  const [isRecording, setIsRecording] = useState(false);
  const [isAiSpeaking, setIsAiSpeaking] = useState(false);
  const [activeSpeechText, setActiveSpeechText] = useState('');
  const [disputeSlots, setDisputeSlots] = useState<DisputeSlots>({});
  const [dialogueHistory, setDialogueHistory] = useState<DialogueTurn[]>([]);
  const [activePreset, setActivePreset] = useState<VoiceDemoPreset | null>(null);
  const [presetStep, setPresetStep] = useState<number>(0);

  const dialogueEndRef = useRef<HTMLDivElement>(null);

  const getSpeechRate = () => {
    if (voiceCadence === 'ultra-clear') return 0.85;
    if (voiceCadence === 'natural') return 0.92;
    return 1.02;
  };

  // Initialize initial greeting when modal opens or language changes
  useEffect(() => {
    if (!isOpen) return;

    const langInfo = SUPPORTED_LANGUAGES[selectedLanguage];
    const initialTurn: DialogueTurn = {
      id: `ai-init-${Date.now()}`,
      speaker: 'AI_ADVISOR',
      language: selectedLanguage,
      audioText: langInfo.greetingPrompt,
      translatedText: 'Welcome. Please tell us what happened with your money or failed transaction...',
      timestamp: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
    };

    setDialogueHistory([initialTurn]);
    setDisputeSlots({});
    setPresetStep(0);
    setActivePreset(null);

    // Speak initial greeting aloud with deliberate articulation & boost
    setIsAiSpeaking(true);
    speakNativeVoice(
      langInfo.greetingPrompt,
      selectedLanguage,
      { rate: getSpeechRate(), pitch: 1.06, volume: 1.0 },
      () => {
        setIsAiSpeaking(false);
      }
    );
  }, [isOpen, selectedLanguage]);

  const handleSoundCheck = () => {
    const langInfo = SUPPORTED_LANGUAGES[selectedLanguage];
    window.speechSynthesis?.cancel();
    setIsAiSpeaking(true);
    speakNativeVoice(
      langInfo.greetingPrompt,
      selectedLanguage,
      { rate: getSpeechRate(), pitch: 1.06, volume: 1.0 },
      () => {
        setIsAiSpeaking(false);
      }
    );
  };

  useEffect(() => {
    dialogueEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [dialogueHistory]);

  if (!isOpen) return null;

  const langInfo = SUPPORTED_LANGUAGES[selectedLanguage];

  // Handle Voice Utterance Processing
  const handleProcessSpeech = (utterance: string, translatedEnglish?: string) => {
    const customerTurn: DialogueTurn = {
      id: `cust-${Date.now()}`,
      speaker: 'CUSTOMER',
      language: selectedLanguage,
      audioText: utterance,
      translatedText: translatedEnglish,
      timestamp: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
    };

    const analysis = analyzeVoiceUtterance(disputeSlots, utterance, selectedLanguage);
    setDisputeSlots(analysis.updatedSlots);

    const nextDialogue = [...dialogueHistory, customerTurn];

    if (analysis.suggestedPrompt) {
      const aiTurn: DialogueTurn = {
        id: `ai-${Date.now()}`,
        speaker: 'AI_ADVISOR',
        language: selectedLanguage,
        audioText: analysis.suggestedPrompt.prompt,
        translatedText: analysis.suggestedPrompt.translatedEnglish,
        timestamp: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
        pendingSlotQuestion: analysis.missingSlot,
      };
      nextDialogue.push(aiTurn);

      // Play AI voice reply in native language with deliberate articulation & boost
      setIsAiSpeaking(true);
      speakNativeVoice(
        analysis.suggestedPrompt.prompt,
        selectedLanguage,
        { rate: getSpeechRate(), pitch: 1.06, volume: 1.0 },
        () => {
          setIsAiSpeaking(false);
        }
      );
    }

    setDialogueHistory(nextDialogue);
    setActiveSpeechText('');
  };

  // Toggle live microphone with Web Speech API
  const handleToggleRecord = () => {
    if (isRecording) {
      setIsRecording(false);
      return;
    }

    if (typeof window !== 'undefined' && 'webkitSpeechRecognition' in window) {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const SpeechRecognition = (window as any).webkitSpeechRecognition;
      const recognition = new SpeechRecognition();
      recognition.lang = langInfo.speechCode;
      recognition.continuous = false;
      recognition.interimResults = true;

      recognition.onstart = () => {
        setIsRecording(true);
      };

      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      recognition.onresult = (event: any) => {
        const transcript = event.results[0][0].transcript;
        setActiveSpeechText(transcript);
        if (event.results[0].isFinal) {
          setIsRecording(false);
          handleProcessSpeech(transcript);
        }
      };

      recognition.onerror = () => {
        setIsRecording(false);
      };

      recognition.onend = () => {
        setIsRecording(false);
      };

      recognition.start();
    } else {
      // Simulated interactive prompt if browser lacks Web Speech API
      const sampleText = activePreset ? activePreset.initialSpeech : 'Mo lọ sí ATM Marina ogbọ̀n ẹgbẹrun kò jáde.';
      handleProcessSpeech(sampleText, 'I went to Marina ATM, ₦30,000 did not dispense.');
    }
  };

  // Execute Demo Preset
  const handleRunPreset = (preset: VoiceDemoPreset) => {
    setSelectedLanguage(preset.language);
    setActivePreset(preset);
    setPresetStep(1);

    // Turn 1
    handleProcessSpeech(preset.initialSpeech, preset.translatedEnglish);
  };

  // Advance Preset to Turn 2 (Answering Missing Data Interrogation)
  const handleAdvancePresetTurn2 = () => {
    if (!activePreset) return;
    setPresetStep(2);
    handleProcessSpeech(activePreset.followupSpeech, activePreset.followupEnglish);
  };

  const handleReset = () => {
    setDialogueHistory([]);
    setDisputeSlots({});
    setActivePreset(null);
    setPresetStep(0);
    setActiveSpeechText('');
    window.speechSynthesis?.cancel();
  };

  const isAllSlotsFulfilled = Boolean(
    disputeSlots.channel &&
      disputeSlots.amount &&
      disputeSlots.locationOrBank &&
      disputeSlots.accountOrPhone
  );

  const handleConfirmAndLaunchCase = () => {
    const generatedRef = `REV-VOICE-${Date.now().toString(36).toUpperCase()}`;
    onDisputeCreated({
      reference: generatedRef,
      channel: disputeSlots.channel || 'ATM',
      amount: disputeSlots.amount || 25000,
      currency: disputeSlots.currency || 'NGN',
      accountNumber: disputeSlots.accountOrPhone || '0128394910',
      description: `[VOICE_INTAKE_${selectedLanguage.toUpperCase()}] Incident at ${disputeSlots.locationOrBank || 'ATM'}. Customer voice lodgement.`,
      language: selectedLanguage,
    });
    onClose();
  };

  const slotPills: Array<{ key: MandatorySlot; label: string; value: string | undefined }> = [
    { key: 'channel', label: 'Channel', value: disputeSlots.channel },
    {
      key: 'amount',
      label: 'Amount',
      value: disputeSlots.amount
        ? `${disputeSlots.currency || 'NGN'} ${disputeSlots.amount.toLocaleString()}`
        : undefined,
    },
    { key: 'locationOrBank', label: 'Terminal / Bank', value: disputeSlots.locationOrBank },
    { key: 'accountOrPhone', label: 'Account / Phone', value: disputeSlots.accountOrPhone },
  ];

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="voice-modal-title"
      className="fixed inset-0 z-50 flex items-center justify-center p-3 sm:p-4 bg-slate-950/70 backdrop-blur-sm animate-in fade-in duration-200"
    >
      <div className="bg-[var(--surface)] border border-[var(--surface-border)] rounded-2xl shadow-2xl max-w-2xl w-full overflow-hidden flex flex-col max-h-[92vh]">
        {/* Header */}
        <div className="p-4 sm:p-5 border-b border-[var(--surface-border)] bg-[var(--canvas-subdued)] flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-full bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-300 flex items-center justify-center shadow-xs">
              <Languages className="w-5 h-5" />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <h3 id="voice-modal-title" className="text-[16px] font-bold text-[var(--text-primary)]">
                  Voice Dispute Lodgement
                </h3>
                <span className="text-[10px] font-mono-financial font-extrabold uppercase px-2 py-0.5 rounded bg-emerald-100 text-emerald-900 dark:bg-emerald-950 dark:text-emerald-300">
                  Multilingual AI
                </span>
              </div>
              <p className="text-[12px] text-[var(--text-muted)]">
                Speak freely in your native language. Our AI will listen, translate, and verify your details.
              </p>
            </div>
          </div>

          <button
            onClick={onClose}
            className="p-1.5 rounded-full text-[var(--text-muted)] hover:text-[var(--text-primary)] hover:bg-[var(--surface-hover)] transition-colors"
            aria-label="Close"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Language Selector Bar */}
        <div className="px-4 py-2.5 bg-[var(--surface)] border-b border-[var(--surface-border-subtle)] flex items-center justify-between gap-2 overflow-x-auto text-[12px]">
          <div className="flex items-center gap-1.5 overflow-x-auto">
            <span className="text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)] shrink-0 mr-1">
              Language:
            </span>
            {(Object.keys(SUPPORTED_LANGUAGES) as SupportedLanguage[]).map((langKey) => {
              const l = SUPPORTED_LANGUAGES[langKey];
              const isSelected = selectedLanguage === langKey;
              return (
                <button
                  key={langKey}
                  onClick={() => {
                    setSelectedLanguage(langKey);
                    window.speechSynthesis?.cancel();
                  }}
                  className={clsx(
                    'px-3 py-1 rounded-full text-[12px] font-bold flex items-center gap-1.5 shrink-0 transition-all',
                    isSelected
                      ? 'bg-emerald-600 text-white shadow-xs'
                      : 'bg-[var(--canvas-subdued)] border border-[var(--surface-border-subtle)] text-[var(--text-secondary)] hover:border-emerald-400'
                  )}
                >
                  <span>{l.flag}</span>
                  <span>{l.nativeName}</span>
                </button>
              );
            })}
          </div>
        </div>

        {/* Voice Clarity & Sound Check Bar */}
        <div className="px-4 py-2 bg-emerald-50/60 dark:bg-emerald-950/40 border-b border-[var(--surface-border-subtle)] flex items-center justify-between gap-3 text-[11px] overflow-x-auto">
          <div className="flex items-center gap-2.5 shrink-0">
            <button
              onClick={handleSoundCheck}
              className="px-2.5 py-1 rounded-full bg-emerald-600 hover:bg-emerald-700 text-white font-bold flex items-center gap-1.5 shadow-xs transition-all active:scale-95"
              title="Test the crystal clear pronunciation of the selected language"
            >
              <Volume2 className={clsx('w-3.5 h-3.5', isAiSpeaking && 'animate-bounce')} />
              <span>Test Clear Voice 🔊</span>
            </button>

            <span className="text-[10px] text-[var(--text-muted)] font-medium hidden sm:inline">
              Pacing / Cadence:
            </span>

            {/* Cadence Pills */}
            <div className="flex items-center bg-[var(--surface)] border border-[var(--surface-border-subtle)] rounded-full p-0.5 text-[10px] font-bold">
              {(
                [
                  { id: 'ultra-clear', label: '0.85x Ultra-Clear' },
                  { id: 'natural', label: '0.92x Natural' },
                  { id: 'brisk', label: '1.0x Normal' },
                ] as const
              ).map((mode) => (
                <button
                  key={mode.id}
                  onClick={() => setVoiceCadence(mode.id)}
                  className={clsx(
                    'px-2 py-0.5 rounded-full transition-all',
                    voiceCadence === mode.id
                      ? 'bg-emerald-600 text-white shadow-xs'
                      : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)]'
                  )}
                >
                  {mode.label}
                </button>
              ))}
            </div>
          </div>

          <div className="flex items-center gap-2 text-[10px] text-emerald-800 dark:text-emerald-300 font-mono-financial shrink-0">
            <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 animate-pulse" />
            <span>100% Vol • Acoustic Boost • Phonetic Tuning Active</span>
          </div>
        </div>

        {/* Real-Time Mandatory Slot Tracker */}
        <div className="px-4 py-2.5 bg-slate-900 text-white flex items-center justify-between text-[11px] font-mono-financial overflow-x-auto">
          <div className="flex items-center gap-2">
            <span className="text-zinc-400 font-sans uppercase font-bold text-[10px] tracking-wider">
              Dispute Slots:
            </span>
            <div className="flex items-center gap-2">
              {slotPills.map(({ key, label, value }) => (
                <div
                  key={key}
                  className={clsx(
                    'px-2 py-0.5 rounded flex items-center gap-1 text-[11px] transition-all',
                    value
                      ? 'bg-emerald-950 text-emerald-300 border border-emerald-500 font-bold'
                      : 'bg-zinc-800 text-zinc-400 border border-zinc-700'
                  )}
                >
                  {value ? (
                    <CheckCircle2 className="w-3 h-3 text-emerald-400" />
                  ) : (
                    <div className="w-2 h-2 rounded-full bg-zinc-500" />
                  )}
                  <span>{label}:</span>
                  <span className={value ? 'text-white' : 'italic text-zinc-500'}>
                    {value || 'Missing'}
                  </span>
                </div>
              ))}
            </div>
          </div>

          <button
            onClick={handleReset}
            className="text-zinc-400 hover:text-white flex items-center gap-1 text-[10px] ml-2 shrink-0"
          >
            <RotateCcw className="w-3 h-3" /> Reset
          </button>
        </div>

        {/* Dialogue Feed */}
        <div className="flex-1 p-4 overflow-y-auto space-y-3 min-h-[220px] max-h-[340px] bg-[var(--canvas-subdued)]">
          {dialogueHistory.map((turn) => {
            const isCustomer = turn.speaker === 'CUSTOMER';
            return (
              <div
                key={turn.id}
                className={clsx('flex flex-col', isCustomer ? 'items-end' : 'items-start')}
              >
                <div
                  className={clsx(
                    'max-w-[85%] rounded-2xl p-3.5 shadow-xs space-y-1',
                    isCustomer
                      ? 'bg-emerald-600 text-white rounded-br-xs'
                      : 'bg-[var(--surface)] border border-[var(--surface-border)] text-[var(--text-primary)] rounded-bl-xs'
                  )}
                >
                  <div className="flex items-center justify-between gap-3 text-[10px] opacity-80 pb-0.5">
                    <span className="font-bold uppercase tracking-wider">
                      {isCustomer ? '👤 You (Spoken Voice)' : '🤖 SmartReversal Advisor'}
                    </span>
                    <span>{turn.timestamp}</span>
                  </div>

                  <p className="text-[13px] font-medium leading-relaxed">{turn.audioText}</p>

                  {turn.translatedText && (
                    <div
                      className={clsx(
                        'pt-1 mt-1 border-t text-[11px] italic',
                        isCustomer
                          ? 'border-emerald-500/50 text-emerald-100'
                          : 'border-[var(--surface-border-subtle)] text-[var(--text-muted)]'
                      )}
                    >
                      <span>EN: &ldquo;{turn.translatedText}&rdquo;</span>
                    </div>
                  )}

                  {!isCustomer && (
                    <button
                      onClick={() =>
                        speakNativeVoice(turn.audioText, turn.language, {
                          rate: getSpeechRate(),
                          pitch: 1.06,
                          volume: 1.0,
                        })
                      }
                      className="pt-1 flex items-center gap-1.5 text-[11px] font-bold text-emerald-600 dark:text-emerald-400 hover:underline"
                    >
                      <Volume2 className="w-3.5 h-3.5" /> Replay in HD {SUPPORTED_LANGUAGES[turn.language].nativeName} (Sharp & Loud)
                    </button>
                  )}
                </div>
              </div>
            );
          })}
          <div ref={dialogueEndRef} />
        </div>

        {/* Live Audio Input & Waveform Control Area */}
        <div className="p-5 border-t border-[var(--surface-border)] bg-[var(--surface)] space-y-4">
          {/* Active Speaking / Listening Banner */}
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              {isRecording ? (
                <div className="flex flex-col text-rose-600 font-bold text-[12px]">
                  <div className="flex items-center gap-2">
                    <div className="w-2.5 h-2.5 rounded-full bg-rose-600 animate-ping" />
                    <span>Listening in {langInfo.nativeName}... Speak now</span>
                  </div>
                  {activeSpeechText && (
                    <span className="text-[11px] font-normal italic text-[var(--text-primary)] mt-0.5">
                      &ldquo;{activeSpeechText}&rdquo;
                    </span>
                  )}
                </div>
              ) : isAiSpeaking ? (
                <div className="flex items-center gap-2 text-emerald-600 font-bold text-[12px]">
                  <Volume2 className="w-4 h-4 animate-bounce" />
                  <span>SmartReversal Advisor is speaking in {langInfo.nativeName}...</span>
                </div>
              ) : (
                <span className="text-[12px] text-[var(--text-muted)]">
                  Tap the microphone to speak or click a language preset below
                </span>
              )}
            </div>

            {isAiSpeaking && (
              <button
                onClick={() => {
                  window.speechSynthesis?.cancel();
                  setIsAiSpeaking(false);
                }}
                className="text-[11px] text-[var(--text-muted)] hover:text-rose-600 flex items-center gap-1"
              >
                <VolumeX className="w-3.5 h-3.5" /> Stop Voice
              </button>
            )}
          </div>

          {/* Central Pulsating Mic Button */}
          <div className="flex flex-col items-center justify-center py-1">
            <button
              onClick={handleToggleRecord}
              className={clsx(
                'relative w-20 h-20 rounded-full flex items-center justify-center transition-all duration-300 shadow-xl focus:outline-none',
                isRecording
                  ? 'bg-rose-600 text-white scale-110 ring-8 ring-rose-200 dark:ring-rose-950 animate-pulse'
                  : 'bg-emerald-600 hover:bg-emerald-700 text-white hover:scale-105 active:scale-95'
              )}
              aria-label={isRecording ? 'Stop recording' : 'Start voice recording'}
            >
              {isRecording ? <MicOff className="w-9 h-9" /> : <Mic className="w-9 h-9" />}

              {/* Decorative Audio Waves */}
              {isRecording && (
                <span className="absolute -inset-2 rounded-full border-2 border-rose-400 animate-ping opacity-75 pointer-events-none" />
              )}
            </button>
            <span className="text-[11px] font-bold text-[var(--text-secondary)] mt-2">
              {isRecording ? 'Tap to Stop' : `Tap to Speak in ${langInfo.nativeName}`}
            </span>
          </div>

          {/* Quick Demo Voice Presets */}
          <div className="space-y-1.5 pt-1">
            <div className="flex items-center justify-between text-[11px] text-[var(--text-muted)] font-semibold">
              <span className="flex items-center gap-1">
                <Sparkles className="w-3.5 h-3.5 text-amber-500" /> Test Conversational Voice Presets:
              </span>
              {activePreset && presetStep === 1 && !isAllSlotsFulfilled && (
                <button
                  onClick={handleAdvancePresetTurn2}
                  className="text-emerald-600 dark:text-emerald-400 font-bold hover:underline animate-pulse flex items-center gap-1 text-[11px]"
                >
                  Click to reply with missing account number &rarr;
                </button>
              )}
            </div>

            <div className="grid grid-cols-2 sm:grid-cols-3 gap-1.5">
              {VOICE_DEMO_PRESETS.map((p) => (
                <button
                  key={p.id}
                  onClick={() => handleRunPreset(p)}
                  className={clsx(
                    'p-2 rounded border text-left text-[11px] transition-all',
                    activePreset?.id === p.id
                      ? 'border-emerald-500 bg-emerald-50/60 dark:bg-emerald-950/40 font-bold text-emerald-900 dark:text-emerald-300'
                      : 'border-[var(--surface-border-subtle)] bg-[var(--canvas-subdued)] hover:border-emerald-300 text-[var(--text-secondary)]'
                  )}
                >
                  <div className="truncate">{p.title}</div>
                </button>
              ))}
            </div>
          </div>
        </div>

        {/* Footer Actions */}
        <div className="p-4 border-t border-[var(--surface-border)] bg-[var(--canvas-subdued)] flex items-center justify-between">
          <Button variant="ghost" size="sm" onClick={onClose}>
            Cancel
          </Button>

          <Button
            variant="primary"
            size="sm"
            disabled={!isAllSlotsFulfilled}
            onClick={handleConfirmAndLaunchCase}
            rightIcon={<ArrowRight className="w-4 h-4" />}
          >
            {isAllSlotsFulfilled
              ? 'Confirm & Launch Pizza-Tracker Stepper'
              : 'Provide Details to Proceed'}
          </Button>
        </div>
      </div>
    </div>
  );
};
