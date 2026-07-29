---
name: greeting
type: tool
description: Renders a personalized greeting — demo tool for the Espace widget contract (EPIC-03/C5)
command: ["python3", "skills/greeting/tool.py"]
params:
  - name: name
    type: string
    required: true
---

# greeting

Demonstrates the tool invocation contract: reads `{"input": {"name": "..."}}` on stdin,
answers `{"label": "...", "value": "..."}` on stdout.
