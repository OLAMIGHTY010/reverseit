"""Prompt Injection Defense and Untrusted Content Sanitizer.

Implements Prompt A1 (Item 3) and TH-AI-01 / TH-AI-02 mitigations:
- Strips or neutralizes instruction-like text from payloads, notes, and OCR text
- Escapes boundary-breaking XML/JSON tags
- Delimits untrusted content within strict isolation tags
- Detects adversarial prompt injection attempts
"""

import re
from typing import Tuple, List

# Patterns frequently used in prompt injection / instruction override attacks
_INJECTION_PATTERNS = [
    re.compile(r'(?i)\bignore\s+(?:all\s+)?(?:previous|prior|above)\s+instructions?\b'),
    re.compile(r'(?i)\bdisregard\s+(?:all\s+)?(?:system|previous|rule)\s+(?:prompts?|instructions?|directives?)\b'),
    re.compile(r'(?i)\byou\s+are\s+now\s+(?:a|an)\b'),
    re.compile(r'(?i)\bsystem\s*prompt\s*:\b'),
    re.compile(r'(?i)\bdeveloper\s+mode\b'),
    re.compile(r'(?i)\b(?:grant|issue|process|execute)\s+(?:an?\s+)?(?:immediate\s+)?(?:reversal|refund)\b'),
    re.compile(r'(?i)\bbypass\s+(?:the\s+)?(?:approval|gate|maker-checker|verification)\b'),
    re.compile(r'(?i)\boverride\s+(?:the\s+)?(?:status|decision|policy)\b'),
]

# Sensitive boundary delimiters that attackers might try to inject
_DELIMITER_TAGS = re.compile(r'<\/?(?:evidence|fact|derived|note|untrusted_content|system_instruction|context_bundle)[^>]*>', re.IGNORECASE)


class ContentSanitizer:
    """Neutralizes prompt injection attempts and wraps untrusted user/system text."""

    @classmethod
    def sanitize_untrusted_text(cls, raw_text: str) -> Tuple[str, bool, List[str]]:
        """Sanitizes text, escapes structural XML delimiters, and neutralizes instructions.
        
        Returns:
            Tuple of (sanitized_text, has_injection_attempt, detected_patterns)
        """
        if not raw_text:
            return "", False, []

        sanitized = raw_text
        detected = []

        # 1. Defang structural delimiter tags
        def _defang_tag(match):
            tag = match.group(0)
            return tag.replace("<", "&lt;").replace(">", "&gt;")

        sanitized = _DELIMITER_TAGS.sub(_defang_tag, sanitized)

        # 2. Detect & Neutralize instruction-like overrides
        has_injection = False
        for pattern in _INJECTION_PATTERNS:
            matches = pattern.findall(sanitized)
            if matches:
                has_injection = True
                for m in matches:
                    detected.append(m)
                    # Replace with neutralized placeholder removing malicious command words
                    sanitized = pattern.sub("[OVERRIDE_DIRECTIVE_BLOCKED]", sanitized)

        return sanitized, has_injection, detected


    @classmethod
    def wrap_in_untrusted_delimiter(cls, text: str, source_id: str, source_system: str) -> str:
        """Wraps text in standard untrusted content tags with sanitized body."""
        clean_text, _, _ = cls.sanitize_untrusted_text(text)
        return (
            f'<untrusted_content id="{source_id}" source="{source_system}">\n'
            f'{clean_text}\n'
            f'</untrusted_content>'
        )


sanitizer = ContentSanitizer()
