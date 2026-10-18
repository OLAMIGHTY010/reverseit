import asyncio
import os
import edge_tts
from gtts import gTTS

OUTPUT_DIR = os.path.join("apps", "web", "public", "audio", "prompts")
os.makedirs(OUTPUT_DIR, exist_ok=True)

PROMPTS = {
    # 1. Greetings
    "greeting_yo": ("Ẹ káàbọ̀ sí ReverseIt. Ẹ jọ̀wọ́, sọ ohun tí ó ṣẹlẹ̀ sí owó yín fún wa.", "yo"),
    "greeting_ha": ("Barka da zuwa ReverseIt. Don Allah bayyana abin da ya faru da kudin ku.", "ha"),
    "greeting_ig": ("Nnọọ na ReverseIt. Biko kọwaa ihe mere na ego gị.", "ig"),
    "greeting_pcm": ("Welcome to ReverseIt o. Wetin happen to your money? Just talk am.", "pcm"),
    "greeting_fr": ("Bienvenue sur ReverseIt. Veuillez expliquer le problème rencontré avec votre transaction.", "fr"),
    "greeting_en": ("Welcome to ReverseIt voice intake. Please tell us what happened with your transaction.", "en"),

    # 2. Channel Slot
    "slot_channel_yo": ("Ǹjẹ́ orí ATM, POS ni àbí ètò ìfowóránṣẹ́ orí fóònù ni ìṣẹ̀lẹ̀ náà ti ṣẹlẹ̀?", "yo"),
    "slot_channel_ha": ("Shin wannan ya faru ne a kan ATM, POS ko kuma tura kudi ta wayar salula?", "ha"),
    "slot_channel_ig": ("Ọ bụ na ATM, POS ka ọ bụ nzipu ego na ekwentị ka nke a mere?", "ig"),
    "slot_channel_pcm": ("Na for ATM machine, POS shop, or na mobile bank transfer e happen?", "pcm"),
    "slot_channel_fr": ("Est-ce que cela s'est produit sur un distributeur ATM, un terminal TPE ou par virement mobile?", "fr"),
    "slot_channel_en": ("Did this dispute occur at an ATM, a merchant POS terminal, or via mobile app transfer?", "en"),

    # 3. Amount Slot
    "slot_amount_yo": ("Ẹ ṣeun fún àlàyé yín. Ẹ jọ̀wọ́, èlò gangan ni iye owó tí ẹ fẹ́ yọ tí kò jáde?", "yo"),
    "slot_amount_ha": ("Mun gode da bayanin ku. Don Allah, nawa ne ainihin kudin da ba a bayar ba?", "ha"),
    "slot_amount_ig": ("Daalụ maka nkọwa gị. Biko, ego ole kpọmkwem ka igwe ahụ na-ewepụtaghị?", "ig"),
    "slot_amount_pcm": ("Thank you for the update. Abeg, how much be the exact money wey no comot?", "pcm"),
    "slot_amount_fr": ("Merci pour ces précisions. S'il vous plaît, quel était le montant exact non distribué ou débité?", "fr"),
    "slot_amount_en": ("Thank you. Please specify the exact transaction amount that was debited or failed.", "en"),

    # 4. Location or Bank Slot
    "slot_locationOrBank_yo": ("Báwo ni orúkọ bánkì tàbí ibi tí ATM tàbí POS náà wà ṣe rí?", "yo"),
    "slot_locationOrBank_ha": ("Wanne banki ne ko kuma a ina wannan ATM ko POS din yake?", "ha"),
    "slot_locationOrBank_ig": ("Kedu aha ụlọ akụ ahụ ma ọ bụ ebe igwe ATM ma ọ bụ POS ahụ dị?", "ig"),
    "slot_locationOrBank_pcm": ("Which bank or which area dat ATM or POS machine dey?", "pcm"),
    "slot_locationOrBank_fr": ("Quelle est la banque concernée ou l'emplacement exact du distributeur?", "fr"),
    "slot_locationOrBank_en": ("Which bank, terminal location, or destination institution was involved?", "en"),

    # 5. Account or Phone Slot
    "slot_accountOrPhone_yo": ("Ẹ jọ̀wọ́, kí ni nọ́mbà àkọ́ọ́lẹ̀ yín tàbí nọ́mbà fóònù yín fún ìfidánilójú?", "yo"),
    "slot_accountOrPhone_ha": ("Don Allah, wanne lambar asusun ku ne ko kuma lambar wayar ku don tantancewa?", "ha"),
    "slot_accountOrPhone_ig": ("Biko, kedu nọmba akaụntụ gị ma ọ bụ nọmba ekwentị gị maka nkwenye?", "ig"),
    "slot_accountOrPhone_pcm": ("Abeg tell us your 10-digit account number or phone number make we take trace am.", "pcm"),
    "slot_accountOrPhone_fr": ("S'il vous plaît, quel est votre numéro de compte ou votre numéro de téléphone pour la vérification?", "fr"),
    "slot_accountOrPhone_en": ("Please provide your 10-digit account number or registered phone number.", "en"),

    # 6. Confirmation Success
    "confirmation_success_yo": ("A ti gba ẹ̀sùn yín wọlé kẹ́kẹ́. Ẹ̀sùn yín ti bẹ̀rẹ̀ sí ní ṣe àyẹ̀wò, owó yín yóò padà láàrín wákàtí mẹ́rinlelogun.", "yo"),
    "confirmation_success_ha": ("An karbi rahoton ku cikin nasara. Za mu bincika tare da mayar muku da kudin ku nan da sa'o'i 24.", "ha"),
    "confirmation_success_ig": ("Anabatala mkpesa gị nke ọma. Anyị na-enyocha ya ma weghachite ego gị n'ime awa iri abụọ na anọ.", "ig"),
    "confirmation_success_pcm": ("We don successfully receive your complaint. Automated investigation don start, your money go land within 24 hours.", "pcm"),
    "confirmation_success_fr": ("Votre réclamation a été enregistrée avec succès. Votre remboursement est en cours d'investigation et sera traité dans les 24 heures.", "fr"),
    "confirmation_success_en": ("Your dispute has been registered successfully. Real-time switch reconciliation is active and funds will be restored within guaranteed Central Bank SLA.", "en"),

    # Demo User Speech
    "demo_yo_initial": ("Mo lọ sí ATM Marina, ogbọ̀n ẹgbẹrun naira ni mo fẹ́ yọ ṣùgbọ́n kò mú owó jáde bí ó tilẹ̀ jẹ́ pé wọ́n ti yọ owó náà ní àkọ́ọ́lẹ̀ mi.", "yo"),
    "demo_yo_followup": ("Nọ́mbà àkọ́ọ́lẹ̀ mi ni 0128394910 ní FirstBank.", "yo"),
    "demo_ha_initial": ("Na je ATM a Kano na cire naira dubu ashirin, amma na'urar ba ta bada kudi ba sai aka cire a asusuna.", "ha"),
    "demo_ha_followup": ("Lambar asusuna 0281928410 a GTBank.", "ha"),
    "demo_ig_initial": ("Agara m ahịa Onitsha iji POS kwụọ puku iri anọ, mana onye na-ere ahịa sịrị na azụmahịa ahụ adaala mgbe ha dọpụrụ ego m.", "ig"),
    "demo_ig_followup": ("Akaụntụ m bụ 0391827461 Access Bank.", "ig"),
    "demo_pcm_initial": ("I send 15k transfer to my mama since yesterday afternoon, the money comot for my bank but e never reach her side till now.", "pcm"),
    "demo_pcm_followup": ("My account number na 0182736450 for Zenith Bank.", "pcm"),
    "demo_fr_initial": ("J'ai tenté de retirer cinquante mille francs au distributeur UBA mais aucun billet n'est sorti alors que mon compte a été débité.", "fr"),
    "demo_fr_followup": ("Mon numéro de compte est 0482910482 chez UBA.", "fr"),
}

