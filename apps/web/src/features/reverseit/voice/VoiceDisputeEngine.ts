/**
 * Multilingual Voice Dispute Intake & Conversational Slot-Filling Engine
 * Supports: Yorùbá, Hausa, Igbo, Nigerian Pidgin, Français, and English
 */

export type SupportedLanguage = 'yo' | 'ha' | 'ig' | 'pcm' | 'fr' | 'en';

export interface LanguageInfo {
  code: SupportedLanguage;
  name: string;
  nativeName: string;
  flag: string;
  speechCode: string;
  greetingPrompt: string;
}

export const SUPPORTED_LANGUAGES: Record<SupportedLanguage, LanguageInfo> = {
  yo: {
    code: 'yo',
    name: 'Yoruba',
    nativeName: 'Yorùbá',
    flag: '🇳🇬',
    speechCode: 'yo-NG',
    greetingPrompt: 'Ẹ kúnlẹ̀ o. Ẹ jọ̀wọ́ sọ ohun tí ó ṣẹlẹ̀ sí owó yín...',
  },
  ha: {
    code: 'ha',
    name: 'Hausa',
    nativeName: 'Hausa',
    flag: '🇳🇬',
    speechCode: 'ha-NG',
    greetingPrompt: 'Sannu da zuwa. Don Allah bayyana abin da ya faru da kudin ku...',
  },
  ig: {
    code: 'ig',
    name: 'Igbo',
    nativeName: 'Asụsụ Igbo',
    flag: '🇳🇬',
    speechCode: 'ig-NG',
    greetingPrompt: 'Nnọọ. Biko kọwaa ihe mere na ego gị...',
  },
  pcm: {
    code: 'pcm',
    name: 'Pidgin',
    nativeName: 'Naija Pidgin',
    flag: '🇳🇬',
    speechCode: 'pcm-NG',
    greetingPrompt: 'Welcome o. Wetin happen to your money? Just talk am...',
  },
  fr: {
    code: 'fr',
    name: 'French',
    nativeName: 'Français',
    flag: '🇫🇷',
    speechCode: 'fr-FR',
    greetingPrompt: 'Bienvenue. Veuillez expliquer le problème rencontré avec votre transaction...',
  },
  en: {
    code: 'en',
    name: 'English',
    nativeName: 'English (Nigeria)',
    flag: '🇬🇧',
    speechCode: 'en-NG',
    greetingPrompt: 'Welcome to SmartReversal voice intake. Please tell us what happened...',
  },
};

export type MandatorySlot = 'channel' | 'amount' | 'locationOrBank' | 'accountOrPhone';

export interface DisputeSlots {
  channel?: 'ATM' | 'POS' | 'MOBILE_TRANSFER' | 'WEB';
  amount?: number;
  currency?: string;
  locationOrBank?: string;
  accountOrPhone?: string;
  incidentTime?: string;
  rawTranscript?: string;
  translatedEnglish?: string;
}

export interface DialogueTurn {
  id: string;
  speaker: 'CUSTOMER' | 'AI_ADVISOR';
  language: SupportedLanguage;
  audioText: string;
  translatedText?: string;
  timestamp: string;
  fulfilledSlot?: MandatorySlot;
  pendingSlotQuestion?: MandatorySlot;
}

export const SLOT_INTERROGATION_PROMPTS: Record<
  MandatorySlot | 'confirmation_success',
  Record<SupportedLanguage, { prompt: string; translatedEnglish: string }>
