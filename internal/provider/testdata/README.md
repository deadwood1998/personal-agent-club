# Provider fixtures

Offline stand-ins for the provider responses club reads, the same files the Lark gateway's agent bridge tests use.
None was recorded from a real account: they are assembled from the providers' own documentation, so they need
calibrating against real traffic.

- `manus-api-v2.json`: Manus API v2 responses (`agent.list`, `task.list`, `task.listMessages`, `task.sendMessage`,
  error envelopes) in the shapes the OpenAPI pages at open.manus.ai/docs/v2 describe.
- `grok-bot-0.78.1.json`: Grok Bot desktop-protocol responses in the shapes grok-bot-sdk 0.78.1 (MIT) reads;
  transcript bodies are base64 JSON. The `user-message` entry kind is a guess until calibrated.
- `groq-openai-v1.json`: Groq's OpenAI-compatible API (`GET /models`, `POST /chat/completions`, error envelopes) in
  the OpenAI shapes Groq documents for https://api.groq.com/openai/v1.
