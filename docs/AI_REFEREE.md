# AI LLM Referee Specification

The AI Referee is an intelligent disambiguation layer that arbitrates edge cases where algorithmic heuristic scoring falls slightly below the strict 95% threshold (between 75% and 94%) or when multiple candidates have almost identical scores (e.g., Radio Edit vs Album Version).

---

## 1. Responsibilities

1. **Verify Exact Recording**: Confirm that the candidate represents the exact same musical recording (not a cover, not a live performance, not a remix).
2. **Handle Incomplete Metadata**: Discern when a YouTube video title uses colloquial or foreign formatting that confuses basic string metrics.
3. **Disambiguate Multiple Matches**: Select the single best recording when two candidates are nearly identical (e.g., favoring official Label Audio Track ATV over a music video).
4. **Enforce Strict Rejection**: Reject when no candidate meets the 95% certainty bar.

---

## 2. Structured JSON Output Schema

The AI Referee is instructed to respond strictly with valid JSON conforming to:

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "RefereeVerdict",
  "type": "object",
  "properties": {
    "verdict": {
      "type": "string",
      "enum": ["MATCH", "NO_MATCH"]
    },
    "matched_index": {
      "type": "integer",
      "description": "0-based index of the chosen candidate if verdict is MATCH, else -1"
    },
    "confidence": {
      "type": "number",
      "minimum": 0.0,
      "maximum": 1.0,
      "description": "Confidence score from 0.0 to 1.0 (95%+ required for acceptance)"
    },
    "reasoning": {
      "type": "string",
      "description": "Brief explanation of why the candidate was accepted or rejected"
    }
  },
  "required": ["verdict", "matched_index", "confidence", "reasoning"]
}
```

---

## 3. System Prompt Template

```text
You are an expert music metadata referee operating under strict precision guidelines.
Your objective is to determine if ANY of the provided YouTube candidates is an EXACT match for the Source Track from Spotify.

CRITICAL RULES:
1. PRECISION IS PARAMOUNT. Never match a song that is a different version, cover, live version (unless source is live), acoustic version, remix (unless source is remix), karaoke, or parody.
2. If candidate is by a tribute band, different artist, or cover artist, REJECT immediately (verdict: NO_MATCH).
3. If source track duration differs by more than 10 seconds from candidate duration, scrutinize heavily. Music videos with long intros/dialogue should NOT be matched if an official audio track is available.
4. If multiple candidates are valid, choose the official studio audio track (type: ATV / Topic channel) over music videos.
5. If no candidate reaches >= 0.95 confidence, return verdict: "NO_MATCH".

Respond ONLY with valid JSON conforming to the RefereeVerdict schema.
```

---

## 4. Supported Providers & Configuration

| Provider | Model Identifier | Config Key / Env Var | Description |
| :--- | :--- | :--- | :--- |
| **Antigravity** *(Recommended)* | Integrated Gemini | `antigravity_path` / `AGY_PATH` | **No API key needed!** Uses your active Antigravity CLI / IDE session via `agy`. |
| **Google Gemini** | `gemini-2.5-flash` / `gemini-1.5-flash` | `GEMINI_API_KEY` | Direct API key (Free tier available at [Google AI Studio](https://aistudio.google.com/apikey)). |
| **OpenAI** | `gpt-4o-mini` / `gpt-4o` | `OPENAI_API_KEY` | Paid / usage-based API key. |
| **Anthropic Claude** | `claude-3-5-haiku` / `claude-3-5-sonnet` | `ANTHROPIC_API_KEY` | Paid / usage-based API key. |
| **Ollama** | `llama3`, `mistral`, `qwen` | `OLLAMA_ENDPOINT` (e.g. `http://localhost:11434`) | **100% Free & Local.** No API key or internet required. |
| **Mock** | `mock` | `none` | Offline synthetic testing referee for dry runs. |