> = {
  channel: {
    yo: {
      prompt: 'Ǹjẹ́ orí ATM, POS ni àbí ètò ìfowóránṣẹ́ orí fóònù ni ìṣẹ̀lẹ̀ náà ti ṣẹlẹ̀?',
      translatedEnglish: 'Did this incident occur at an ATM, a POS terminal, or via mobile transfer?',
    },
    ha: {
      prompt: 'Shin wannan ya faru ne a kan ATM, POS ko kuma tura kudi ta wayar salula?',
      translatedEnglish: 'Did this incident occur at an ATM, a POS terminal, or via mobile transfer?',
    },
    ig: {
      prompt: 'Ọ bụ na ATM, POS ka ọ bụ nzipu ego na ekwentị ka nke a mere?',
      translatedEnglish: 'Did this incident occur at an ATM, a POS terminal, or via mobile transfer?',
    },
    pcm: {
      prompt: 'Na for ATM machine, POS shop, or na mobile bank transfer e happen?',
      translatedEnglish: 'Did this incident occur at an ATM, a POS terminal, or via mobile transfer?',
    },
    fr: {
      prompt: 'Est-ce que cela s\'est produit sur un distributeur ATM, un terminal TPE ou par virement mobile?',
      translatedEnglish: 'Did this happen at an ATM, POS terminal, or mobile transfer?',
    },
    en: {
      prompt: 'Did this dispute occur at an ATM, a merchant POS terminal, or via mobile app transfer?',
      translatedEnglish: 'Did this dispute occur at an ATM, a merchant POS terminal, or via mobile app transfer?',
    },
  },
  amount: {
    yo: {
      prompt: 'Ẹ ṣeun fún àlàyé yín. Ẹ jọ̀wọ́, èlò gangan ni iye owó tí ẹ fẹ́ yọ tí kò jáde?',
      translatedEnglish: 'Thank you for explaining. Please, what was the exact amount that failed to dispense?',
    },
    ha: {
      prompt: 'Mun gode da bayanin ku. Don Allah, nawa ne ainihin kudin da ba a bayar ba?',
      translatedEnglish: 'Thank you for explaining. Please, what was the exact amount that was not dispensed?',
    },
    ig: {
      prompt: 'Daalụ maka nkọwa gị. Biko, ego ole kpọmkwem ka igwe ahụ na-ewepụtaghị?',
      translatedEnglish: 'Thank you for explaining. Please, how much exactly did the machine fail to release?',
    },
    pcm: {
      prompt: 'Thank you for the update. Abeg, how much be the exact money wey no comot?',
      translatedEnglish: 'Thank you for the update. Please, what was the exact money that was deducted?',
    },
    fr: {
      prompt: 'Merci pour ces précisions. S\'il vous plaît, quel était le montant exact non distribué ou débité?',
      translatedEnglish: 'Thank you. Please, what was the exact amount debited or not dispensed?',
    },
    en: {
      prompt: 'Thank you. Please specify the exact transaction amount that was debited or failed.',
      translatedEnglish: 'Thank you. Please specify the exact transaction amount that was debited or failed.',
    },
  },
  locationOrBank: {
    yo: {
      prompt: 'Báwo ni orúkọ bánkì tàbí ibi tí ATM tàbí POS náà wà ṣe rí?',
      translatedEnglish: 'What is the name of the bank or location where the ATM or POS terminal was located?',
    },
    ha: {
      prompt: 'Wanne banki ne ko kuma a ina wannan ATM ko POS din yake?',
      translatedEnglish: 'Which bank or where was this ATM or POS located?',
    },
    ig: {
      prompt: 'Kedu aha ụlọ akụ ahụ ma ọ bụ ebe igwe ATM ma ọ bụ POS ahụ dị?',
      translatedEnglish: 'What is the bank name or where is the ATM or POS located?',
    },
    pcm: {
      prompt: 'Which bank or which area dat ATM or POS machine dey?',
      translatedEnglish: 'Which bank or area was that ATM or POS terminal located?',
    },
    fr: {
      prompt: 'Quelle est la banque concernée ou l\'emplacement exact du distributeur?',
      translatedEnglish: 'Which bank or exact location was the ATM/terminal at?',
    },
    en: {
      prompt: 'Which bank, terminal location, or destination institution was involved?',
      translatedEnglish: 'Which bank, terminal location, or destination institution was involved?',
    },
  },
  accountOrPhone: {
    yo: {
      prompt: 'Ẹ jọ̀wọ́, kí ni nọ́mbà àkọ́ọ́lẹ̀ yín tàbí nọ́mbà fóònù yín fún ìfidánilójú?',
      translatedEnglish: 'Please, what is your 10-digit account number or phone number for verification?',
    },
    ha: {
      prompt: 'Don Allah, wanne lambar asusun ku ne ko kuma lambar wayar ku don tantancewa?',
      translatedEnglish: 'Please, what is your account number or phone number for verification?',
    },
    ig: {
      prompt: 'Biko, kedu nọmba akaụntụ gị ma ọ bụ nọmba ekwentị gị maka nkwenye?',
      translatedEnglish: 'Please, what is your account number or phone number for verification?',
    },
    pcm: {
      prompt: 'Abeg tell us your 10-digit account number or phone number make we take trace am.',
      translatedEnglish: 'Please tell us your 10-digit account number or phone number to trace the transaction.',
    },
    fr: {
      prompt: 'S\'il vous plaît, quel est votre numéro de compte ou votre numéro de téléphone pour la vérification?',
      translatedEnglish: 'Please, what is your account number or phone number for verification?',
    },
    en: {
      prompt: 'Please provide your 10-digit account number or registered phone number.',
      translatedEnglish: 'Please provide your 10-digit account number or registered phone number.',
    },
  },
  confirmation_success: {
    yo: {
      prompt: 'A ti gba ẹ̀sùn yín wọlé kẹ́kẹ́. Ẹ̀sùn yín ti bẹ̀rẹ̀ sí ní ṣe àyẹ̀wò, owó yín yóò padà láàrín wákàtí mẹ́rinlelogun.',
      translatedEnglish: 'Your claim has been successfully received. Reversal investigation is in progress and funds will be restored within 24 hours.',
    },
    ha: {
      prompt: 'An karbi rahoton ku cikin nasara. Za mu bincika tare da mayar muku da kudin ku nan da sa\'o\'i 24.',
      translatedEnglish: 'Your report has been successfully received. We will investigate and restore your funds within 24 hours.',
    },
    ig: {
      prompt: 'Anabatala mkpesa gị nke ọma. Anyị na-enyocha ya ma weghachite ego gị n\'ime awa iri abụọ na anọ.',
      translatedEnglish: 'Your dispute has been accepted successfully. We are reviewing and will return your funds within 24 hours.',
    },
    pcm: {
      prompt: 'We don successfully receive your complaint. Automated investigation don start, your money go land within 24 hours.',
      translatedEnglish: 'Your complaint has been successfully received. Automated investigation has started, funds will arrive within 24 hours.',
    },
    fr: {
      prompt: 'Votre réclamation a été enregistrée avec succès. Votre remboursement est en cours d\'investigation et sera traité dans les 24 heures.',
      translatedEnglish: 'Your claim has been registered. Investigation is underway and refund will be processed within 24 hours.',
    },
    en: {
      prompt: 'Your dispute has been registered successfully. Real-time switch reconciliation is active and funds will be restored within guaranteed Central Bank SLA.',
      translatedEnglish: 'Your dispute has been registered successfully. Real-time switch reconciliation is active and funds will be restored within guaranteed Central Bank SLA.',
    },
  },
};