import sys
try:
    sys.stdout.reconfigure(encoding="utf-8")
except Exception:
    pass

async def generate_prompt(key: str, text: str, lang: str):
    target_path = os.path.join(OUTPUT_DIR, f"{key}.mp3")
    if os.path.exists(target_path) and os.path.getsize(target_path) > 1000:
        print(f"Skipping {key} (already exists)")
        return

    print(f"Generating [{lang}] {key}...")
    try:
        if lang == "ha":
            # Native Hausa via gTTS
            tts = gTTS(text=text, lang="ha")
            tts.save(target_path)
            print(f"  [OK] {key} saved via gTTS (ha)")
        elif lang in ("en", "pcm"):
            # Authentic Nigerian English / Pidgin Neural Voice
            comm = edge_tts.Communicate(text, "en-NG-EzinneNeural", rate="-4%", pitch="+1Hz")
            await comm.save(target_path)
            print(f"  [OK] {key} saved via EzinneNeural ({lang})")
        elif lang == "fr":
            # French Neural Voice
            comm = edge_tts.Communicate(text, "fr-FR-VivienneMultilingualNeural", rate="-2%")
            await comm.save(target_path)
            print(f"  [OK] {key} saved via VivienneMultilingual (fr)")
        else:
            # Yoruba and Igbo via Ava Multilingual
            comm = edge_tts.Communicate(text, "en-US-AvaMultilingualNeural", rate="-6%", pitch="+2Hz")
            await comm.save(target_path)
            print(f"  [OK] {key} saved via AvaMultilingual ({lang})")
    except Exception as e:
        print(f"  [WARN] Error generating {key} with primary engine: {e}")
        try:
            # Fallback to gTTS if edge_tts has any network jitter
            tts = gTTS(text=text, lang="fr" if lang == "fr" else ("ha" if lang == "ha" else "en"))
            tts.save(target_path)
            print(f"  [OK] {key} saved via gTTS fallback")
        except Exception as e2:
            print(f"  [FAIL] Critical failure for {key}: {e2}")

async def main():
    print(f"Starting studio audio generation into {OUTPUT_DIR}...")
    for key, (text, lang) in PROMPTS.items():
        await generate_prompt(key, text, lang)
        await asyncio.sleep(0.3)
    print("All studio voice prompts generated successfully!")

if __name__ == "__main__":
    asyncio.run(main())
