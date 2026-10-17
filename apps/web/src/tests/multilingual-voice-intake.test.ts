import { describe, it, expect } from 'vitest';
import {
  extractAmountFromVoiceText,
  extractChannelFromVoiceText,
  extractLocationOrBank,
  extractAccountOrPhone,
  analyzeVoiceUtterance,
  SLOT_INTERROGATION_PROMPTS,
  SUPPORTED_LANGUAGES,
} from '../features/reverseit/voice/VoiceDisputeEngine';

describe('Multilingual Voice Dispute Intake & Conversational Slot Engine', () => {
  describe('Spoken Currency & Number Parsing Across African Languages', () => {
    it('should parse Yorùbá spoken amounts correctly', () => {
      expect(extractAmountFromVoiceText('Mo fẹ́ yọ ogbọ̀n ẹgbẹrun ní ATM')).toBe(30000);
      expect(extractAmountFromVoiceText('Wọ́n yọ ogun ẹgbẹrun naira')).toBe(20000);
      expect(extractAmountFromVoiceText('Ẹgbẹrun mẹ́wàá ni mo foworánṣẹ́')).toBe(10000);
      expect(extractAmountFromVoiceText('Aadọta ẹgbẹrun naira')).toBe(50000);
    });

    it('should parse Hausa spoken amounts correctly', () => {
      expect(extractAmountFromVoiceText('Na cire naira dubu ashirin a ATM')).toBe(20000);
      expect(extractAmountFromVoiceText('Dubu talatin ne ba a bayar ba')).toBe(30000);
      expect(extractAmountFromVoiceText('Dubu hamsin aka cire a asusuna')).toBe(50000);
      expect(extractAmountFromVoiceText('Kudin ya kai dubu dari')).toBe(100000);
    });

    it('should parse Igbo spoken amounts correctly', () => {
      expect(extractAmountFromVoiceText('Achọrọ m iwepụ puku iri abụọ na igwe ATM')).toBe(20000);
      expect(extractAmountFromVoiceText('Ha dọpụrụ puku iri anọ na akaụntụ m')).toBe(40000);
      expect(extractAmountFromVoiceText('Ego ahụ bụ puku iri ise')).toBe(50000);
    });

    it('should parse French spoken amounts correctly', () => {
      expect(extractAmountFromVoiceText('J\'ai tenté de retirer vingt mille francs')).toBe(20000);
      expect(extractAmountFromVoiceText('Le montant est cinquante mille francs CFA')).toBe(50000);
      expect(extractAmountFromVoiceText('Cent mille francs ont été débités')).toBe(100000);
    });

    it('should parse Pidgin and standard currency figures', () => {
      expect(extractAmountFromVoiceText('I send 25k to my brother')).toBe(25000);
      expect(extractAmountFromVoiceText('Debit alert of ₦45,000 enter')).toBe(45000);
      expect(extractAmountFromVoiceText('Machine hold my 20 thousand naira')).toBe(20000);
    });
  });

  describe('Channel & Entity Recognition', () => {
    it('should detect ATM channel across local language terms', () => {
      expect(extractChannelFromVoiceText('Mo lọ sí ATM Marina')).toBe('ATM');
      expect(extractChannelFromVoiceText('Na je na\'urar ATM a Kano')).toBe('ATM');
      expect(extractChannelFromVoiceText('Igwe ATM anabataghị ego m')).toBe('ATM');
      expect(extractChannelFromVoiceText('Distributeur automatique bancaire')).toBe('ATM');
    });

    it('should detect POS and merchant channels', () => {
      expect(extractChannelFromVoiceText('I swipe my card for POS shop')).toBe('POS');
      expect(extractChannelFromVoiceText('Onye ahịa jiri POS wepụ ego')).toBe('POS');
      expect(extractChannelFromVoiceText('Paiement sur terminal TPE')).toBe('POS');
    });

    it('should detect mobile bank transfers', () => {
      expect(extractChannelFromVoiceText('I do mobile app transfer to mama')).toBe('MOBILE_TRANSFER');
      expect(extractChannelFromVoiceText('Tura kudi ta wayar salula')).toBe('MOBILE_TRANSFER');
      expect(extractChannelFromVoiceText('Virement bancaire mobile')).toBe('MOBILE_TRANSFER');
    });

    it('should extract banks and locations', () => {
      expect(extractLocationOrBank('Mo lọ sí FirstBank Marina')).toBe('FIRSTBANK');
      expect(extractLocationOrBank('Akaụntụ GTBank m')).toBe('GTBANK');
      expect(extractLocationOrBank('Transfer to Zenith bank')).toBe('ZENITH');
      expect(extractLocationOrBank('Chez UBA à Abidjan')).toBe('UBA');
    });

    it('should extract 10-digit NUBAN accounts and phone numbers', () => {
      expect(extractAccountOrPhone('Lambar asusuna 0281928410 a banki')).toBe('0281928410');
      expect(extractAccountOrPhone('Nọ́mbà mi ni 0128394910')).toBe('0128394910');
      expect(extractAccountOrPhone('Call me on 08031234567')).toBe('08031234567');
    });
  });

  describe('Conversational Slot-Filling & Interrogation State Machine', () => {
    it('should identify missing account number in Turn 1 and trigger native interrogation', () => {
      const initialSlots = {};
      const speechTurn1 = 'Mo lọ sí ATM Marina, ogbọ̀n ẹgbẹrun naira kò jáde.';

      const analysis = analyzeVoiceUtterance(initialSlots, speechTurn1, 'yo');

      expect(analysis.updatedSlots.channel).toBe('ATM');
      expect(analysis.updatedSlots.amount).toBe(30000);
      expect(analysis.updatedSlots.locationOrBank).toBe('Marina');
      expect(analysis.updatedSlots.accountOrPhone).toBeUndefined();

      // Must detect that account number is missing
      expect(analysis.isComplete).toBe(false);
      expect(analysis.missingSlot).toBe('accountOrPhone');

      // Must generate the localized Yoruba prompt
      expect(analysis.suggestedPrompt?.prompt).toContain('nọ́mbà àkọ́ọ́lẹ̀ yín');
      expect(analysis.suggestedPrompt?.translatedEnglish).toContain('account number');
    });

    it('should fulfill the missing slot in Turn 2 and generate confirmation prompt', () => {
      const slotsWithTurn1 = {
        channel: 'ATM' as const,
        amount: 30000,
        currency: 'NGN',
        locationOrBank: 'Marina',
      };

      const speechTurn2 = 'Nọ́mbà àkọ́ọ́lẹ̀ mi ni 0128394910 ní FirstBank.';
      const analysis = analyzeVoiceUtterance(slotsWithTurn1, speechTurn2, 'yo');

      expect(analysis.updatedSlots.accountOrPhone).toBe('0128394910');
      expect(analysis.isComplete).toBe(true);
      expect(analysis.missingSlot).toBeUndefined();

      // Must return confirmation success in Yoruba
      expect(analysis.suggestedPrompt?.prompt).toBe(SLOT_INTERROGATION_PROMPTS.confirmation_success.yo.prompt);
      expect(analysis.suggestedPrompt?.translatedEnglish).toContain('24 hours');
    });

    it('should handle Hausa multi-turn voice intake seamlessly', () => {
      const speechHa1 = 'Na je ATM a Kano na cire naira dubu ashirin amma bai bada kudi ba.';
      const analysis1 = analyzeVoiceUtterance({}, speechHa1, 'ha');

      expect(analysis1.updatedSlots.channel).toBe('ATM');
      expect(analysis1.updatedSlots.amount).toBe(20000);
      expect(analysis1.updatedSlots.locationOrBank).toBe('Kano');
      expect(analysis1.isComplete).toBe(false);
      expect(analysis1.missingSlot).toBe('accountOrPhone');

      const speechHa2 = 'Lambar asusuna 0281928410 a GTBank Kano.';
      const analysis2 = analyzeVoiceUtterance(analysis1.updatedSlots, speechHa2, 'ha');

      expect(analysis2.updatedSlots.locationOrBank).toBe('Kano');
      expect(analysis2.updatedSlots.accountOrPhone).toBe('0281928410');
      expect(analysis2.isComplete).toBe(true);
      expect(analysis2.suggestedPrompt?.prompt).toBe(SLOT_INTERROGATION_PROMPTS.confirmation_success.ha.prompt);
    });
  });

  describe('Supported Language Roster Integrity', () => {
    it('should configure all 6 priority languages with proper speech codes and greetings', () => {
      const expectedCodes = ['yo', 'ha', 'ig', 'pcm', 'fr', 'en'] as const;
      for (const code of expectedCodes) {
        const lang = SUPPORTED_LANGUAGES[code];
        expect(lang).toBeDefined();
        expect(lang.speechCode).toBeTruthy();
        expect(lang.greetingPrompt).toBeTruthy();
        expect(lang.flag).toBeTruthy();
      }
    });
  });
});