/**
 * Natural Language Number & Amount Extractor across African Languages
 */
export function extractAmountFromVoiceText(text: string): number | undefined {
  const clean = text.toLowerCase();

  // 1. Direct digits (e.g. ₦45,000, 30000, 25k)
  const kMatch = clean.match(/(\d+)\s*(k|thousand)/);
  if (kMatch) {
    return parseInt(kMatch[1], 10) * 1000;
  }

  const directDigit = clean.match(/(?:(?:ngn|naira|₦|cfa|fcfa)\s*)?([0-9]{1,3}(?:,[0-9]{3})+|[0-9]{3,7})(?:\.\d{2})?/);
  if (directDigit) {
    const parsed = parseFloat(directDigit[1].replace(/,/g, ''));
    if (!isNaN(parsed) && parsed > 0) return parsed;
  }

  // 2. Yoruba spoken amounts
  if (clean.includes('ogbọn egberun') || clean.includes('ogbọ̀n ẹgbẹrun')) return 30000;
  if (clean.includes('ogun egberun') || clean.includes('ogun ẹgbẹrun')) return 20000;
  if (clean.includes('egberun mewa') || clean.includes('ẹgbẹrun mẹ́wàá')) return 10000;
  if (clean.includes('aarundinlogoji egberun') || clean.includes('aarundinlogoji')) return 35000;
  if (clean.includes('aadota egberun') || clean.includes('aadọta ẹgbẹrun')) return 50000;
  if (clean.includes('ogorun egberun') || clean.includes('ọgọ́rùn-ún ẹgbẹrun')) return 100000;

  // 3. Hausa spoken amounts
  if (clean.includes('dubu ashirin')) return 20000;
  if (clean.includes('dubu talatin')) return 30000;
  if (clean.includes('dubu arba\'in') || clean.includes('dubu arbain')) return 40000;
  if (clean.includes('dubu hamsin')) return 50000;
  if (clean.includes('dubu goma')) return 10000;
  if (clean.includes('dubu dari')) return 100000;

  // 4. Igbo spoken amounts
  if (clean.includes('puku iri abụọ') || clean.includes('puku iri abuo')) return 20000;
  if (clean.includes('puku iri atọ') || clean.includes('puku iri ato')) return 30000;
  if (clean.includes('puku iri anọ') || clean.includes('puku iri ano')) return 40000;
  if (clean.includes('puku iri ise')) return 50000;
  if (clean.includes('puku iri')) return 10000;

  // 5. French spoken amounts
  if (clean.includes('vingt mille')) return 20000;
  if (clean.includes('trente mille')) return 30000;
  if (clean.includes('quarante mille')) return 40000;
  if (clean.includes('cinquante mille')) return 50000;
  if (clean.includes('soixante mille')) return 60000;
  if (clean.includes('soixante-dix mille')) return 70000;
  if (clean.includes('quatre-vingts mille') || clean.includes('quatre-vingt mille')) return 80000;
  if (clean.includes('cent mille')) return 100000;

  return undefined;
}

