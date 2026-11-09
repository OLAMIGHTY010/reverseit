"""
ReverseIt - Screenshot / receipt input (image -> text).

This module only turns an image into TEXT. parse.py imports it (the same way it
imports regex_parser.py) and runs the text through parse_alert(). It must NOT
import parse.py, otherwise the two files would import each other.

    extract_text_from_image(path) -> str | None
"""
import base64
import os
import re

from dotenv import load_dotenv
from groq import Groq

load_dotenv()

# Groq's vision docs list qwen/qwen3.8-27b as multimodal, i.e. the same model
# parse.py already uses. Override with GROQ_VISION_MODEL if that changes.
VISION_MODEL = os.getenv("GROQ_VISION_MODEL", os.getenv("GROQ_MODEL", "qwen/qwen3.8-27b"))

# Groq free tier caps OUTPUT tokens per minute (1000 for this model), and a
# request is rejected if its max tokens exceed that. A bank alert transcribes
# in roughly 100-200 tokens, so keep this small. Raise only on a paid tier.
IMAGE_MAX_TOKENS = int(os.getenv("GROQ_IMAGE_MAX_TOKENS", "300"))

# Our own conservative caps (check Groq's current limits before raising them).
MAX_IMAGE_BYTES = 4 * 1024 * 1024
IMAGE_MIME = {".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".webp": "image/webp"}

OCR_PROMPT = (
    "Transcribe ALL text visible in this bank alert or receipt screenshot, "
    "exactly as written, keeping numbers, dates, times and reference numbers "
    "character for character. Do not summarize, correct, translate or add "
    "anything. If there is no readable text, reply with exactly: NO_TEXT"
)


def _client():
    key = os.getenv("GROQ_API_KEY")
    if not key:
        print("[Warning] GROQ_API_KEY not found in environment.")
        return None
    return Groq(api_key=key, timeout=30.0, max_retries=1)


def _read_checked(path, allowed_exts, max_bytes, label):
    """Return (ext, bytes) or None if the file is missing, empty, too big or wrong type."""
    ext = os.path.splitext(path)[1].lower()
    if ext not in allowed_exts:
        print(f"[{label}] unsupported file type: {ext or 'none'}")
        return None
    size = os.path.getsize(path)
    if size == 0 or size > max_bytes:
        print(f"[{label}] file size not allowed: {size} bytes")
        return None
    with open(path, "rb") as f:
        return ext, f.read()


# ---------------------------------------------------------------------
# Screenshots / receipts
# ---------------------------------------------------------------------
def extract_text_from_image(image_path: str) -> str | None:
    """Image -> text with the vision model. Returns None on any failure."""
    try:
        checked = _read_checked(image_path, set(IMAGE_MIME), MAX_IMAGE_BYTES, "Image")
        client = _client() if checked else None
        if not client:
            return None
        ext, data = checked
        data_url = f"data:{IMAGE_MIME[ext]};base64,{base64.b64encode(data).decode()}"

        kwargs = {}
        effort = os.getenv("GROQ_REASONING_EFFORT")
        if effort:
            kwargs["reasoning_effort"] = effort

        completion = client.chat.completions.create(
            model=VISION_MODEL,
            messages=[{
                "role": "user",
                "content": [
                    {"type": "text", "text": OCR_PROMPT},
                    {"type": "image_url", "image_url": {"url": data_url}},
                ],
            }],
            temperature=0.0,
            max_completion_tokens=IMAGE_MAX_TOKENS,
            **kwargs,
        )
        text = completion.choices[0].message.content or ""
        text = re.sub(r"<think>.*?</think>", "", text, flags=re.DOTALL).strip()
        if not text or text.upper() == "NO_TEXT":
            return None
        return text
    except Exception as e:
        print(f"[Vision Error]: {e}")
        return None


if __name__ == "__main__":
    import sys
    if len(sys.argv) != 2:
        print("usage: python image_input.py <screenshot.png|jpg|webp>")
    else:
        print(extract_text_from_image(sys.argv[1]))