/**
 * Channel Extraction from Natural Voice
 */
export function extractChannelFromVoiceText(text: string): 'ATM' | 'POS' | 'MOBILE_TRANSFER' | 'WEB' | undefined {
  const clean = text.toLowerCase();

  if (
    clean.includes('atm') ||
    clean.includes('machine') ||
    clean.includes('igwe') ||
    clean.includes('dispense') ||
    clean.includes('distributeur') ||
    clean.includes('guichet')
  ) {
    return 'ATM';
  }
  if (
    clean.includes('pos') ||
    clean.includes('merchant') ||
    clean.includes('shop') ||
    clean.includes('agent') ||
    clean.includes('swipe') ||
    clean.includes('tpe')
  ) {
    return 'POS';
  }
  if (
    clean.includes('transfer') ||
    clean.includes('app') ||
    clean.includes('sent') ||
    clean.includes('send') ||
    clean.includes('tura') ||
    clean.includes('foworanse') ||
    clean.includes('virement')
  ) {
    return 'MOBILE_TRANSFER';
  }
  if (clean.includes('web') || clean.includes('online') || clean.includes('internet') || clean.includes('website')) {
    return 'WEB';
  }

  return undefined;
}

/**
 * Location or Bank Name Extraction
 */
export function extractLocationOrBank(text: string): string | undefined {
  const clean = text.toLowerCase();
  const knownBanks = [
    'gtbank', 'gtb', 'firstbank', 'first bank', 'zenith', 'access', 'access bank',
    'uba', 'wema', 'alat', 'stanbic', 'fidelity', 'opay', 'palmpay', 'kuda', 'ecobank'
  ];

  for (const b of knownBanks) {
    if (clean.includes(b)) {
      return b.toUpperCase();
    }
  }

  // Check for ATM/POS + Location name (e.g. "ATM Marina", "ATM a Kano", "POS Ikeja")
  const atmLocMatch = clean.match(/(?:atm|pos|terminal|guichet)\s+(?:a\s+|at\s+|in\s+|for\s+|ni\s+|sí\s+|si\s+)?([a-zA-Z]{3,20})/);
  if (atmLocMatch && atmLocMatch[1]) {
    const loc = atmLocMatch[1].trim();
    if (!['machine', 'dispenser', 'terminal', 'card', 'error', 'branch', 'kwuo', 'naira', 'cire'].includes(loc)) {
      return loc.charAt(0).toUpperCase() + loc.slice(1);
    }
  }

  const locationMatches = clean.match(/\b(?:at|near|around|ni|sí|si|n'ime|chez)\s+([a-zA-Z]{3,20})/);
  if (locationMatches && locationMatches[1]) {
    const loc = locationMatches[1].trim();
    if (loc.length > 2 && !['the', 'my', 'one', 'some', 'atm', 'pos', 'first'].includes(loc)) {
      return loc.charAt(0).toUpperCase() + loc.slice(1);
    }
  }

  return undefined;
}

/**
 * Account or Phone Number Extraction
 */
export function extractAccountOrPhone(text: string): string | undefined {
  // 10 digit NUBAN account or 11 digit Nigerian phone number
  const match = text.match(/\b(0\d{9,10}|\d{10})\b/);
  if (match) {
    return match[1];
  }
  return undefined;
}

/**
 * Conversational Slot Analyzer & Missing Entity Detector
 */
export function analyzeVoiceUtterance(
  currentSlots: DisputeSlots,
  newUtterance: string,
  language: SupportedLanguage
): {
  updatedSlots: DisputeSlots;
  fulfilledSlot?: MandatorySlot;
  missingSlot?: MandatorySlot;
  suggestedPrompt?: { prompt: string; translatedEnglish: string };
  isComplete: boolean;
} {
  const updatedSlots: DisputeSlots = { ...currentSlots };
  let fulfilledSlot: MandatorySlot | undefined = undefined;

  // Extract slots
  if (!updatedSlots.channel) {
    const ch = extractChannelFromVoiceText(newUtterance);
    if (ch) {
      updatedSlots.channel = ch;
      fulfilledSlot = 'channel';
    }
  }

  if (!updatedSlots.amount) {
    const amt = extractAmountFromVoiceText(newUtterance);
    if (amt) {
      updatedSlots.amount = amt;
      updatedSlots.currency = language === 'fr' ? 'XOF' : 'NGN';
      fulfilledSlot = 'amount';
    }
  }

  if (!updatedSlots.locationOrBank) {
    const loc = extractLocationOrBank(newUtterance);
    if (loc) {
      updatedSlots.locationOrBank = loc;
      fulfilledSlot = 'locationOrBank';
    }
  }

  if (!updatedSlots.accountOrPhone) {
    const acc = extractAccountOrPhone(newUtterance);
    if (acc) {
      updatedSlots.accountOrPhone = acc;
      fulfilledSlot = 'accountOrPhone';
    }
  }

  // Determine which mandatory slot is still missing
  const mandatoryOrder: MandatorySlot[] = ['channel', 'amount', 'locationOrBank', 'accountOrPhone'];
  const missingSlot = mandatoryOrder.find((slot) => !updatedSlots[slot]);

  const isComplete = !missingSlot;
  const suggestedPrompt = missingSlot
    ? SLOT_INTERROGATION_PROMPTS[missingSlot][language]
    : SLOT_INTERROGATION_PROMPTS.confirmation_success[language];

  return {
    updatedSlots,
    fulfilledSlot,
    missingSlot,
    suggestedPrompt,
    isComplete,
  };
}

/**
 * Phonetic Normalizer for African & Regional Languages
 * Converts diacritics and complex tone marks into crisp, articulate phonetic equivalents
 * that speech synthesis engines pronounce smoothly without choking or skipping syllables.
 */
export function normalizeTextForSpeech(text: string, language: SupportedLanguage): string {
  if (!text) return '';

  if (language === 'yo') {
    return text
      // Convert specific Yoruba words to their natural phonetic sound
      .replace(/Ẹ kúnlẹ̀ o/gi, 'E kuunle o')
      .replace(/Ẹ jọ̀wọ́/gi, 'E jowo')
      .replace(/ẹ jọ̀wọ́/gi, 'e jowo')
      .replace(/Ẹ ṣeun púpọ̀/gi, 'E sheun pupo')
      .replace(/ẹ ṣeun púpọ̀/gi, 'e sheun pupo')
      .replace(/Ẹ ṣeun/gi, 'E sheun')
      .replace(/ẹ ṣeun/gi, 'e sheun')
      .replace(/ṣẹlẹ̀/gi, 'shele')
      .replace(/ìṣẹ̀lẹ̀/gi, 'ishele')
      .replace(/ẹ̀sùn/gi, 'esun')
      .replace(/ẹjọ́ yìí/gi, 'ejo yii')
      .replace(/ẹjọ́/gi, 'ejo')
      .replace(/ọ̀rọ̀/gi, 'oro')
      .replace(/kúlẹ̀kúlẹ̀/gi, 'kulekule')
      .replace(/kíákíá/gi, 'kiakia')
      .replace(/àtúnṣe/gi, 'atunshe')
      .replace(/láàárín/gi, 'laa-arin')
      .replace(/wákàtí/gi, 'wakati')
      .replace(/sílẹ̀/gi, 'sile')
      .replace(/nọ́mbà/gi, 'namba')
      .replace(/àkọ́ọ́lẹ̀/gi, 'akoo-le')
      .replace(/mẹ́rinlelogun/gi, 'merin lelogun')
      .replace(/fóònù/gi, 'foonu')
      .replace(/ìfowóránṣẹ́/gi, 'ifoworanshe')
      .replace(/ìfidánilójú/gi, 'ifidaniloju')
      .replace(/kẹ́kẹ́/gi, 'keke')
      .replace(/ogbọ̀n/gi, 'ogbon')
      .replace(/ẹgbẹrun/gi, 'egberun')
      .replace(/ogún/gi, 'ogun')
      .replace(/mẹ́wàá/gi, 'mewaa')
      .replace(/aadọta/gi, 'aadota')
      .replace(/bánkì/gi, 'banki')
      .replace(/ṣùgbọ́n/gi, 'sugbon')
      .replace(/ṣe/gi, 'she')
      .replace(/ṣ/g, 'sh')
      .replace(/Ṣ/g, 'Sh')
      // Normalize combined diacritics
      .normalize('NFD')
      .replace(/[\u0300-\u036f]/g, '') // remove tone diacritics
      .replace(/ẹ/gi, 'e')
      .replace(/ọ/gi, 'o');
  }

  if (language === 'ha') {
    return text
      .replace(/sa'o'i/gi, 'sa-o-i')
      .replace(/na'ura/gi, 'na-ura')
      .replace(/na'urar/gi, 'na-urar')
      .replace(/kudin ku/gi, 'kudin ku')
      .replace(/tantancewa/gi, 'tantancewa')
      .replace(/ƙ/g, 'k')
      .replace(/Ƙ/g, 'K')
      .replace(/ɗ/g, 'd')
      .replace(/Ɗ/g, 'D')
      .replace(/ɓ/g, 'b')
      .replace(/Ɓ/g, 'B')
      .replace(/ƴ/g, 'y')
      .replace(/Ƴ/g, 'Y')
      .normalize('NFD')
      .replace(/[\u0300-\u036f]/g, '');
  }

  if (language === 'ig') {
    return text
      .replace(/nọmba/gi, 'namba')
      .replace(/akaụntụ/gi, 'akauntu')
      .replace(/abụọ/gi, 'abuo')
      .replace(/atọ/gi, 'ato')
      .replace(/anọ/gi, 'ano')
      .replace(/daalụ nke ukwuu/gi, 'daalu nke ukwuu')
      .replace(/daalụ/gi, 'daalu')
      .replace(/ụlọ akụ/gi, 'ulo aku')
      .replace(/kedu/gi, 'kedu')
      .replace(/kọwaa/gi, 'kowa')
      .replace(/kpọmkwem/gi, 'kpomkwem')
      .replace(/na-ewepụtaghị/gi, 'na-eweputaghi')
      .replace(/nkọwa/gi, 'nkowa')
      .replace(/ekwentị/gi, 'ekwenti')
      .replace(/ọ/g, 'o')
      .replace(/Ọ/g, 'O')
      .replace(/ụ/g, 'u')
      .replace(/Ụ/g, 'U')
      .replace(/ị/g, 'i')
      .replace(/Ị/g, 'I')
      .replace(/ṅ/g, 'n')
      .replace(/Ṅ/g, 'N')
      .normalize('NFD')
      .replace(/[\u0300-\u036f]/g, '');
  }

  if (language === 'pcm') {
    return text
      .replace(/sey/gi, 'say')
      .replace(/abeg/gi, 'ah-beg')
      .replace(/wetin/gi, 'weh-tin')
      .replace(/dey/gi, 'dey')
      .replace(/comot/gi, 'comot')
      .replace(/sharp sharp/gi, 'sharp sharp');
  }

  if (language === 'fr') {
    return text
      .replace(/[’']/g, "'")
      .replace(/[«»]/g, '"')
      .replace(/distributeur TPE/gi, 'terminal TPE');
  }

  return text;
}

/**
 * Wakes up audio DAC, laptop speakers, and Bluetooth receivers with an ultra-short,
 * pleasant acoustic warm-up tone so that the first spoken word is never truncated.
 */
export function wakeUpAudioDevice(): void {
  try {
    if (typeof window === 'undefined') return;
    const AudioCtx =
      window.AudioContext ||
      (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext;
    if (!AudioCtx) return;
    const ctx = new AudioCtx();
    const osc = ctx.createOscillator();
    const gain = ctx.createGain();

    osc.type = 'sine';
    osc.frequency.setValueAtTime(523.25, ctx.currentTime); // C5 clean tone
    osc.frequency.exponentialRampToValueAtTime(783.99, ctx.currentTime + 0.05); // G5

    gain.gain.setValueAtTime(0.04, ctx.currentTime);
    gain.gain.exponentialRampToValueAtTime(0.0001, ctx.currentTime + 0.05);

    osc.connect(gain);
    gain.connect(ctx.destination);

    osc.start(ctx.currentTime);
    osc.stop(ctx.currentTime + 0.06);
  } catch {
    // safe fallback
  }
}

/**
 * Queries the browser speech engine and selects the highest-clarity,
 * most articulate neural / natural voice available.
 */
export function findBestVoiceForLanguage(language: SupportedLanguage): SpeechSynthesisVoice | null {
  if (typeof window === 'undefined' || !('speechSynthesis' in window)) return null;
  const voices = window.speechSynthesis.getVoices();
  if (!voices || voices.length === 0) return null;

  const langInfo = SUPPORTED_LANGUAGES[language];
  const targetCode = langInfo.speechCode.toLowerCase().replace(/_/g, '-');
  const langPrefix = language.toLowerCase();

  // 1. Direct language code match (e.g. 'yo-ng', 'ha-ng', 'fr-fr', 'en-ng')
  let matched = voices.find(
    (v) =>
      v.lang.toLowerCase().replace(/_/g, '-') === targetCode ||
      v.lang.toLowerCase().startsWith(langPrefix) ||
      v.name.toLowerCase().includes(langInfo.name.toLowerCase())
  );
  if (matched) return matched;

  // 2. For African languages (Yoruba, Hausa, Igbo, Pidgin), prioritize Nigerian / African English accents
  if (['yo', 'ha', 'ig', 'pcm'].includes(language)) {
    matched = voices.find(
      (v) =>
        v.lang.toLowerCase().includes('en-ng') ||
        v.name.toLowerCase().includes('nigeria') ||
        v.name.toLowerCase().includes('ekele') ||
        v.name.toLowerCase().includes('african')
    );
    if (matched) return matched;

    // 3. Fallback to British / Commonwealth Natural/Neural voices (pronounces open vowels sharply)
    matched = voices.find(
      (v) =>
        (v.name.includes('Natural') ||
          v.name.includes('Neural') ||
          v.name.includes('Google') ||
          v.name.includes('Online')) &&
        (v.lang.includes('en-GB') || v.lang.includes('en-ZA') || v.lang.includes('en-IE'))
    );
    if (matched) return matched;

    // 4. Any UK English voice
    matched = voices.find((v) => v.lang.toLowerCase().includes('en-gb'));
    if (matched) return matched;
  }

  // 5. For French, pick best French neural voice
  if (language === 'fr') {
    matched = voices.find(
      (v) =>
        v.lang.toLowerCase().startsWith('fr') &&
        (v.name.includes('Natural') ||
          v.name.includes('Neural') ||
          v.name.includes('Google') ||
          v.name.includes('Hortense') ||
          v.name.includes('Julie'))
    );
    if (matched) return matched;
    matched = voices.find((v) => v.lang.toLowerCase().startsWith('fr'));
    if (matched) return matched;
  }

  // 6. Find any Natural / Neural English voice
  matched = voices.find(
    (v) =>
      v.lang.toLowerCase().startsWith('en') &&
      (v.name.includes('Natural') ||
        v.name.includes('Neural') ||
        v.name.includes('Google') ||
        v.name.includes('Jenny') ||
        v.name.includes('Sonia'))
  );
  if (matched) return matched;

  // 7. Fallback to browser default
  return voices.find((v) => v.default) || voices[0] || null;
}

export interface SpeechOptions {
  rate?: number;
  pitch?: number;
  volume?: number;
}

/**
 * Speech Synthesis Helper (Native Browser Voice with Accents and Clarity Normalization)
 */
export function speakNativeVoice(
  text: string,
  language: SupportedLanguage,
  optionsOrOnEnd?: SpeechOptions | (() => void),
  onEndCallback?: () => void
): void {
  if (typeof window === 'undefined' || !('speechSynthesis' in window)) {
    if (typeof optionsOrOnEnd === 'function') optionsOrOnEnd();
    if (onEndCallback) onEndCallback();
    return;
  }

  const options: SpeechOptions =
    typeof optionsOrOnEnd === 'object' ? optionsOrOnEnd : {};
  const onEnd = typeof optionsOrOnEnd === 'function' ? optionsOrOnEnd : onEndCallback;

  // Cancel any ongoing speech
  window.speechSynthesis.cancel();

  // Acoustic wake-up for speaker hardware
  wakeUpAudioDevice();

  // Normalize phonetic text so African tone diacritics don't choke the synthesizer
  const speechText = normalizeTextForSpeech(text, language);
  const utterance = new SpeechSynthesisUtterance(speechText);
  const langInfo = SUPPORTED_LANGUAGES[language];

  // Rate: 0.88 for African languages ensures articulate, deliberate delivery so words are crystal clear and sharp
  const defaultRate = ['yo', 'ha', 'ig', 'pcm'].includes(language) ? 0.88 : 0.94;
  const rate = options.rate ?? defaultRate;
  const pitch = options.pitch ?? 1.06; // 1.06 provides bright, sharp treble clarity
  const volume = options.volume ?? 1.0; // 100% volume for maximum loudness and audibility

  utterance.rate = rate;
  utterance.pitch = pitch;
  utterance.volume = volume;

  const bestVoice = findBestVoiceForLanguage(language);
  if (bestVoice) {
    utterance.voice = bestVoice;
    // Set utterance.lang to match the chosen voice's language so the browser engine doesn't reject or distort it
    utterance.lang = bestVoice.lang || langInfo.speechCode;
  } else {
    utterance.lang = langInfo.speechCode;
  }

  if (onEnd) {
    utterance.onend = () => onEnd();
    utterance.onerror = () => onEnd();
  }

  setTimeout(() => {
    window.speechSynthesis.speak(utterance);
  }, 70);
}

/**
 * Preset Demo Scenarios for Testing Multilingual Voice Conversational Flow
 */
export interface VoiceDemoPreset {
  id: string;
  language: SupportedLanguage;
  title: string;
  initialSpeech: string;
  translatedEnglish: string;
  followupSpeech: string;
  followupEnglish: string;
}

export const VOICE_DEMO_PRESETS: VoiceDemoPreset[] = [
  {
    id: 'demo-yo',
    language: 'yo',
    title: '🇳🇬 Yorùbá: ATM Marina Failed Dispense',
    initialSpeech: 'Mo lọ sí ATM Marina, ogbọ̀n ẹgbẹrun naira ni mo fẹ́ yọ ṣùgbọ́n kò mú owó jáde bí ó tilẹ̀ jẹ́ pé wọ́n ti yọ owó náà ní àkọ́ọ́lẹ̀ mi.',
    translatedEnglish: 'I went to Marina ATM, I wanted to withdraw ₦30,000 but it did not dispense cash even though my account was debited.',
    followupSpeech: 'Nọ́mbà àkọ́ọ́lẹ̀ mi ni 0128394910 ní FirstBank.',
    followupEnglish: 'My account number is 0128394910 at FirstBank.',
  },
  {
    id: 'demo-ha',
    language: 'ha',
    title: '🇳🇬 Hausa: Kano ATM Cash Dispenser Issue',
    initialSpeech: 'Na je ATM a Kano na cire naira dubu ashirin, amma na\'urar ba ta bada kudi ba sai aka cire a asusuna.',
    translatedEnglish: 'I went to the ATM in Kano to withdraw 20,000 naira, but the machine did not dispense cash and my account was debited.',
    followupSpeech: 'Lambar asusuna 0281928410 a GTBank.',
    followupEnglish: 'My account number is 0281928410 at GTBank.',
  },
  {
    id: 'demo-ig',
    language: 'ig',
    title: '🇳🇬 Igbo: Onitsha Market POS Duplicate Debit',
    initialSpeech: 'Agara m ahịa Onitsha iji POS kwụọ puku iri anọ, mana onye na-ere ahịa sịrị na azụmahịa ahụ adaala mgbe ha dọpụrụ ego m.',
    translatedEnglish: 'I went to Onitsha market to pay 40,000 via POS, but the vendor claimed the transaction failed while my money was deducted.',
    followupSpeech: 'Akaụntụ m bụ 0391827461 Access Bank.',
    followupEnglish: 'My account is 0391827461 at Access Bank.',
  },
  {
    id: 'demo-pcm',
    language: 'pcm',
    title: '🇳🇬 Pidgin: Mobile Transfer Hanging in Transit',
    initialSpeech: 'I send 15k transfer to my mama since yesterday afternoon, the money comot for my bank but e never reach her side till now.',
    translatedEnglish: 'I sent a ₦15,000 transfer to my mother yesterday afternoon, the money left my account but hasn\'t reached her till now.',
    followupSpeech: 'My account number na 0182736450 for Zenith Bank.',
    followupEnglish: 'My account number is 0182736450 at Zenith Bank.',
  },
  {
    id: 'demo-fr',
    language: 'fr',
    title: '🇫🇷 French: Distributeur Automatique Débit Sans Billet',
    initialSpeech: 'J\'ai tenté de retirer cinquante mille francs au distributeur UBA mais aucun billet n\'est sorti alors que mon compte a été débité.',
    translatedEnglish: 'I attempted to withdraw 50,000 francs at the UBA ATM but no cash was dispensed even though my account was debited.',
    followupSpeech: 'Mon numéro de compte est 0482910482 chez UBA.',
    followupEnglish: 'My account number is 0482910482 at UBA.',
  },
];
